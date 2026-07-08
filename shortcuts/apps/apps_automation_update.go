// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// AppsAutomationUpdate is the unified trigger-modify entry. Webhook URL/Token
// actions dispatch to apps_automation_webhook.go via bool action flags on the
// same command (--reset-url / --enable-token / --disable-token / --reset-token)
// rather than as separate +automation-* commands, because the automation spec
// (docx/INixwF5apisF4kkNvOrcwLtInig §范围) fixes the 6 shared verbs to
// list/get/create/update/enable/disable — the webhook credential lifecycle is
// intentionally packed into --update via action flags, not a family of new
// commands. Otherwise Execute sends a PUT to update the trigger condition.
var AppsAutomationUpdate = common.Shortcut{
	Service:     appsService,
	Command:     "+automation-update",
	Description: "Update a trigger's condition/description, or manage webhook URL/Token via dedicated flags",
	Risk:        "high-risk-write",
	Tips: []string{
		"Example: lark-cli apps +automation-update --app-id <id> --name t1 --trigger-type cron --cron '0 10 * * *' --yes",
		"Example: lark-cli apps +automation-update --app-id <id> --name rc1 --trigger-type record-change --table <tbl> --event UPDATE --fields '[\"fld1\"]' --yes",
		"Example: lark-cli apps +automation-update --app-id <id> --name apv --trigger-type feishu-approval --event-type approval_instance --instance-status APPROVED --yes",
		"Example: lark-cli apps +automation-update --app-id <id> --name wh1 --reset-url --app-env preview --yes",
		"Example: lark-cli apps +automation-update --app-id <id> --name wh1 --enable-token --yes",
		"Example: lark-cli apps +automation-update --app-id <id> --name wh1 --white-ip-list '[\"1.1.1.1\"]' --yes",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "name", Desc: "trigger name", Required: true},
		{Name: "trigger-type", Desc: "type of the trigger being updated (for condition PATCH)"},
		{Name: "description", Desc: "new description"},
		{Name: "cron", Desc: "[cron] new 5-field cron expression"},
		{Name: "timezone", Desc: "[cron] new timezone"},
		{Name: "table", Desc: "[record-change] dataloom table id"},
		{Name: "event", Desc: "[record-change] INSERT | UPDATE | UPSERT | DELETE"},
		{Name: "fields", Desc: "[record-change] JSON array of field ids for UPDATE/UPSERT, [\"*\"] = all"},
		{Name: "approval-code", Desc: "[feishu-approval] optional; omit to match all approval definitions"},
		{Name: "event-type", Desc: "[feishu-approval] approval_instance | approval_task"},
		{Name: "instance-status", Type: "string_array", Desc: "[feishu-approval] statuses for approval_instance"},
		{Name: "task-status", Type: "string_array", Desc: "[feishu-approval] statuses for approval_task"},
		{Name: "white-ip-list", Desc: "[webhook] full replacement JSON array of allowed IPs"},
		{Name: "reset-url", Type: "bool", Desc: "[webhook] rotate callback URL for --app-env (old URL invalidated)"},
		{Name: "app-env", Desc: "[webhook] preview | runtime (required with --reset-url)"},
		{Name: "enable-token", Type: "bool", Desc: "[webhook] enable bearer token (shown once)"},
		{Name: "disable-token", Type: "bool", Desc: "[webhook] disable bearer token (irreversible)"},
		{Name: "reset-token", Type: "bool", Desc: "[webhook] rotate bearer token (old token invalidated, shown once)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := automationValidateName(ctx, rctx); err != nil {
			return err
		}
		// webhook action flags are mutually exclusive; at most one per invocation.
		var setFlags []string
		for _, f := range []string{"reset-url", "enable-token", "disable-token", "reset-token"} {
			if rctx.Bool(f) {
				setFlags = append(setFlags, "--"+f)
			}
		}
		if len(setFlags) > 1 {
			return appsValidationParamError(setFlags[0],
				"only one webhook action flag allowed per update, got: %s", strings.Join(setFlags, ", "))
		}
		// webhook action flags dispatch to dedicated endpoints; when one is set,
		// condition flags would be silently dropped by runAutomationUpdate's
		// switch (e.g. `--reset-token --cron '0 9 * * *'` used to only reset the
		// token). Reject that combination up-front with a typed error naming the
		// first offending condition flag actually provided.
		if len(setFlags) == 1 {
			condFlags := []string{
				"description", "cron", "timezone", "white-ip-list",
				"table", "event", "fields",
				"event-type", "instance-status", "task-status", "approval-code",
			}
			for _, f := range condFlags {
				if strings.TrimSpace(rctx.Str(f)) != "" || len(rctx.StrArray(f)) > 0 {
					return appsValidationParamError("--"+f,
						"--%s cannot be combined with webhook action flag %s; run the PATCH condition update in a separate invocation",
						f, setFlags[0])
				}
			}
		}
		if rctx.Bool("reset-url") && strings.TrimSpace(rctx.Str("app-env")) == "" {
			return appsValidationParamError("--app-env", "--reset-url requires --app-env preview|runtime")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		name := strings.TrimSpace(rctx.Str("name"))
		switch {
		case rctx.Bool("reset-url"):
			return common.NewDryRunAPI().POST(automationWebhookURLResetPath(appID, name)).Desc("Reset webhook URL")
		case rctx.Bool("enable-token"), rctx.Bool("disable-token"):
			return common.NewDryRunAPI().PATCH(automationWebhookTokenStatusPath(appID, name)).Desc("Set webhook token status")
		case rctx.Bool("reset-token"):
			return common.NewDryRunAPI().POST(automationWebhookTokenResetPath(appID, name)).Desc("Reset webhook token")
		default:
			body, _ := buildAutomationUpdateBody(rctx)
			return common.NewDryRunAPI().PUT(automationItemPath(appID, name)).Desc("Update trigger condition").Body(body)
		}
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return runAutomationUpdate(rctx)
	},
}

