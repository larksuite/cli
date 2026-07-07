// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
)

var folderSystemIDs = map[string]bool{
	"INBOX": true, "SENT": true, "DRAFT": true, "TRASH": true, "SPAM": true, "ARCHIVED": true,
}

var folderAliasToSystemID = map[string]string{
	"inbox": "INBOX", "收件箱": "INBOX",
	"sent": "SENT", "已发送": "SENT",
	"draft": "DRAFT", "drafts": "DRAFT", "草稿": "DRAFT",
	"trash": "TRASH", "deleted": "TRASH", "已删除": "TRASH",
	"spam": "SPAM", "junk": "SPAM", "垃圾邮件": "SPAM",
	"archived": "ARCHIVED", "archive": "ARCHIVED", "归档": "ARCHIVED",
}

var labelSystemAliases = map[string]string{
	"important": "IMPORTANT", "priority": "IMPORTANT", "重要邮件": "IMPORTANT",
	"flagged": "FLAGGED", "已加旗标": "FLAGGED",
	"other": "OTHER", "其他邮件": "OTHER",
}

var labelSystemIDs = map[string]bool{"IMPORTANT": true, "FLAGGED": true, "OTHER": true}

type folderInfo struct {
	ID             string
	Name           string
	ParentFolderID string
}

type labelInfo struct {
	ID   string
	Name string
}

func normalizeMailMessageReceivedParams(ctx context.Context, rt event.APIClient, params map[string]string) error {
	if params == nil {
		return nil
	}
	if strings.TrimSpace(params["mailbox"]) == "" {
		params["mailbox"] = "me"
	}
	if params["msg_format"] == "" {
		params["msg_format"] = "metadata"
	}
	if err := validateMessageFormat(params["msg_format"]); err != nil {
		return err
	}
	if params["mailbox"] == "me" {
		email, err := fetchMailboxPrimaryEmail(ctx, rt, "me")
		if err != nil {
			return enhanceProfileError(err)
		}
		params["mailbox"] = email
	}
	if err := normalizeIDListParam(ctx, rt, params, "folder_ids", "folders", resolveFolderSystemAliasOrID, listMailboxFolders); err != nil {
		return err
	}
	if err := normalizeIDListParam(ctx, rt, params, "label_ids", "labels", resolveLabelSystemID, listMailboxLabels); err != nil {
		return err
	}
	return nil
}

func validateMessageFormat(format string) error {
	switch format {
	case "", "event", "minimal", "metadata", "plain_text_full", "full":
		return nil
	default:
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"invalid msg_format %q: must be event, minimal, metadata, plain_text_full, or full", format).
			WithParam("--param")
	}
}

func normalizeIDListParam[T namedItem](
	ctx context.Context,
	rt event.APIClient,
	params map[string]string,
	idsName string,
	namesName string,
	systemResolver func(string) (string, bool),
	listFn func(context.Context, event.APIClient, string) ([]T, error),
) error {
	ids, err := parseJSONArrayParam(params[idsName], idsName)
	if err != nil {
		return err
	}
	names, err := parseJSONArrayParam(params[namesName], namesName)
	if err != nil {
		return err
	}
	set := make(map[string]bool)
	for _, raw := range ids {
		if id, ok := systemResolver(raw); ok {
			set[id] = true
			continue
		}
		set[raw] = true
	}
	var customNames []string
	for _, raw := range names {
		if id, ok := systemResolver(raw); ok {
			set[id] = true
			continue
		}
		customNames = append(customNames, raw)
	}
	if len(customNames) > 0 {
		items, err := listFn(ctx, rt, normalizedMailbox(params))
		if err != nil {
			return err
		}
		for _, name := range customNames {
			id, err := resolveByName(name, normalizedMailbox(params), items)
			if err != nil {
				return err
			}
			if id != "" {
				set[id] = true
			}
		}
	}
	if len(set) > 0 {
		params[idsName] = strings.Join(setKeys(set), ",")
	}
	delete(params, namesName)
	return nil
}

func normalizedMailbox(params map[string]string) string {
	if params == nil {
		return ""
	}
	return strings.TrimSpace(params["mailbox"])
}

func mailboxPath(mailboxID string, segments ...string) string {
	parts := make([]string, 0, len(segments)+1)
	parts = append(parts, url.PathEscape(mailboxID))
	for _, seg := range segments {
		if seg != "" {
			parts = append(parts, url.PathEscape(seg))
		}
	}
	return "/open-apis/mail/v1/user_mailboxes/" + strings.Join(parts, "/")
}

func parseJSONArrayParam(input, name string) ([]string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(trimmed), &values); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"invalid param %s: expected JSON array of strings", name).
			WithParam("--param").
			WithCause(err)
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out, nil
}

