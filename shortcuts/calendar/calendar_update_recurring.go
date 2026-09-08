// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// calendar +update: --apply-to=all and --apply-to=this-and-following
// implementations. Split from calendar_update.go so the historical single-event
// path stays isolated.

package calendar

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const updateLogPrefix = "[calendar +update]"

// executeCalendarUpdateAll implements --apply-to=all.
//
//   - If the caller changed --start/--end (time change), exceptions are dropped
//     first (their original occurrence slot is now meaningless), then the
//     master is patched with the new time.
//   - Otherwise the caller changed non-time fields (summary, description,
//     rrule, attendees). We apply the exact same PATCH / attendee-batch calls
//     to every exception first, then to the master. Only fields the caller
//     explicitly set are propagated — the exception's independent customization
//     of other fields is preserved.
func executeCalendarUpdateAll(ctx context.Context, runtime *common.RuntimeContext, calendarID string, current *calendarEvent, targetEventID string) error {
	master, err := ensureMasterEvent(runtime, calendarID, current, targetEventID)
	if err != nil {
		return err
	}
	masterID := master.EventID
	if masterID == "" {
		masterID = masterEventID(targetEventID)
		master.EventID = masterID
	}

	if !runtime.Bool(flagSkipRoomCheck) {
		// Reuse the existing precheck against the master; the precheck understands
		// time and rrule change semantics.
		body, _, buildErr := buildCalendarUpdateEventData(runtime)
		if buildErr != nil {
			return buildErr
		}
		if err := runRoomAvailabilityPrecheck(ctx, runtime, calendarID, masterID, body); err != nil {
			return err
		}
	}

	window, err := recurringWindowFromMaster(master, time.Now())
	if err != nil {
		return err
	}
	timeChanged, err := masterTimeChanged(runtime, master)
	if err != nil {
		return err
	}
	// A time-change apply-to=all also destroys exceptions (deleteException=true,
	// i.e. is_deleted=true), so scan cancelled placeholders too — they need to
	// be destroyed for the same reason live ones do. A non-time PATCH walks the
	// same rows to replay the field changes, and there is nothing to replay
	// onto a cancelled row.
	exceptions, err := listExceptionsInWindow(ctx, runtime, calendarID, masterID, window, timeChanged)
	if err != nil {
		return err
	}

	nowSec := time.Now().Unix()
	future, past := splitFutureThenPast(exceptions, nowSec)

	errOut := runtime.IO().ErrOut

	var worker *exceptionWorker
	if len(exceptions) > 0 {
		if timeChanged {
			fmt.Fprintf(errOut, "%s time change: deleting %d exception(s) first, then patching master %s\n",
				updateLogPrefix, len(exceptions), masterID)
			worker, err = runExceptionDeletion(ctx, runtime, calendarID, future, past)
			if err != nil {
				return err
			}
		} else {
			fmt.Fprintf(errOut, "%s non-time change: patching %d exception(s) with the same fields, then the master %s\n",
				updateLogPrefix, len(exceptions), masterID)
			worker, err = runExceptionPatch(ctx, runtime, calendarID, future, past)
			if err != nil {
				return err
			}
		}
	}

	fmt.Fprintf(errOut, "%s patching master event %s\n", updateLogPrefix, masterID)
	event, addedCount, removedCount, err := applyUpdateToEvent(runtime, calendarID, masterID, !timeChanged)
	if err != nil {
		return withStepContext(err, "failed to update master event %s", masterID)
	}
	updatedEvent := calendarUpdateResult(masterID, event, addedCount, removedCount)
	updatedEvent["action"] = "patched"
	result := map[string]interface{}{
		"calendar_id":   calendarID,
		"apply_to":      applyToAll,
		"updated_event": updatedEvent,
	}
	if worker != nil {
		exceptionsSummary := map[string]interface{}{
			"total":   len(exceptions),
			"updated": int(worker.processed.Load()),
			"failed":  int(worker.failed.Load()),
		}
		if failures := worker.Failures(); len(failures) > 0 {
			exceptionsSummary["failures"] = failures
		}
		result["exceptions"] = exceptionsSummary
	}
	runtime.OutFormat(result, nil, func(w io.Writer) {
		writeUpdatePretty(w, result, targetEventID, applyToAll, "updated_event", "Updated event", "Exceptions updated")
	})
	return nil
}

