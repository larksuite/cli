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
// rather than as separate +automation-* commands: the automation feature
// scoped itself to six shared verbs (list/get/create/update/enable/disable),
// so the webhook credential lifecycle is intentionally packed into --update
// via action flags, not a family of new commands. Otherwise Execute sends a
// PUT to update the trigger condition.
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
		{Name: "table", Desc: "[record-change] table name (from `+db-table-list`); dataloom tables key by name, not id"},
		{Name: "event", Desc: "[record-change] INSERT | UPDATE | UPSERT | DELETE"},
		{Name: "fields", Desc: "[record-change] JSON array of field ids for UPDATE/UPSERT, [\"*\"] = all"},
		{Name: "approval-code", Desc: "[feishu-approval] approval definition code; omit to match all approval definitions"},
		{Name: "event-type", Desc: "[feishu-approval] approval_instance | approval_task"},
		{Name: "instance-status", Type: "string_array", Desc: "[feishu-approval] statuses for approval_instance"},
		{Name: "task-status", Type: "string_array", Desc: "[feishu-approval] statuses for approval_task"},
		{Name: "white-ip-list", Desc: "[webhook] full replacement JSON array of allowed IPs"},
		{Name: "reset-url", Type: "bool", Desc: "[webhook] rotate callback URL for --app-env (old URL invalidated)"},
		{Name: "app-env", Desc: "[webhook] preview | runtime (required with --reset-url)"},
		{Name: "enable-token", Type: "bool", Desc: "[webhook] enable bearer token (shown once)"},
		{Name: "disable-token", Type: "bool", Desc: "[webhook] disable bearer token; re-enable generates a new token"},
		{Name: "reset-token", Type: "bool", Desc: "[webhook] rotate bearer token (old token invalidated, shown once)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := automationValidateName(ctx, rctx); err != nil {
			return err
		}
		// --app-env is only consumed by --reset-url; on any other update path
		// (other webhook action, condition update) it was silently dropped and
		// dry-run happily previewed the request that DID reach the backend,
		// misleading callers who inspected --dry-run before committing. Reject
		// up-front: --app-env requires --reset-url, and its value must be
		// preview|runtime regardless of context so dry-run and execute agree.
		if appEnv := strings.TrimSpace(rctx.Str("app-env")); appEnv != "" {
			if !rctx.Bool("reset-url") {
				return appsValidationParamError("--app-env",
					"--app-env is only used with --reset-url; drop --app-env or add --reset-url")
			}
			if appEnv != "preview" && appEnv != "runtime" {
				return appsValidationParamError("--app-env",
					"--app-env must be preview or runtime, got %q", appEnv)
			}
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
			if rctx.Bool("reset-url") && strings.TrimSpace(rctx.Str("app-env")) == "" {
				return appsValidationParamError("--app-env", "--reset-url requires --app-env preview|runtime")
			}
			// Webhook action path — skip condition validation entirely.
			return nil
		}

		// Condition path. Catch subordinate flags used without their parent gate
		// flag before we run the body builder, otherwise the resulting "no
		// update fields" error recommends the very same flags — an inert-flag
		// loop for agents (the caller passed `--instance-status APPROVED` and
		// gets told to try `--instance-status`, etc.). Point at the missing
		// parent instead.
		if err := checkUpdateSubordinateFlags(rctx); err != nil {
			return err
		}

		// --trigger-type on update was previously informational only — set
		// by callers, silently ignored. Two hazards followed:
		//   1. --trigger-type bogus passed local validation
		//   2. --cron '0 9 * * *' --white-ip-list '["1.1.1.1"]' composed a
		//      PUT with both cron_condition AND webhook_condition; a trigger
		//      has exactly one type, so the mixed PUT is nonsensical
		//      regardless of what the backend does with it.
		// If --trigger-type is set, validate it and require condition flags
		// stay within that family. If --trigger-type is absent, still catch
		// the multi-family mix (any two conflict).
		families := familiesInUse(rctx)
		if cliType := strings.TrimSpace(rctx.Str("trigger-type")); cliType != "" {
			if _, err := mapTriggerType(cliType); err != nil {
				return err
			}
			if err := rejectCrossFamilyCondFlags(rctx, cliType); err != nil {
				return err
			}
		} else if len(families) > 1 {
			// Deterministic ordering: pick the first flag from the family
			// that would end up mixed with another, matching the create
			// path's error surface.
			return appsValidationParamError("--trigger-type",
				"condition flags from multiple trigger types set (%s); pass --trigger-type to disambiguate or drop the extras",
				familiesMixedList(families))
		}

		// Run buildAutomationUpdateBody up-front so per-flag validation errors
		// (illegal cron, malformed --white-ip-list, bad --fields JSON) surface
		// during Validate rather than only during Execute. Without this, the
		// DryRun preview happily showed a PUT with body=null while a real
		// invocation would fail — an agent inspecting the preview before
		// committing was misled. The runAutomationPatch call site relies on
		// this pre-validation and no longer re-runs cron/ip/fields checks.
		body, err := buildAutomationUpdateBody(rctx)
		if err != nil {
			return err
		}
		if len(body) == 0 {
			return noUpdateFieldsError()
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		name := strings.TrimSpace(rctx.Str("name"))
		switch {
		case rctx.Bool("reset-url"):
			return common.NewDryRunAPI().
				POST(automationWebhookURLResetPath(appID, name)).
				Desc("Reset webhook URL").
				Body(webhookURLResetBody(rctx.Str("app-env")))
		case rctx.Bool("enable-token"):
			return common.NewDryRunAPI().
				PATCH(automationWebhookTokenStatusPath(appID, name)).
				Desc("Set webhook token status").
				Body(webhookTokenStatusBody(true))
		case rctx.Bool("disable-token"):
			return common.NewDryRunAPI().
				PATCH(automationWebhookTokenStatusPath(appID, name)).
				Desc("Set webhook token status").
				Body(webhookTokenStatusBody(false))
		case rctx.Bool("reset-token"):
			return common.NewDryRunAPI().
				POST(automationWebhookTokenResetPath(appID, name)).
				Desc("Reset webhook token").
				Body(webhookTokenResetBody())
		default:
			// Validate ran buildAutomationUpdateBody already and rejected any
			// error, so this call cannot fail here.
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
// Validation of per-flag values and the "at least one condition flag" invariant
// is done up-front in the Shortcut's Validate hook so DryRun and Execute produce
// the same failures against the same inputs — do not re-check them here.
func runAutomationPatch(rctx *common.RuntimeContext) error {
	appID, err := requireAppID(rctx.Str("app-id"))
	if err != nil {
		return err
	}
	name := strings.TrimSpace(rctx.Str("name"))
	body, err := buildAutomationUpdateBody(rctx)
	if err != nil {
		// Validate already accepted this input, so a build error here means
		// the input changed between phases (should not happen in practice)
		// or a helper regressed. Surface it verbatim rather than swallowing.
		return err
	}
	data, err := rctx.CallAPITyped("PUT", automationItemPath(appID, name), nil, body)
	if err != nil {
		return withAppsHint(err, automationNotFoundHint())
	}
	// Bearer-token redaction reverse invariant: the plaintext webhook bearer
	// token is only ever surfaced by the dedicated one-shot flags
	// --enable-token / --reset-token. Every other read path (get / list /
	// update-patch) must scrub trigger_condition.token_value. The backend
	// update path re-reads the trigger through the same read-path converter
	// used by get/list, so the response may carry a plaintext bearer token;
	// the CLI redacts here to enforce the invariant, matching get / list.
	redacted := redactWebhookToken(data)
	trigger, _ := redacted["trigger"].(map[string]interface{})
	rctx.OutFormat(redacted, nil, func(w io.Writer) {
		fmt.Fprintf(w, "updated trigger: %v\n", trigger["name"])
	})
	return nil
}

// checkUpdateSubordinateFlags surfaces "requires --parent" errors for flags
// that only make sense in combination with a parent condition-gate flag.
// Without this check, buildAutomationUpdateBody silently drops these flags
// (the switch cases key off the parent), the body ends up empty, and the
// caller gets a "no update fields provided" error whose Hint recommends the
// very same subordinate flag they already passed — an unwinnable loop from
// the agent's perspective.
func checkUpdateSubordinateFlags(rctx *common.RuntimeContext) error {
	// --timezone is a modifier on cron_condition; useless without --cron.
	if strings.TrimSpace(rctx.Str("timezone")) != "" && strings.TrimSpace(rctx.Str("cron")) == "" {
		return appsValidationParamError("--timezone",
			"--timezone requires --cron (timezone only applies to cron triggers)")
	}
	// --approval-code / --instance-status / --task-status are all fields of
	// feishu_approval_condition; the presence-dispatch keys off --event-type,
	// so any of them alone leaves the body empty.
	eventType := strings.TrimSpace(rctx.Str("event-type"))
	if eventType == "" {
		if strings.TrimSpace(rctx.Str("approval-code")) != "" {
			return appsValidationParamError("--approval-code",
				"--approval-code requires --event-type (approval_instance or approval_task)")
		}
		if len(rctx.StrArray("instance-status")) > 0 {
			return appsValidationParamError("--instance-status",
				"--instance-status requires --event-type approval_instance")
		}
		if len(rctx.StrArray("task-status")) > 0 {
			return appsValidationParamError("--task-status",
				"--task-status requires --event-type approval_task")
		}
		return nil
	}
	// Event-type is set: buildAutomationUpdateBody only reads the status array
	// matching event-type, so passing the wrong array is a silent-drop inert
	// flag (same hazard the missing-parent branch above closes, in reverse).
	// Reject up-front and name the mismatched flag as the failing Param.
	if eventType == "approval_instance" && len(rctx.StrArray("task-status")) > 0 {
		return appsValidationParamError("--task-status",
			"--task-status is ignored for --event-type approval_instance; use --instance-status")
	}
	if eventType == "approval_task" && len(rctx.StrArray("instance-status")) > 0 {
		return appsValidationParamError("--instance-status",
			"--instance-status is ignored for --event-type approval_task; use --task-status")
	}
	return nil
}

// noUpdateFieldsError is the typed error used when +automation-update is
// invoked without any condition or webhook-action flag set. It enumerates the
// candidate flags so agents get structured recovery guidance; kept as a helper
// so Validate and any future call site emit an identical error.
func noUpdateFieldsError() error {
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

// buildAutomationUpdateBody assembles PUT body with only provided fields.
// Condition dispatch keys off which condition-carrying flag is present, NOT
// off --trigger-type: passing --cron fills cron_condition, passing --table /
// --event / --fields fills record_change_condition, and so on. --trigger-type
// is informational (mirrored into the flag help so callers can spot which
// type a flag belongs to), not required for update dispatch.
func buildAutomationUpdateBody(rctx *common.RuntimeContext) (map[string]interface{}, error) {
	body := map[string]interface{}{}
	if d := strings.TrimSpace(rctx.Str("description")); d != "" {
		if err := validateAutomationDescriptionLen(d); err != nil {
			return nil, err
		}
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
