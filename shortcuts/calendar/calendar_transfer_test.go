// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

const (
	transferCalendarID = "cal_test123"
	transferEventID    = "uid_abc_1742515200"
	transferReceiverID = "ou_receiver"
)

func transferGetEventStub(event map[string]interface{}) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/" + transferCalendarID + "/events/" + transferEventID,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": event},
		},
	}
}

func transferPostStub() *httpmock.Stub {
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/" + transferCalendarID + "/events/" + transferEventID + "/transfer",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	}
}

func transferArgs(extra ...string) []string {
	return append([]string{
		"+transfer",
		"--calendar-id", transferCalendarID,
		"--event-id", transferEventID,
		"--to-user-id", transferReceiverID,
		"--as", "bot",
		"--yes",
	}, extra...)
}

// An explicit --calendar-id may be shared, and the transfer response says
// nothing about the original organizer. Reporting false there is the bug: it
// made agents claim the original organizer stayed as an attendee. The field is
// omitted instead, and stderr explains the shared-calendar rule.
func TestTransfer_NamedCalendar_OmitsOutcomeAndExplains(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(transferGetEventStub(map[string]interface{}{
		"event_id": transferEventID,
		"summary":  "One-off sync",
	}))
	post := transferPostStub()
	reg.Register(post)

	if err := mountAndRun(t, CalendarTransfer, transferArgs(), f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(post.CapturedBodies) != 1 {
		t.Fatalf("want exactly 1 transfer call, got %d", len(post.CapturedBodies))
	}
	var body map[string]interface{}
	if err := json.Unmarshal(post.CapturedBodies[0], &body); err != nil {
		t.Fatalf("unmarshal transfer body: %v", err)
	}
	if body["to_user_id"] != transferReceiverID {
		t.Errorf("to_user_id=%v, want %s", body["to_user_id"], transferReceiverID)
	}
	// The request still forwards what the caller asked for; only the report
	// withholds an outcome the command cannot know.
	if body["need_remove_original_organizer"] != false {
		t.Errorf("need_remove_original_organizer=%v, want the flag value false", body["need_remove_original_organizer"])
	}
	out := stdout.String()
	if !strings.Contains(out, transferReceiverID) {
		t.Errorf("stdout should report the new organizer, got: %s", out)
	}
	if strings.Contains(out, "original_organizer_removed") {
		t.Errorf("an unknown outcome must not be reported, got: %s", out)
	}
	if errOut := stderr.String(); !strings.Contains(errOut, "shared calendar") {
		t.Errorf("stderr should explain the shared-calendar rule, got: %s", errOut)
	}
}

// The primary calendar alias is a primary calendar by definition, so the server
// never force-removes and the flag value is the truth.
func TestTransfer_PrimaryCalendar_ReportsOrganizerKept(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/" + transferEventID,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{"event_id": transferEventID}},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/transfer",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	})

	err := mountAndRun(t, CalendarTransfer, []string{
		"+transfer",
		"--event-id", transferEventID,
		"--to-user-id", transferReceiverID,
		"--as", "bot",
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out := stdout.String(); !strings.Contains(out, `"original_organizer_removed": false`) {
		t.Errorf("a primary calendar keeps the original organizer, got: %s", out)
	}
	if errOut := stderr.String(); strings.Contains(errOut, "shared calendar") {
		t.Errorf("a known outcome needs no shared-calendar note, got: %s", errOut)
	}
}

// Asking for the removal settles the outcome on any calendar.
func TestTransfer_RemoveOriginalOrganizer_ForwardsFlag(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(transferGetEventStub(map[string]interface{}{"event_id": transferEventID}))
	post := transferPostStub()
	reg.Register(post)

	if err := mountAndRun(t, CalendarTransfer, transferArgs("--"+flagRemoveOriginalOrganizer), f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(post.CapturedBodies[0], &body); err != nil {
		t.Fatalf("unmarshal transfer body: %v", err)
	}
	if body["need_remove_original_organizer"] != true {
		t.Errorf("need_remove_original_organizer=%v, want true", body["need_remove_original_organizer"])
	}
	if out := stdout.String(); !strings.Contains(out, `"original_organizer_removed": true`) {
		t.Errorf("stdout should report the removal, got: %s", out)
	}
	if errOut := stderr.String(); strings.Contains(errOut, "shared calendar") {
		t.Errorf("a known outcome needs no shared-calendar note, got: %s", errOut)
	}
}

// A recurring event must not be transferred until the caller has acknowledged
// that the whole series moves: the API cannot transfer a single occurrence.
func TestTransfer_RecurringEvent_BlockedWithoutSeriesConfirmation(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(transferGetEventStub(map[string]interface{}{
		"event_id":   transferEventID,
		"summary":    "Daily standup",
		"recurrence": "FREQ=DAILY;INTERVAL=1",
	}))
	transferred := false
	reg.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      "/transfer",
		Optional: true,
		OnMatch:  func(*http.Request) { transferred = true },
		Body:     map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	})

	err := mountAndRun(t, CalendarTransfer, transferArgs(), f, stdout)
	if err == nil {
		t.Fatal("want error for an unconfirmed recurring transfer")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype=%q, want %q", ve.Subtype, errs.SubtypeFailedPrecondition)
	}
	if ve.Param != "--"+flagTransferSeries {
		t.Errorf("param=%q, want --%s", ve.Param, flagTransferSeries)
	}
	if !strings.Contains(ve.Hint, flagTransferSeries) {
		t.Errorf("hint should name --%s, got %q", flagTransferSeries, ve.Hint)
	}
	if transferred {
		t.Error("transfer must not be issued before the series is confirmed")
	}
}

