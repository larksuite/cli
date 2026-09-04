// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

const mailRulesReorderSchemaPath = "mail.user_mailbox.rules.reorder"

func isMailRulesReorder(opts *ServiceMethodOptions) bool {
	return opts != nil && opts.SchemaPath == mailRulesReorderSchemaPath
}

func completeMailRulesReorderRequest(ctx context.Context, ac *client.APIClient, opts *ServiceMethodOptions, request *client.RawApiRequest) error {
	if !isMailRulesReorder(opts) {
		return nil
	}

	requestedIDs, key, ok, err := mailRulesRequestedIDs(request.Data)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if duplicates := duplicateStrings(requestedIDs); len(duplicates) > 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"duplicate mail rule id(s): %s", strings.Join(duplicates, ", ")).
			WithParam(key)
	}

	currentIDs, err := fetchCurrentMailRuleIDs(ctx, ac, *request)
	if err != nil {
		return err
	}
	completion := completeMailRuleIDs(requestedIDs, currentIDs)
	if len(completion.unknown) > 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"unknown mail rule id(s): %s", strings.Join(completion.unknown, ", ")).
			WithParam(key).
			WithHint("run: lark-cli mail user_mailbox.rules list --params '{\"user_mailbox_id\":\"<mailbox>\"}', then retry reorder with existing rule IDs")
	}
	if len(completion.ambiguous) > 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"ambiguous mail rule id prefix(es): %s", strings.Join(completion.ambiguous, "; ")).
			WithParam(key).
			WithHint("use a longer rule ID prefix or the full rule ID")
	}
	if duplicates := duplicateStrings(completion.resolved); len(duplicates) > 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"duplicate mail rule id(s): %s", strings.Join(duplicates, ", ")).
			WithParam(key)
	}

	body := request.Data.(map[string]interface{})
	body[key] = completion.completed
	return nil
}

func mailRulesRequestedIDs(data interface{}) ([]string, string, bool, error) {
	body, ok := data.(map[string]interface{})
	if !ok || body == nil {
		return nil, "", false, nil
	}
	for _, key := range []string{"rule_ids", "ruleIds"} {
		raw, ok := body[key]
		if !ok {
			continue
		}
		ids, err := stringList(raw)
		if err != nil {
			return nil, key, true, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"%s must be an array of rule ID strings", key).
				WithParam(key).
				WithCause(err)
		}
		return ids, key, true, nil
	}
	return nil, "", false, nil
}

func stringList(raw interface{}) ([]string, error) {
	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...), nil
	case []interface{}:
		out := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "item %d is %T", i, item)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "value is %T", raw)
	}
}

func duplicateStrings(values []string) []string {
	seen := map[string]bool{}
	dups := map[string]bool{}
	var ordered []string
	for _, value := range values {
		if seen[value] {
			if !dups[value] {
				dups[value] = true
				ordered = append(ordered, value)
			}
			continue
		}
		seen[value] = true
	}
	return ordered
}

func fetchCurrentMailRuleIDs(ctx context.Context, ac *client.APIClient, request client.RawApiRequest) ([]string, error) {
	listURL, ok := strings.CutSuffix(request.URL, "/reorder")
	if !ok {
		return nil, errs.NewInternalError(errs.SubtypeUnknown,
			"mail rules reorder URL does not end with /reorder: %s", request.URL)
	}
	params := map[string]interface{}{}
	for k, v := range request.Params {
		params[k] = v
	}
	if _, ok := params["page_size"]; !ok {
		params["page_size"] = 100
	}
	result, err := ac.PaginateAll(ctx, client.RawApiRequest{
		Method: "GET",
		URL:    listURL,
		Params: params,
		As:     request.As,
	}, client.PaginationOptions{PageLimit: 0, PageDelay: -1, Identity: request.As})
	if err != nil {
		return nil, withMailRuleListHint(err)
	}
	if err := ac.CheckResponse(result, request.As); err != nil {
		return nil, withMailRuleListHint(err)
	}
	return mailRuleIDsFromListResult(result), nil
}