// writeUpdatePretty renders the +update pretty output in the shape the user
// asked for: (1) header stating which apply_to scope ran on which event_id,
// (2) the updated master/single event details, (3) an optional exceptions
// summary. eventKey is the top-level key in `result` that carries the event
// details ("updated_event"); eventLabel and exceptionsLabel are section titles.
func writeUpdatePretty(w io.Writer, result map[string]interface{}, targetEventID, scope, eventKey, eventLabel, exceptionsLabel string) {
	fmt.Fprintf(w, "Applied `%s` on %s\n\n", scope, targetEventID)
	if ev, ok := result[eventKey].(map[string]interface{}); ok {
		fmt.Fprintf(w, "%s:\n", eventLabel)
		output.PrintTable(w, []map[string]interface{}{ev})
		fmt.Fprintln(w)
	}
	if exceptions, ok := result["exceptions"].(map[string]interface{}); ok {
		fmt.Fprintf(w, "%s:\n", exceptionsLabel)
		output.PrintTable(w, []map[string]interface{}{exceptions})
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "Event updated successfully")
}

// runExceptionDeletion deletes every exception in future-first order using the
// shared worker and returns the worker so the caller can surface Failures().
// A returned error indicates only context cancellation; per-item failures do
// not abort the batch. delete_exception=true so each row is destroyed
// (is_deleted=true) rather than left as a cancelled placeholder — this path
// only runs when a series-wide time change has invalidated the original
// occurrence slot, so a lingering cancelled marker would only add noise to a
// subsequent series-view.
func runExceptionDeletion(ctx context.Context, runtime *common.RuntimeContext, calendarID string, future, past []exceptionInstance) (*exceptionWorker, error) {
	worker := &exceptionWorker{
		Concurrency: 3,
		Runtime:     runtime,
		Label:       updateLogPrefix,
		Total:       len(future) + len(past),
		Do: func(_ context.Context, ex exceptionInstance) error {
			return deleteEventOnce(runtime, calendarID, ex.EventID, false, true)
		},
	}
	if err := worker.Run(ctx, future); err != nil {
		return worker, err
	}
	if err := worker.Run(ctx, past); err != nil {
		return worker, err
	}
	worker.finalSummary(runtime.IO().ErrOut, "deleted")
	return worker, nil
}

// runExceptionPatch applies the same field-level update (built from the same
// flags) to every exception. Attendee add/remove is replayed per exception —
// participants have separate acceptance state, so a name/description change
// stays semantically consistent while attendee changes propagate to the
// series as a whole. Per-item failures are recorded on the worker and do not
// abort the batch.
//
// This runs only in the apply-to=all non-time branch (a real time change
// deletes exceptions instead). It therefore always strips start_time/end_time
// from the PATCH body — even when the caller passed --start/--end, they must
// have echoed the master's time (that's why we're here), and pushing the
// master's time onto every exception would overwrite each exception's own
// occurrence slot.
func runExceptionPatch(ctx context.Context, runtime *common.RuntimeContext, calendarID string, future, past []exceptionInstance) (*exceptionWorker, error) {
	worker := &exceptionWorker{
		Concurrency: 3,
		Runtime:     runtime,
		Label:       updateLogPrefix,
		Total:       len(future) + len(past),
		Do: func(_ context.Context, ex exceptionInstance) error {
			if _, _, _, err := applyUpdateToEvent(runtime, calendarID, ex.EventID, true); err != nil {
				return err
			}
			return nil
		},
	}
	if err := worker.Run(ctx, future); err != nil {
		return worker, err
	}
	if err := worker.Run(ctx, past); err != nil {
		return worker, err
	}
	worker.finalSummary(runtime.IO().ErrOut, "patched")
	return worker, nil
}

