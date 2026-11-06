// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	dashboardPushTimeLayout = "2006-01-02 15:04"
	dashboardPushImageMode  = "image"
	dashboardPushNoRepeat   = "NO_REPEAT"
	dashboardPushDaily      = "DAILY"
)

type dashboardPushValue struct {
	ValueType string                 `json:"value_type"`
	Value     any                    `json:"value"`
	ExtraInfo map[string]interface{} `json:"extra_info,omitempty"`
}

type dashboardPushStep struct {
	ID    string                 `json:"id"`
	Type  string                 `json:"type"`
	Title string                 `json:"title"`
	Next  *string                `json:"next"`
	Data  map[string]interface{} `json:"data"`
}

type dashboardPushWorkflow struct {
	ClientToken string              `json:"client_token"`
	Title       string              `json:"title"`
	Steps       []dashboardPushStep `json:"steps"`
}

var BaseDashboardPushCreate = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-push-create",
	Description: "Create and enable a scheduled dashboard screenshot workflow",
	Risk:        "write",
	Scopes:      []string{"base:workflow:create", "base:workflow:update"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "base-token", Desc: "target base token", Required: true},
		{Name: "dashboard-id", Desc: "dashboard block ID", Required: true},
		{Name: "title", Desc: "workflow and message title", Required: true},
		{Name: "send-at", Desc: "send time in Base timezone (yyyy-MM-dd HH:mm)", Required: true},
		{Name: "repeat", Desc: "schedule frequency", Required: true, Enum: []string{dashboardPushNoRepeat, dashboardPushDaily}},
		{Name: "receiver", Type: "string_array", Desc: "receiver open ID (ou_ user or oc_ group); repeat for multiple receivers", Required: true},
		{Name: "client-token", Desc: "idempotency token for workflow creation", Required: true},
		{Name: "content-mode", Desc: "dashboard message content mode", Default: dashboardPushImageMode, Enum: []string{dashboardPushImageMode}},
	},
	Tips: []string{
		"The target environment must already support Dashboard image segments in public Workflow create/get.",
		"The send time is interpreted in the Base timezone; this command does not convert it from the local timezone.",
		"Screenshots render with the Base owner identity. Confirm every receiver may see the dashboard data.",
		"AI dashboard summaries are not supported in this release; --content-mode only accepts image.",
		"If enablement is not confirmed, follow the returned get-first recovery hint instead of creating another workflow.",
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		_, err := buildDashboardPushWorkflow(runtime)
		return err
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		workflow, _ := buildDashboardPushWorkflow(runtime)
		return common.NewDryRunAPI().
			POST("/open-apis/base/v3/bases/:base_token/workflows").
			Body(workflow).
			Desc("Create the disabled dashboard screenshot workflow.").
			PATCH("/open-apis/base/v3/bases/:base_token/workflows/:workflow_id/enable").
			Body(map[string]interface{}{}).
			Desc("Enable the workflow only after create returns a workflow ID.").
			Set("base_token", strings.TrimSpace(runtime.Str("base-token"))).
			Set("workflow_id", "<created_workflow_id>")
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		return runDashboardPushCreate(runtime)
	},
}

func buildDashboardPushWorkflow(runtime *common.RuntimeContext) (dashboardPushWorkflow, error) {
	baseToken := strings.TrimSpace(runtime.Str("base-token"))
	dashboardID := strings.TrimSpace(runtime.Str("dashboard-id"))
	title := strings.TrimSpace(runtime.Str("title"))
	sendAt := strings.TrimSpace(runtime.Str("send-at"))
	repeat := strings.TrimSpace(runtime.Str("repeat"))
	clientToken := strings.TrimSpace(runtime.Str("client-token"))
	contentMode := strings.TrimSpace(runtime.Str("content-mode"))

	for _, required := range []struct {
		flag  string
		value string
	}{
		{flag: "--base-token", value: baseToken},
		{flag: "--dashboard-id", value: dashboardID},
		{flag: "--title", value: title},
		{flag: "--send-at", value: sendAt},
		{flag: "--repeat", value: repeat},
		{flag: "--client-token", value: clientToken},
	} {
		if required.value == "" {
			return dashboardPushWorkflow{}, baseFlagErrorf("%s must not be blank", required.flag)
		}
	}
	if contentMode != dashboardPushImageMode {
		return dashboardPushWorkflow{}, baseFlagErrorf("--content-mode only supports image; AI dashboard summaries are not supported")
	}
	parsedTime, err := time.Parse(dashboardPushTimeLayout, sendAt)
	if err != nil || parsedTime.Format(dashboardPushTimeLayout) != sendAt {
		return dashboardPushWorkflow{}, baseFlagErrorf("--send-at must use strict yyyy-MM-dd HH:mm format")
	}
	if repeat != dashboardPushNoRepeat && repeat != dashboardPushDaily {
		return dashboardPushWorkflow{}, baseFlagErrorf("--repeat must be one of %s, %s", dashboardPushNoRepeat, dashboardPushDaily)
	}

	receivers, err := dashboardPushReceivers(runtime.StrArray("receiver"))
	if err != nil {
		return dashboardPushWorkflow{}, err
	}
	messageID := "dashboard_push_message"
	return dashboardPushWorkflow{
		ClientToken: clientToken,
		Title:       title,
		Steps: []dashboardPushStep{
			{
				ID: "dashboard_push_timer", Type: "TimerTrigger", Title: "Schedule dashboard screenshot", Next: &messageID,
				Data: map[string]interface{}{
					"rule": repeat, "start_time": sendAt, "is_never_end": repeat != dashboardPushNoRepeat,
				},
			},
			{
				ID: "dashboard_push_message", Type: "LarkMessageAction", Title: title, Next: nil,
				Data: map[string]interface{}{
					"receiver": receivers, "send_to_everyone": false,
					"title": []dashboardPushValue{{ValueType: "text", Value: title}},
					"content": []dashboardPushValue{{
						ValueType: "ref", Value: "$.dashboard.image",
						ExtraInfo: map[string]interface{}{"dashboard_name": dashboardID},
					}},
					"btn_list": []interface{}{},
				},
			},
		},
	}, nil
}

