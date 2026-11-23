// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// joinEventPath is the unified "join by share token" endpoint. Every share
// form (link, QR code, share card, RSVP card) collapses to a single opaque
// share_token here; there is deliberately no "join by event_id" path, so the
// caller can never forge a plaintext event id to join an arbitrary event.
const joinEventPath = "/open-apis/calendar/v4/calendars/join_event"

var CalendarJoinEvent = common.Shortcut{
	Service:     "calendar",
	Command:     "+join-event",
	Description: "Join a calendar event via a share token (from a share link/QR code or a im share/RSVP card)",
	Risk:        "write",
	Scopes:      []string{"calendar:calendar.event:join"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{
			Name:     "token",
			Aliases:  []string{"share-token"},
			Desc:     "share token from a share link/QR code or an IM share/RSVP card",
			Required: true,
		},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := strings.TrimSpace(runtime.Str("token"))
		return common.NewDryRunAPI().
			POST(joinEventPath).
			Body(map[string]any{"share_token": token})
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := rejectCalendarAutoBotFallback(runtime); err != nil {
			return err
		}
		token := strings.TrimSpace(runtime.Str("token"))
		if token == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "share token cannot be empty").WithParam("--token")
		}
		if err := common.RejectDangerousCharsTyped("--token", token); err != nil {
			return err
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := strings.TrimSpace(runtime.Str("token"))

		_, err := runtime.CallAPITyped("POST", joinEventPath, nil,
			map[string]any{"share_token": token})
		if err != nil {
			return err
		}

		// The API returns an empty body on success; echo the join outcome so
		// the JSON contract stays a self-describing object rather than a bare ack.
		runtime.Out(map[string]any{
			"joined": true,
		}, nil)
		return nil
	},
}
