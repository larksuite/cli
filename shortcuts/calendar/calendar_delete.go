// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// calendar +delete — delete a calendar event with explicit recurring-event
// scope handling. See calendar_recurring.go for --apply-to semantics.

package calendar

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const deleteLogPrefix = "[calendar +delete]"

var CalendarDelete = common.Shortcut{
	Service:     "calendar",
	Command:     "+delete",
	Description: "Delete a calendar event; requires --apply-to for recurring events and exceptions",
	Risk:        "high-risk-write",
	Scopes:      []string{"calendar:calendar.event:read", "calendar:calendar.event:delete"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "event-id", Desc: "event ID to delete", Required: true},
		{Name: "calendar-id", Desc: "calendar ID (default: primary)"},
		{
			Name: flagApplyTo,
			Enum: applyToValues,
			Desc: "recurring scope: single (this occurrence / exception only) | all (whole series and every exception) | this-and-following (truncate the series at this instance and drop every exception on/after it). Required on recurring events; ignored on non-recurring events.",
		},
		{Name: "notify", Type: "bool", Default: "true", Desc: "send delete notification to attendees for the master event; exception cleanup silently uses need_notification=false so participants are not spammed"},
	},
	Tips: []string{
		"Deleting an entire recurring series also removes every exception; pass --apply-to=all to confirm intent.",
		"Deleting `this-and-following` truncates the master (UNTIL = midnight of the pivot day in the event's timezone) and removes exceptions on/after that instance.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateCalendarDelete(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return dryRunCalendarDelete(runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeCalendarDelete(ctx, runtime)
	},
}

func validateCalendarDelete(runtime *common.RuntimeContext) error {
	if err := rejectCalendarAutoBotFallback(runtime); err != nil {
		return err
	}
	for _, flag := range []string{"event-id", "calendar-id"} {
		if val := strings.TrimSpace(runtime.Str(flag)); val != "" {
			if err := common.RejectDangerousCharsTyped("--"+flag, val); err != nil {
				return err
			}
		}
	}
	if strings.TrimSpace(runtime.Str("event-id")) == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "specify --event-id").WithParam("--event-id")
	}
	return nil
}

func dryRunCalendarDelete(runtime *common.RuntimeContext) *common.DryRunAPI {
	calendarID := strings.TrimSpace(runtime.Str("calendar-id"))
	displayCalendarID := calendarID
	if displayCalendarID == "" || displayCalendarID == PrimaryCalendarIDStr {
		displayCalendarID = "<primary>"
	}
	eventID := strings.TrimSpace(runtime.Str("event-id"))
	scope := strings.TrimSpace(runtime.Str(flagApplyTo))
	if scope == "" {
		scope = "<inferred from event kind>"
	}

	d := common.NewDryRunAPI().
		Set("calendar_id", displayCalendarID).
		Set("event_id", eventID).
		Set("apply_to", scope)

	// Every path starts with a GET so we can classify the event (master /
	// exception / plain instance / normal) before choosing the delete
	// strategy. The one exception is a malformed id, where Execute short-
	// circuits with a validation error; the dry run mirrors that by omitting
	// the read step.
	if hasStandardEventIDShape(eventID) {
		d.GET("/open-apis/calendar/v4/calendars/:calendar_id/events/:event_id").
			Desc("[1] Read event to detect recurring kind (master / exception / plain instance / normal)")
	}
	switch scope {
	case applyToAll:
		d.GET("/open-apis/calendar/v4/calendars/:calendar_id/events/:master_event_id/instances").
			Desc("[2] Page /instances across the master's window to list exceptions")
		d.DELETE("/open-apis/calendar/v4/calendars/:calendar_id/events/:exception_event_id").
			Desc("[3] Delete every non-cancelled exception").
			Params(map[string]interface{}{"need_notification": false})
		d.DELETE("/open-apis/calendar/v4/calendars/:calendar_id/events/:master_event_id").
			Desc("[4] Delete the master event last").
			Params(map[string]interface{}{"need_notification": runtime.Bool("notify")})
	case applyToThisAndFollowing:
		d.GET("/open-apis/calendar/v4/calendars/:calendar_id/events/:master_event_id/instances").
			Desc("[2] Page /instances from the pivot day midnight to master end (in the event's timezone)")
		d.DELETE("/open-apis/calendar/v4/calendars/:calendar_id/events/:exception_event_id").
			Desc("[3] Delete exceptions with originalTime >= pivot").
			Params(map[string]interface{}{"need_notification": false})
		d.PATCH("/open-apis/calendar/v4/calendars/:calendar_id/events/:master_event_id").
			Desc("[4] Truncate master rrule with UNTIL = pivot-day midnight (in the event's timezone)").
			Body(map[string]interface{}{"recurrence": "<rewritten with UNTIL>", "need_notification": runtime.Bool("notify")})
	default:
		d.DELETE("/open-apis/calendar/v4/calendars/:calendar_id/events/:event_id").
			Desc(fmt.Sprintf("[2] Delete this event (--apply-to=%s)", applyToSingle)).
			Params(map[string]interface{}{"need_notification": runtime.Bool("notify")})
	}
	return d
}

