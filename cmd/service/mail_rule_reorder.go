// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
)

func completeMailRuleReorderRequest(ctx context.Context, ac *client.APIClient, request client.RawApiRequest) (client.RawApiRequest, error) {
	if !isMailRuleReorderRequest(request) {
		return request, nil
	}
	input, err := normalizeMailRuleIDs(request.Data)
	if err != nil {
		return client.RawApiRequest{}, err
	}
	listReq := client.RawApiRequest{
		Method: "GET",
		URL:    strings.TrimSuffix(request.URL, "/reorder"),
		As:     request.As,
	}
	resp, err := ac.DoAPI(ctx, listReq)
	if err != nil {
		return client.RawApiRequest{}, err
	}
	parsed, err := client.ParseJSONResponse(resp)
	if err != nil {
		return client.RawApiRequest{}, client.WrapJSONResponseParseError(err, resp.RawBody)
	}
	if err := ac.CheckResponse(parsed, request.As); err != nil {
		return client.RawApiRequest{}, err
	}
	parsedMap, _ := parsed.(map[string]interface{})
	current, err := extractMailRuleOrder(parsedMap)
	if err != nil {
		return client.RawApiRequest{}, err
	}
	completed, err := completeMailRuleOrder(input, current)
	if err != nil {
		return client.RawApiRequest{}, err
	}
	data, _ := request.Data.(map[string]interface{})
	next := make(map[string]interface{}, len(data)+1)
	for k, v := range data {
		next[k] = v
	}
	next["rule_ids"] = completed
	request.Data = next
	return request, nil
}

func isMailRuleReorderRequest(request client.RawApiRequest) bool {
	method := strings.ToUpper(request.Method)
	return (method == "POST" || method == "PUT") &&
		strings.HasPrefix(request.URL, "/open-apis/mail/v1/user_mailboxes/") &&
		strings.HasSuffix(request.URL, "/rules/reorder")
}

func normalizeMailRuleIDs(data interface{}) ([]string, error) {
	body, ok := data.(map[string]interface{})
	if !ok {
		return nil, mailRuleReorderValidation("--data must be a JSON object with rule_ids")
	}
	raw, ok := body["rule_ids"]
	if !ok {
		return nil, mailRuleReorderValidation("provide at least one rule ID")
	}
	var values []interface{}
	switch ids := raw.(type) {
	case []interface{}:
		values = ids
	case []string:
		values = make([]interface{}, 0, len(ids))
		for _, id := range ids {
			values = append(values, id)
		}
	default:
		return nil, mailRuleReorderValidation("rule_ids must be an array")
	}
	ids := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, rawID := range values {
		id := strings.TrimSpace(strValue(rawID))
		if id == "" || seen[id] {
			continue
		}
		ids = append(ids, id)
		seen[id] = true
	}
	if len(ids) == 0 {
		return nil, mailRuleReorderValidation("provide at least one rule ID")
	}
	return ids, nil
}

func extractMailRuleOrder(data map[string]interface{}) ([]string, error) {
	rules, ok := firstMailRuleList(data)
	if !ok {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "rules list response missing rules")
	}
	order := make([]string, 0, len(rules))
	for _, raw := range rules {
		rule, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		id := strings.TrimSpace(strValue(rule["rule_id"]))
		if id == "" {
			id = strings.TrimSpace(strValue(rule["id"]))
		}
		if id != "" {
			order = append(order, id)
		}
	}
	return order, nil
}

func firstMailRuleList(data map[string]interface{}) ([]interface{}, bool) {
	for _, key := range []string{"rules", "items", "user_mailbox_rules"} {
		if rules, ok := data[key].([]interface{}); ok {
			return rules, true
		}
	}
	if nested, ok := data["data"].(map[string]interface{}); ok {
		return firstMailRuleList(nested)
	}
	return nil, false
}

func completeMailRuleOrder(inputIDs []string, currentRuleIDs []string) ([]string, error) {
	input, err := normalizeMailRuleIDs(map[string]interface{}{"rule_ids": inputIDs})
	if err != nil {
		return nil, err
	}
	if len(currentRuleIDs) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition, "current mailbox has no inbox rules to reorder")
	}
	existing := map[string]bool{}
	for _, id := range currentRuleIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			existing[id] = true
		}
	}
	if len(existing) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition, "current mailbox has no inbox rules to reorder")
	}
	selectedSet := map[string]bool{}
	var missing []string
	for _, id := range input {
		if !existing[id] {
			missing = append(missing, id)
			continue
		}
		selectedSet[id] = true
	}
	if len(missing) > 0 {
		return nil, mailRuleReorderValidation("rule ID not found: %s", strings.Join(missing, ", "))
	}
	completed := append([]string(nil), input...)
	for _, id := range currentRuleIDs {
		id = strings.TrimSpace(id)
		if id != "" && !selectedSet[id] {
			completed = append(completed, id)
		}
	}
	return completed, nil
}

func mailRuleReorderValidation(format string, args ...interface{}) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...).WithParam("--data")
}

func strValue(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}