// An exception instance carries recurring_event_id rather than recurrence, and
// still transfers the whole series.
func TestTransfer_RecurringException_BlockedWithoutSeriesConfirmation(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(transferGetEventStub(map[string]interface{}{
		"event_id":           transferEventID,
		"is_exception":       true,
		"recurring_event_id": "uid_abc_0",
	}))

	err := mountAndRun(t, CalendarTransfer, transferArgs(), f, stdout)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("want failed_precondition validation error, got %v (%T)", err, err)
	}
}

// --transfer-series is the caller's confirmation, so the transfer is the only
// call the command makes.
func TestTransfer_SeriesConfirmed_SkipsRecurrenceRead(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	eventRead := false
	eventStub := transferGetEventStub(map[string]interface{}{
		"event_id":   transferEventID,
		"recurrence": "FREQ=DAILY;INTERVAL=1",
	})
	eventStub.Optional = true
	eventStub.OnMatch = func(*http.Request) { eventRead = true }
	reg.Register(eventStub)
	post := transferPostStub()
	reg.Register(post)

	if err := mountAndRun(t, CalendarTransfer, transferArgs("--"+flagTransferSeries), f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventRead {
		t.Error("--transfer-series should skip the recurrence pre-read")
	}
	if len(post.CapturedBodies) != 1 {
		t.Fatalf("want exactly 1 transfer call, got %d", len(post.CapturedBodies))
	}
}

// The pre-read guards against a silent whole-series transfer, so a failed read
// must block rather than fall through to the transfer.
func TestTransfer_RecurrenceReadFailure_BlocksTransfer(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/" + transferCalendarID + "/events/" + transferEventID,
		Body:   map[string]interface{}{"code": 193001, "msg": "event not found"},
	})
	transferred := false
	reg.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      "/transfer",
		Optional: true,
		OnMatch:  func(*http.Request) { transferred = true },
		Body:     map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	})

	if err := mountAndRun(t, CalendarTransfer, transferArgs(), f, stdout); err == nil {
		t.Fatal("want error when the recurrence pre-read fails")
	}
	if transferred {
		t.Error("transfer must not be issued when the recurrence check could not run")
	}
}

func TestTransfer_InvalidReceiverID_Typed(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarTransfer, []string{
		"+transfer",
		"--calendar-id", transferCalendarID,
		"--event-id", transferEventID,
		"--to-user-id", "user@example.com",
		"--as", "bot",
		"--yes",
	}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype=%q", ve.Subtype)
	}
	if ve.Param != "--to-user-id" {
		t.Errorf("param=%q, want --to-user-id", ve.Param)
	}
}

