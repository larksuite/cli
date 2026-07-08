// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// AppsAutomationDisable disables a trigger. Maps to the shared status endpoint.
var AppsAutomationDisable = common.Shortcut{
	Service:     appsService,
	Command:     "+automation-disable",
	Description: "Disable an automation trigger (stops auto-firing; does not delete)",
	Risk:        "write",
	Tips:        []string{"Example: lark-cli apps +automation-disable --app-id <id> --name <trigger_name>"},
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
			Desc("Disable automation trigger").
			Body(statusBodyFromAction(false))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return runAutomationStatus(rctx, false)
	},
}
