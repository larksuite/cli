// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsSessionCreate creates a new session under an existing Miaoda app.
var AppsSessionCreate = common.Shortcut{
	Service:     appsService,
	Command:     "+session-create",
	Description: "Create a session under a Miaoda app",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +session-create --app-id <app_id>",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST(sessionsPath(rctx.Str("app-id"))).
			Desc("Create a session under a Miaoda app")
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		data, err := rctx.CallAPITyped("POST", sessionsPath(rctx.Str("app-id")), nil, nil)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "session created: %s\n", common.GetString(data, "session_id"))
		})
		return nil
	},
}

// sessionsPath builds the collection path for an app's sessions.
func sessionsPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/sessions", apiBasePath, validate.EncodePathSegment(strings.TrimSpace(appID)))
}
