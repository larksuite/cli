// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

const (
	mailRulesReorderSchemaPath = "mail.user_mailbox.rules.reorder"
	mailRulesListPathSuffix    = "/rules"
	mailRulesReorderPathSuffix = "/rules/reorder"
)

func isMailRulesReorder(opts *ServiceMethodOptions) bool {
	return opts != nil && opts.SchemaPath == mailRulesReorderSchemaPath
}

func serviceDryRunMailRulesReorder(f *cmdutil.Factory, request client.RawApiRequest, config *core.CliConfig, opts *ServiceMethodOptions) error {
	listRequest := mailRulesListRequest(request, "")
	body := map[string]interface{}{"rule_ids": []string{"<user_rule_id_1>", "<user_rule_id_2>", "<remaining_rule_ids_in_current_order>"}}
	if bodyMap, ok := request.Data.(map[string]interface{}); ok {
		body = cloneMap(bodyMap)
		body["rule_ids"] = dryRunCompleteRuleIDs(bodyMap["rule_ids"])
	}
	dr := cmdutil.NewDryRunAPI().
		GET(listRequest.URL).
		Params(listRequest.Params).
		Desc("fetch current mailbox rules before reordering").
		POST(request.URL).
		Params(request.Params).
		Body(body).
		Desc("submit reorder with a complete rule_ids list")
	return cmdutil.WriteDryRun(dr, serviceDryRunOutputOptions(f, opts))
}

func completeMailRulesReorderRequest(ctx context.Context, ac *client.APIClient, request client.RawApiRequest, checkErr func(interface{}, core.Identity) error) (client.RawApiRequest, error) {
	userRuleIDs, body, err := userRuleIDsFromReorderBody(request.Data)
	if err != nil {
		return request, err
	}
	if duplicates := duplicateStrings(userRuleIDs); len(duplicates) > 0 {
		return request, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"duplicate rule_ids in reorder request: %s", strings.Join(duplicates, ", ")).
			WithParam("rule_ids")
	}

	currentRuleIDs, err := fetchAllMailRuleIDs(ctx, ac, request, checkErr)
	if err != nil {
		return request, err
	}
	completedRuleIDs, err := completeRuleIDs(userRuleIDs, currentRuleIDs)
	if err != nil {
		return request, err
	}
	body["rule_ids"] = completedRuleIDs
	request.Data = body
	return request, nil
}

func userRuleIDsFromReorderBody(data interface{}) ([]string, map[string]interface{}, error) {
	body, ok := data.(map[string]interface{})
	if !ok || body == nil {
		return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--data must be a JSON object with non-empty rule_ids").WithParam("--data")
	}
	raw, ok := body["rule_ids"]
	if !ok {
		return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--data.rule_ids is required").WithParam("rule_ids")
	}
	ruleIDs, err := stringList(raw)
	if err != nil {
		return nil, nil, err
	}
	if len(ruleIDs) == 0 {
		return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--data.rule_ids must not be empty").WithParam("rule_ids")
	}
	return ruleIDs, cloneMap(body), nil
}

func stringList(raw interface{}) ([]string, error) {
	items, ok := raw.([]interface{})
	if !ok {
		if typed, ok := raw.([]string); ok {
			items = make([]interface{}, len(typed))
			for i, v := range typed {
				items[i] = v
			}
		} else {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--data.rule_ids must be an array of strings; got %T", raw).WithParam("rule_ids")
		}
	}
	out := make([]string, 0, len(items))
	for i, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--data.rule_ids[%d] must be a string; got %T", i, item).WithParam("rule_ids")
		}
		trimmed := strings.TrimSpace(s)
		if trimmed == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--data.rule_ids[%d] must not be blank", i).WithParam("rule_ids")
		}
		out = append(out, trimmed)
	}
	return out, nil
}

func dryRunCompleteRuleIDs(raw interface{}) []string {
	ruleIDs, err := stringList(raw)
	if err != nil || len(ruleIDs) == 0 {
		return []string{"<user_rule_id_1>", "<user_rule_id_2>", "<remaining_rule_ids_in_current_order>"}
	}
	return append(ruleIDs, "<remaining_rule_ids_in_current_order>")
}

