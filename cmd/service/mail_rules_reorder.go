// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
)

const mailRuleReorderMethodID = "user_mailbox.rule.reorder"

func isMailRuleReorder(method meta.Method) bool {
	return method.ID == mailRuleReorderMethodID
}

func completeMailRuleReorder(ctx context.Context, ac *client.APIClient, request client.RawApiRequest) (client.RawApiRequest, error) {
	inputIDs, err := extractRuleIDs(request.Data)
	if err != nil {
		return request, err
	}
	currentIDs, err := listMailRuleIDs(ctx, ac, request)
	if err != nil {
		return request, err
	}
	fullIDs, err := mergeRuleIDs(inputIDs, currentIDs)
	if err != nil {
		return request, err
	}
	data := cloneJSONMap(request.Data)
	data["rule_ids"] = fullIDs
	request.Data = data
	return request, nil
}

func extractRuleIDs(data any) ([]string, error) {
	body, ok := data.(map[string]any)
	if !ok {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--data.rule_ids must be a string array").WithParam("--data.rule_ids")
	}
	raw, ok := body["rule_ids"]
	if !ok {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--data.rule_ids must be a string array").WithParam("--data.rule_ids")
	}

	switch ids := raw.(type) {
	case []string:
		return validateRuleIDs(ids)
	case []any:
		out := make([]string, 0, len(ids))
		for _, item := range ids {
			id, ok := item.(string)
			if !ok {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--data.rule_ids must be a string array").WithParam("--data.rule_ids")
			}
			out = append(out, id)
		}
		return validateRuleIDs(out)
	default:
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--data.rule_ids must be a string array").WithParam("--data.rule_ids")
	}
}

func validateRuleIDs(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--data.rule_ids must contain at least one rule ID").WithParam("--data.rule_ids")
	}
	seen := map[string]struct{}{}
	for _, id := range ids {
		if id == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--data.rule_ids must not contain empty rule IDs").WithParam("--data.rule_ids")
		}
		if _, ok := seen[id]; ok {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "duplicate rule ID in --data.rule_ids: %s", id).WithParam("--data.rule_ids")
		}
		seen[id] = struct{}{}
	}
	return ids, nil
}

func mergeRuleIDs(inputIDs, currentIDs []string) ([]string, error) {
	inputSet := map[string]struct{}{}
	for _, id := range inputIDs {
		inputSet[id] = struct{}{}
	}
	currentSet := map[string]struct{}{}
	for _, id := range currentIDs {
		currentSet[id] = struct{}{}
	}
	for _, id := range inputIDs {
		if _, ok := currentSet[id]; !ok {
			return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition, "unknown rule ID in --data.rule_ids: %s", id).
				WithParam("--data.rule_ids").
				WithHint("run: lark-cli mail user_mailbox.rules list --params '{\"user_mailbox_id\":\"<mailbox>\"}' --as user")
		}
	}

	nextInput := 0
	out := make([]string, 0, len(currentIDs))
	for _, id := range currentIDs {
		if _, selected := inputSet[id]; selected {
			out = append(out, inputIDs[nextInput])
			nextInput++
			continue
		}
		out = append(out, id)
	}
	return out, nil
}

func listMailRuleIDs(ctx context.Context, ac *client.APIClient, request client.RawApiRequest) ([]string, error) {
	listRequest := request
	listRequest.Method = "GET"
	listRequest.URL = strings.TrimSuffix(request.URL, "/reorder")
	listRequest.Params = nil
	listRequest.Data = nil
	listRequest.ExtraOpts = nil

	resp, err := ac.DoAPI(ctx, listRequest)
	if err != nil {
		return nil, err
	}
	result, err := client.ParseJSONResponse(resp)
	if err != nil {
		return nil, client.WrapJSONResponseParseError(err, resp.RawBody)
	}
	if apiErr := ac.CheckResponse(result, request.As); apiErr != nil {
		return nil, apiErr
	}
	return extractCurrentRuleIDs(result), nil
}

func extractCurrentRuleIDs(result any) []string {
	resultMap, _ := result.(map[string]any)
	data, _ := resultMap["data"].(map[string]any)
	items, _ := data["items"].([]any)
	ids := make([]string, 0, len(items))
	for _, item := range items {
		rule, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := rule["id"].(string)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func cloneJSONMap(data any) map[string]any {
	src, _ := data.(map[string]any)
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mailRuleReorderDryRun(f *cmdutil.Factory, request client.RawApiRequest, config *core.CliConfig, format string) error {
	if _, err := extractRuleIDs(request.Data); err != nil {
		return err
	}
	dr := cmdutil.NewDryRunAPI().
		Desc("mail rule reorder completes partial rule_ids at execution time").
		GET(strings.TrimSuffix(request.URL, "/reorder")).
		Desc("Step 1: list current mail rules to read the full rule ID order").
		POST(request.URL).
		Desc("Step 2: replace --data.rule_ids with the completed full order, then reorder")
	if !util.IsNil(request.Data) {
		dr.Body(map[string]any{
			"rule_ids": "<completed from current list; selected slots keep current positions and use input order>",
			"input":    request.Data,
		})
	}
	dr.Set("as", string(request.As))
	dr.Set("appId", config.AppID)
	if config.UserOpenId != "" {
		dr.Set("userOpenId", config.UserOpenId)
	}
	return printDryRunAPI(f.IOStreams.Out, dr, format)
}

func printDryRunAPI(w io.Writer, dr *cmdutil.DryRunAPI, format string) error {
	fmt.Fprintln(w, "=== Dry Run ===")
	if format == "pretty" {
		fmt.Fprint(w, dr.Format())
		return nil
	}
	output.PrintJson(w, dr)
	return nil
}
