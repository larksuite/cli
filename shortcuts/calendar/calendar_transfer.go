// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// calendar +transfer — hand the organizer role of an event to another user or bot

package calendar

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	flagTransferSeries          = "transfer-series"
	flagRemoveOriginalOrganizer = "remove-original-organizer"
)

const sharedCalendarOrganizerNotice = "[calendar +transfer] note: original_organizer_removed is omitted because --calendar-id may name either kind of calendar. " +
	"A shared calendar has no organizer of its own, so transferring an event that lives on one always removes the original organizer; " +
	"on a primary calendar the original organizer stays as an attendee. Do not claim either outcome without checking the event.\n"

const seriesConfirmationHint = "this transfers the ENTIRE recurring series (every occurrence and exception), not a single occurrence — " +
	"the API cannot transfer one occurrence. Re-run with --transfer-series to confirm."

var CalendarTransfer = common.Shortcut{
	Service:           "calendar",
	Command:           "+transfer",
	Description:       "Transfer the organizer role of a calendar event to another user or bot",
	Risk:              "high-risk-write",
	Scopes:            []string{"calendar:calendar.event:transfer"},
	ConditionalScopes: []string{"calendar:calendar.event:read"},
	AuthTypes:         []string{"user", "bot"},
	HasFormat:         true,
	Flags: []common.Flag{
		{Name: "event-id", Desc: "event ID to transfer (uid_originalTime)", Required: true},
		{Name: "to-user-id", Desc: "receiver open_id (ou_...); becomes the new organizer. May be a user or a bot", Required: true},
		{Name: "calendar-id", Desc: "calendar ID the event lives on (default: primary)"},
		{Name: flagRemoveOriginalOrganizer, Type: "bool", Default: "false", Desc: "remove the original organizer instead of keeping them as an attendee; the server forces this on a shared calendar, where original_organizer_removed is omitted from the result"},
		{Name: flagTransferSeries, Type: "bool", Default: "false", Desc: "confirm transferring the entire recurring series; required for recurring events because a single occurrence cannot be transferred"},
	},
	Tips: []string{
		"Transferring is irreversible and also moves meeting minutes, notes and attachments to the new organizer; pass --yes to confirm.",
		"--as must be the event's current organizer (user or bot); --to-user-id may be a user or a bot, so all four user/bot directions are expressed by those two flags.",
		`Example: lark-cli calendar +transfer --event-id <uid_originalTime> --to-user-id ou_xxx --yes`,
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateCalendarTransfer(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return dryRunCalendarTransfer(runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeCalendarTransfer(ctx, runtime)
	},
}

func validateCalendarTransfer(runtime *common.RuntimeContext) error {
	if err := rejectCalendarAutoBotFallback(runtime); err != nil {
		return err
	}
	for _, flag := range []string{"event-id", "to-user-id", "calendar-id"} {
		if val := strings.TrimSpace(runtime.Str(flag)); val != "" {
			if err := common.RejectDangerousCharsTyped("--"+flag, val); err != nil {
				return err
			}
		}
	}
	if strings.TrimSpace(runtime.Str("event-id")) == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "specify --event-id").WithParam("--event-id")
	}
	toUserID := strings.TrimSpace(runtime.Str("to-user-id"))
	if toUserID == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "specify --to-user-id").WithParam("--to-user-id")
	}
	if !strings.HasPrefix(toUserID, "ou_") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"invalid receiver id format %q: --to-user-id should be an open_id starting with 'ou_'", toUserID).WithParam("--to-user-id")
	}
	return nil
}

const (
	larkErrCalendarNoAccessRole         = 191002
	larkErrCalendarWrongCalendarType    = 191004
	larkErrCalendarCannotInviteReceiver = 193109
	larkErrCalendarEventNotInOrganizer  = 193110
	larkErrCalendarCrossTenantTransfer  = 193111
)

func withCalendarTransferRecovery(err error) error {
	p, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	var hint string
	switch p.Code {
	case larkErrCalendarNoAccessRole:
		hint = "the calling identity has no edit access to this calendar (WRITER or OWNER is required). This is not a missing OAuth scope, so do not run `lark-cli auth login`: set --as to the event's current organizer, or ask the calendar owner to grant access"
	case larkErrCalendarWrongCalendarType:
		hint = "only events on a primary or shared calendar can be transferred; resource, mailbox, and imported Google/Exchange calendars cannot"
	case larkErrCalendarCannotInviteReceiver:
		hint = "executive-mode collaboration rules block inviting this receiver. This is not a missing OAuth scope, so do not run `lark-cli auth login`: pick another receiver, or have the receiver start the transfer"
	case larkErrCalendarEventNotInOrganizer:
		hint = "the transfer must run against the organizer's own calendar; --calendar-id defaults to the primary calendar, so pass the organizer's calendar id explicitly when the event lives on a shared calendar"
	case larkErrCalendarCrossTenantTransfer:
		hint = "cross-tenant transfer is not supported; invite the receiver as an attendee instead"
	default:
		return err
	}
	return withStepContext(err, "%s", hint)
}

