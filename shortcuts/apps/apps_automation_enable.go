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

// AppsAutomationEnable enables (activates) a trigger. Maps to the shared status endpoint.
var AppsAutomationEnable = common.Shortcut{
	Service:     appsService,
	Command:     "+automation-enable",
	Description: "Enable (activate) an automation trigger",
	Risk:        "write",
	Tips:        []string{"Example: lark-cli apps +automation-enable --app-id <id> --name <trigger_name>"},
	Scopes:      []string{"spark:app:write"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "name", Desc: "trigger name", Required: true},
	},
	Validate: automationValidateName,
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			PATCH(automationItemPath(appID, strings.TrimSpace(rctx.Str("name")))).
			Desc("Enable automation trigger").
			Body(statusBodyFromAction(true))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return runAutomationStatus(rctx, true)
	},
}

// runAutomationStatus is shared by enable/disable: PATCH .../triggers/{name}
// with {"status": ...}. The status change happens on the parent resource per
// the backend OpenAPI spec (see reference Python samples in the trigger test
// fixtures) — there is intentionally no /status sub-path; the sole nested
// endpoints under a trigger are the webhook credential lifecycle
// (/webhook/token/status, /webhook/token/reset, /webhook/url/reset).
//
// The status endpoint returns {"success": true} on success. Pretty output is
// synthesized from rctx.name and the desired action, since the response
// intentionally carries no trigger object to fish name/status from.
func runAutomationStatus(rctx *common.RuntimeContext, enable bool) error {
	appID, err := requireAppID(rctx.Str("app-id"))
	if err != nil {
		return err
	}
	name := strings.TrimSpace(rctx.Str("name"))
	data, err := rctx.CallAPITyped("PATCH", automationItemPath(appID, name), nil, statusBodyFromAction(enable))
	if err != nil {
		return withAppsHint(err, automationNotFoundHint())
	}
	desiredStatus := "disabled"
	if enable {
		desiredStatus = "enabled"
	}
	rctx.OutFormat(data, nil, func(w io.Writer) {
		fmt.Fprintf(w, "trigger %s status: %s\n", name, desiredStatus)
	})
	return nil
}
