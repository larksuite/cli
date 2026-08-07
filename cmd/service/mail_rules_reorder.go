// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
)

const mailRulesReorderSchemaPath = "mail.user_mailbox.rules.reorder"

func maybeCompleteMailRulesReorderIDs(ctx context.Context, ac *client.APIClient, opts *ServiceMethodOptions, request *client.RawApiRequest, checkErr func(interface{}, core.Identity) error) error {
	if opts.SchemaPath != mailRulesReorderSchemaPath {
		return nil
	}

	body, ok := toStringAnyMap(request.Data)
	if !ok {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "mail rules reorder requires a JSON object body").WithParam("--data")
	}

	requestedIDs, err := mailRuleIDsFromBody(body)
	if err != nil {
		return err
	}
	if err := validateRequestedMailRuleIDs(requestedIDs); err != nil {
		return err
	}

	existingIDs, err := listAllMailRuleIDs(ctx, ac, *request, checkErr)
	if err != nil {
		return err
	}

	completedIDs, err := completeMailRuleIDs(requestedIDs, existingIDs)
	if err != nil {
		return err
	}
	body["rule_ids"] = completedIDs
	request.Data = body
	return nil
}

func toStringAnyMap(v any) (map[string]any, bool) {
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	return nil, false
}

func mailRuleIDsFromBody(body map[string]any) ([]string, error) {
	raw, ok := body["rule_ids"]
	if !ok {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "rule_ids is required").WithParam("rule_ids")
	}
	switch ids := raw.(type) {
	case []string:
		return append([]string(nil), ids...), nil
	case []any:
		out := make([]string, 0, len(ids))
		for i, id := range ids {
			s, ok := id.(string)
			if !ok {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "rule_ids[%d] must be a string", i).WithParam("rule_ids")
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "rule_ids must be an array of strings").WithParam("rule_ids")
	}
}

func validateRequestedMailRuleIDs(requestedIDs []string) error {
	if len(requestedIDs) == 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "at least one rule_id is required for reorder").WithParam("rule_ids")
	}
	seen := map[string]struct{}{}
	for _, id := range requestedIDs {
		if id == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "rule_ids cannot contain an empty string").WithParam("rule_ids")
		}
		if _, ok := seen[id]; ok {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "duplicate rule_id: %s", id).WithParam("rule_ids")
		}
		seen[id] = struct{}{}
	}
	return nil
}

func completeMailRuleIDs(requestedIDs, existingIDs []string) ([]string, error) {
	existingSet := make(map[string]struct{}, len(existingIDs))
	for _, id := range existingIDs {
		if id == "" {
			continue
		}
		existingSet[id] = struct{}{}
	}
	for _, id := range requestedIDs {
		if _, ok := existingSet[id]; !ok {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "unknown rule_id: %s", id).WithParam("rule_ids")
		}
	}

	requestedSet := make(map[string]struct{}, len(requestedIDs))
	completed := make([]string, 0, len(existingIDs))
	for _, id := range requestedIDs {
		requestedSet[id] = struct{}{}
		completed = append(completed, id)
	}
	for _, id := range existingIDs {
		if _, ok := requestedSet[id]; !ok {
			completed = append(completed, id)
		}
	}
	return completed, nil
}

func listAllMailRuleIDs(ctx context.Context, ac *client.APIClient, reorderRequest client.RawApiRequest, checkErr func(interface{}, core.Identity) error) ([]string, error) {
	listRequest := client.RawApiRequest{
		Method: "GET",
		URL:    mailRulesListURL(reorderRequest.URL),
		Params: copyRequestParamsWithoutPageToken(reorderRequest.Params),
		As:     reorderRequest.As,
	}
	var allIDs []string
	pageToken := ""
	for {
		pageRequest := listRequest
		pageRequest.Params = copyRequestParamsWithoutPageToken(listRequest.Params)
		if pageToken != "" {
			pageRequest.Params["page_token"] = pageToken
		}

		result, err := ac.CallAPI(ctx, pageRequest)
		if err != nil {
			return nil, err
		}
		if apiErr := checkErr(result, reorderRequest.As); apiErr != nil {
			return nil, apiErr
		}

		ids, err := extractMailRuleIDs(result)
		if err != nil {
			return nil, err
		}
		allIDs = append(allIDs, ids...)

		nextToken, hasMore, err := nextMailRulesPageToken(result)
		if err != nil {
			return nil, err
		}
		if !hasMore {
			break
		}
		pageToken = nextToken
	}
	return allIDs, nil
}

func mailRulesListURL(reorderURL string) string {
	return strings.TrimSuffix(reorderURL, "/reorder")
}

func copyRequestParamsWithoutPageToken(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		if k == "page_token" {
			continue
		}
		out[k] = v
	}
	return out
}

func extractMailRuleIDs(result any) ([]string, error) {
	data, ok := nestedMap(result, "data")
	if !ok {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rules list response missing data")
	}
	rawRules, ok := data["items"]
	if !ok {
		rawRules, ok = data["rules"]
	}
	if !ok || rawRules == nil {
		return []string{}, nil
	}
	rules, ok := rawRules.([]any)
	if !ok {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rules list response items is not an array")
	}

	ids := make([]string, 0, len(rules))
	for i, item := range rules {
		rule, ok := item.(map[string]any)
		if !ok {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rules list item %d is not an object", i)
		}
		id, ok := rule["rule_id"].(string)
		if !ok || id == "" {
			id, ok = rule["id"].(string)
		}
		if !ok || id == "" {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rules list item %d missing rule_id", i)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func nextMailRulesPageToken(result any) (string, bool, error) {
	data, ok := nestedMap(result, "data")
	if !ok {
		return "", false, errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rules list response missing data")
	}
	hasMore, _ := data["has_more"].(bool)
	if !hasMore {
		return "", false, nil
	}
	if token, ok := data["page_token"].(string); ok && token != "" {
		return token, true, nil
	}
	if token, ok := data["next_page_token"].(string); ok && token != "" {
		return token, true, nil
	}
	return "", false, errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rules list response missing next page token")
}

func nestedMap(result any, key string) (map[string]any, bool) {
	m, ok := toStringAnyMap(result)
	if !ok {
		return nil, false
	}
	return toStringAnyMap(m[key])
}
