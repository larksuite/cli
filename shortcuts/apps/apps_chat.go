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

// AppsChat sends a user message to a session, starting/continuing a conversation.
// Async: the message is queued and the response carries no business payload (no
// turn_id, no next_poll_after_ms — the turn is not generated yet). Poll
// +session-get; it returns next_poll_after_ms, and once the turn runs its handle
// is in latest_turn.turn_id.

// Turn cost varies sharply by init state: the first +chat on a not-initialized
// app runs a one-time design + first-generation pass server-side (~20-50 min);
// chat on an already-initialized app is incremental and finishes in minutes.
// The init-state check and matching polling cadence live in the lark-apps
// skill reference (references/lark-apps-cloud-dev.md) — the canonical source.
var AppsChat = common.Shortcut{
	Service:     appsService,
	Command:     "+chat",
	Description: "Send a message to a session to start/continue a conversation",
	Risk:        "write",
	Tips: []string{
		`Example: lark-cli apps +chat --app-id <app_id> --session-id <session_id> --message "做一个待办清单页面"`,
		`Example: lark-cli apps +chat --app-id <app_id> --session-id <session_id> --message "把首页标题改为 我的待办"`,
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "session-id", Desc: "session ID", Required: true},
		{Name: "message", Desc: "user message text", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		if strings.TrimSpace(rctx.Str("session-id")) == "" {
			return appsValidationParamError("--session-id", "--session-id is required")
		}
		// Do not echo --message content in the error (spec §4 redaction).
		if strings.TrimSpace(rctx.Str("message")) == "" {
			return appsValidationParamError("--message", "--message is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST(chatPath(rctx.Str("app-id"), rctx.Str("session-id"))).
			Desc("Send a message to a session").
			Body(buildChatBody(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		data, err := rctx.CallAPITyped("POST", chatPath(rctx.Str("app-id"), rctx.Str("session-id")), nil, buildChatBody(rctx))
		if err != nil {
			return withAppsHint(err, "if the session_id is unknown or invalid, list this app's sessions with `lark-cli apps +session-list --app-id "+strings.TrimSpace(rctx.Str("app-id"))+"`")
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "message sent; poll +session-get for turn status\n")
		})
		return nil
	},
}

func chatPath(appID, sessionID string) string {
	return sessionPath(appID, sessionID) + "/chat"
}

func buildChatBody(rctx *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{
		"message": strings.TrimSpace(rctx.Str("message")),
	}
}
