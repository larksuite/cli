// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
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
		{Name: "table", Desc: "[record-change] table name (from `+db-table-list`); dataloom tables key by name, not id"},
		{Name: "event", Desc: "[record-change] INSERT | UPDATE | UPSERT | DELETE"},
		{Name: "fields", Desc: "[record-change] JSON array of field ids for UPDATE/UPSERT, [\"*\"] = all"},
		{Name: "white-ip-list", Desc: "[webhook] JSON array of allowed IPs"},
		{Name: "approval-code", Desc: "[feishu-approval] approval definition code; omit to match all approval definitions"},
		{Name: "event-type", Desc: "[feishu-approval] approval_instance | approval_task"},
		{Name: "instance-status", Type: "string_array", Desc: "[feishu-approval] statuses for approval_instance"},
		{Name: "task-status", Type: "string_array", Desc: "[feishu-approval] statuses for approval_task"},
		{Name: "status", Desc: "optional initial status: enabled | disabled (default disabled; backend supports create+enable in one call)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if strings.TrimSpace(rctx.Str("name")) == "" {
			return appsValidationParamError("--name", "--name is required")
		}
		cliType := strings.TrimSpace(rctx.Str("trigger-type"))
		if cliType == "" {
			return appsValidationParamError("--trigger-type", "--trigger-type is required (cron/record-change/webhook/feishu-approval)")
		}
		// mapTriggerType also runs inside buildAutomationCreateBody, but
		// re-running it up-front keeps the cross-family guard's error
		// reachable — otherwise an unknown --trigger-type would bail out
		// with the same guard's "belongs to trigger-type" wording, which
		// misleads callers who typoed the type itself.
		if _, err := mapTriggerType(cliType); err != nil {
			return err
		}
		// Reject condition flags that do not belong to the selected type.
		// buildAutomationCreateBody's switch used to silently drop them
		// (e.g. --trigger-type webhook --cron '0 9 * * *' created a webhook
		// with no cron, though the caller believed --cron was set).
		if err := rejectCrossFamilyCondFlags(rctx, cliType); err != nil {
			return err
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
		// Bearer-token redaction reverse invariant: the backend create path
		// re-reads the freshly created trigger through the same read-path
		// converter used by get/list — theoretically capable of returning a
		// plaintext bearer token. On a fresh create the token is not yet
		// enabled and this response should not carry plaintext, but redact
		// for defense-in-depth and to keep every read-shaped output path
		// (create / get / list / update-patch) consistently scrubbed.
		redacted := redactWebhookToken(data)
		trigger, _ := redacted["trigger"].(map[string]interface{})
		rctx.OutFormat(redacted, nil, func(w io.Writer) {
			fmt.Fprintf(w, "created trigger: %v  [%v]  status: %v\n",
				trigger["name"], trigger["trigger_type"], trigger["status"])
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
	name := strings.TrimSpace(rctx.Str("name"))
	if err := validateAutomationNameLen(name); err != nil {
		return nil, err
	}
	body := map[string]interface{}{
		"name":         name,
		"trigger_type": snake,
	}
	if d := strings.TrimSpace(rctx.Str("description")); d != "" {
		if err := validateAutomationDescriptionLen(d); err != nil {
			return nil, err
		}
		body["description"] = d
	}
	// --status is an optional passthrough: when set, backend creates + enables
	// (or leaves disabled) in one call. Omitting the field lets the backend
	// default (disabled) apply, matching the spec's default-disabled invariant.
	if s := strings.TrimSpace(rctx.Str("status")); s != "" {
		if s != "enabled" && s != "disabled" {
			return nil, appsValidationParamError("--status",
				"--status must be enabled or disabled, got %q", s)
		}
		body["status"] = s
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

// parseIPListFlag parses --white-ip-list JSON array; empty → nil (field
// omitted). Each entry is validated as an IPv4/IPv6 address or CIDR, matching
// the defense-in-depth stance the record-change --event whitelist takes —
// silent acceptance of malformed IPs would let a typoed entry (`"1.1.1.1 "`
// with trailing space, `"not-an-ip"`, or `"10.0.0.256"`) narrow the webhook
// caller allowlist to nothing while the operator believes it is enforcing
// origin restrictions.
func parseIPListFlag(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return nil, appsValidationParamError("--white-ip-list", "--white-ip-list must be a JSON array of strings: %v", err)
	}
	out := make([]string, 0, len(arr))
	for i, entry := range arr {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			return nil, appsValidationParamError("--white-ip-list",
				"--white-ip-list entry %d is empty; either drop it or provide a valid IP/CIDR", i)
		}
		if net.ParseIP(trimmed) != nil {
			out = append(out, trimmed)
			continue
		}
		if _, _, cidrErr := net.ParseCIDR(trimmed); cidrErr == nil {
			out = append(out, trimmed)
			continue
		}
		return nil, appsValidationParamError("--white-ip-list",
			"--white-ip-list entry %d %q is not a valid IPv4/IPv6 address or CIDR block", i, entry)
	}
	return out, nil
}