// masterTimeChanged reports whether --start / --end (once parsed) actually
// differ from the master's stored time. A caller that re-passes the master's
// current time — common when a script echoes the fetched event back — should
// not force the "time change" branch, which would delete every exception. The
// caller must have set both flags; --start and --end are wired to travel
// together upstream. Returns false when either flag was omitted or the input
// exactly matches the master.
//
// Timed and all-day masters carry different fields (`timestamp` vs `date`), and
// common.ParseTime normalises date-only input against time.Local, whereas Lark
// stores all-day dates at UTC midnight (see masterStartUnix). Comparing the two
// via unix seconds therefore never matches for a legitimate echo of an all-day
// event. We branch on the master shape: all-day masters compare on the raw
// YYYY-MM-DD date, timed masters compare on the unix-second string.
func masterTimeChanged(runtime *common.RuntimeContext, master *calendarEvent) (bool, error) {
	if !(runtime.Cmd.Flags().Changed("start") && runtime.Cmd.Flags().Changed("end")) {
		return false, nil
	}
	if master == nil || master.StartTime == nil || master.EndTime == nil {
		return true, nil
	}
	startInput := strings.TrimSpace(runtime.Str("start"))
	endInput := strings.TrimSpace(runtime.Str("end"))

	if isAllDayEvent(master) {
		// All-day master: only a bare YYYY-MM-DD input can echo the stored
		// `date` unchanged. A timestamped or datetime input inherently drops
		// all-day semantics and is a real time change.
		startDate, ok := parseDateOnly(startInput, time.UTC)
		if !ok {
			return true, nil
		}
		endDate, ok := parseDateOnly(endInput, time.UTC)
		if !ok {
			return true, nil
		}
		return !(startDate == master.StartTime.Date && endDate == master.EndTime.Date), nil
	}

	startTs, err := common.ParseTime(startInput)
	if err != nil {
		return false, errs.NewValidationError(errs.SubtypeInvalidArgument, "--start: %v", err).WithParam("--start")
	}
	endTs, err := common.ParseTime(endInput, "end")
	if err != nil {
		return false, errs.NewValidationError(errs.SubtypeInvalidArgument, "--end: %v", err).WithParam("--end")
	}
	return !(startTs == master.StartTime.Timestamp && endTs == master.EndTime.Timestamp), nil
}

// parseDateOnly returns the canonical YYYY-MM-DD form when s is exactly a
// date (no time component). Used to compare user input against an all-day
// master's stored `date` without going through timezone-sensitive unix math.
func parseDateOnly(s string, loc *time.Location) (string, bool) {
	t, err := time.ParseInLocation("2006-01-02", s, loc)
	if err != nil {
		return "", false
	}
	return t.Format("2006-01-02"), true
}