func fetchMailboxPrimaryEmail(ctx context.Context, rt event.APIClient, mailboxID string) (string, error) {
	if rt == nil {
		return "", errs.NewInternalError(errs.SubtypeUnknown, "runtime API client is required to resolve mailbox")
	}
	raw, err := rt.CallAPI(ctx, "GET", mailboxPath(mailboxID, "profile"), nil)
	if err != nil {
		return "", err
	}
	data := responseData(raw)
	if email := extractPrimaryEmail(data); email != "" {
		return email, nil
	}
	return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "profile API returned no primary_email_address")
}

func extractPrimaryEmail(data map[string]interface{}) string {
	if email, ok := data["primary_email_address"].(string); ok && strings.TrimSpace(email) != "" {
		return strings.TrimSpace(email)
	}
	if mailbox, ok := data["user_mailbox"].(map[string]interface{}); ok {
		if email, ok := mailbox["primary_email_address"].(string); ok && strings.TrimSpace(email) != "" {
			return strings.TrimSpace(email)
		}
	}
	return ""
}

func listMailboxFolders(ctx context.Context, rt event.APIClient, mailboxID string) ([]folderInfo, error) {
	if rt == nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "runtime API client is required to resolve folders")
	}
	raw, err := rt.CallAPI(ctx, "GET", mailboxPath(mailboxID, "folders"), nil)
	if err != nil {
		return nil, err
	}
	items, _ := responseData(raw)["items"].([]interface{})
	out := make([]folderInfo, 0, len(items))
	for _, item := range items {
		m, _ := item.(map[string]interface{})
		id := strVal(m["id"])
		if id != "" {
			out = append(out, folderInfo{ID: id, Name: strVal(m["name"]), ParentFolderID: strVal(m["parent_folder_id"])})
		}
	}
	return out, nil
}

func listMailboxLabels(ctx context.Context, rt event.APIClient, mailboxID string) ([]labelInfo, error) {
	if rt == nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "runtime API client is required to resolve labels")
	}
	raw, err := rt.CallAPI(ctx, "GET", mailboxPath(mailboxID, "labels"), nil)
	if err != nil {
		return nil, err
	}
	items, _ := responseData(raw)["items"].([]interface{})
	out := make([]labelInfo, 0, len(items))
	for _, item := range items {
		m, _ := item.(map[string]interface{})
		id := strVal(m["id"])
		if id != "" {
			out = append(out, labelInfo{ID: id, Name: strVal(m["name"])})
		}
	}
	return out, nil
}

type namedItem interface {
	folderInfo | labelInfo
}

func resolveByName[T namedItem](input, mailboxID string, items []T) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	for _, item := range items {
		if getID(item) == value {
			return value, nil
		}
	}
	lower := strings.ToLower(value)
	var matches []string
	seen := make(map[string]bool)
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(getName(item))) != lower {
			continue
		}
		id := getID(item)
		if id != "" && !seen[id] {
			matches = append(matches, id)
			seen[id] = true
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"ambiguous mailbox filter %q in %s: matched %d entries", value, mailboxID, len(matches)).
			WithParam("--param")
	}
	return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
		"mailbox filter %q not found in %s", value, mailboxID).
		WithParam("--param")
}

func getID[T namedItem](item T) string {
	switch v := any(item).(type) {
	case folderInfo:
		return v.ID
	case labelInfo:
		return v.ID
	default:
		return ""
	}
}

func getName[T namedItem](item T) string {
	switch v := any(item).(type) {
	case folderInfo:
		return v.Name
	case labelInfo:
		return v.Name
	default:
		return ""
	}
}

func resolveFolderSystemAliasOrID(input string) (string, bool) {
	if id, ok := folderAliasToSystemID[strings.ToLower(strings.TrimSpace(input))]; ok {
		return id, true
	}
	return normalizeSystemID(input, folderSystemIDs)
}

func resolveLabelSystemID(input string) (string, bool) {
	if id, ok := labelSystemAliases[strings.ToLower(strings.TrimSpace(input))]; ok {
		return id, true
	}
	return normalizeSystemID(input, labelSystemIDs)
}

func normalizeSystemID(input string, systemIDs map[string]bool) (string, bool) {
	canonical := strings.ToUpper(strings.TrimSpace(input))
	if canonical == "" {
		return "", false
	}
	if systemIDs[canonical] {
		return canonical, true
	}
	return "", false
}

func setKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		if key != "" {
			keys = append(keys, key)
		}
	}
	sortStrings(keys)
	return keys
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func responseData(raw json.RawMessage) map[string]interface{} {
	var envelope map[string]interface{}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil
	}
	if data, ok := envelope["data"].(map[string]interface{}); ok {
		return data
	}
	return envelope
}

func strVal(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func enhanceProfileError(err error) error {
	if p, ok := errs.ProblemOf(err); ok {
		if p.Category == errs.CategoryAuthorization {
			p.Message = "unable to resolve mailbox address: " + p.Message
			p.Hint = "run `lark-cli auth login --scope \"mail:user_mailbox:readonly\"` to grant mailbox profile access"
			return err
		}
	}
	return err
}
