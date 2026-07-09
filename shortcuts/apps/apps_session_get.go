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

// AppsSessionGet reads a session's current status, queued turns, and latest turn.
// Single-shot: the caller drives polling using next_poll_after_ms.
var AppsSessionGet = common.Shortcut{
	Service:     appsService,
	Command:     "+session-get",
	Description: "Read a session's current status, queued turns, and latest turn",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +session-get --app-id <app_id> --session-id <session_id>",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "session-id", Desc: "session ID", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		if strings.TrimSpace(rctx.Str("session-id")) == "" {
			return appsValidationParamError("--session-id", "--session-id is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET(sessionPath(rctx.Str("app-id"), rctx.Str("session-id"))).
			Desc("Read a session's status")
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		data, err := rctx.CallAPITyped("GET", sessionPath(rctx.Str("app-id"), rctx.Str("session-id")), nil, nil)
		if err != nil {
			return withAppsHint(err, "if the session_id is unknown or invalid, list this app's sessions with `lark-cli apps +session-list --app-id "+strings.TrimSpace(rctx.Str("app-id"))+"`")
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "session: %s\n", common.GetString(data, "session_id"))
			fmt.Fprintf(w, "active: %v  streaming: %v\n", data["is_active"], data["is_streaming"])
			if lt, ok := data["latest_turn"].(map[string]interface{}); ok {
				fmt.Fprintf(w, "latest turn: %v (%v)\n", lt["turn_id"], lt["status"])
			}
			fmt.Fprintf(w, "queued: %v\n", data["queued_count"])
			fmt.Fprintf(w, "next poll after: %vms\n", data["next_poll_after_ms"])
		})
		return nil
	},
}

// sessionPath builds the single-session path under an app. Defined here (first
// consumer) so it never sits unused. Reused by Task 4 (+session-stop) and Task 5 (+chat).
func sessionPath(appID, sessionID string) string {
	return fmt.Sprintf("%s/apps/%s/sessions/%s",
		apiBasePath,
		validate.EncodePathSegment(strings.TrimSpace(appID)),
		validate.EncodePathSegment(strings.TrimSpace(sessionID)))
}