// applyUpdateToEvent runs the field PATCH + attendee add/remove trio against a
// single event id. Extracted so the master and every exception share exactly
// the same code path — including field selection logic (only user-set flags
// are propagated).
//
// omitTime, when true, strips start_time/end_time from the PATCH body before
// sending. Used by the apply-to=all non-time branch: masterTimeChanged already
// confirmed the caller's --start/--end just echo the master's stored time
// (common when a script fetches the event and passes it back), so those flags
// were logically not a time change — but buildCalendarUpdateEventData only
// knows the flag was set. Sending the master's time on to every exception PATCH
// would overwrite each exception's own occurrence slot with the master's first
// instance time, which is the bug this parameter guards against. The single-
// event and time-change paths pass false; only the non-time apply-to=all path
// needs the strip.
func applyUpdateToEvent(runtime *common.RuntimeContext, calendarID, eventID string, omitTime bool) (map[string]interface{}, int, int, error) {
	body, hasEventFields, err := buildCalendarUpdateEventData(runtime)
	if err != nil {
		return nil, 0, 0, err
	}
	if omitTime {
		delete(body, "start_time")
		delete(body, "end_time")
		delete(body, "need_notification")
		hasEventFields = len(body) > 0
		if hasEventFields {
			body["need_notification"] = runtime.Bool("notify")
		}
	}
	event := map[string]interface{}{}
	if hasEventFields {
		data, err := runtime.CallAPITyped("PATCH", calendarUpdateEventPath(calendarID, eventID),
			map[string]interface{}{"user_id_type": "open_id"}, body)
		if err != nil {
			return nil, 0, 0, err
		}
		if v, _ := data["event"].(map[string]interface{}); v != nil {
			event = v
		}
	}
	removed := 0
	if removeStr := runtime.Str("remove-attendee-ids"); strings.TrimSpace(removeStr) != "" {
		deleteIDs, err := attendeeDeleteIDs(removeStr)
		if err != nil {
			return nil, 0, 0, err
		}
		_, err = runtime.CallAPITyped("POST", calendarUpdateAttendeesPath(calendarID, eventID)+"/batch_delete",
			map[string]interface{}{"user_id_type": "open_id"},
			map[string]interface{}{"delete_ids": deleteIDs, "need_notification": runtime.Bool("notify")})
		if err != nil {
			return nil, 0, 0, err
		}
		removed = len(deleteIDs)
	}
	added := 0
	if addStr := runtime.Str("add-attendee-ids"); strings.TrimSpace(addStr) != "" {
		attendees, err := parseAttendees(addStr, "")
		if err != nil {
			return nil, 0, 0, withParam(err, "--add-attendee-ids")
		}
		_, err = runtime.CallAPITyped("POST", calendarUpdateAttendeesPath(calendarID, eventID),
			map[string]interface{}{"user_id_type": "open_id"},
			map[string]interface{}{"attendees": attendees, "need_notification": runtime.Bool("notify")})
		if err != nil {
			return nil, 0, 0, err
		}
		added = len(attendees)
	}
	return event, added, removed, nil
}