func executeCalendarDelete(ctx context.Context, runtime *common.RuntimeContext) error {
	calendarID := strings.TrimSpace(runtime.Str("calendar-id"))
	if calendarID == "" {
		calendarID = PrimaryCalendarIDStr
	}
	eventID := strings.TrimSpace(runtime.Str("event-id"))
	notify := runtime.Bool("notify")

	current, err := resolveCalendarEventOrMaster(runtime, calendarID, eventID)
	if err != nil {
		return err
	}
	kind := classifyRecurringEvent(current)
	scope, err := validateApplyTo(runtime, kind, eventID)
	if err != nil {
		return err
	}

	switch scope {
	case applyToSingle:
		return deleteApplyToSingle(runtime, calendarID, eventID, notify)
	case applyToAll:
		return deleteApplyToAll(ctx, runtime, calendarID, current, eventID, notify)
	case applyToThisAndFollowing:
		return deleteApplyToThisAndFollowing(ctx, runtime, calendarID, current, eventID, notify)
	}
	return errs.NewInternalError(errs.SubtypeUnknown, "unhandled apply-to scope %q", scope)
}

func deleteApplyToSingle(runtime *common.RuntimeContext, calendarID, eventID string, notify bool) error {
	if err := deleteEventOnce(runtime, calendarID, eventID, notify, false); err != nil {
		return err
	}
	result := map[string]interface{}{
		"calendar_id": calendarID,
		"apply_to":    applyToSingle,
		"deleted_event": map[string]interface{}{
			"event_id": eventID,
			"action":   "deleted",
		},
	}
	writeCalendarDeleteResult(runtime, result, eventID, applyToSingle, "deleted_event", "Deleted event")
	return nil
}

