// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/validate"
	shortmail "github.com/larksuite/cli/shortcuts/mail"
)

func fetchMailboxPrimaryEmail(ctx context.Context, rt event.APIClient, mailbox string) (string, error) {
	raw, err := rt.CallAPI(ctx, "GET", shortmail.MailboxPath(mailbox, "profile"), nil)
	if err != nil {
		return "", err
	}
	var resp struct {
		Data struct {
			Email string `json:"primary_email_address"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	if resp.Data.Email == "" {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse,
			"profile API returned no primary_email_address")
	}
	return resp.Data.Email, nil
}

func fetchMessageForWatch(ctx context.Context, rt event.APIClient, mailbox, messageID, format string) (map[string]interface{}, error) {
	path := fmt.Sprintf("/open-apis/mail/v1/user_mailboxes/%s/messages/%s?format=%s",
		validate.EncodePathSegment(mailbox), validate.EncodePathSegment(messageID), validate.EncodePathSegment(format))
	raw, err := rt.CallAPI(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	msg, _ := resp.Data["message"].(map[string]interface{})
	if msg == nil {
		return resp.Data, nil
	}
	return msg, nil
}

func resolveWatchFilterIDs(ctx context.Context, rt event.APIClient, mailboxID, explicitIDsInput, namesInput, explicitFlagName, namesFlagName, kind string) ([]string, error) {
	explicitIDs, err := parseJSONArrayFlag(explicitIDsInput, explicitFlagName)
	if err != nil {
		return nil, err
	}
	names, err := parseJSONArrayFlag(namesInput, namesFlagName)
	if err != nil {
		return nil, err
	}

	set := map[string]bool{}
	resolveSystem := systemResolver(kind)
	for _, raw := range explicitIDs {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if id, ok := resolveSystem(value); ok {
			set[id] = true
		} else {
			set[value] = true
		}
	}

	var remainingNames []string
	for _, raw := range names {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if id, ok := resolveSystem(value); ok {
			set[id] = true
			continue
		}
		remainingNames = append(remainingNames, value)
	}
	resolvedNames, err := resolveNamesByAPI(ctx, rt, mailboxID, remainingNames, kind)
	if err != nil {
		return nil, err
	}
	for _, id := range resolvedNames {
		if id != "" {
			set[id] = true
		}
	}
	return sortedSetKeys(set), nil
}

func resolveNamesByAPI(ctx context.Context, rt event.APIClient, mailbox string, names []string, kind string) ([]string, error) {
	var path string
	switch kind {
	case "label":
		path = shortmail.MailboxPath(mailbox, "labels")
	case "folder":
		path = shortmail.MailboxPath(mailbox, "folders")
	default:
		return nil, nil
	}
	raw, err := rt.CallAPI(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data map[string][]map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	key := kind + "s"
	items := resp.Data[key]
	var out []string
	seen := map[string]bool{}
	for _, raw := range names {
		value := stringsTrim(raw)
		if value == "" {
			continue
		}
		matches := matchingIDs(value, kind, items)
		switch len(matches) {
		case 0:
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"%s %q not_exists in mailbox %s; please use an ID or an exact name", kind, value, mailbox)
		case 1:
			if !seen[matches[0]] {
				seen[matches[0]] = true
				out = append(out, matches[0])
			}
		default:
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"%s name %q matches multiple IDs (%s); please use an ID", kind, value, strings.Join(matches, ","))
		}
	}
	return out, nil
}

func matchingIDs(value, kind string, items []map[string]interface{}) []string {
	for _, item := range items {
		if id := itemID(kind, item); id != "" && id == value {
			return []string{id}
		}
	}
	lower := strings.ToLower(value)
	var matches []string
	seen := map[string]bool{}
	for _, item := range items {
		name, _ := item["name"].(string)
		if strings.ToLower(stringsTrim(name)) != lower {
			continue
		}
		id := itemID(kind, item)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		matches = append(matches, id)
	}
	return matches
}

func itemID(kind string, item map[string]interface{}) string {
	id, _ := item[kind+"_id"].(string)
	if id == "" {
		id, _ = item["id"].(string)
	}
	return id
}

func systemResolver(kind string) func(string) (string, bool) {
	if kind == "label" {
		return shortmail.ResolveLabelSystemIDForWatch
	}
	return shortmail.ResolveFolderSystemAliasOrIDForWatch
}

func stringsTrim(s string) string {
	return strings.TrimSpace(s)
}

func parseJSONArrayFlag(input, flagName string) ([]string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, nil
	}
	if !strings.HasPrefix(trimmed, "[") {
		return splitComma(trimmed), nil
	}
	var values []string
	if err := json.Unmarshal([]byte(trimmed), &values); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"invalid --%s: expected JSON array of strings", strings.ReplaceAll(flagName, "_", "-")).
			WithParam("--" + strings.ReplaceAll(flagName, "_", "-")).
			WithCause(err)
	}
	return values, nil
}

func splitComma(input string) []string {
	var out []string
	for _, part := range strings.Split(input, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func sortedSetKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		if k != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}
