// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// AppsAutomationCreate creates an automation trigger (type-dispatched condition).
var AppsAutomationCreate = common.Shortcut{
	Service:     appsService,
	Command:     "+automation-create",
	Description: "Create an automation trigger (cron/record-change/webhook/feishu-approval); created disabled",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +automation-create --app-id <id> --name daily --trigger-type cron --cron '0 9 * * *'",
		"Example: lark-cli apps +automation-create --app-id <id> --name onUpd --trigger-type record-change --table <tbl> --event UPDATE",
		"Example: lark-cli apps +automation-create --app-id <id> --name hook --trigger-type webhook",
		"Example: lark-cli apps +automation-create --app-id <id> --name apv --trigger-type feishu-approval --event-type approval_instance --instance-status APPROVED",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "name", Desc: "trigger name (unique within app, <=100 chars)", Required: true},
		{Name: "trigger-type", Desc: "cron | record-change | webhook | feishu-approval", Required: true},
		{Name: "description", Desc: "optional description (<=50 chars)"},
		{Name: "cron", Desc: "[cron] 5-field cron expression, e.g. '0 9 * * *' (min interval 30m)"},
		{Name: "timezone", Desc: "[cron] IANA timezone (default Asia/Shanghai)"},
		{Name: "table", Desc: "[record-change] dataloom table id"},
		{Name: "event", Desc: "[record-change] INSERT | UPDATE | UPSERT | DELETE"},
		{Name: "fields", Desc: "[record-change] JSON array of field ids for UPDATE/UPSERT, [\"*\"] = all"},
		{Name: "white-ip-list", Desc: "[webhook] JSON array of allowed IPs"},
		{Name: "approval-code", Desc: "[feishu-approval] optional; omit to match all approval definitions"},
		{Name: "event-type", Desc: "[feishu-approval] approval_instance | approval_task"},
		{Name: "instance-status", Type: "string_array", Desc: "[feishu-approval] statuses for approval_instance"},
		{Name: "task-status", Type: "string_array", Desc: "[feishu-approval] statuses for approval_task"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if strings.TrimSpace(rctx.Str("name")) == "" {
			return appsValidationParamError("--name", "--name is required")
		}
		if strings.TrimSpace(rctx.Str("trigger-type")) == "" {
			return appsValidationParamError("--trigger-type", "--trigger-type is required (cron/record-change/webhook/feishu-approval)")
		}
		_, err := buildAutomationCreateBody(rctx)
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		body, _ := buildAutomationCreateBody(rctx)
		return common.NewDryRunAPI().
			POST(automationListPath(appID)).
			Desc("Create automation trigger").
			Body(body)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		body, err := buildAutomationCreateBody(rctx)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("POST", automationListPath(appID), nil, body)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "created trigger: %v  [%v]  status: %v\n",
				data["name"], data["trigger_type"], data["status"])
		})
		return nil
	},
}

// buildAutomationCreateBody assembles {name, description?, trigger_type, <type>_condition}.
func buildAutomationCreateBody(rctx *common.RuntimeContext) (map[string]interface{}, error) {
	cliType := strings.TrimSpace(rctx.Str("trigger-type"))
	snake, err := mapTriggerType(cliType)
	if err != nil {
		return nil, err
	}
	body := map[string]interface{}{
		"name":         strings.TrimSpace(rctx.Str("name")),
		"trigger_type": snake,
	}
	if d := strings.TrimSpace(rctx.Str("description")); d != "" {
		body["description"] = d
	}
	switch cliType {
	case "cron":
		cond, err := buildCronCondition(rctx.Str("cron"), rctx.Str("timezone"))
		if err != nil {
			return nil, err
		}
		body["cron_condition"] = cond
	case "record-change":
		fields, err := parseFieldsFlag(rctx.Str("fields"))
		if err != nil {
			return nil, err
		}
		cond, err := buildRecordChangeCondition(rctx.Str("table"), rctx.Str("event"), fields)
		if err != nil {
			return nil, err
		}
		body["record_change_condition"] = cond
	case "webhook":
		ipList, err := parseIPListFlag(rctx.Str("white-ip-list"))
		if err != nil {
			return nil, err
		}
		body["webhook_condition"] = buildWebhookCondition(ipList)
	case "feishu-approval":
		eventType := strings.TrimSpace(rctx.Str("event-type"))
		if eventType == "" {
			return nil, appsValidationParamError("--event-type", "--event-type is required for feishu-approval (approval_instance/approval_task)")
		}
		raw := rctx.StrArray("instance-status")
		if eventType == "approval_task" {
			raw = rctx.StrArray("task-status")
		}
		// buildApprovalCondition stores the passed statuses verbatim (it only
		// uppercases for validation), so normalize to the uppercase enum here to
		// guarantee the backend receives canonical values (foundation review).
		statuses := normalizeApprovalStatuses(raw)
		cond, err := buildApprovalCondition(rctx.Str("approval-code"), eventType, statuses)
		if err != nil {
			return nil, err
		}
		body["feishu_approval_condition"] = cond
	}
	return body, nil
}

// normalizeApprovalStatuses trims and uppercases each status so the body carries
// the canonical enum values expected by the backend.
func normalizeApprovalStatuses(raw []string) []string {
	if len(raw) == 0 {
		return raw
	}
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		out = append(out, strings.ToUpper(strings.TrimSpace(s)))
	}
	return out
}

// parseFieldsFlag parses --fields JSON array; empty → nil.
func parseFieldsFlag(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil, appsValidationParamError("--fields", "--fields must be a JSON array of strings: %v", err)
	}
	return arr, nil
}

// parseIPListFlag parses --white-ip-list JSON array; empty → nil (field omitted).
func parseIPListFlag(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil, appsValidationParamError("--white-ip-list", "--white-ip-list must be a JSON array of strings: %v", err)
	}
	return arr, nil
}