// deleteApplyToAll executes: list exceptions → delete them (future first,
// then past) with concurrency & retry → delete the master last. The master
// id is derived when the caller passed an exception id. Per-exception
// failures do not abort the run; the summary reports them.
func deleteApplyToAll(ctx context.Context, runtime *common.RuntimeContext, calendarID string, current *calendarEvent, eventID string, notify bool) error {
	errOut := runtime.IO().ErrOut
	master, err := ensureMasterEvent(runtime, calendarID, current, eventID)
	if err != nil {
		return err
	}
	masterID := master.EventID
	if masterID == "" {
		masterID = masterEventID(eventID)
	}
	window, err := recurringWindowFromMaster(master, time.Now())
	if err != nil {
		return err
	}
	// apply-to=all destroys every exception (deleteException=true → is_deleted=true),
	// so include cancelled placeholders in the scan — the cleanup pass wants
	// to destroy them alongside the live rows.
	exceptions, err := listExceptionsInWindow(ctx, runtime, calendarID, masterID, window, true)
	if err != nil {
		return err
	}

	var worker *exceptionWorker
	// Only spin up the worker + progress ticker when there is actual exception
	// work to do — a series with no live exceptions should proceed straight to
	// the master delete without emitting "0 exception(s)" noise.
	if len(exceptions) > 0 {
		worker = &exceptionWorker{
			Concurrency: 3,
			Runtime:     runtime,
			Label:       deleteLogPrefix,
			Total:       len(exceptions),
			Do: func(_ context.Context, ex exceptionInstance) error {
				return deleteEventOnce(runtime, calendarID, ex.EventID, false, true)
			},
		}
		nowSec := time.Now().Unix()
		future, past := splitFutureThenPast(exceptions, nowSec)
		fmt.Fprintf(errOut, "%s deleting %d exception(s) then the master event\n",
			deleteLogPrefix, len(exceptions))
		if err := worker.Run(ctx, future); err != nil {
			return err
		}
		if err := worker.Run(ctx, past); err != nil {
			return err
		}
		worker.finalSummary(errOut, "deleted")
	}

	fmt.Fprintf(errOut, "%s deleting master event %s\n", deleteLogPrefix, masterID)
	if err := deleteEventOnce(runtime, calendarID, masterID, notify, false); err != nil {
		return withStepContext(err, "failed to delete master event %s", masterID)
	}

	deletedEvent := map[string]interface{}{
		"event_id":        eventID,
		"master_event_id": masterID,
		"action":          "deleted",
	}
	result := map[string]interface{}{
		"calendar_id":   calendarID,
		"apply_to":      applyToAll,
		"deleted_event": deletedEvent,
	}
	if worker != nil {
		exceptionsSummary := map[string]interface{}{
			"total":   len(exceptions),
			"deleted": int(worker.processed.Load()),
			"failed":  int(worker.failed.Load()),
		}
		if failures := worker.Failures(); len(failures) > 0 {
			exceptionsSummary["failures"] = failures
		}
		result["exceptions"] = exceptionsSummary
	}
	writeCalendarDeleteResult(runtime, result, eventID, applyToAll, "deleted_event", "Deleted event")
	return nil
}