// executeCalendarUpdateThisAndFollowing implements --apply-to=this-and-following.
//
// Steps:
//  1. delete exceptions on/after the pivot instance
//  2. PATCH the master's rrule with UNTIL = pivot - 1s (truncating the old
//     series before the pivot)
//  3. POST a new event starting at the pivot instance, inheriting the master's
//     summary/description/attendees/vchat/reminders/location, overlaid by any
//     flag values the caller passed. This is the "carry the edits forward"
//     step users expect from Google/Apple.
func executeCalendarUpdateThisAndFollowing(ctx context.Context, runtime *common.RuntimeContext, calendarID string, current *calendarEvent, targetEventID string) error {
	pivotUnix, ok := parseInstanceOriginalTime(targetEventID)
	if !ok {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--%s=%s needs a recurring instance event_id (shape `{uid}_{originalTime}` with a positive originalTime); got %q",
			flagApplyTo, applyToThisAndFollowing, targetEventID).WithParam("--event-id")
	}
	master, err := ensureMasterEvent(runtime, calendarID, current, targetEventID)
	if err != nil {
		return err
	}
	masterID := master.EventID
	if masterID == "" {
		masterID = masterEventID(targetEventID)
		master.EventID = masterID
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
	// this-and-following always destroys exceptions on/after the pivot
	// (deleteException=true → is_deleted=true), so include cancelled
	// placeholders in the scan so they get destroyed alongside the live rows.
	all, err := listExceptionsInWindow(ctx, runtime, calendarID, masterID,
		recurringWindow{Start: pivotDay.Unix(), End: windowEnd}, true)
	if err != nil {
		return err
	}
	future := filterOnOrAfter(all, pivotUnix)

	errOut := runtime.IO().ErrOut
	var worker *exceptionWorker
	if len(future) > 0 {
		fmt.Fprintf(errOut, "%s this-and-following: deleting %d exception(s) on/after pivot %d (%s)\n",
			updateLogPrefix, len(future), pivotUnix, formatPivotDatetime(pivotUnix, loc))
		worker, err = runExceptionDeletion(ctx, runtime, calendarID, future, nil)
		if err != nil {
			return err
		}
	}
	truncatedRRule := truncateRecurrenceUntil(master.Recurrence, pivotDay)
	fmt.Fprintf(errOut, "%s truncating master rrule at %s (UNTIL=%s)\n",
		updateLogPrefix, pivotDay.Format(time.RFC3339), truncatedRRule)
	patchResp, err := runtime.CallAPITyped("PATCH", calendarUpdateEventPath(calendarID, masterID),
		map[string]interface{}{"user_id_type": "open_id"},
		map[string]interface{}{
			"recurrence":        truncatedRRule,
			"need_notification": runtime.Bool("notify"),
		})
	if err != nil {
		return withStepContext(err, "future exceptions deleted but master rrule truncation failed for %s", masterID)
	}
	truncatedMaster, err := parseCalendarEvent(patchResp)
	if err != nil {
		return withStepContext(err, "master rrule truncated but response parsing failed for %s", masterID)
	}

	newEventID, newEvent, addedCount, removedCount, err := createFollowingSeries(runtime, calendarID, master, current, pivotUnix)
	if err != nil {
		return withStepContext(err, "master truncated but the new (following) series could not be created; you may need to recreate it manually with `+create`")
	}
	fmt.Fprintf(errOut, "%s created new following series event_id=%s\n", updateLogPrefix, newEventID)

	truncatedEvent := map[string]interface{}{
		"master_event_id": masterID,
		"rrule_truncated": truncatedRRule,
	}
	if start := formatCalendarEventTime(eventTimeAsMap(truncatedMaster.StartTime)); start != "" {
		truncatedEvent["start"] = start
	}
	if end := formatCalendarEventTime(eventTimeAsMap(truncatedMaster.EndTime)); end != "" {
		truncatedEvent["end"] = end
	}
	followEvent := calendarUpdateResult(newEventID, newEvent, addedCount, removedCount)
	if rrule, _ := newEvent["recurrence"].(string); rrule != "" {
		followEvent["rrule"] = rrule
	}

	result := map[string]interface{}{
		"calendar_id":     calendarID,
		"apply_to":        applyToThisAndFollowing,
		"truncated_event": truncatedEvent,
		"follow_event":    followEvent,
	}
	if worker != nil {
		exceptions := map[string]interface{}{
			"total":   len(future),
			"deleted": int(worker.processed.Load()),
			"failed":  int(worker.failed.Load()),
		}
		if failures := worker.Failures(); len(failures) > 0 {
			exceptions["failures"] = failures
		}
		result["exceptions"] = exceptions
	}
	runtime.OutFormat(result, nil, func(w io.Writer) {
		writeThisAndFollowingPretty(w, result, targetEventID)
	})
	return nil
}

// writeThisAndFollowingPretty renders the three sections users expect in this
// order (only the ones with content):
//  1. Header: which apply_to scope was executed on which event_id.
//  2. Truncated event: the pivot-instance id, its master id, and the master's
//     new (truncated) recurrence rule.
//  3. Follow-up event: the newly created series' event_id, start/end (RFC3339),
//     rrule, attendee counts, summary/description.
//  4. Exception deletion summary (skipped when there was nothing to delete).
func writeThisAndFollowingPretty(w io.Writer, result map[string]interface{}, targetEventID string) {
	fmt.Fprintf(w, "Applied `%s` on %s\n\n", applyToThisAndFollowing, targetEventID)
	if truncated, ok := result["truncated_event"].(map[string]interface{}); ok {
		fmt.Fprintln(w, "Truncated event:")
		output.PrintTable(w, []map[string]interface{}{truncated})
		fmt.Fprintln(w)
	}
	if follow, ok := result["follow_event"].(map[string]interface{}); ok {
		fmt.Fprintln(w, "Follow-up event:")
		output.PrintTable(w, []map[string]interface{}{follow})
		fmt.Fprintln(w)
	}
	if exceptions, ok := result["exceptions"].(map[string]interface{}); ok {
		fmt.Fprintln(w, "Exceptions removed:")
		output.PrintTable(w, []map[string]interface{}{exceptions})
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "Recurring series updated successfully")
}

