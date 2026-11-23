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

// AppsAutomationGet gets a single trigger's full config (webhook token redacted).
var AppsAutomationGet = common.Shortcut{
	Service:     appsService,
	Command:     "+automation-get",
	Description: "Get an automation trigger's config (webhook Bearer Token redacted)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +automation-get --app-id <app_id> --name <trigger_name>",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "name", Desc: "trigger name", Required: true},
	},
	Validate: automationValidateName,
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			GET(automationItemPath(appID, strings.TrimSpace(rctx.Str("name")))).
			Desc("Get automation trigger")
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		name := strings.TrimSpace(rctx.Str("name"))
		data, err := rctx.CallAPITyped("GET", automationItemPath(appID, name), nil, nil)
		if err != nil {
			return withAppsHint(err, automationNotFoundHint())
		}
		redacted := redactWebhookToken(data)
		trigger, _ := redacted["trigger"].(map[string]interface{})
		rctx.OutFormat(redacted, nil, func(w io.Writer) {
			fmt.Fprintf(w, "name: %v\ntype: %v\nstatus: %v\n",
				trigger["name"], trigger["trigger_type"], trigger["status"])
		})
		return nil
	},
}

// automationValidateName validates --app-id and --name presence. Shared by get/update/enable/disable.
func automationValidateName(ctx context.Context, rctx *common.RuntimeContext) error {
	if _, err := requireAppID(rctx.Str("app-id")); err != nil {
		return err
	}
	if strings.TrimSpace(rctx.Str("name")) == "" {
		return appsValidationParamError("--name", "--name is required").
			WithHint("find trigger names with `lark-cli apps +automation-list --app-id <app_id>`")
	}
	return nil
}

// automationNotFoundHint is the shared recovery hint when a trigger name may not exist.
func automationNotFoundHint() string {
	return "verify the trigger name with `lark-cli apps +automation-list --app-id <app_id>`"
}