// deleteApplyToThisAndFollowing executes: list exceptions on/after the pivot
// day midnight → delete them (per-item failures recorded, batch continues)
// → PATCH master rrule with UNTIL = pivot day midnight (in event's timezone).
func deleteApplyToThisAndFollowing(ctx context.Context, runtime *common.RuntimeContext, calendarID string, current *calendarEvent, eventID string, notify bool) error {
	pivotUnix, ok := parseInstanceOriginalTime(eventID)
	if !ok {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--%s=%s needs a recurring instance event_id (shape `{uid}_{originalTime}` with a positive originalTime); got %q",
			flagApplyTo, applyToThisAndFollowing, eventID).WithParam("--event-id")
	}
	errOut := runtime.IO().ErrOut
	master, err := ensureMasterEvent(runtime, calendarID, current, eventID)
	if err != nil {
		return err
	}
	masterID := master.EventID
	if masterID == "" {
		masterID = masterEventID(eventID)
	}
	if strings.TrimSpace(master.Recurrence) == "" {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"master event %s has no recurrence rule; cannot truncate", masterID)
	}

	// The scan window starts at the pivot day's midnight (in the event's
	// timezone) — every exception with originalTime >= pivot is on that same
	// day or later, so the scan is bounded to the tail of the series.
	loc := eventLocation(master)
	pivotDay := pivotDayMidnight(pivotUnix, loc)
	windowEnd := recurringEndFromRRule(master.Recurrence, pivotDay.Unix(), time.Now())
	if windowEnd <= pivotDay.Unix() {
		windowEnd = pivotDay.Unix()
	}
	all, err := listExceptionsInWindow(ctx, runtime, calendarID, masterID,
		recurringWindow{Start: pivotDay.Unix(), End: windowEnd}, true)
	if err != nil {
		return err
	}
	future := filterOnOrAfter(all, pivotUnix)

	var worker *exceptionWorker
	// Only run the worker + progress ticker when there is exception work to
	// do. A this-and-following with no live exceptions on/after the pivot
	// goes straight to the master truncation.
	if len(future) > 0 {
		worker = &exceptionWorker{
			Concurrency: 3,
			Runtime:     runtime,
			Label:       deleteLogPrefix,
			Total:       len(future),
			Do: func(_ context.Context, ex exceptionInstance) error {
				return deleteEventOnce(runtime, calendarID, ex.EventID, false, true)
			},
		}
		fmt.Fprintf(errOut, "%s deleting %d exception(s) on/after pivot %d (%s) then truncating master rrule at %s\n",
			deleteLogPrefix, len(future), pivotUnix,
			formatPivotDatetime(pivotUnix, loc),
			pivotDay.Format(time.RFC3339))
		if err := worker.Run(ctx, future); err != nil {
			return err
		}
		worker.finalSummary(errOut, "deleted")
	}

	newRRule := truncateRecurrenceUntil(master.Recurrence, pivotDay)
	patchBody := map[string]interface{}{
		"recurrence":        newRRule,
		"need_notification": notify,
	}
	if _, err := runtime.CallAPITyped("PATCH",
		calendarUpdateEventPath(calendarID, masterID),
		map[string]interface{}{"user_id_type": "open_id"},
		patchBody); err != nil {
		return withStepContext(err, "failed to truncate master event %s", masterID)
	}

	truncatedEvent := map[string]interface{}{
		"event_id":        eventID,
		"master_event_id": masterID,
		"action":          "truncated",
	}
	if strings.TrimSpace(newRRule) != "" {
		truncatedEvent["recurrence_truncated"] = newRRule
	}
	result := map[string]interface{}{
		"calendar_id":     calendarID,
		"apply_to":        applyToThisAndFollowing,
		"truncated_event": truncatedEvent,
	}
	if worker != nil {
		exceptionsSummary := map[string]interface{}{
			"total":   len(future),
			"deleted": int(worker.processed.Load()),
			"failed":  int(worker.failed.Load()),
		}
		if failures := worker.Failures(); len(failures) > 0 {
			exceptionsSummary["failures"] = failures
		}
		result["exceptions"] = exceptionsSummary
	}
	writeCalendarDeleteResult(runtime, result, eventID, applyToThisAndFollowing, "truncated_event", "Truncated event")
	return nil
}

