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
			POST(automationStatusPath(appID, strings.TrimSpace(rctx.Str("name")))).
			Desc("Enable automation trigger").
			Body(statusBodyFromAction(true))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return runAutomationStatus(rctx, true)
	},
}

// runAutomationStatus is shared by enable/disable: POST .../status with {status}.
func runAutomationStatus(rctx *common.RuntimeContext, enable bool) error {
	appID, err := requireAppID(rctx.Str("app-id"))
	if err != nil {
		return err
	}
	name := strings.TrimSpace(rctx.Str("name"))
	data, err := rctx.CallAPITyped("POST", automationStatusPath(appID, name), nil, statusBodyFromAction(enable))
	if err != nil {
		return withAppsHint(err, automationNotFoundHint())
	}
	rctx.OutFormat(data, nil, func(w io.Writer) {
		fmt.Fprintf(w, "trigger %v status: %v\n", data["name"], data["status"])
	})
	return nil
}
