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

const (
	mailRulesSchemaPath = "mail.user_mailbox.rules"
)

func completeMailRulesReorder(ctx context.Context, ac *client.APIClient, request *client.RawApiRequest, schemaPath string, identity core.Identity) error {
	if !isMailRulesReorderRequest(request, schemaPath) {
		return nil
	}

	body, ok := request.Data.(map[string]any)
	if !ok {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "mail rules reorder requires JSON object request body").WithParam("--data")
	}
	inputIDs, err := stringSliceField(body, "rule_ids")
	if err != nil {
		return err
	}
	if len(inputIDs) == 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "rule_ids must contain at least one rule ID").WithParam("rule_ids")
	}
	if dup := firstDuplicate(inputIDs); dup != "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "duplicate rule_id %q in rule_ids", dup).WithParam("rule_ids")
	}

	allIDs, err := listMailRuleIDs(ctx, ac, request, identity)
	if err != nil {
		return err
	}
	if len(allIDs) == 0 {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "mail rules list returned no rules to reorder").WithParam("rule_ids")
	}

	known := make(map[string]struct{}, len(allIDs))
	for _, id := range allIDs {
		known[id] = struct{}{}
	}
	for _, id := range inputIDs {
		if _, ok := known[id]; !ok {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "rule_id %q does not exist in current mail rules", id).WithParam("rule_ids")
		}
	}

	selected := make(map[string]struct{}, len(inputIDs))
	completed := make([]string, 0, len(allIDs))
	for _, id := range inputIDs {
		selected[id] = struct{}{}
		completed = append(completed, id)
	}
	for _, id := range allIDs {
		if _, ok := selected[id]; !ok {
			completed = append(completed, id)
		}
	}
	body["rule_ids"] = completed
	request.Data = body
	return nil
}

func isMailRulesReorderRequest(request *client.RawApiRequest, schemaPath string) bool {
	if request == nil {
		return false
	}
	return schemaPath == mailRulesSchemaPath+".reorder" &&
		strings.EqualFold(request.Method, "POST") &&
		strings.HasSuffix(request.URL, "/reorder")
}

func listMailRuleIDs(ctx context.Context, ac *client.APIClient, reorderRequest *client.RawApiRequest, identity core.Identity) ([]string, error) {
	listURL := strings.TrimSuffix(reorderRequest.URL, "/reorder")
	result, err := ac.PaginateAll(ctx, client.RawApiRequest{
		Method: "GET",
		URL:    listURL,
		Params: map[string]any{"page_size": 100},
		As:     identity,
	}, client.PaginationOptions{Identity: identity})
	if err != nil {
		return nil, err
	}
	if err := ac.CheckResponse(result, identity); err != nil {
		return nil, err
	}

	data, ok := result.(map[string]any)
	if !ok {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rules list response must be a JSON object")
	}
	items, ok := data["items"].([]any)
	if !ok {
		if nested, hasData := data["data"].(map[string]any); hasData {
			items, ok = nested["items"].([]any)
		}
	}
	if !ok {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rules list response missing items array")
	}
	if hasMore, _ := data["has_more"].(bool); hasMore {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rules list pagination did not return all pages")
	}

	ids := make([]string, 0, len(items))
	for i, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rules list item %d must be a JSON object", i)
		}
		id, _ := item["rule_id"].(string)
		if id == "" {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "mail rules list item %d missing rule_id", i)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func stringSliceField(body map[string]any, name string) ([]string, error) {
	raw, ok := body[name]
	if !ok {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s is required", name).WithParam(name)
	}
	switch values := raw.(type) {
	case []string:
		out := make([]string, 0, len(values))
		for i, id := range values {
			if id == "" {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s[%d] must be a non-empty string", name, i).WithParam(name)
			}
			out = append(out, id)
		}
		return out, nil
	case []any:
		out := make([]string, 0, len(values))
		for i, value := range values {
			id, ok := value.(string)
			if !ok || id == "" {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s[%d] must be a non-empty string", name, i).WithParam(name)
			}
			out = append(out, id)
		}
		return out, nil
	default:
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s must be an array of strings, got %T", name, raw).WithParam(name)
	}
}

func firstDuplicate(values []string) string {
	seen := map[string]struct{}{}
	for _, value := range values {
		if _, ok := seen[value]; ok {
			return value
		}
		seen[value] = struct{}{}
	}
	return ""
}