func mailRuleIDsFromListResult(result interface{}) []string {
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return nil
	}
	data, ok := resultMap["data"].(map[string]interface{})
	if !ok {
		return nil
	}
	arrayField := output.FindArrayField(data)
	if arrayField == "" {
		for _, key := range []string{"rules", "rule_list", "items"} {
			if _, ok := data[key]; ok {
				arrayField = key
				break
			}
		}
	}
	items, ok := data[arrayField].([]interface{})
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		for _, key := range []string{"rule_id", "id", "ruleId"} {
			if id, ok := m[key].(string); ok && id != "" {
				ids = append(ids, id)
				break
			}
		}
	}
	return ids
}

type mailRuleIDCompletion struct {
	completed []string
	resolved  []string
	unknown   []string
	ambiguous []string
}

func completeMailRuleIDs(requestedIDs, currentIDs []string) mailRuleIDCompletion {
	result := mailRuleIDCompletion{
		resolved: make([]string, 0, len(requestedIDs)),
	}
	for _, id := range requestedIDs {
		resolved, matches := resolveMailRuleID(id, currentIDs)
		switch {
		case len(matches) == 0:
			result.unknown = append(result.unknown, id)
		case len(matches) > 1:
			result.ambiguous = append(result.ambiguous, fmt.Sprintf("%s (%s)", id, strings.Join(matches, ", ")))
		default:
			result.resolved = append(result.resolved, resolved)
		}
	}
	if len(result.unknown) > 0 || len(result.ambiguous) > 0 {
		return result
	}

	requested := map[string]bool{}
	for _, id := range result.resolved {
		requested[id] = true
	}
	result.completed = append([]string(nil), result.resolved...)
	for _, id := range currentIDs {
		if !requested[id] {
			result.completed = append(result.completed, id)
		}
	}
	return result
}

func resolveMailRuleID(requestedID string, currentIDs []string) (string, []string) {
	for _, currentID := range currentIDs {
		if requestedID == currentID {
			return currentID, []string{currentID}
		}
	}
	var matches []string
	for _, currentID := range currentIDs {
		if strings.HasPrefix(currentID, requestedID) {
			matches = append(matches, currentID)
		}
	}
	if len(matches) == 1 {
		return matches[0], matches
	}
	return "", matches
}

func withMailRuleListHint(err error) error {
	return withMailRuleHint(err, "could not fetch the complete mail rule list; reorder was not called")
}

func withMailRuleReorderHint(err error) error {
	return withMailRuleHint(err, "mail rules may have changed; run lark-cli mail user_mailbox.rules list again, then retry reorder")
}

func withMailRuleHint(err error, hint string) error {
	var validation *errs.ValidationError
	if errors.As(err, &validation) {
		return validation.WithHint(hint)
	}
	var authentication *errs.AuthenticationError
	if errors.As(err, &authentication) {
		return authentication.WithHint(hint)
	}
	var permission *errs.PermissionError
	if errors.As(err, &permission) {
		return permission.WithHint(hint)
	}
	var config *errs.ConfigError
	if errors.As(err, &config) {
		return config.WithHint(hint)
	}
	var network *errs.NetworkError
	if errors.As(err, &network) {
		return network.WithHint(hint)
	}
	var api *errs.APIError
	if errors.As(err, &api) {
		return api.WithHint(hint)
	}
	var securityPolicy *errs.SecurityPolicyError
	if errors.As(err, &securityPolicy) {
		return securityPolicy.WithHint(hint)
	}
	var contentSafety *errs.ContentSafetyError
	if errors.As(err, &contentSafety) {
		return contentSafety.WithHint(hint)
	}
	var internal *errs.InternalError
	if errors.As(err, &internal) {
		return internal.WithHint(hint)
	}
	var confirmation *errs.ConfirmationRequiredError
	if errors.As(err, &confirmation) {
		return confirmation.WithHint(hint)
	}
	return err
}

func mailRulesReorderCheckResponse(ac *client.APIClient, result interface{}, identity core.Identity) error {
	if err := ac.CheckResponse(result, identity); err != nil {
		return withMailRuleReorderHint(err)
	}
	return nil
}
