// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// newAttendeeValidateRuntime builds a RuntimeContext with the add/remove
// attendee-id flags set, for exercising validateCalendarUpdateAttendees.
func newAttendeeValidateRuntime(t *testing.T, add, remove string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("add-attendee-ids", "", "")
	cmd.Flags().String("remove-attendee-ids", "", "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if add != "" {
		_ = cmd.Flags().Set("add-attendee-ids", add)
	}
	if remove != "" {
		_ = cmd.Flags().Set("remove-attendee-ids", remove)
	}
	return &common.RuntimeContext{Cmd: cmd}
}

// assertValidationParam asserts err is a *errs.ValidationError whose Param
// equals wantParam, and returns it for any further message assertions.
func assertValidationParam(t *testing.T, err error, wantParam string) *errs.ValidationError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T (%v)", err, err)
	}
	if ve.Param != wantParam {
		t.Errorf("Param = %q, want %q", ve.Param, wantParam)
	}
	return ve
}

// ---------------------------------------------------------------------------
// withStepContext helper
// ---------------------------------------------------------------------------

func TestWithStepContext_Nil(t *testing.T) {
	if got := withStepContext(nil, "step %d", 1); got != nil {
		t.Fatalf("withStepContext(nil) = %v, want nil", got)
	}
}

func TestWithStepContext_AppendsToTypedHint(t *testing.T) {
	// A typed error keeps its classification; the context is appended to Hint.
	inner := errs.NewAPIError(errs.SubtypeUnknown, "boom").WithHint("first")
	got := withStepContext(inner, "after steps %v", []string{"event"})
	var ae *errs.APIError
	if !errors.As(got, &ae) {
		t.Fatalf("want *errs.APIError, got %T", got)
	}
	if ae.Hint == "" || !strings.Contains(ae.Hint, "first") || !strings.Contains(ae.Hint, "after steps") {
		t.Errorf("hint should append context, got %q", ae.Hint)
	}
}

func TestWithStepContext_SetsHintWhenEmpty(t *testing.T) {
	inner := errs.NewAPIError(errs.SubtypeUnknown, "boom")
	got := withStepContext(inner, "after steps %v", []string{"event"})
	var ae *errs.APIError
	if !errors.As(got, &ae) {
		t.Fatalf("want *errs.APIError, got %T", got)
	}
	if !strings.Contains(ae.Hint, "after steps") {
		t.Errorf("hint should be set, got %q", ae.Hint)
	}
}

func TestWithStepContext_UnclassifiedFallsBackToInternal(t *testing.T) {
	// A plain, unclassified error is wrapped into a typed internal error so the
	// envelope still tells the truth.
	got := withStepContext(errors.New("raw failure"), "after steps %v", []string{"event"})
	var ie *errs.InternalError
	if !errors.As(got, &ie) {
		t.Fatalf("want *errs.InternalError, got %T", got)
	}
	if ie.Subtype != errs.SubtypeSDKError {
		t.Errorf("subtype=%q, want sdk_error", ie.Subtype)
	}
	if !strings.Contains(ie.Message, "raw failure") {
		t.Errorf("message should preserve original, got %q", ie.Message)
	}
}

// ---------------------------------------------------------------------------
// withParam helper
// ---------------------------------------------------------------------------

func TestWithParam_AttachesToValidationError(t *testing.T) {
	inner := errs.NewValidationError(errs.SubtypeInvalidArgument, "boom")
	got := withParam(inner, "--attendee-ids")
	ve := assertValidationParam(t, got, "--attendee-ids")
	if ve != inner {
		t.Errorf("withParam should return the same underlying error, got a different pointer")
	}
	if ve.Message != "boom" {
		t.Errorf("message mutated: got %q, want %q", ve.Message, "boom")
	}
}

func TestWithParam_NonValidationPassesThrough(t *testing.T) {
	inner := errs.NewInternalError(errs.SubtypeSDKError, "io failure")
	got := withParam(inner, "--attendee-ids")
	if got != inner {
		t.Fatalf("non-validation error should pass through unchanged, got %v", got)
	}
	var ve *errs.ValidationError
	if errors.As(got, &ve) {
		t.Fatalf("non-validation error must not become a ValidationError")
	}
}

