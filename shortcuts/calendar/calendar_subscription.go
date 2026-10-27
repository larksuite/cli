// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// CalendarSubscribe subscribes the selected identity to an explicit calendar.
var CalendarSubscribe = common.Shortcut{
	Service:     "calendar",
	Command:     "+subscribe",
	Description: "Subscribe to a calendar as the current user or bot",
	Risk:        "write",
	Scopes:      []string{"calendar:calendar:subscribe"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "calendar-id", Desc: "calendar ID to subscribe to", Required: true},
	},
	Validate: validateCalendarSubscription,
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST("/open-apis/calendar/v4/calendars/:calendar_id/subscribe").
			Set("calendar_id", runtime.Str("calendar-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		calendarID := runtime.Str("calendar-id")
		if _, err := runtime.CallAPITyped("POST", calendarSubscriptionPath(calendarID, "subscribe"), nil, nil); err != nil {
			return withCalendarSubscribeRecovery(err)
		}
		// A successful response is the whole result: the server returns the
		// subscribed calendar entity, but callers only need to know the call
		// succeeded, so emit an empty data object.
		runtime.Out(map[string]interface{}{}, nil)
		return nil
	},
}

// CalendarUnsubscribe cancels the selected identity's calendar subscription.
var CalendarUnsubscribe = common.Shortcut{
	Service:     "calendar",
	Command:     "+unsubscribe",
	Description: "Unsubscribe from a calendar as the current user or bot",
	Risk:        "write",
	Scopes:      []string{"calendar:calendar:subscribe"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "calendar-id", Desc: "calendar ID to unsubscribe from", Required: true},
	},
	Validate: validateCalendarSubscription,
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST("/open-apis/calendar/v4/calendars/:calendar_id/unsubscribe").
			Set("calendar_id", runtime.Str("calendar-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		calendarID := runtime.Str("calendar-id")
		if _, err := runtime.CallAPITyped("POST", calendarSubscriptionPath(calendarID, "unsubscribe"), nil, nil); err != nil {
			return withCalendarUnsubscribeRecovery(err)
		}
		// The upstream response carries no payload; success is the result.
		runtime.Out(map[string]interface{}{}, nil)
		return nil
	},
}

func validateCalendarSubscription(ctx context.Context, runtime *common.RuntimeContext) error {
	if err := rejectCalendarAutoBotFallback(runtime); err != nil {
		return err
	}
	calendarID := runtime.Str("calendar-id")
	if strings.TrimSpace(calendarID) == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "calendar ID cannot be empty").WithParam("--calendar-id")
	}
	if err := validate.ResourceName(calendarID, "--calendar-id"); err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).
			WithParam("--calendar-id").WithCause(err)
	}
	return nil
}

func calendarSubscriptionPath(calendarID, action string) string {
	return fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/%s", validate.EncodePathSegment(calendarID), action)
}

// withCalendarSubscribeRecovery attaches a subscribe-specific hint for the
// business error codes documented on the Subscribe Calendar endpoint. Codes
// absent from that doc (for example the unsubscribe-only 191007 / 191008) fall
// through untouched.
func withCalendarSubscribeRecovery(err error) error {
	return applyCalendarSubscriptionHint(err, map[int]string{
		190007: "enable the app's bot capability before subscribing with `--as bot`",
		191000: "calendar not found; verify --calendar-id (look it up with `lark-cli calendar calendars primary|list|search`)",
		191001: "invalid --calendar-id; verify it (look it up with `lark-cli calendar calendars primary|list|search`)",
		191002: "the current identity cannot access this calendar; subscribe only to a calendar it is allowed to see",
		191003: "the calendar has been deleted; verify --calendar-id",
		191004: "only shared or primary calendars can be subscribed to; inspect the type with `lark-cli calendar calendars get`",
		191005: "the current identity has reached the 1000-calendar subscription limit; unsubscribe from an unused calendar with `lark-cli calendar +unsubscribe`, then retry",
		191006: "the target calendar cannot be subscribed to: a bot primary calendar, another tenant's calendar, or a private calendar is not allowed; confirm the public scope, type, and access role with `lark-cli calendar calendars get`",
		195100: "the current identity or specified user has left the tenant; retry with a valid identity",
	})
}

// withCalendarUnsubscribeRecovery attaches an unsubscribe-specific hint for the
// business error codes documented on the Unsubscribe Calendar endpoint. Codes
// absent from that doc (for example the subscribe-only 191005 / 191006) fall
// through untouched.
func withCalendarUnsubscribeRecovery(err error) error {
	return applyCalendarSubscriptionHint(err, map[int]string{
		190007: "enable the app's bot capability before unsubscribing with `--as bot`",
		191000: "calendar not found; verify --calendar-id (look it up with `lark-cli calendar calendars primary|list|search`)",
		191001: "invalid --calendar-id; verify it (look it up with `lark-cli calendar calendars primary|list|search`)",
		191002: "the current identity cannot access this calendar",
		191003: "the calendar has been deleted; verify --calendar-id",
		191004: "only shared or primary calendars apply to this interface; inspect the type with `lark-cli calendar calendars get`",
		191007: "the target calendar is already unsubscribed for the current identity; check --calendar-id and --as",
		191008: "the current identity's primary calendar cannot be unsubscribed; choose a different subscribed calendar",
		195100: "the current identity or specified user has left the tenant; retry with a valid identity",
	})
}

// applyCalendarSubscriptionHint annotates a typed Lark APIError with the
// per-endpoint hint for its code, preserving the error's classification, code,
// log id, and cause. A non-API error or an unmapped code passes through
// unchanged so server-supplied detail survives.
func applyCalendarSubscriptionHint(err error, hints map[int]string) error {
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	hint, ok := hints[apiErr.Code]
	if !ok {
		return err
	}
	return withStepContext(err, "%s", hint)
}