// Only the codes whose default agent reaction is wrong get a transfer-scoped
// hint. The two 403s look like a missing OAuth scope; the rest need a next
// action the server message does not name. Other codes stay as classified.
func TestTransfer_UpstreamCodes_CarryScopedRecovery(t *testing.T) {
	cases := []struct {
		code     int
		msg      string
		wantHint []string
	}{
		{larkErrCalendarNoAccessRole, "no calendar access_role", []string{"--as", "do not run `lark-cli auth login`"}},
		{larkErrCalendarWrongCalendarType, "invalid calendar type", []string{"primary or shared calendar"}},
		{larkErrCalendarCannotInviteReceiver, "no permission to invite the receiver", []string{"do not run `lark-cli auth login`", "another receiver"}},
		{larkErrCalendarEventNotInOrganizer, "the event is not in the organizer calendar", []string{"--calendar-id", "shared calendar"}},
		{larkErrCalendarCrossTenantTransfer, "cannot transfer the event to another tenant", []string{"attendee"}},
	}
	for _, tc := range cases {
		t.Run(tc.msg, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
			reg.Register(transferGetEventStub(map[string]interface{}{"event_id": transferEventID}))
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/transfer",
				Body:   map[string]interface{}{"code": tc.code, "msg": tc.msg},
			})

			err := mountAndRun(t, CalendarTransfer, transferArgs(), f, stdout)
			if err == nil {
				t.Fatalf("want error for code %d", tc.code)
			}
			var ae *errs.APIError
			if !errors.As(err, &ae) {
				t.Fatalf("want *errs.APIError, got %T", err)
			}
			if ae.Code != tc.code {
				t.Errorf("code=%d, want the upstream code %d preserved", ae.Code, tc.code)
			}
			for _, want := range tc.wantHint {
				if !strings.Contains(ae.Hint, want) {
					t.Errorf("hint for code %d should mention %q, got %q", tc.code, want, ae.Hint)
				}
			}
		})
	}
}

// A failed pre-read is not a transfer-scoped code, so recovery stays the
// existing step note. Agents must not treat "pass --transfer-series" as a
// substitute for a missing event.
func TestTransfer_PreReadFailure_KeepsStepContext(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/" + transferCalendarID + "/events/" + transferEventID,
		Body:   map[string]interface{}{"code": 193001, "msg": "event not found"},
	})

	err := mountAndRun(t, CalendarTransfer, transferArgs(), f, stdout)
	if err == nil {
		t.Fatal("want error when the pre-read fails")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *errs.APIError, got %T", err)
	}
	if ae.Code != 193001 {
		t.Errorf("code=%d, want 193001 preserved", ae.Code)
	}
	if !strings.Contains(ae.Hint, "recurring") {
		t.Errorf("hint should keep the pre-read step context, got %q", ae.Hint)
	}
}

// 190014 already carries the server's field-level reason, which is more precise
// than any flag this command could guess at, so the transfer recovery must leave
// it alone.
func TestTransfer_InvalidParamsDetail_KeepsServerReasonOnly(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(transferGetEventStub(map[string]interface{}{"event_id": transferEventID}))
	const reason = "The value of event_id(bad_id) is invalid. Please provide a valid value for the field."
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/transfer",
		Body: map[string]interface{}{
			"code": 190014,
			"msg":  "invalid request parameters. check details for more info.",
			"error": map[string]interface{}{
				"details": []interface{}{map[string]interface{}{"value": reason}},
			},
		},
	})

	err := mountAndRun(t, CalendarTransfer, transferArgs(), f, stdout)
	if err == nil {
		t.Fatal("want error for code 190014")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *errs.APIError, got %T", err)
	}
	if ae.Hint != reason {
		t.Errorf("hint=%q, want the server reason alone", ae.Hint)
	}
}

func TestTransfer_DryRun_ShowsPrecheckAndTransfer(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())

	if err := mountAndRun(t, CalendarTransfer, transferArgs("--dry-run"), f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"GET", "POST", "/transfer", "to_user_id", transferReceiverID, "need_remove_original_organizer",
		"/open-apis/calendar/v4/calendars/" + transferCalendarID + "/events/" + transferEventID,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run should contain %q, got: %s", want, out)
		}
	}
	if strings.Contains(out, `"url": "/open-apis/calendar/v4/calendars/`+transferCalendarID+`"`) {
		t.Errorf("dry-run should not preview a calendar read, got: %s", out)
	}
}