func dashboardPushReceivers(ids []string) ([]dashboardPushValue, error) {
	if len(ids) == 0 {
		return nil, baseFlagErrorf("--receiver requires at least one ou_ user or oc_ group ID")
	}
	seen := make(map[string]struct{}, len(ids))
	receivers := make([]dashboardPushValue, 0, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			return nil, baseFlagErrorf("--receiver must not contain blank values")
		}
		valueType := ""
		switch {
		case strings.HasPrefix(id, "ou_"):
			valueType = "user"
		case strings.HasPrefix(id, "oc_"):
			valueType = "group"
		default:
			return nil, baseFlagErrorf("--receiver %q must start with ou_ for a user or oc_ for a group", id)
		}
		if _, exists := seen[id]; exists {
			return nil, baseFlagErrorf("--receiver contains duplicate ID %q", id)
		}
		seen[id] = struct{}{}
		receivers = append(receivers, dashboardPushValue{ValueType: valueType, Value: map[string]interface{}{"id": id}})
	}
	return receivers, nil
}

func runDashboardPushCreate(runtime *common.RuntimeContext) error {
	workflow, err := buildDashboardPushWorkflow(runtime)
	if err != nil {
		return err
	}
	baseToken := strings.TrimSpace(runtime.Str("base-token"))
	created, err := baseV3Call(runtime, "POST", baseV3Path("bases", baseToken, "workflows"), nil, workflow)
	if err != nil {
		return err
	}
	workflowID, _ := created["workflow_id"].(string)
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "workflow create response is missing workflow_id")
	}

	_, enableErr := baseV3Call(runtime, "PATCH", baseV3Path("bases", baseToken, "workflows", workflowID, "enable"), nil, map[string]interface{}{})
	if enableErr != nil {
		return dashboardPushEnableFailure(runtime, baseToken, workflowID, enableErr)
	}
	runtime.Out(map[string]interface{}{
		"workflow_id": workflowID,
		"created":     true, "enable_outcome": "confirmed", "status": "enabled", "enabled": true,
	}, nil)
	return nil
}

func dashboardPushEnableFailure(runtime *common.RuntimeContext, baseToken, workflowID string, err error) error {
	presented := runtime.PresentError(err)
	problem, typed := errs.ProblemOf(presented)
	rejected := typed && !problem.Retryable && (problem.Category == errs.CategoryValidation ||
		problem.Category == errs.CategoryAuthorization ||
		(problem.Category == errs.CategoryAPI && problem.Subtype == errs.SubtypeNotFound))

	outcome, status, recoveryHint := "unknown", "unknown",
		"Run +workflow-get first; stop if enabled, run +workflow-enable only if disabled, and retry get if status remains unknown."
	var enabled any
	if rejected {
		outcome, status, enabled = "rejected", "disabled", false
		recoveryHint = "Correct the rejected prerequisite, then run +workflow-enable for the returned workflow ID; do not create another workflow."
	}
	errorData := map[string]interface{}{"type": "internal", "subtype": "unknown", "message": presented.Error()}
	if typed {
		errorData = map[string]interface{}{
			"type": problem.Category, "subtype": problem.Subtype, "code": problem.Code,
			"message": problem.Message, "hint": problem.Hint, "log_id": problem.LogID,
			"retryable": problem.Retryable,
		}
	}
	return runtime.OutPartialFailure(map[string]interface{}{
		"workflow_id": workflowID, "created": true,
		"enable_outcome": outcome, "status": status, "enabled": enabled,
		"error": errorData, "recovery_hint": recoveryHint,
		"recovery_command": "lark-cli base +workflow-get --base-token " + baseToken + " --workflow-id " + workflowID,
	}, nil)
}