// createFollowingSeries assembles the request body for the new "following"
// series and POSTs /events. Inheritance rules (base = master, overlaid by
// caller flags):
//   - summary / description: caller flag when Changed(), else master value
//   - start / end: caller flag when Changed(), else the pivot instance's
//     start/end (so the new series begins exactly where the old one was
//     truncated)
//   - rrule: caller flag when Changed(), else the master's original rrule
//     with its UNTIL kept verbatim and COUNT dropped. UNTIL is a fixed
//     absolute date so it copies cleanly; COUNT anchors on the old dtstart
//     and has no meaning against a new pivot.
//   - vchat / reminders / attendee_ability / free_busy_status / location /
//     visibility: inherited from the master untouched.
//
// The create endpoint does not accept attendees inline, so participants and
// meeting rooms are propagated in a follow-up POST /attendees: the master's
// existing attendees are fetched and re-added on the new series, then
// --remove-attendee-ids is subtracted and --add-attendee-ids is unioned in.
func createFollowingSeries(runtime *common.RuntimeContext, calendarID string, master, pivot *calendarEvent, pivotUnix int64) (string, map[string]interface{}, int, int, error) {
	body := map[string]interface{}{
		"summary":          "",
		"attendee_ability": firstNonEmpty(master.AttendeeAbility, "can_modify_event"),
		"free_busy_status": firstNonEmpty(master.FreeBusyStatus, "busy"),
		"reminders": func() interface{} {
			if len(master.Reminders) == 0 {
				return []map[string]int{{"minutes": 5}}
			}
			return master.Reminders
		}(),
		"color": master.Color,
	}
	if master.VChat != nil {
		body["vchat"] = master.VChat
	}
	if master.Location != nil {
		body["location"] = master.Location
	}
	if master.Visibility != "" {
		body["visibility"] = master.Visibility
	}

	if master.Attachments != nil {
		body["attachments"] = master.Attachments
	}

	if master.EventCheckIn != nil {
		body["event_check_in"] = master.EventCheckIn
	}

	if runtime.Cmd.Flags().Changed("summary") {
		body["summary"] = runtime.Str("summary")
	} else if master.Summary != "" {
		body["summary"] = master.Summary
	}
	if runtime.Cmd.Flags().Changed("description") {
		body["description_rich"] = runtime.Str("description")
	} else if master.DescriptionRich != "" {
		body["description_rich"] = master.DescriptionRich
	} else if master.Description != "" {
		body["description_rich"] = master.Description
	}

	startTs, endTs, err := resolveFollowingSeriesTimes(runtime, pivot, pivotUnix)
	if err != nil {
		return "", nil, 0, 0, err
	}
	startTimeMap := map[string]string{"timestamp": startTs}
	if master.StartTime != nil && master.StartTime.Timezone != "" {
		startTimeMap["timezone"] = master.StartTime.Timezone
	}
	body["start_time"] = startTimeMap
	endTimeMap := map[string]string{"timestamp": endTs}
	if master.EndTime != nil && master.EndTime.Timezone != "" {
		endTimeMap["timezone"] = master.EndTime.Timezone
	}
	body["end_time"] = endTimeMap

	newRRule := ""
	if runtime.Cmd.Flags().Changed("rrule") {
		newRRule = strings.TrimSpace(runtime.Str("rrule"))
	} else {
		newRRule = strings.TrimSpace(inheritRRuleForFollowing(master.Recurrence))
	}
	if newRRule != "" {
		body["recurrence"] = newRRule
	}

	data, err := runtime.CallAPITyped("POST",
		fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events", validate.EncodePathSegment(calendarID)),
		nil, body)
	if err != nil {
		return "", nil, 0, 0, err
	}
	newEvent, _ := data["event"].(map[string]interface{})
	newEventID, _ := newEvent["event_id"].(string)
	if newEventID == "" {
		return "", nil, 0, 0, errs.NewInternalError(errs.SubtypeInvalidResponse, "create returned empty event_id for the new following series")
	}

	// Attendees / rooms: inherit master's, subtract --remove, union --add.
	inherited, err := fetchInheritableAttendees(runtime, calendarID, master.EventID)
	if err != nil {
		return newEventID, newEvent, 0, 0, withStepContext(err, "new series %s created but master attendee inheritance failed", newEventID)
	}
	removeIDs := map[string]struct{}{}
	removedCount := 0
	if removeStr := runtime.Str("remove-attendee-ids"); strings.TrimSpace(removeStr) != "" {
		ids, err := parseCalendarAttendeeIDs(removeStr)
		if err != nil {
			return newEventID, newEvent, 0, 0, withParam(err, "--remove-attendee-ids")
		}
		for _, id := range ids {
			removeIDs[id] = struct{}{}
		}
		removedCount = len(ids)
	}
	final := filterOutAttendees(inherited, removeIDs)
	addedCount := 0
	if addStr := runtime.Str("add-attendee-ids"); strings.TrimSpace(addStr) != "" {
		added, err := parseAttendees(addStr, "")
		if err != nil {
			return newEventID, newEvent, 0, 0, withParam(err, "--add-attendee-ids")
		}
		final = mergeAttendees(final, added)
		addedCount = len(added)
	}
	if len(final) > 0 {
		_, err = runtime.CallAPITyped("POST",
			calendarUpdateAttendeesPath(calendarID, newEventID),
			map[string]interface{}{"user_id_type": "open_id"},
			map[string]interface{}{"attendees": final, "need_notification": runtime.Bool("notify")})
		if err != nil {
			return newEventID, newEvent, addedCount, removedCount, withStepContext(err, "new series %s created but attendee sync failed", newEventID)
		}
	}
	return newEventID, newEvent, addedCount, removedCount, nil
}

