// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

// attendeesResponseFixture returns a canned upstream response covering the four
// upstream attendee types with representative sparse fields.
func attendeesResponseFixture(hasMore bool, pageToken string) map[string]interface{} {
	return map[string]interface{}{
		"code": 0, "msg": "ok",
		"data": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{
					"type":         "user",
					"attendee_id":  "att_user1",
					"user_id":      "ou_user1",
					"display_name": "张三",
					"rsvp_status":  "accept",
					"is_external":  false,
				},
				map[string]interface{}{
					"type":         "resource",
					"attendee_id":  "att_room1",
					"room_id":      "omm_room1",
					"display_name": "会议室 A",
					"rsvp_status":  "accept",
				},
				map[string]interface{}{
					"type":         "chat",
					"attendee_id":  "att_chat1",
					"chat_id":      "oc_chat1",
					"display_name": "风险评审群",
					"rsvp_status":  "needs_action",
				},
				map[string]interface{}{
					"type":              "third_party",
					"attendee_id":       "att_ext1",
					"third_party_email": "guest@example.com",
					"display_name":      "外部访客",
					"rsvp_status":       "needs_action",
					"is_external":       true,
				},
			},
			"has_more":   hasMore,
			"page_token": pageToken,
		},
	}
}

func TestListAttendees_Success_FlatShapeAndTypeFieldsOmitEmpty(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_1/attendees",
		Body:   attendeesResponseFixture(false, ""),
	})

	err := mountAndRun(t, CalendarListAttendees, []string{
		"+list-attendees",
		"--event-id", "evt_1",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Attendees []map[string]interface{} `json:"attendees"`
			HasMore   bool                     `json:"has_more"`
			PageToken string                   `json:"page_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v\nraw=%s", err, stdout.String())
	}
	if !envelope.OK {
		t.Fatalf("envelope.ok=false")
	}
	if got := len(envelope.Data.Attendees); got != 4 {
		t.Fatalf("attendees len=%d, want 4", got)
	}

	// Upstream order preserved: user, resource, chat, third_party.
	wantTypes := []string{"user", "resource", "chat", "third_party"}
	for i, wt := range wantTypes {
		if got, _ := envelope.Data.Attendees[i]["type"].(string); got != wt {
			t.Errorf("attendees[%d].type=%q, want %q", i, got, wt)
		}
	}

	// Type-specific fields only appear where relevant.
	user := envelope.Data.Attendees[0]
	if got, _ := user["user_id"].(string); got != "ou_user1" {
		t.Errorf("user.user_id=%q, want ou_user1", got)
	}
	if _, ok := user["attendee_id"]; ok {
		t.Errorf("attendee_id must not be surfaced in output")
	}
	if _, ok := user["room_id"]; ok {
		t.Errorf("user attendee should not carry room_id")
	}
	if _, ok := user["chat_id"]; ok {
		t.Errorf("user attendee should not carry chat_id")
	}

	resource := envelope.Data.Attendees[1]
	if got, _ := resource["room_id"].(string); got != "omm_room1" {
		t.Errorf("resource.room_id=%q, want omm_room1", got)
	}
	if _, ok := resource["user_id"]; ok {
		t.Errorf("resource attendee should not carry user_id")
	}

	chat := envelope.Data.Attendees[2]
	if got, _ := chat["chat_id"].(string); got != "oc_chat1" {
		t.Errorf("chat.chat_id=%q, want oc_chat1", got)
	}
	if _, ok := chat["rsvp_status"]; ok {
		t.Errorf("chat attendee must not carry rsvp_status (groups have no group-level RSVP)")
	}

	third := envelope.Data.Attendees[3]
	if got, _ := third["third_party_email"].(string); got != "guest@example.com" {
		t.Errorf("third_party.third_party_email=%q, want guest@example.com", got)
	}
	if got, _ := third["is_external"].(bool); !got {
		t.Errorf("third_party.is_external=false, want true")
	}
}

func TestListAttendees_TypeFilter_KeepsMatchingOnly(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_1/attendees",
		Body:   attendeesResponseFixture(false, ""),
	})

	err := mountAndRun(t, CalendarListAttendees, []string{
		"+list-attendees",
		"--event-id", "evt_1",
		"--type", "resource",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var envelope struct {
		Data struct {
			Attendees []map[string]interface{} `json:"attendees"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v\nraw=%s", err, stdout.String())
	}
	if got := len(envelope.Data.Attendees); got != 1 {
		t.Fatalf("attendees len=%d, want 1 (resource only)", got)
	}
	if got, _ := envelope.Data.Attendees[0]["type"].(string); got != "resource" {
		t.Errorf("filtered attendee type=%q, want resource", got)
	}
}