func calendarTransferIDs(runtime *common.RuntimeContext) (calendarID string, eventID string) {
	calendarID = strings.TrimSpace(runtime.Str("calendar-id"))
	if calendarID == "" {
		calendarID = PrimaryCalendarIDStr
	}
	return calendarID, strings.TrimSpace(runtime.Str("event-id"))
}

func calendarTransferPath(calendarID, eventID string) string {
	return fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/%s/transfer",
		validate.EncodePathSegment(calendarID), validate.EncodePathSegment(eventID))
}

func calendarTransferBody(runtime *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{
		"to_user_id":                     strings.TrimSpace(runtime.Str("to-user-id")),
		"need_remove_original_organizer": runtime.Bool(flagRemoveOriginalOrganizer),
	}
}

func transferOutcomeKnown(runtime *common.RuntimeContext, calendarID string) bool {
	return runtime.Bool(flagRemoveOriginalOrganizer) || calendarID == PrimaryCalendarIDStr
}

func dryRunCalendarTransfer(runtime *common.RuntimeContext) *common.DryRunAPI {
	calendarID, eventID := calendarTransferIDs(runtime)
	d := common.NewDryRunAPI().Set("calendar_id", calendarID).Set("event_id", eventID)
	steps := 0
	if !runtime.Bool(flagTransferSeries) {
		steps++
		d.GET("/open-apis/calendar/v4/calendars/:calendar_id/events/:event_id").
			Desc(fmt.Sprintf("[%d] Read the event to detect a recurring series (skipped when --%s is set)", steps, flagTransferSeries))
	}
	steps++
	return d.POST("/open-apis/calendar/v4/calendars/:calendar_id/events/:event_id/transfer").
		Desc(fmt.Sprintf("[%d] Transfer the organizer role", steps)).
		Params(map[string]interface{}{"user_id_type": "open_id"}).
		Body(calendarTransferBody(runtime))
}

func blockUnconfirmedSeriesTransfer(runtime *common.RuntimeContext, calendarID, eventID string) error {
	event, err := readTransferEvent(runtime, calendarID, eventID)
	if err != nil {
		return err
	}
	if event.Recurrence == "" && event.RecurringEventID == "" {
		return nil
	}
	return errs.NewValidationError(errs.SubtypeFailedPrecondition, "%s", seriesConfirmationHint).
		WithParam("--" + flagTransferSeries).
		WithHint("confirm the whole-series transfer with the user, then re-run with --" + flagTransferSeries)
}

func readTransferEvent(runtime *common.RuntimeContext, calendarID, eventID string) (*calendarEvent, error) {
	data, err := runtime.CallAPITyped("GET",
		fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/%s",
			validate.EncodePathSegment(calendarID), validate.EncodePathSegment(eventID)),
		nil, nil)
	if err != nil {
		return nil, withStepContext(withCalendarTransferRecovery(err),
			"failed to read event %s while checking whether it is recurring; pass --%s to confirm a whole-series transfer and skip this check", eventID, flagTransferSeries)
	}
	return parseCalendarEvent(data)
}

func executeCalendarTransfer(ctx context.Context, runtime *common.RuntimeContext) error {
	calendarID, eventID := calendarTransferIDs(runtime)
	toUserID := strings.TrimSpace(runtime.Str("to-user-id"))

	if !runtime.Bool(flagTransferSeries) {
		if err := blockUnconfirmedSeriesTransfer(runtime, calendarID, eventID); err != nil {
			return err
		}
	}
	if _, err := runtime.CallAPITyped("POST", calendarTransferPath(calendarID, eventID),
		map[string]interface{}{"user_id_type": "open_id"}, calendarTransferBody(runtime)); err != nil {
		return withCalendarTransferRecovery(err)
	}

	result := map[string]interface{}{
		"calendar_id":      calendarID,
		"event_id":         eventID,
		"new_organizer_id": toUserID,
	}
	if transferOutcomeKnown(runtime, calendarID) {
		result["original_organizer_removed"] = runtime.Bool(flagRemoveOriginalOrganizer)
	} else {
		fmt.Fprint(runtime.IO().ErrOut, sharedCalendarOrganizerNotice)
	}
	runtime.OutFormat(result, nil, func(w io.Writer) {
		output.PrintTable(w, []map[string]interface{}{result})
		fmt.Fprintln(w, "\nOrganizer transferred successfully")
	})
	return nil
}