// resolveCalendarEventOrMaster GETs the event by id. If the id is an instance
// shape (`{uid}_{originalTime}` with originalTime > 0) and the server responds
// with 193001 (not found) — which is how the API reports "the instance has no
// stored record because it is a plain unmaterialised occurrence of a
// recurring series" — we fall back to the master and clone it as if the
// caller had passed the instance id. When the master itself does not exist,
// we surface a `not_found` error with the raw event id, so callers get an
// actionable message instead of the generic 193001.
//
// The event-id shape check lives here so every entry point (delete / update)
// gets the same typed rejection without repeating the guard in Execute.
func resolveCalendarEventOrMaster(runtime *common.RuntimeContext, calendarID, eventID string) (*calendarEvent, error) {
	if !hasStandardEventIDShape(eventID) {
		return nil, errMalformedEventID(eventID)
	}
	event, err := fetchCalendarEvent(runtime, calendarID, eventID)
	if err == nil {
		return event, nil
	}
	if !isEventNotFound(err) {
		return nil, err
	}
	// Try the master fallback: any instance-shaped id can degrade to the
	// master. If the id is already the master (`_0`), masterEventID is a
	// no-op and this second GET is redundant, so short-circuit.
	masterID := masterEventID(eventID)
	if masterID == eventID {
		return nil, errs.NewAPIError(errs.SubtypeNotFound,
			"calendar event %s not found", eventID)
	}
	master, mErr := fetchCalendarEvent(runtime, calendarID, masterID)
	if mErr != nil {
		if isEventNotFound(mErr) {
			return nil, errs.NewAPIError(errs.SubtypeNotFound,
				"calendar event %s not found (also tried master %s)", eventID, masterID)
		}
		return nil, mErr
	}
	if master == nil {
		return nil, errs.NewAPIError(errs.SubtypeNotFound,
			"calendar event %s not found (also tried master %s)", eventID, masterID)
	}
	// Clone the master into a synthetic snapshot for the instance the caller
	// asked about. The classifier keys off (IsException, Recurrence, EventID
	// shape); dropping Recurrence lets classifyRecurringEvent tag this as a
	// plain recurring instance (EventID has a positive originalTime) rather
	// than a master. ensureMasterEvent still does the real master GET when
	// downstream needs the master's stored recurrence.
	shadow := *master
	shadow.EventID = eventID
	// 根据eventID的originalTime对齐shadow的StartTime和EndTime
	if originalTime, ok := parseInstanceOriginalTime(eventID); ok &&
		master.StartTime != nil &&
		master.EndTime != nil &&
		master.StartTime.Timestamp != "" &&
		master.EndTime.Timestamp != "" {
		st, e1 := strconv.ParseInt(master.StartTime.Timestamp, 10, 64)
		et, e2 := strconv.ParseInt(master.EndTime.Timestamp, 10, 64)
		if e1 != nil || e2 != nil {
			parseErr := e1
			if parseErr == nil {
				parseErr = e2
			}
			return nil, withStepContext(parseErr, "failed to parse start time %s or end time %s", master.StartTime.Timestamp, master.EndTime.Timestamp)
		}
		duration := et - st
		shadow.StartTime.Timestamp = strconv.FormatInt(originalTime, 10)
		shadow.EndTime.Timestamp = strconv.FormatInt(originalTime+duration, 10)
	}
	shadow.IsException = false
	shadow.Recurrence = ""
	return &shadow, nil
}

// ensureMasterEvent returns the master calendar event. It avoids a redundant
// GET when the caller already has a snapshot carrying the master's recurrence
// rule — that snapshot is either the real fetched master or the shadow
// returned by resolveCalendarEventOrMaster when the caller passed an
// unmaterialised instance id.
func ensureMasterEvent(runtime *common.RuntimeContext, calendarID string, current *calendarEvent, eventID string) (*calendarEvent, error) {
	if current != nil && current.Recurrence != "" && !current.IsException {
		return current, nil
	}
	masterID := masterEventID(eventID)
	master, err := fetchCalendarEvent(runtime, calendarID, masterID)
	if err != nil {
		return nil, withStepContext(err, "failed to read master event %s", masterID)
	}
	if master.Recurrence == "" {
		return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"resolved master event %s has no recurrence rule; --apply-to=all / this-and-following require a recurring series", masterID)
	}
	return master, nil
}

func writeCalendarDeleteResult(runtime *common.RuntimeContext, result map[string]interface{}, targetEventID, scope, eventKey, eventLabel string) {
	runtime.OutFormat(result, nil, func(w io.Writer) {
		writeDeletePretty(w, result, targetEventID, scope, eventKey, eventLabel)
	})
}

// writeDeletePretty renders the +delete pretty output in the shape the user
// asked for: (1) header stating which apply_to scope ran on which event_id,
// (2) the deleted/truncated event details, (3) an optional exceptions summary.
// eventKey is the top-level key in `result` that carries the event details
// ("deleted_event" or "truncated_event"); eventLabel is the section title.
func writeDeletePretty(w io.Writer, result map[string]interface{}, targetEventID, scope, eventKey, eventLabel string) {
	fmt.Fprintf(w, "Applied `%s` on %s\n\n", scope, targetEventID)
	if ev, ok := result[eventKey].(map[string]interface{}); ok {
		fmt.Fprintf(w, "%s:\n", eventLabel)
		output.PrintTable(w, []map[string]interface{}{ev})
		fmt.Fprintln(w)
	}
	if exceptions, ok := result["exceptions"].(map[string]interface{}); ok {
		fmt.Fprintln(w, "Exceptions removed:")
		output.PrintTable(w, []map[string]interface{}{exceptions})
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "Event deleted successfully")
}