func TestWithParam_NilPassesThrough(t *testing.T) {
	if got := withParam(nil, "--attendee-ids"); got != nil {
		t.Fatalf("withParam(nil) = %v, want nil", got)
	}
}

// ---------------------------------------------------------------------------
// Part A — re-wrap sites: the parseAttendees error, attributed by the caller's
// flag, must be the inner typed error (not a re-wrapped nesting).
// ---------------------------------------------------------------------------

func TestParseAttendees_AttributedToCreateFlag(t *testing.T) {
	_, err := parseAttendees("bad-id", "")
	// create's add path: withParam(err, "--attendee-ids")
	got := withParam(err, "--attendee-ids")
	assertValidationParam(t, got, "--attendee-ids")
}

func TestParseAttendees_AttributedToAddFlag(t *testing.T) {
	_, err := parseAttendees("bad-id", "")
	// update's add path: withParam(err, "--add-attendee-ids")
	got := withParam(err, "--add-attendee-ids")
	assertValidationParam(t, got, "--add-attendee-ids")
}

func TestParseAttendees_InnerStaysFlagAgnostic(t *testing.T) {
	// The shared inner parser must not pre-attribute a flag; callers do.
	_, err := parseAttendees("bad-id", "")
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	if ve.Param != "" {
		t.Errorf("inner parseAttendees should stay flag-agnostic, got Param = %q", ve.Param)
	}
}

// ---------------------------------------------------------------------------
// Part B — direct attendee-id format validations carry their flag.
// ---------------------------------------------------------------------------

func TestParseRoomFindAttendees_FormatErrorParam(t *testing.T) {
	_, _, err := parseRoomFindAttendees("bad-id", "")
	assertValidationParam(t, err, "--"+flagAttendees)
}

func TestParseRoomFindAttendees_RejectsRoomID(t *testing.T) {
	// room find only supports ou_/oc_; omm_ rooms are not valid attendees.
	_, _, err := parseRoomFindAttendees("omm_room", "")
	assertValidationParam(t, err, "--"+flagAttendees)
}

func TestParseCalendarAttendeeIDs_StaysFlagAgnostic(t *testing.T) {
	// parseCalendarAttendeeIDs serves BOTH --add-attendee-ids and
	// --remove-attendee-ids, so it must not pre-attribute a flag.
	_, err := parseCalendarAttendeeIDs("bad-id")
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	if ve.Param != "" {
		t.Errorf("shared parser should stay flag-agnostic, got Param = %q", ve.Param)
	}
}

func TestValidateCalendarUpdateAttendees_RemoveFormatParam(t *testing.T) {
	// The remove path attributes its parser error to --remove-attendee-ids.
	rt := newAttendeeValidateRuntime(t, "", "bad-id")
	err := validateCalendarUpdateAttendees(rt)
	assertValidationParam(t, err, "--remove-attendee-ids")
}

func TestValidateCalendarUpdateAttendees_AddFormatParam(t *testing.T) {
	// The add path attributes its parser error to --add-attendee-ids.
	rt := newAttendeeValidateRuntime(t, "bad-id", "")
	err := validateCalendarUpdateAttendees(rt)
	assertValidationParam(t, err, "--add-attendee-ids")
}

// attendeeDeleteIDs's switch default is defensive: parseCalendarAttendeeIDs
// already rejects any non-ou_/oc_/omm_ id, so only a well-formed id reaches the
// switch and the valid branches map it. This asserts the happy path maps types.
func TestAttendeeDeleteIDs_MapsKnownTypes(t *testing.T) {
	got, err := attendeeDeleteIDs("ou_a,oc_b,omm_c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 delete ids, got %d: %v", len(got), got)
	}
	wantTypes := map[string]string{"user": "user_id", "chat": "chat_id", "resource": "room_id"}
	for _, m := range got {
		key, ok := wantTypes[m["type"]]
		if !ok {
			t.Errorf("unexpected type %q in %v", m["type"], m)
			continue
		}
		if m[key] == "" {
			t.Errorf("missing %s for type %q in %v", key, m["type"], m)
		}
	}
}

func TestParseCalendarAttendeeIDs_Valid(t *testing.T) {
	ids, err := parseCalendarAttendeeIDs(" ou_a , oc_b , ou_a ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 || ids[0] != "ou_a" || ids[1] != "oc_b" {
		t.Errorf("dedup/trim failed: got %v", ids)
	}
}
