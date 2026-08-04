// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

const mailRuleReorderSuffix = "/user_mailboxes/"

func completeMailRuleReorderRequest(ctx context.Context, ac *client.APIClient, opts *ServiceMethodOptions, request client.RawApiRequest, checkErr func(interface{}, core.Identity) error) (client.RawApiRequest, error) {
	if !isMailRuleReorderMethod(opts, request) {
		return request, nil
	}

	requestedIDs, body, err := mailRuleRequestedIDs(request.Data)
	if err != nil {
		return request, err
	}
	currentIDs, err := fetchAllMailRuleIDs(ctx, ac, request, checkErr)
	if err != nil {
		return request, err
	}
	completedIDs, err := completeMailRuleIDs(requestedIDs, currentIDs)
	if err != nil {
		return request, err
	}

	body["rule_ids"] = completedIDs
	request.Data = body
	return request, nil
}

func isMailRuleReorderMethod(opts *ServiceMethodOptions, request client.RawApiRequest) bool {
	if opts == nil {
		return false
	}
	if opts.Method.ID == "ReorderUserMailboxRule" || opts.SchemaPath == "mail.user_mailbox.rules.reorder" {
		return true
	}
	path := strings.Trim(request.URL, "/")
	return strings.HasPrefix(path, "open-apis/mail/v1/user_mailboxes/") &&
		strings.HasSuffix(path, "/rules/reorder") &&
		strings.Contains(path, mailRuleReorderSuffix)
}

func mailRuleRequestedIDs(data interface{}) ([]string, map[string]interface{}, error) {
	body, ok := data.(map[string]interface{})
	if !ok || body == nil {
		return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "rule_ids is required").WithParam("rule_ids")
	}
	raw, ok := body["rule_ids"]
	if !ok {
		return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "rule_ids is required").WithParam("rule_ids")
	}
	ids, ok := stringSlice(raw)
	if !ok {
		return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "rule_ids must be an array of strings").WithParam("rule_ids")
	}
	if len(ids) == 0 {
		return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "rule_ids must contain at least one rule ID").WithParam("rule_ids")
	}
	return ids, body, nil
}

func fetchAllMailRuleIDs(ctx context.Context, ac *client.APIClient, reorderRequest client.RawApiRequest, checkErr func(interface{}, core.Identity) error) ([]string, error) {
	listURL := strings.TrimSuffix(reorderRequest.URL, "/reorder")
	if listURL == reorderRequest.URL {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rule reorder path %q does not end with /reorder", reorderRequest.URL)
	}

	params := copyParams(reorderRequest.Params)
	delete(params, "page_token")
	params["page_size"] = 100

	var ids []string
	var pageToken string
	for {
		if pageToken != "" {
			params["page_token"] = pageToken
		} else {
			delete(params, "page_token")
		}
		result, err := ac.CallAPI(ctx, client.RawApiRequest{
			Method:    "GET",
			URL:       listURL,
			Params:    copyParams(params),
			As:        reorderRequest.As,
			ExtraOpts: reorderRequest.ExtraOpts,
		})
		if err != nil {
			return nil, err
		}
		if err := checkErr(result, reorderRequest.As); err != nil {
			return nil, err
		}
		pageIDs, hasMore, nextToken, err := extractMailRulePage(result)
		if err != nil {
			return nil, err
		}
		ids = append(ids, pageIDs...)
		if !hasMore {
			break
		}
		if nextToken == "" {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rule list response has has_more=true but no page_token")
		}
		pageToken = nextToken
	}
	return ids, nil
}

func extractMailRulePage(result interface{}) ([]string, bool, string, error) {
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return nil, false, "", errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rule list response must be a JSON object")
	}
	data, ok := resultMap["data"].(map[string]interface{})
	if !ok {
		return nil, false, "", errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rule list response missing data object")
	}
	arrayField := output.FindArrayField(data)
	if _, ok := data["rules"].([]interface{}); ok {
		arrayField = "rules"
	}
	if arrayField == "" {
		return nil, false, "", nil
	}
	items, ok := data[arrayField].([]interface{})
	if !ok {
		return nil, false, "", errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rule list field %s must be an array", arrayField)
	}

	ids := make([]string, 0, len(items))
	for i, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			return nil, false, "", errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rule list item %d must be an object", i)
		}
		ruleID, ok := mailRuleID(itemMap)
		if !ok {
			return nil, false, "", errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rule list item %d missing rule_id", i)
		}
		ids = append(ids, ruleID)
	}

	hasMore, _ := data["has_more"].(bool)
	pageToken, _ := data["page_token"].(string)
	if pageToken == "" {
		pageToken, _ = data["next_page_token"].(string)
	}
	return ids, hasMore, pageToken, nil
}

func mailRuleID(item map[string]interface{}) (string, bool) {
	for _, key := range []string{"rule_id", "ruleId", "id"} {
		if id, ok := item[key].(string); ok && strings.TrimSpace(id) != "" {
			return id, true
		}
	}
	return "", false
}

func completeMailRuleIDs(requestedIDs, currentIDs []string) ([]string, error) {
	seenRequested := make(map[string]bool, len(requestedIDs))
	for _, id := range requestedIDs {
		if strings.TrimSpace(id) == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "rule_ids contains an empty rule ID").WithParam("rule_ids")
		}
		if seenRequested[id] {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "duplicate rule_id %q in rule_ids", id).WithParam("rule_ids")
		}
		seenRequested[id] = true
	}

	currentSet := make(map[string]bool, len(currentIDs))
	for _, id := range currentIDs {
		currentSet[id] = true
	}
	if len(currentIDs) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "cannot reorder rules because current mailbox rule list is empty").WithParam("rule_ids")
	}
	for _, id := range requestedIDs {
		if !currentSet[id] {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "rule_id %q was not found in current mailbox rules", id).WithParam("rule_ids")
		}
	}

	completed := append([]string(nil), requestedIDs...)
	for _, id := range currentIDs {
		if !seenRequested[id] {
			completed = append(completed, id)
		}
	}
	return completed, nil
}

func copyParams(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func stringSlice(raw interface{}) ([]string, bool) {
	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...), true
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}