// inheritRRuleForFollowing keeps every rrule segment except COUNT. UNTIL is a
// fixed wall-clock cutoff and copies cleanly onto the new series; COUNT is an
// occurrence total anchored on the old dtstart and would be misinterpreted
// against the new pivot, so we drop it.
func inheritRRuleForFollowing(rrule string) string {
	body := strings.TrimSpace(rrule)
	prefix := ""
	if strings.HasPrefix(body, "RRULE:") {
		prefix = "RRULE:"
		body = strings.TrimPrefix(body, "RRULE:")
	}
	var kept []string
	for _, part := range strings.Split(body, ";") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if strings.HasPrefix(strings.ToUpper(p), "COUNT=") {
			continue
		}
		kept = append(kept, p)
	}
	if len(kept) == 0 {
		return ""
	}
	return prefix + strings.Join(kept, ";")
}

// fetchInheritableAttendees pages through the master's attendee list and
// projects each entry back into the create/attendees request shape
// ({type, user_id|chat_id|room_id, ...}). Third-party attendees are skipped
// because /events/{id}/attendees does not accept them.
func fetchInheritableAttendees(runtime *common.RuntimeContext, calendarID, masterID string) ([]map[string]interface{}, error) {
	if strings.TrimSpace(masterID) == "" {
		return nil, nil
	}
	out := []map[string]interface{}{}
	pageToken := ""
	for {
		params := map[string]interface{}{"page_size": 100, "user_id_type": "open_id"}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		data, err := runtime.CallAPITyped("GET",
			fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/%s/attendees",
				validate.EncodePathSegment(calendarID), validate.EncodePathSegment(masterID)),
			params, nil)
		if err != nil {
			return nil, err
		}
		for _, raw := range common.GetSlice(data, "items") {
			item, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if projected := projectInheritedAttendee(item); projected != nil {
				out = append(out, projected)
			}
		}
		hasMore, _ := data["has_more"].(bool)
		pageToken, _ = data["page_token"].(string)
		if !hasMore || pageToken == "" {
			break
		}
	}
	return out, nil
}

