// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
)

func maybeCompleteMailRuleReorderRequest(ctx context.Context, ac *client.APIClient, opts *ServiceMethodOptions, request *client.RawApiRequest, checkErr func(interface{}, core.Identity) error) error {
	if !isMailRuleReorder(opts, request) {
		return nil
	}
	body, inputRuleIDs, err := mailRuleReorderBody(request.Data)
	if err != nil {
		return err
	}

	listResult, err := ac.CallAPI(ctx, client.RawApiRequest{
		Method: "GET",
		URL:    mailRuleListURL(request.URL),
		As:     request.As,
	})
	if err != nil {
		return err
	}
	if err := checkErr(listResult, request.As); err != nil {
		return err
	}

	body["rule_ids"] = completeMailRuleIDs(inputRuleIDs, extractMailRuleIDs(listResult))
	request.Data = body
	return nil
}

func isMailRuleReorder(opts *ServiceMethodOptions, request *client.RawApiRequest) bool {
	if opts == nil {
		return false
	}
	if opts.SchemaPath == "mail.user_mailbox.rules.reorder" || opts.Method.ID == "mail.user_mailbox.rules.reorder" {
		return true
	}
	return strings.HasPrefix(opts.ServicePath, "/open-apis/mail/") &&
		strings.Contains(request.URL, "/user_mailboxes/") &&
		strings.HasSuffix(strings.TrimRight(request.URL, "/"), "/rules/reorder")
}

func mailRuleReorderBody(data interface{}) (map[string]interface{}, []string, error) {
	body, ok := data.(map[string]interface{})
	if !ok {
		return nil, nil, mailRuleIDsValidationError()
	}
	ruleIDs, ok := body["rule_ids"]
	if !ok {
		return nil, nil, mailRuleIDsValidationError()
	}
	ids := stringSlice(ruleIDs)
	if len(ids) == 0 {
		return nil, nil, mailRuleIDsValidationError()
	}
	return body, ids, nil
}

func mailRuleIDsValidationError() error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "rule_ids must contain at least one id").WithParam("rule_ids")
}

func mailRuleListURL(reorderURL string) string {
	return strings.TrimSuffix(strings.TrimRight(reorderURL, "/"), "/reorder")
}

func completeMailRuleIDs(inputRuleIDs, currentRuleIDs []string) []string {
	userIDs := dedupeStrings(inputRuleIDs)
	currentIDs := dedupeStrings(currentRuleIDs)
	currentSet := make(map[string]bool, len(currentIDs))
	for _, id := range currentIDs {
		currentSet[id] = true
	}
	userSet := make(map[string]bool, len(userIDs))
	var inCurrent []string
	var notInCurrent []string
	for _, id := range userIDs {
		userSet[id] = true
		if currentSet[id] {
			inCurrent = append(inCurrent, id)
		} else {
			notInCurrent = append(notInCurrent, id)
		}
	}

	out := append([]string{}, inCurrent...)
	for _, id := range currentIDs {
		if !userSet[id] {
			out = append(out, id)
		}
	}
	return append(out, notInCurrent...)
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func extractMailRuleIDs(result interface{}) []string {
	if root, ok := result.(map[string]interface{}); ok {
		if data, ok := root["data"].(map[string]interface{}); ok {
			for _, key := range []string{"items", "rules", "rule_list", "user_mailbox_rules"} {
				if ids := ruleIDsFromArray(data[key]); len(ids) > 0 {
					return ids
				}
			}
			return collectRuleIDs(data)
		}
	}
	return collectRuleIDs(result)
}

func ruleIDsFromArray(value interface{}) []string {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			if id, ok := m["rule_id"]; ok {
				ids = append(ids, fmt.Sprint(id))
			}
		}
	}
	return ids
}

func collectRuleIDs(value interface{}) []string {
	switch typed := value.(type) {
	case []interface{}:
		var ids []string
		for _, item := range typed {
			ids = append(ids, collectRuleIDs(item)...)
		}
		return ids
	case map[string]interface{}:
		if id, ok := typed["rule_id"]; ok {
			return []string{fmt.Sprint(id)}
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var ids []string
		for _, key := range keys {
			ids = append(ids, collectRuleIDs(typed[key])...)
		}
		return ids
	default:
		return nil
	}
}

func stringSlice(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, fmt.Sprint(item))
		}
		return out
	default:
		return nil
	}
}
