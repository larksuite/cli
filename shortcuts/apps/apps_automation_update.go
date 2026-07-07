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
// actions are dispatched to apps_automation_webhook.go; otherwise PATCH condition.
var AppsAutomationUpdate = common.Shortcut{
	Service:     appsService,
	Command:     "+automation-update",
	Description: "Update a trigger's condition/description, or manage webhook URL/Token via dedicated flags",
	Risk:        "high-risk-write",
	Tips: []string{
		"Example: lark-cli apps +automation-update --app-id <id> --name t1 --trigger-type cron --cron '0 10 * * *' --yes",
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
		n := 0
		for _, f := range []string{"reset-url", "enable-token", "disable-token", "reset-token"} {
			if rctx.Bool(f) {
				n++
			}
		}
		if n > 1 {
			return appsValidationParamError("--reset-url",
				"only one webhook action flag allowed per update (reset-url/enable-token/disable-token/reset-token)")
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
			return common.NewDryRunAPI().POST(automationWebhookTokenStatusPath(appID, name)).Desc("Set webhook token status")
		case rctx.Bool("reset-token"):
			return common.NewDryRunAPI().POST(automationWebhookTokenResetPath(appID, name)).Desc("Reset webhook token")
		default:
			return common.NewDryRunAPI().PATCH(automationItemPath(appID, name)).Desc("Update trigger condition").Body(buildAutomationUpdateBody(rctx))
		}
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return runAutomationUpdate(rctx)
	},
}

// runAutomationUpdate dispatches by webhook action flag; default is PATCH condition.
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

// runAutomationPatch PATCHes only the changed condition/description fields (Rule-7-1).
func runAutomationPatch(rctx *common.RuntimeContext) error {
	appID, err := requireAppID(rctx.Str("app-id"))
	if err != nil {
		return err
	}
	name := strings.TrimSpace(rctx.Str("name"))
	body := buildAutomationUpdateBody(rctx)
	if len(body) == 0 {
		return appsValidationParamError("--cron",
			"no update fields provided; pass --cron/--timezone/--white-ip-list/--description or a webhook action flag")
	}
	data, err := rctx.CallAPITyped("PATCH", automationItemPath(appID, name), nil, body)
	if err != nil {
		return withAppsHint(err, automationNotFoundHint())
	}
	rctx.OutFormat(data, nil, func(w io.Writer) {
		fmt.Fprintf(w, "updated trigger: %v\n", data["name"])
	})
	return nil
}

// buildAutomationUpdateBody assembles PATCH body with only provided fields.
func buildAutomationUpdateBody(rctx *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{}
	if d := strings.TrimSpace(rctx.Str("description")); d != "" {
		body["description"] = d
	}
	if c := strings.TrimSpace(rctx.Str("cron")); c != "" {
		if cond, err := buildCronCondition(c, rctx.Str("timezone")); err == nil {
			body["cron_condition"] = cond
		}
	}
	if raw := strings.TrimSpace(rctx.Str("white-ip-list")); raw != "" {
		if ipList, err := parseIPListFlag(raw); err == nil {
			body["webhook_condition"] = buildWebhookCondition(ipList)
		}
	}
	return body
}