// runAutomationUpdate dispatches by webhook action flag; default is PUT condition.
func runAutomationUpdate(rctx *common.RuntimeContext) error {
	switch {
	case rctx.Bool("reset-url"):
		return runWebhookURLReset(rctx)
	case rctx.Bool("enable-token"):
		return runWebhookTokenStatus(rctx, true)
	case rctx.Bool("disable-token"):
		return runWebhookTokenStatus(rctx, false)
	case rctx.Bool("reset-token"):
		return runWebhookTokenReset(rctx)
	default:
		return runAutomationPatch(rctx)
	}
}

// runAutomationPatch sends the trigger update PUT with only the changed fields.
func runAutomationPatch(rctx *common.RuntimeContext) error {
	appID, err := requireAppID(rctx.Str("app-id"))
	if err != nil {
		return err
	}
	name := strings.TrimSpace(rctx.Str("name"))
	// Validate all condition-carrying flags up-front so illegal values surface a
	// field-specific error instead of being silently dropped by
	// buildAutomationUpdateBody (which would either report the generic
	// no-fields error or PATCH without the intended field).
	if c := strings.TrimSpace(rctx.Str("cron")); c != "" {
		if err := validateCronExpr(c); err != nil {
			return err
		}
	}
	if raw := strings.TrimSpace(rctx.Str("white-ip-list")); raw != "" {
		if _, err := parseIPListFlag(raw); err != nil {
			return err
		}
	}
	if raw := strings.TrimSpace(rctx.Str("fields")); raw != "" {
		if _, err := parseFieldsFlag(raw); err != nil {
			return err
		}
	}
	body, err := buildAutomationUpdateBody(rctx)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		// No specific flag failed; every condition-carrying flag is a legitimate
		// candidate. Emit a Param-less validation error and enumerate candidates
		// in Params so `apps +update` precedent style is followed and agents get
		// structured recovery guidance.
		reason := "no update fields provided; pass at least one condition flag or a webhook action flag"
		return appsValidationError("%s", reason).
			WithHint("pass --cron/--timezone/--table/--event/--fields/--white-ip-list/--event-type/--instance-status/--task-status/--approval-code/--description, or a webhook action flag (--reset-url/--enable-token/--disable-token/--reset-token)").
			WithParams(
				appsInvalidParam("--cron", reason),
				appsInvalidParam("--timezone", reason),
				appsInvalidParam("--table", reason),
				appsInvalidParam("--event", reason),
				appsInvalidParam("--fields", reason),
				appsInvalidParam("--white-ip-list", reason),
				appsInvalidParam("--event-type", reason),
				appsInvalidParam("--instance-status", reason),
				appsInvalidParam("--task-status", reason),
				appsInvalidParam("--approval-code", reason),
				appsInvalidParam("--description", reason),
			)
	}
	data, err := rctx.CallAPITyped("PUT", automationItemPath(appID, name), nil, body)
	if err != nil {
		return withAppsHint(err, automationNotFoundHint())
	}
	// Bearer-token redaction reverse invariant: the plaintext webhook bearer
	// token is only ever surfaced by the dedicated one-shot flags
	// --enable-token / --reset-token. Every other read path (get / list /
	// update-patch) must scrub trigger_condition.token_value. UpdateTrigger
	// backend behaviour "re-reads GetTriggerModel and returns TriggerInfo"
	// shares the decrypting webhook-condition converter with the get path,
	// so the PATCH response may carry a plaintext bearer token; the CLI
	// redacts here to enforce the invariant, matching get / list.
	redacted := redactWebhookToken(data)
	rctx.OutFormat(redacted, nil, func(w io.Writer) {
		fmt.Fprintf(w, "updated trigger: %v\n", redacted["name"])
	})
	return nil
}