// projectInheritedAttendee reduces a fetched attendee entry to the fields the
// create/attendees endpoint accepts on a new event. Returns nil when the entry
// is not carriable (unknown type, third-party without shape guarantees, or
// missing the id field its type requires).
func projectInheritedAttendee(a map[string]interface{}) map[string]interface{} {
	t, _ := a["type"].(string)
	switch t {
	case "user":
		userID, _ := a["user_id"].(string)
		if userID == "" {
			return nil
		}
		return map[string]interface{}{"type": "user", "user_id": userID}
	case "resource":
		roomID, _ := a["room_id"].(string)
		if roomID == "" {
			return nil
		}
		out := map[string]interface{}{"type": "resource", "room_id": roomID}
		if v, ok := a["resource_customization"]; ok && v != nil {
			out["resource_customization"] = v
		}
		return out
	case "chat":
		chatID, _ := a["chat_id"].(string)
		if chatID == "" {
			return nil
		}
		return map[string]interface{}{"type": "chat", "chat_id": chatID}
	case "third_party":
		thirdPartyEmail, _ := a["third_party_email"].(string)
		if thirdPartyEmail == "" {
			return nil
		}
		return map[string]interface{}{"type": "third_party", "third_party_email": thirdPartyEmail}
	}
	return nil
}

// filterOutAttendees drops entries whose id matches a value in removeIDs. It
// only inspects the id field appropriate to each type.
func filterOutAttendees(list []map[string]interface{}, removeIDs map[string]struct{}) []map[string]interface{} {
	if len(removeIDs) == 0 {
		return list
	}
	out := make([]map[string]interface{}, 0, len(list))
	for _, a := range list {
		if attendeeMatchesRemoval(a, removeIDs) {
			continue
		}
		out = append(out, a)
	}
	return out
}

func attendeeMatchesRemoval(a map[string]interface{}, removeIDs map[string]struct{}) bool {
	for _, key := range []string{"user_id", "chat_id", "room_id"} {
		if id, _ := a[key].(string); id != "" {
			if _, ok := removeIDs[id]; ok {
				return true
			}
		}
	}
	return false
}

// mergeAttendees appends parsed --add-attendee-ids entries (map[string]string
// as produced by parseAttendees) onto the inherited list without duplicating
// an entry already present.
func mergeAttendees(inherited []map[string]interface{}, added []map[string]string) []map[string]interface{} {
	seen := map[string]struct{}{}
	for _, a := range inherited {
		if key := attendeeKey(a); key != "" {
			seen[key] = struct{}{}
		}
	}
	out := inherited
	for _, a := range added {
		entry := map[string]interface{}{}
		for k, v := range a {
			entry[k] = v
		}
		key := attendeeKey(entry)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, entry)
	}
	return out
}

func attendeeKey(a map[string]interface{}) string {
	t, _ := a["type"].(string)
	for _, key := range []string{"user_id", "chat_id", "room_id"} {
		if id, _ := a[key].(string); id != "" {
			return t + ":" + id
		}
	}
	return ""
}

// resolveFollowingSeriesTimes returns the (start, end) unix-seconds strings
// to use for the new series. If the caller explicitly set --start/--end, they
// take precedence; otherwise the pivot instance's own times are reused so the
// new series begins right where the caller pointed.
func resolveFollowingSeriesTimes(runtime *common.RuntimeContext, pivot *calendarEvent, pivotUnix int64) (string, string, error) {
	if runtime.Cmd.Flags().Changed("start") && runtime.Cmd.Flags().Changed("end") {
		startTs, err := common.ParseTime(runtime.Str("start"))
		if err != nil {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--start: %v", err).WithParam("--start")
		}
		endTs, err := common.ParseTime(runtime.Str("end"), "end")
		if err != nil {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--end: %v", err).WithParam("--end")
		}
		return startTs, endTs, nil
	}
	// Fall back to the pivot instance times when the caller only changed
	// non-time fields.
	if pivot != nil && pivot.StartTime != nil && pivot.EndTime != nil {
		startTs := pivot.StartTime.Timestamp
		endTs := pivot.EndTime.Timestamp
		if startTs != "" && endTs != "" {
			return startTs, endTs, nil
		}
	}
	// Last-resort: same duration as one hour starting at pivot. The instances
	// API always returns instance timestamps, so this should not happen in
	// practice.
	return fmt.Sprintf("%d", pivotUnix), fmt.Sprintf("%d", pivotUnix+3600), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