func TestListAttendees_TypeFilter_Repeatable(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_1/attendees",
		Body:   attendeesResponseFixture(false, ""),
	})

	err := mountAndRun(t, CalendarListAttendees, []string{
		"+list-attendees",
		"--event-id", "evt_1",
		"--type", "user",
		"--type", "chat",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var envelope struct {
		Data struct {
			Attendees []map[string]interface{} `json:"attendees"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v\nraw=%s", err, stdout.String())
	}
	if got := len(envelope.Data.Attendees); got != 2 {
		t.Fatalf("attendees len=%d, want 2 (user + chat)", got)
	}
	seen := map[string]bool{}
	for _, a := range envelope.Data.Attendees {
		if t, _ := a["type"].(string); t != "" {
			seen[t] = true
		}
	}
	if !seen["user"] || !seen["chat"] {
		t.Errorf("filtered types=%v, want user and chat", seen)
	}
}

func TestListAttendees_InvalidType_Typed(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarListAttendees, []string{
		"+list-attendees",
		"--event-id", "evt_1",
		"--type", "bogus",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T (%v)", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype=%q, want invalid_argument", ve.Subtype)
	}
	if ve.Param != "--type" {
		t.Errorf("param=%q, want --type", ve.Param)
	}
}

func TestListAttendees_MissingEventID_Typed(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarListAttendees, []string{
		"+list-attendees",
		"--event-id", "",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "event-id") {
		t.Fatalf("expected event-id in error, got: %v", err)
	}
}

func TestListAttendees_PaginationFlagsPassThroughAndSurface(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	var capturedURL string
	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_1/attendees",
		Body:   attendeesResponseFixture(true, "next-page-token-xyz"),
		OnMatch: func(req *http.Request) {
			capturedURL = req.URL.String()
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, CalendarListAttendees, []string{
		"+list-attendees",
		"--event-id", "evt_1",
		"--page-size", "50",
		"--page-token", "prev-token",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedURL == "" {
		t.Fatal("stub did not capture request URL")
	}
	for _, want := range []string{"page_size=50", "page_token=prev-token"} {
		if !strings.Contains(capturedURL, want) {
			t.Errorf("captured URL missing %q, got: %s", want, capturedURL)
		}
	}

	var envelope struct {
		Data struct {
			HasMore   bool   `json:"has_more"`
			PageToken string `json:"page_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v\nraw=%s", err, stdout.String())
	}
	if !envelope.Data.HasMore {
		t.Error("expected has_more=true to be surfaced")
	}
	if envelope.Data.PageToken != "next-page-token-xyz" {
		t.Errorf("page_token=%q, want next-page-token-xyz", envelope.Data.PageToken)
	}
}

func TestListAttendees_PageSizeDefaultsTo20(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	var capturedURL string
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_1/attendees",
		Body:   attendeesResponseFixture(false, ""),
		OnMatch: func(req *http.Request) {
			capturedURL = req.URL.String()
		},
	})

	err := mountAndRun(t, CalendarListAttendees, []string{
		"+list-attendees",
		"--event-id", "evt_1",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(capturedURL, "page_size=20") {
		t.Errorf("default page_size=20 must be sent upstream; captured URL: %s", capturedURL)
	}
}

func TestListAttendees_PageSizeBelowFloor_RaisedAndHinted(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, defaultConfig())
	var capturedURL string
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_1/attendees",
		Body:   attendeesResponseFixture(false, ""),
		OnMatch: func(req *http.Request) {
			capturedURL = req.URL.String()
		},
	})

	err := mountAndRun(t, CalendarListAttendees, []string{
		"+list-attendees",
		"--event-id", "evt_1",
		"--page-size", "3",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(capturedURL, "page_size=10") {
		t.Errorf("expected page-size to be raised to floor 10; captured URL: %s", capturedURL)
	}
	if !strings.Contains(stderr.String(), "below the minimum") {
		t.Errorf("expected stderr hint about page-size floor, got: %s", stderr.String())
	}
}

func TestListAttendees_PageSizeAtFloor_NoHint(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, defaultConfig())
	var capturedURL string
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_1/attendees",
		Body:   attendeesResponseFixture(false, ""),
		OnMatch: func(req *http.Request) {
			capturedURL = req.URL.String()
		},
	})

	err := mountAndRun(t, CalendarListAttendees, []string{
		"+list-attendees",
		"--event-id", "evt_1",
		"--page-size", "10",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(capturedURL, "page_size=10") {
		t.Errorf("expected page-size 10 forwarded verbatim; captured URL: %s", capturedURL)
	}
	if strings.Contains(stderr.String(), "below the minimum") {
		t.Errorf("no floor hint expected when page-size == floor, got: %s", stderr.String())
	}
}

func TestListAttendees_PageSizeAboveCeiling_ClampedAndHinted(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, defaultConfig())
	var capturedURL string
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_1/attendees",
		Body:   attendeesResponseFixture(false, ""),
		OnMatch: func(req *http.Request) {
			capturedURL = req.URL.String()
		},
	})

	err := mountAndRun(t, CalendarListAttendees, []string{
		"+list-attendees",
		"--event-id", "evt_1",
		"--page-size", "500",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(capturedURL, "page_size=100") {
		t.Errorf("expected page-size clamped to ceiling 100; captured URL: %s", capturedURL)
	}
	if !strings.Contains(stderr.String(), "exceeds the maximum") {
		t.Errorf("expected stderr hint about page-size ceiling, got: %s", stderr.String())
	}
}

func TestListAttendees_PageSizeAtCeiling_NoHint(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, defaultConfig())
	var capturedURL string
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_1/attendees",
		Body:   attendeesResponseFixture(false, ""),
		OnMatch: func(req *http.Request) {
			capturedURL = req.URL.String()
		},
	})

	err := mountAndRun(t, CalendarListAttendees, []string{
		"+list-attendees",
		"--event-id", "evt_1",
		"--page-size", "100",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(capturedURL, "page_size=100") {
		t.Errorf("expected page-size 100 forwarded verbatim; captured URL: %s", capturedURL)
	}
	if strings.Contains(stderr.String(), "exceeds the maximum") {
		t.Errorf("no ceiling hint expected when page-size == ceiling, got: %s", stderr.String())
	}
}

func TestListAttendees_DryRun_TargetsAttendeesEndpoint(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, CalendarListAttendees, []string{
		"+list-attendees",
		"--event-id", "evt_dry",
		"--calendar-id", "cal_dry",
		"--type", "resource",
		"--page-size", "20",
		"--page-token", "tok",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		`"method": "GET"`,
		`/calendars/cal_dry/events/evt_dry/attendees`,
		`"page_size": 20`,
		`"page_token": "tok"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q, got: %s", want, out)
		}
	}
}

func TestListAttendees_DryRun_PrimaryCalendarDefault(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, CalendarListAttendees, []string{
		"+list-attendees",
		"--event-id", "evt_dry",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/calendars/%3Cprimary%3E/events/evt_dry/attendees") &&
		!strings.Contains(out, "/calendars/<primary>/events/evt_dry/attendees") {
		t.Errorf("dry-run should target primary calendar placeholder, got: %s", out)
	}
}

func TestListAttendees_APIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_1/attendees",
		Body:   map[string]interface{}{"code": 190001, "msg": "permission denied"},
	})

	err := mountAndRun(t, CalendarListAttendees, []string{
		"+list-attendees",
		"--event-id", "evt_1",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T (%v)", err, err)
	}
	if ae.Code != 190001 {
		t.Errorf("code=%d, want 190001", ae.Code)
	}
}

func TestListAttendees_EmptyItems_PrettyBanner(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_1/attendees",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{},
			},
		},
	})

	err := mountAndRun(t, CalendarListAttendees, []string{
		"+list-attendees",
		"--event-id", "evt_1",
		"--format", "pretty",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("No attendees found")) {
		t.Errorf("expected empty banner, got: %s", stdout.String())
	}
}
