// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
)

const mailRuleReorderSchemaPath = "mail.user_mailbox.rules.reorder"

func needsServiceRequestPreparation(opts *ServiceMethodOptions) bool {
	return opts != nil && opts.SchemaPath == mailRuleReorderSchemaPath
}

func prepareServiceRequest(opts *ServiceMethodOptions, ac *client.APIClient, request *client.RawApiRequest) error {
	if !needsServiceRequestPreparation(opts) {
		return nil
	}
	return prepareMailRuleReorderRequest(opts, ac, request)
}

func prepareMailRuleReorderRequest(opts *ServiceMethodOptions, ac *client.APIClient, request *client.RawApiRequest) error {
	inputIDs, err := mailRuleReorderInputIDs(request.Data)
	if err != nil {
		return err
	}

	listResult, err := ac.CallAPI(opts.Ctx, client.RawApiRequest{
		Method: "GET",
		URL:    mailRuleListURL(request.URL),
		As:     request.As,
	})
	if err != nil {
		return err
	}
	if err := ac.CheckResponse(listResult, request.As); err != nil {
		return err
	}

	currentIDs, err := mailRuleListIDs(listResult)
	if err != nil {
		return err
	}
	if len(currentIDs) == 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"mail user mailbox rules reorder requires current mailbox rules, but list returned no rules").
			WithParam("--data.rule_ids")
	}

	known := make(map[string]bool, len(currentIDs))
	for _, id := range currentIDs {
		known[id] = true
	}
	for _, id := range inputIDs {
		if !known[id] {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--data.rule_ids contains unknown rule_id %q", id).
				WithParam("--data.rule_ids")
		}
	}

	selected := make(map[string]bool, len(inputIDs))
	merged := make([]string, 0, len(currentIDs))
	for _, id := range inputIDs {
		selected[id] = true
		merged = append(merged, id)
	}
	for _, id := range currentIDs {
		if !selected[id] {
			merged = append(merged, id)
		}
	}

	body := request.Data.(map[string]interface{})
	body["rule_ids"] = merged
	return nil
}

func mailRuleReorderInputIDs(data interface{}) ([]string, error) {
	body, ok := data.(map[string]interface{})
	if !ok || body == nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--data must be a JSON object containing rule_ids").WithParam("--data")
	}
	raw, ok := body["rule_ids"]
	if !ok {
		return nil, mailRuleIDsValidationError("--data.rule_ids is required")
	}
	rawIDs, ok := raw.([]interface{})
	if !ok {
		return nil, mailRuleIDsValidationError("--data.rule_ids must be a non-empty string array")
	}
	if len(rawIDs) == 0 {
		return nil, mailRuleIDsValidationError("--data.rule_ids must not be empty")
	}

	ids := make([]string, 0, len(rawIDs))
	seen := make(map[string]bool, len(rawIDs))
	for i, rawID := range rawIDs {
		id, ok := rawID.(string)
		if !ok || id == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--data.rule_ids[%d] must be a non-empty string", i).WithParam("--data.rule_ids")
		}
		if seen[id] {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--data.rule_ids contains duplicate rule_id %q", id).WithParam("--data.rule_ids")
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

func mailRuleIDsValidationError(message string) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, message).WithParam("--data.rule_ids")
}

func mailRuleListIDs(result interface{}) ([]string, error) {
	resultMap, ok := result.(map[string]interface{})
	if !ok || resultMap == nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"mail rules list response must be a JSON object")
	}
	data, ok := resultMap["data"].(map[string]interface{})
	if !ok || data == nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"mail rules list response missing data object")
	}
	items, ok := data["items"].([]interface{})
	if !ok {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"mail rules list response missing data.items array")
	}
	ids := make([]string, 0, len(items))
	for i, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok || itemMap == nil {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
				"mail rules list response data.items[%d] must be an object", i)
		}
		id, ok := itemMap["id"].(string)
		if !ok || id == "" {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
				"mail rules list response data.items[%d].id must be a non-empty string", i)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func mailRuleListURL(reorderURL string) string {
	return strings.TrimSuffix(reorderURL, "/reorder")
}