func completeRuleIDs(userRuleIDs, currentRuleIDs []string) ([]string, error) {
	currentSet := make(map[string]bool, len(currentRuleIDs))
	for _, id := range currentRuleIDs {
		currentSet[id] = true
	}
	var unknown []string
	for _, id := range userRuleIDs {
		if !currentSet[id] {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"unknown rule_ids in reorder request: %s", strings.Join(unknown, ", ")).
			WithParam("rule_ids")
	}
	seen := make(map[string]bool, len(userRuleIDs))
	completed := make([]string, 0, len(currentRuleIDs))
	for _, id := range userRuleIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		completed = append(completed, id)
	}
	for _, id := range currentRuleIDs {
		if !seen[id] {
			completed = append(completed, id)
		}
	}
	if len(completed) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"completed rule_ids must not be empty").WithParam("rule_ids")
	}
	return completed, nil
}

func fetchAllMailRuleIDs(ctx context.Context, ac *client.APIClient, reorderRequest client.RawApiRequest, checkErr func(interface{}, core.Identity) error) ([]string, error) {
	var all []string
	var pageToken string
	seenPageTokens := map[string]bool{}
	for {
		result, err := ac.CallAPI(ctx, mailRulesListRequest(reorderRequest, pageToken))
		if err != nil {
			return nil, err
		}
		if checkErr != nil {
			if apiErr := checkErr(result, reorderRequest.As); apiErr != nil {
				return nil, apiErr
			}
		}
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
				"mail rules list response is not a JSON object")
		}
		data, _ := resultMap["data"].(map[string]interface{})
		items := mailRuleItems(data)
		for _, item := range items {
			ruleID, ok := mailRuleID(item)
			if !ok {
				return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
					"mail rules list response item missing rule_id")
			}
			all = append(all, ruleID)
		}
		if !mailRulesHasMore(data) {
			break
		}
		pageToken = mailRulesNextPageToken(data)
		if pageToken == "" {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
				"mail rules list response has_more=true but no page token")
		}
		if seenPageTokens[pageToken] {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
				"mail rules list response repeated page token %q", pageToken)
		}
		seenPageTokens[pageToken] = true
	}
	return all, nil
}

func mailRulesListRequest(reorderRequest client.RawApiRequest, pageToken string) client.RawApiRequest {
	params := cloneMap(reorderRequest.Params)
	if pageToken != "" {
		params["page_token"] = pageToken
	}
	return client.RawApiRequest{
		Method:    "GET",
		URL:       strings.TrimSuffix(reorderRequest.URL, mailRulesReorderPathSuffix) + mailRulesListPathSuffix,
		Params:    params,
		As:        reorderRequest.As,
		ExtraOpts: reorderRequest.ExtraOpts,
	}
}

func mailRuleItems(data map[string]interface{}) []interface{} {
	if data == nil {
		return nil
	}
	for _, key := range []string{"items", "rules"} {
		if items, ok := data[key].([]interface{}); ok {
			return items
		}
	}
	return nil
}

func mailRuleID(item interface{}) (string, bool) {
	m, ok := item.(map[string]interface{})
	if !ok {
		return "", false
	}
	for _, key := range []string{"rule_id", "id"} {
		if id, ok := m[key].(string); ok && strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id), true
		}
	}
	return "", false
}

func mailRulesHasMore(data map[string]interface{}) bool {
	if data == nil {
		return false
	}
	hasMore, _ := data["has_more"].(bool)
	return hasMore
}

func mailRulesNextPageToken(data map[string]interface{}) string {
	if data == nil {
		return ""
	}
	for _, key := range []string{"page_token", "next_page_token"} {
		if token, ok := data[key].(string); ok && token != "" {
			return token
		}
	}
	return ""
}

func duplicateStrings(values []string) []string {
	seen := map[string]bool{}
	reported := map[string]bool{}
	var duplicates []string
	for _, value := range values {
		if seen[value] && !reported[value] {
			duplicates = append(duplicates, value)
			reported[value] = true
		}
		seen[value] = true
	}
	return duplicates
}

func cloneMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
