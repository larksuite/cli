// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

const (
	mailRuleReorderSchemaPath = "mail.user_mailbox.rules.reorder"
	mailRuleIDsParam          = "rule_ids"
	mailRuleDryRunRemainingID = "__remaining_rule_ids_from_list_response__"
)

func completeMailRuleReorderRequest(ctx context.Context, ac *client.APIClient, request client.RawApiRequest) (client.RawApiRequest, error) {
	explicit, err := mailRuleReorderExplicitIDs(request.Data)
	if err != nil {
		return request, err
	}
	listReq := mailRuleReorderListRequest(request)
	result, err := ac.CallAPI(ctx, listReq)
	if err != nil {
		return request, err
	}
	if err := ac.CheckResponse(result, request.As); err != nil {
		return request, err
	}
	completed, err := mergeMailRuleIDs(explicit, extractMailRuleIDsFromListResponse(result))
	if err != nil {
		return request, err
	}
	body, ok := request.Data.(map[string]interface{})
	if !ok {
		return request, mailRuleReorderValidation("--data must be a JSON object").WithParam("--data")
	}
	body[mailRuleIDsParam] = completed
	request.Data = body
	return request, nil
}

func serviceDryRunMailRuleReorder(f *cmdutil.Factory, request client.RawApiRequest, config *core.CliConfig, format string) error {
	explicit, err := mailRuleReorderExplicitIDs(request.Data)
	if err != nil {
		return err
	}
	completed := append([]string(nil), explicit...)
	completed = append(completed, mailRuleDryRunRemainingID)

	body := map[string]interface{}{}
	if requestBody, ok := request.Data.(map[string]interface{}); ok {
		for k, v := range requestBody {
			body[k] = v
		}
	}
	body[mailRuleIDsParam] = completed

	dr := cmdutil.NewDryRunAPI().
		GET(mailRuleReorderListRequest(request).URL).
		Desc("List current mail rules").
		POST(request.URL).
		Desc("Reorder mail rules with completed rule_ids").
		Body(body).
		Set("as", string(request.As)).
		Set("appId", config.AppID).
		Set("completed_rule_ids", completed)
	if config.UserOpenId != "" {
		dr.Set("userOpenId", config.UserOpenId)
	}

	fmt.Fprintln(f.IOStreams.Out, "=== Dry Run ===")
	if format == "pretty" {
		fmt.Fprint(f.IOStreams.Out, dr.Format())
		return nil
	}
	return outputDryRunJSON(f, dr)
}

func outputDryRunJSON(f *cmdutil.Factory, dr *cmdutil.DryRunAPI) error {
	output.PrintJson(f.IOStreams.Out, dr)
	return nil
}

func mailRuleReorderListRequest(request client.RawApiRequest) client.RawApiRequest {
	return client.RawApiRequest{
		Method: "GET",
		URL:    trimReorderSuffix(request.URL),
		As:     request.As,
	}
}

func trimReorderSuffix(rawURL string) string {
	const suffix = "/reorder"
	if strings.HasSuffix(rawURL, suffix) {
		return strings.TrimSuffix(rawURL, suffix)
	}
	return rawURL
}

func mailRuleReorderExplicitIDs(data interface{}) ([]string, error) {
	body, ok := data.(map[string]interface{})
	if !ok || body == nil {
		return nil, mailRuleReorderValidation("--data must be a JSON object").WithParam("--data")
	}
	raw, ok := body[mailRuleIDsParam]
	if !ok {
		return nil, mailRuleReorderValidation("data.rule_ids is required").WithParam("data.rule_ids")
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil, mailRuleReorderValidation("data.rule_ids must be a non-empty string array").WithParam("data.rule_ids")
	}
	if len(items) == 0 {
		return nil, mailRuleReorderValidation("data.rule_ids must be non-empty").WithParam("data.rule_ids")
	}

	ids := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for i, item := range items {
		id, ok := item.(string)
		if !ok {
			return nil, mailRuleReorderValidation("data.rule_ids[%d] must be a string", i).WithParam("data.rule_ids")
		}
		if id == "" {
			return nil, mailRuleReorderValidation("data.rule_ids[%d] must be non-empty", i).WithParam("data.rule_ids")
		}
		if _, exists := seen[id]; exists {
			return nil, mailRuleReorderValidation("data.rule_ids contains duplicate rule id: %s", id).WithParam("data.rule_ids")
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func extractMailRuleIDsFromListResponse(result interface{}) []string {
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return nil
	}
	data, ok := resultMap["data"].(map[string]interface{})
	if !ok {
		return nil
	}
	items, ok := data["items"].([]interface{})
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id, ok := itemMap["rule_id"].(string)
		if ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func mergeMailRuleIDs(explicit, all []string) ([]string, error) {
	allSet := make(map[string]struct{}, len(all))
	for _, id := range all {
		allSet[id] = struct{}{}
	}
	for _, id := range explicit {
		if _, ok := allSet[id]; !ok {
			return nil, mailRuleReorderValidation("rule_ids contains unknown rule id: %s", id).WithParam("data.rule_ids")
		}
	}

	completed := append([]string(nil), explicit...)
	explicitSet := make(map[string]struct{}, len(explicit))
	for _, id := range explicit {
		explicitSet[id] = struct{}{}
	}
	for _, id := range all {
		if _, ok := explicitSet[id]; ok {
			continue
		}
		completed = append(completed, id)
	}
	return completed, nil
}

func mailRuleReorderValidation(format string, args ...interface{}) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...)
}