// buildAutomationUpdateBody assembles PATCH body with only provided fields.
// Condition-carrying flags dispatch by --trigger-type where semantics overlap
// (e.g. --table belongs only to record-change), so callers must set
// --trigger-type when updating the matching condition.
func buildAutomationUpdateBody(rctx *common.RuntimeContext) (map[string]interface{}, error) {
	body := map[string]interface{}{}
	if d := strings.TrimSpace(rctx.Str("description")); d != "" {
		body["description"] = d
	}
	if c := strings.TrimSpace(rctx.Str("cron")); c != "" {
		cond, err := buildCronCondition(c, rctx.Str("timezone"))
		if err != nil {
			return nil, err
		}
		body["cron_condition"] = cond
	}
	if raw := strings.TrimSpace(rctx.Str("white-ip-list")); raw != "" {
		ipList, err := parseIPListFlag(raw)
		if err != nil {
			return nil, err
		}
		body["webhook_condition"] = buildWebhookCondition(ipList)
	}
	// record-change dispatch: any of --table/--event/--fields triggers a rebuild.
	// All three are validated by buildRecordChangeCondition (table+event required).
	if strings.TrimSpace(rctx.Str("table")) != "" ||
		strings.TrimSpace(rctx.Str("event")) != "" ||
		strings.TrimSpace(rctx.Str("fields")) != "" {
		fields, err := parseFieldsFlag(rctx.Str("fields"))
		if err != nil {
			return nil, err
		}
		cond, err := buildRecordChangeCondition(rctx.Str("table"), rctx.Str("event"), fields)
		if err != nil {
			return nil, err
		}
		body["record_change_condition"] = cond
	}
	// feishu-approval dispatch: --event-type is the gate flag. Statuses are picked
	// from --instance-status or --task-status per event-type.
	if eventType := strings.TrimSpace(rctx.Str("event-type")); eventType != "" {
		raw := rctx.StrArray("instance-status")
		if eventType == "approval_task" {
			raw = rctx.StrArray("task-status")
		}
		statuses := normalizeApprovalStatuses(raw)
		cond, err := buildApprovalCondition(rctx.Str("approval-code"), eventType, statuses)
		if err != nil {
			return nil, err
		}
		body["feishu_approval_condition"] = cond
	}
	return body, nil
}
