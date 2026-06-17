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

const sessionStopHint = "verify --app-id and --session-id are correct (list sessions with `lark-cli apps +session-list --app-id <app_id>`); --turn-id must be the latest turn from `lark-cli apps +session-get --app-id <app_id> --session-id <session_id>`"

// AppsSessionStop interrupts the RUNNING turn of a session. No-op if the turn
// is queued or already finished. Does not close the session.
var AppsSessionStop = common.Shortcut{
	Service:     appsService,
	Command:     "+session-stop",
	Description: "Stop (interrupt) the running turn of a session",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +session-stop --app-id <app_id> --session-id <session_id> --turn-id <turn_id>",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "session-id", Desc: "session ID", Required: true},
		{Name: "turn-id", Desc: "turn ID to stop (from +session-get latest_turn.turn_id)", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		if strings.TrimSpace(rctx.Str("session-id")) == "" {
			return appsValidationParamError("--session-id", "--session-id is required")
		}
		if strings.TrimSpace(rctx.Str("turn-id")) == "" {
			return appsValidationParamError("--turn-id", "--turn-id is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST(stopPath(rctx.Str("app-id"), rctx.Str("session-id"))).
			Desc("Stop the running turn of a session").
			Body(buildStopBody(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		data, err := rctx.CallAPITyped("POST", stopPath(rctx.Str("app-id"), rctx.Str("session-id")), nil, buildStopBody(rctx))
		if err != nil {
			return withAppsHint(err, sessionStopHint)
		}
		turnID := strings.TrimSpace(rctx.Str("turn-id"))
		rctx.OutFormat(data, nil, func(w io.Writer) {
			stopped, _ := data["stopped"].(bool)
			if stopped {
				fmt.Fprintf(w, "stopped turn %s. %v\n", turnID, data["message"])
			} else {
				fmt.Fprintf(w, "no-op: turn %s not stopped. %v\n", turnID, data["message"])
			}
		})
		return nil
	},
}

func stopPath(appID, sessionID string) string {
	return sessionPath(appID, sessionID) + "/stop"
}

func buildStopBody(rctx *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{
		"turn_id": strings.TrimSpace(rctx.Str("turn-id")),
	}
}
