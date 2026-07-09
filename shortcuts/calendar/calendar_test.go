// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// codeInvalidParamsWithDetail is the Lark "invalid params" code (190014) used
// across the API-error fixtures below. It mirrors the value registered in
// internal/errclass/codemeta_calendar.go.
const codeInvalidParamsWithDetail = 190014

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// warmOnce ensures the Lark SDK's internal token cache is populated exactly
// once per test binary.  The SDK caches tenant tokens by app credentials, so
// only the very first API call in the process actually hits the token endpoint.
var warmOnce sync.Once

func warmTokenCache(t *testing.T) {
	t.Helper()
	warmOnce.Do(func() {
		f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
		reg.Register(&httpmock.Stub{
			URL:  "/open-apis/test/v1/warm",
			Body: map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
		})
		s := common.Shortcut{
			Service:   "test",
			Command:   "+warm",
			AuthTypes: []string{"bot"},
			Execute: func(_ context.Context, rctx *common.RuntimeContext) error {
				_, err := rctx.CallAPITyped("GET", "/open-apis/test/v1/warm", nil, nil)
				return err
			},
		}
		parent := &cobra.Command{Use: "test"}
		s.Mount(parent, f)
		parent.SetArgs([]string{"+warm"})
		parent.SilenceErrors = true
		parent.SilenceUsage = true
		parent.Execute()
	})
}

func mountAndRun(t *testing.T, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	warmTokenCache(t)
	parent := &cobra.Command{Use: "test"}
	s.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func defaultConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
		UserOpenId: "ou_testuser",
	}
}

func noLoginConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
}

func noLoginBotDefaultConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
		DefaultAs: "bot",
	}
}

// ---------------------------------------------------------------------------
// CalendarCreate tests
// ---------------------------------------------------------------------------

func TestCreate_CreateEventOnly(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"event": map[string]interface{}{
					"event_id": "evt_001",
					"summary":  "Test Meeting",
					"start_time": map[string]interface{}{
						"timestamp": "1742515200",
					},
					"end_time": map[string]interface{}{
						"timestamp": "1742518800",
					},
				},
			},
		},
	})

	err := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Test Meeting",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-03-21T01:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "evt_001") {
		t.Errorf("stdout should contain event_id, got: %s", stdout.String())
	}
}

func TestCreate_CreateEventOnly_PrettyFormat(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"event": map[string]interface{}{
					"event_id": "evt_001",
					"summary":  "Test Meeting",
					"start_time": map[string]interface{}{
						"timestamp": "1742515200",
					},
					"end_time": map[string]interface{}{
						"timestamp": "1742518800",
					},
				},
			},
		},
	})

	err := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Test Meeting",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-03-21T01:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--as", "bot",
		"--format", "pretty",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "evt_001") {
		t.Errorf("stdout should contain event_id, got: %s", out)
	}
	if !strings.Contains(out, "Event created successfully") {
		t.Errorf("stdout should contain success message, got: %s", out)
	}
}

func TestBuildEventData_DefaultVChat(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("summary", "", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("rrule", "", "")
	cmd.Flags().Set("summary", "Team Sync")
	cmd.Flags().Set("description", "Weekly meeting")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	eventData := buildEventData(runtime, "1742515200", "1742518800")

	vchat, ok := eventData["vchat"].(map[string]string)
	if !ok {
		t.Fatalf("vchat = %T, want map[string]string", eventData["vchat"])
	}
	if got := vchat["vc_type"]; got != "vc" {
		t.Fatalf("vchat.vc_type = %q, want %q", got, "vc")
	}
}

func TestCreate_WithAttendees_Success(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"event": map[string]interface{}{
					"event_id": "evt_002",
					"summary":  "Team Sync",
					"start_time": map[string]interface{}{
						"timestamp": "1742515200",
					},
					"end_time": map[string]interface{}{
						"timestamp": "1742518800",
					},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/events/evt_002/attendees",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Team Sync",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-03-21T01:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--attendee-ids", "ou_user1,ou_user2,oc_group1",
		"--as", "bot",
	}, f, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreate_WithAttendees_APIError_RollsBack(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"event": map[string]interface{}{
					"event_id": "evt_003",
					"summary":  "Bad Attendees",
					"start_time": map[string]interface{}{
						"timestamp": "1742515200",
					},
					"end_time": map[string]interface{}{
						"timestamp": "1742518800",
					},
				},
			},
		},
	})
	// Attendees API returns business error
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/events/evt_003/attendees",
		Body: map[string]interface{}{
			"code": 190002,
			"msg":  "invalid user_id",
		},
	})
	// Rollback: delete the event
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/events/evt_003",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})

	err := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Bad Attendees",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-03-21T01:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--attendee-ids", "ou_invalid",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error for invalid attendees, got nil")
	}
	// Enrich-in-place: classification of the add-attendees failure is preserved
	// (APIError / code 190002) and the rollback context rides on the Hint.
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T", err)
	}
	if ae.Code != 190002 {
		t.Errorf("expected preserved code 190002, got %d", ae.Code)
	}
	if !strings.Contains(ae.Hint, "rolled back successfully") {
		t.Fatalf("hint should mention rollback, got: %q", ae.Hint)
	}
}

func TestCreate_CreateEvent_APIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events",
		Body: map[string]interface{}{
			"code": 190001,
			"msg":  "permission denied",
		},
	})

	err := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Denied",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-03-21T01:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

func TestCreate_EndBeforeStart(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Invalid",
		"--start", "2025-03-21T10:00:00+08:00",
		"--end", "2025-03-21T09:00:00+08:00",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected validation error for end < start, got nil")
	}
	if !strings.Contains(err.Error(), "end time must be after start time") {
		t.Errorf("error should mention end/start, got: %v", err)
	}
}

func TestCreate_ExplicitCalendarId(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_explicit/events",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"event": map[string]interface{}{
					"event_id":   "evt_004",
					"summary":    "Explicit Cal",
					"start_time": map[string]interface{}{"timestamp": "1742515200"},
					"end_time":   map[string]interface{}{"timestamp": "1742518800"},
				},
			},
		},
	})

	err := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Explicit Cal",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-03-21T01:00:00+08:00",
		"--calendar-id", "cal_explicit",
		"--as", "bot",
	}, f, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreate_NoEventIdReturned(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"event": map[string]interface{}{},
			},
		},
	})

	err := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "No ID",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-03-21T01:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error when no event_id returned, got nil")
	}
}

func TestCreate_CreateEvent_InvalidParamsWithDetail(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events",
		Body: map[string]interface{}{
			"code": codeInvalidParamsWithDetail,
			"msg":  "invalid params",
			"error": map[string]interface{}{
				"details": []interface{}{
					map[string]interface{}{"value": "end_time should be later than start_time"},
				},
			},
		},
	})

	err := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Bad Time",
		"--start", "2025-03-21T10:00:00+08:00",
		"--end", "2025-03-21T11:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error for 190014, got nil")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T", err)
	}
	if ae.Subtype != errs.SubtypeInvalidParameters {
		t.Errorf("subtype=%q, want invalid_parameters", ae.Subtype)
	}
	if ae.Code != codeInvalidParamsWithDetail {
		t.Errorf("expected code %d, got %d", codeInvalidParamsWithDetail, ae.Code)
	}
	if !strings.Contains(ae.Hint, "end_time should be later than start_time") {
		t.Errorf("expected detail value in hint, got %q", ae.Hint)
	}
}

func TestCreate_CreateEvent_InvalidParamsWithoutDetailValue(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events",
		Body: map[string]interface{}{
			"code": codeInvalidParamsWithDetail,
			"msg":  "invalid params",
		},
	})

	err := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Bad Time",
		"--start", "2025-03-21T10:00:00+08:00",
		"--end", "2025-03-21T11:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error for 190014, got nil")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T", err)
	}
	if ae.Subtype != errs.SubtypeInvalidParameters {
		t.Errorf("subtype=%q, want invalid_parameters", ae.Subtype)
	}
	if ae.Code != codeInvalidParamsWithDetail {
		t.Errorf("expected code %d, got %d", codeInvalidParamsWithDetail, ae.Code)
	}
}

func TestCreate_CreateEvent_InvalidParams_ErrorNotMap(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method:      "POST",
		URL:         "/open-apis/calendar/v4/calendars/cal_test123/events",
		RawBody:     []byte(`{"code":190014,"msg":"invalid params","error":"just a string"}`),
		ContentType: "text/plain",
	})

	err := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Bad Time",
		"--start", "2025-03-21T10:00:00+08:00",
		"--end", "2025-03-21T11:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error for 190014, got nil")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T", err)
	}
	if ae.Code != codeInvalidParamsWithDetail {
		t.Errorf("expected code %d, got %d", codeInvalidParamsWithDetail, ae.Code)
	}
}

func TestCreate_CreateEvent_InvalidParams_NoDetailsKey(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events",
		Body: map[string]interface{}{
			"code": codeInvalidParamsWithDetail,
			"msg":  "invalid params",
			"error": map[string]interface{}{
				"other_key": "no details here",
			},
		},
	})

	err := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Bad Time",
		"--start", "2025-03-21T10:00:00+08:00",
		"--end", "2025-03-21T11:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error for 190014, got nil")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T", err)
	}
	if ae.Code != codeInvalidParamsWithDetail {
		t.Errorf("expected code %d, got %d", codeInvalidParamsWithDetail, ae.Code)
	}
}

func TestCreate_CreateEvent_InvalidParams_DetailItemNotMap(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events",
		Body: map[string]interface{}{
			"code": codeInvalidParamsWithDetail,
			"msg":  "invalid params",
			"error": map[string]interface{}{
				"details": []interface{}{nil},
			},
		},
	})

	err := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Bad Time",
		"--start", "2025-03-21T10:00:00+08:00",
		"--end", "2025-03-21T11:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error for 190014, got nil")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T", err)
	}
	if ae.Code != codeInvalidParamsWithDetail {
		t.Errorf("expected code %d, got %d", codeInvalidParamsWithDetail, ae.Code)
	}
}

func TestCreate_WithAttendees_InvalidParamsWithDetail_RollsBack(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"event": map[string]interface{}{
					"event_id":   "evt_190014",
					"summary":    "Bad Attendees",
					"start_time": map[string]interface{}{"timestamp": "1742515200"},
					"end_time":   map[string]interface{}{"timestamp": "1742518800"},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/events/evt_190014/attendees",
		Body: map[string]interface{}{
			"code": codeInvalidParamsWithDetail,
			"msg":  "invalid params",
			"error": map[string]interface{}{
				"details": []interface{}{
					map[string]interface{}{"value": "invalid attendee open_id"},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/events/evt_190014",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})

	err := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Bad Attendees",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-03-21T01:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--attendee-ids", "ou_invalid",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error for invalid attendees with 190014, got nil")
	}
	// Enrich-in-place: the underlying typed add-attendees failure is returned
	// unchanged except that the rollback context is appended to its Hint. Its
	// classification (APIError / code 190014) and the lifted server detail are
	// preserved.
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T", err)
	}
	if ae.Code != codeInvalidParamsWithDetail {
		t.Errorf("expected preserved code %d, got %d", codeInvalidParamsWithDetail, ae.Code)
	}
	if !strings.Contains(ae.Hint, "invalid attendee open_id") {
		t.Errorf("expected lifted server detail preserved in hint, got: %q", ae.Hint)
	}
	if !strings.Contains(ae.Hint, "rolled back successfully") {
		t.Errorf("expected rollback context appended to hint, got: %q", ae.Hint)
	}
}

func TestCreate_ApprovalRoomMissingReason_GuidesRawAttendeesAPI(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"event": map[string]interface{}{
					"event_id":   "evt_approval_room",
					"summary":    "Approval Room",
					"start_time": map[string]interface{}{"timestamp": "1742515200"},
					"end_time":   map[string]interface{}{"timestamp": "1742518800"},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/events/evt_approval_room/attendees",
		Body: map[string]interface{}{
			"code": codeInvalidParamsWithDetail,
			"msg":  "invalid params",
			"error": map[string]interface{}{
				"details": []interface{}{
					map[string]interface{}{"value": "attendees[0].approval_reason is required for approval meeting rooms"},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/events/evt_approval_room",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})

	err := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Approval Room",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-03-21T01:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--attendee-ids", "omm_room1",
		"--as", "user",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error for approval room missing approval_reason, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf returned !ok for %T", err)
	}
	if p.Category != errs.CategoryAPI {
		t.Errorf("category=%q, want %q", p.Category, errs.CategoryAPI)
	}
	if p.Subtype != errs.SubtypeInvalidParameters {
		t.Errorf("subtype=%q, want %q", p.Subtype, errs.SubtypeInvalidParameters)
	}
	if p.Code != codeInvalidParamsWithDetail {
		t.Errorf("code=%d, want %d", p.Code, codeInvalidParamsWithDetail)
	}
	for _, want := range []string{"approval_reason", "calendar event.attendees create", "--as user", "rolled back successfully"} {
		if !strings.Contains(p.Hint, want) {
			t.Errorf("hint should contain %q, got: %q", want, p.Hint)
		}
	}
}

// When the add-attendees call fails AND the rollback DELETE also fails, the
// primary error stays the add failure (classification preserved) and the Hint
// must surface BOTH the rollback failure reason and the orphan event_id so the
// user can clean up manually.
func TestCreate_WithAttendees_RollbackAlsoFails(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"event": map[string]interface{}{
					"event_id":   "evt_orphan",
					"summary":    "Bad Attendees",
					"start_time": map[string]interface{}{"timestamp": "1742515200"},
					"end_time":   map[string]interface{}{"timestamp": "1742518800"},
				},
			},
		},
	})
	// Add-attendees fails with a business code.
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/events/evt_orphan/attendees",
		Body:   map[string]interface{}{"code": 190002, "msg": "invalid user_id"},
	})
	// Rollback DELETE also fails with a distinct business code.
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/events/evt_orphan",
		Body:   map[string]interface{}{"code": 230098, "msg": "delete blocked"},
	})

	err := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Bad Attendees",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-03-21T01:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--attendee-ids", "ou_invalid",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error when both add and rollback fail, got nil")
	}
	// Primary error is the add failure: classification preserved (code 190002).
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T", err)
	}
	if ae.Code != 190002 {
		t.Errorf("expected preserved add-failure code 190002, got %d", ae.Code)
	}
	// The Hint must surface the rollback failure (its signal) and the orphan id.
	if !strings.Contains(ae.Hint, "rollback also failed") {
		t.Errorf("expected rollback-failure context in hint, got: %q", ae.Hint)
	}
	if !strings.Contains(ae.Hint, "delete blocked") {
		t.Errorf("expected rollbackErr signal in hint, got: %q", ae.Hint)
	}
	if !strings.Contains(ae.Hint, "orphan event_id=evt_orphan") {
		t.Errorf("expected orphan event_id in hint, got: %q", ae.Hint)
	}
}

// ---------------------------------------------------------------------------
// CalendarUpdate tests
// ---------------------------------------------------------------------------

func TestUpdate_PatchEventOnly(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events/evt_update1",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"event": map[string]interface{}{
					"event_id": "evt_update1",
					"summary":  "Updated Meeting",
					"start_time": map[string]interface{}{
						"timestamp": "1742518800",
					},
					"end_time": map[string]interface{}{
						"timestamp": "1742522400",
					},
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, CalendarUpdate, []string{
		"+update",
		"--event-id", "evt_update1",
		"--calendar-id", "cal_test123",
		"--summary", "Updated Meeting",
		"--description", "Updated description",
		"--start", "2025-03-21T01:00:00+08:00",
		"--end", "2025-03-21T02:00:00+08:00",
		"--notify=false",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured patch body: %v", err)
	}
	if body["summary"] != "Updated Meeting" || body["description"] != "Updated description" {
		t.Fatalf("unexpected patch body: %#v", body)
	}
	if body["need_notification"] != false {
		t.Fatalf("need_notification = %#v, want false", body["need_notification"])
	}
	if !strings.Contains(stdout.String(), "evt_update1") {
		t.Fatalf("stdout should contain event id, got: %s", stdout.String())
	}
}

func TestUpdate_AddAttendees(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events/evt_update2/attendees",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	}
	reg.Register(stub)

	err := mountAndRun(t, CalendarUpdate, []string{
		"+update",
		"--event-id", "evt_update2",
		"--calendar-id", "cal_test123",
		"--add-attendee-ids", "ou_user1,oc_group1,omm_room1",
		"--as", "bot",
	}, f, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := decodeCalendarCapturedBody(t, stub)
	attendees, _ := body["attendees"].([]interface{})
	if !calendarBodyHasAttendee(attendees, "user", "user_id", "ou_user1") ||
		!calendarBodyHasAttendee(attendees, "chat", "chat_id", "oc_group1") ||
		!calendarBodyHasAttendee(attendees, "resource", "room_id", "omm_room1") {
		t.Fatalf("unexpected add attendees body: %#v", body)
	}
}

func TestUpdate_RemoveAttendees(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events/evt_update3/attendees/batch_delete",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	}
	reg.Register(stub)

	err := mountAndRun(t, CalendarUpdate, []string{
		"+update",
		"--event-id", "evt_update3",
		"--calendar-id", "cal_test123",
		"--remove-attendee-ids", "ou_user1,oc_group1,omm_room1",
		"--notify=false",
		"--as", "bot",
	}, f, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := decodeCalendarCapturedBody(t, stub)
	deleteIDs, _ := body["delete_ids"].([]interface{})
	if body["need_notification"] != false {
		t.Fatalf("need_notification = %#v, want false", body["need_notification"])
	}
	if !calendarBodyHasAttendee(deleteIDs, "user", "user_id", "ou_user1") ||
		!calendarBodyHasAttendee(deleteIDs, "chat", "chat_id", "oc_group1") ||
		!calendarBodyHasAttendee(deleteIDs, "resource", "room_id", "omm_room1") {
		t.Fatalf("unexpected remove attendees body: %#v", body)
	}
}

func TestUpdate_CombinedPatchRemoveAdd(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	patchStub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/events/evt_update4",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{"event_id": "evt_update4", "summary": "Combined"}},
		},
	}
	removeStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/events/evt_update4/attendees/batch_delete",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	}
	addStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/events/evt_update4/attendees",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	}
	reg.Register(patchStub)
	reg.Register(removeStub)
	reg.Register(addStub)

	err := mountAndRun(t, CalendarUpdate, []string{
		"+update",
		"--event-id", "evt_update4",
		"--summary", "Combined",
		"--remove-attendee-ids", "ou_old",
		"--add-attendee-ids", "ou_new",
		"--as", "bot",
	}, f, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(patchStub.CapturedBody) == 0 || len(removeStub.CapturedBody) == 0 || len(addStub.CapturedBody) == 0 {
		t.Fatalf("expected patch, remove, and add requests to be captured")
	}
}

func TestUpdate_DryRun_MultiStep(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, CalendarUpdate, []string{
		"+update",
		"--event-id", "evt_dry",
		"--calendar-id", "cal_test123",
		"--summary", "Dry",
		"--remove-attendee-ids", "omm_oldroom",
		"--add-attendee-ids", "ou_new,omm_newroom",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"PATCH", "batch_delete", "attendees", "omm_oldroom", "omm_newroom"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run should contain %q, got: %s", want, out)
		}
	}
}

func TestUpdate_Validation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no fields",
			args: []string{"+update", "--event-id", "evt_1", "--as", "bot"},
			want: "nothing to update",
		},
		{
			name: "invalid attendee",
			args: []string{"+update", "--event-id", "evt_1", "--add-attendee-ids", "bad", "--as", "bot"},
			want: "invalid attendee id format",
		},
		{
			name: "duplicate add remove",
			args: []string{"+update", "--event-id", "evt_1", "--add-attendee-ids", "ou_same", "--remove-attendee-ids", "ou_same", "--as", "bot"},
			want: "appears in both",
		},
		{
			name: "start without end",
			args: []string{"+update", "--event-id", "evt_1", "--start", "2025-03-21T00:00:00+08:00", "--as", "bot"},
			want: "must be specified together",
		},
		{
			name: "end before start",
			args: []string{"+update", "--event-id", "evt_1", "--start", "2025-03-21T10:00:00+08:00", "--end", "2025-03-21T09:00:00+08:00", "--as", "bot"},
			want: "end time must be after start time",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
			err := mountAndRun(t, CalendarUpdate, tc.args, f, nil)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func decodeCalendarCapturedBody(t *testing.T, stub *httpmock.Stub) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v\nraw=%s", err, string(stub.CapturedBody))
	}
	return body
}

func calendarBodyHasAttendee(items []interface{}, typ, key, value string) bool {
	for _, item := range items {
		m, _ := item.(map[string]interface{})
		if m["type"] == typ && m[key] == value {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// CalendarAgenda tests
// ---------------------------------------------------------------------------

func TestCalendarShortcuts_RequireLoginUnlessExplicitBot(t *testing.T) {
	cases := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
	}{
		{
			name:     "agenda",
			shortcut: CalendarAgenda,
			args:     []string{"+agenda", "--start", "2025-03-21", "--end", "2025-03-21"},
		},
		{
			name:     "create",
			shortcut: CalendarCreate,
			args:     []string{"+create", "--summary", "Test Meeting", "--start", "2025-03-21T00:00:00+08:00", "--end", "2025-03-21T01:00:00+08:00"},
		},
		{
			name:     "update",
			shortcut: CalendarUpdate,
			args:     []string{"+update", "--event-id", "evt_1", "--summary", "Updated"},
		},
		{
			name:     "freebusy",
			shortcut: CalendarFreebusy,
			args:     []string{"+freebusy", "--start", "2025-03-21", "--end", "2025-03-21"},
		},
		{
			name:     "room-find",
			shortcut: CalendarRoomFind,
			args:     []string{"+room-find", "--slot", "2025-03-21T00:00:00+08:00~2025-03-21T01:00:00+08:00"},
		},
		{
			name:     "rsvp",
			shortcut: CalendarRsvp,
			args:     []string{"+rsvp", "--event-id", "evt_rsvp1", "--rsvp-status", "accept"},
		},
		{
			name:     "suggestion",
			shortcut: CalendarSuggestion,
			args:     []string{"+suggestion", "--start", "2025-03-21", "--end", "2025-03-21"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, noLoginConfig())

			err := mountAndRun(t, tc.shortcut, tc.args, f, nil)
			if err == nil {
				t.Fatal("expected auth guard error")
			}
			if !strings.Contains(err.Error(), "auth login") {
				t.Fatalf("expected auth login guidance, got: %v", err)
			}
			if !strings.Contains(err.Error(), "--as bot") {
				t.Fatalf("expected explicit bot guidance, got: %v", err)
			}
		})
	}
}

func TestAgenda_ExplicitBotBypassesLoginGuard(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, noLoginConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/events/instance_view",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{},
			},
		},
	})

	err := mountAndRun(t, CalendarAgenda, []string{
		"+agenda",
		"--start", "2025-03-21",
		"--end", "2025-03-21",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgenda_DefaultAsBotBypassesLoginGuard(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, noLoginBotDefaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/events/instance_view",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{},
			},
		},
	})

	err := mountAndRun(t, CalendarAgenda, []string{
		"+agenda",
		"--start", "2025-03-21",
		"--end", "2025-03-21",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgenda_Success(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/events/instance_view",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"event_id": "evt_a1",
						"summary":  "Morning standup",
						"status":   "confirmed",
						"start_time": map[string]interface{}{
							"timestamp": "1742515200",
						},
						"end_time": map[string]interface{}{
							"timestamp": "1742518800",
						},
					},
					map[string]interface{}{
						"event_id": "evt_a2",
						"summary":  "All Day Event",
						"status":   "confirmed",
						"start_time": map[string]interface{}{
							"date": "2025-03-21",
						},
						"end_time": map[string]interface{}{
							"date": "2025-03-21",
						},
					},
				},
			},
		},
	})

	err := mountAndRun(t, CalendarAgenda, []string{
		"+agenda",
		"--start", "2025-03-21",
		"--end", "2025-03-21",
		"--format", "prettry",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "evt_a1") {
		t.Errorf("stdout should contain event_id, got: %s", stdout.String())
	}
}

func TestAgenda_EmptyResult(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/events/instance_view",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{},
			},
		},
	})

	err := mountAndRun(t, CalendarAgenda, []string{
		"+agenda",
		"--start", "2025-03-21",
		"--end", "2025-03-21",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var envelope map[string]interface{}
	if json.Unmarshal(stdout.Bytes(), &envelope) == nil {
		if data, ok := envelope["data"].([]interface{}); ok && len(data) != 0 {
			t.Errorf("expected empty data array, got %d items", len(data))
		}
	}
}

func TestAgenda_FiltersCancelledEvents(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/events/instance_view",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"event_id":   "evt_confirmed",
						"summary":    "Active Event",
						"status":     "confirmed",
						"start_time": map[string]interface{}{"timestamp": "1742515200"},
						"end_time":   map[string]interface{}{"timestamp": "1742518800"},
					},
					map[string]interface{}{
						"event_id":   "evt_cancelled",
						"summary":    "Cancelled Event",
						"status":     "cancelled",
						"start_time": map[string]interface{}{"timestamp": "1742519000"},
						"end_time":   map[string]interface{}{"timestamp": "1742522600"},
					},
				},
			},
		},
	})

	err := mountAndRun(t, CalendarAgenda, []string{
		"+agenda",
		"--start", "2025-03-21",
		"--end", "2025-03-21",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "evt_confirmed") {
		t.Errorf("stdout should contain confirmed event, got: %s", out)
	}
	if strings.Contains(out, "evt_cancelled") {
		t.Errorf("stdout should not contain cancelled event, got: %s", out)
	}
}

func TestAgenda_ExplicitCalendarId(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_my/events/instance_view",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{},
			},
		},
	})

	err := mountAndRun(t, CalendarAgenda, []string{
		"+agenda",
		"--start", "2025-03-21",
		"--end", "2025-03-21",
		"--calendar-id", "cal_my",
		"--as", "bot",
	}, f, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgenda_InvalidParamsWithDetail(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/events/instance_view",
		Body: map[string]interface{}{
			"code": codeInvalidParamsWithDetail,
			"msg":  "invalid params",
			"error": map[string]interface{}{
				"details": []interface{}{
					map[string]interface{}{"value": "start_time is required"},
				},
			},
		},
	})

	err := mountAndRun(t, CalendarAgenda, []string{
		"+agenda",
		"--start", "2025-03-21",
		"--end", "2025-03-21",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error for 190014, got nil")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T", err)
	}
	if ae.Subtype != errs.SubtypeInvalidParameters {
		t.Errorf("subtype=%q, want invalid_parameters", ae.Subtype)
	}
	if ae.Code != codeInvalidParamsWithDetail {
		t.Errorf("expected code %d, got %d", codeInvalidParamsWithDetail, ae.Code)
	}
	if !strings.Contains(ae.Hint, "start_time is required") {
		t.Errorf("expected detail value in hint, got %q", ae.Hint)
	}
}

func TestAgenda_NonAPIError_Passthrough(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/events/instance_view",
		RawBody: []byte("this is not json"),
	})

	err := mountAndRun(t, CalendarAgenda, []string{
		"+agenda",
		"--start", "2025-03-21",
		"--end", "2025-03-21",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error for non-JSON response, got nil")
	}
	// A non-JSON 200 body is not an API business error: it surfaces as a typed
	// InternalError{SubtypeInvalidResponse} from WrapJSONResponseParseError.
	var ie *errs.InternalError
	if !errors.As(err, &ie) {
		t.Fatalf("expected *errs.InternalError, got %T", err)
	}
	if ie.Subtype != errs.SubtypeInvalidResponse {
		t.Errorf("subtype=%q, want invalid_response", ie.Subtype)
	}
}

// TestAgenda_TimeRangeExceeded_RecursiveSplit pins that a 193103 ("time range
// exceeds 40-day limit") response from CallAPITyped is caught, the range is
// split, and the successful sub-range results are aggregated. The stubs are
// consumed in registration order: full range → 193103, then the two halves
// succeed.
func TestAgenda_TimeRangeExceeded_RecursiveSplit(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	// Full range rejected with the time-range-exceeded code.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/events/instance_view",
		Body:   map[string]interface{}{"code": 193103, "msg": "time range exceeds limit"},
	})
	// Left half succeeds with one event.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/events/instance_view",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"event_id":   "evt_left",
						"summary":    "Left",
						"status":     "confirmed",
						"start_time": map[string]interface{}{"timestamp": "1742515200"},
						"end_time":   map[string]interface{}{"timestamp": "1742518800"},
					},
				},
			},
		},
	})
	// Right half succeeds with one event.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/events/instance_view",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"event_id":   "evt_right",
						"summary":    "Right",
						"status":     "confirmed",
						"start_time": map[string]interface{}{"timestamp": "1742519000"},
						"end_time":   map[string]interface{}{"timestamp": "1742522600"},
					},
				},
			},
		},
	})

	err := mountAndRun(t, CalendarAgenda, []string{
		"+agenda",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-03-21T06:00:00+08:00",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "evt_left") || !strings.Contains(out, "evt_right") {
		t.Errorf("expected aggregated split results, got: %s", out)
	}
}

// TestAgenda_TooManyInstances_SplitExhausted pins that when the range is already
// at or below the minimum split window and the server still returns 193104, the
// recursion stops and surfaces a typed APIError carrying code 193104 (exit 1).
func TestAgenda_TooManyInstances_SplitExhausted(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method:   "GET",
		URL:      "/events/instance_view",
		Reusable: true,
		Body:     map[string]interface{}{"code": 193104, "msg": "too many instances"},
	})

	// A 1-hour span is below minSplitWindowSeconds (2h), so the 193104 branch
	// cannot split further and must surface the typed error.
	err := mountAndRun(t, CalendarAgenda, []string{
		"+agenda",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-03-21T01:00:00+08:00",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error when split is exhausted, got nil")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T", err)
	}
	if ae.Code != 193104 {
		t.Errorf("code=%d, want 193104", ae.Code)
	}
	if output.ExitCodeOf(err) != output.ExitAPI {
		t.Errorf("exit=%d, want ExitAPI", output.ExitCodeOf(err))
	}
	if !strings.Contains(ae.Error(), "narrow the range") {
		t.Errorf("expected narrow-the-range guidance, got: %q", ae.Error())
	}
}

// ---------------------------------------------------------------------------
// CalendarFreebusy tests
// ---------------------------------------------------------------------------

func TestFreebusy_Success(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/list",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"freebusy_list": []interface{}{
					map[string]interface{}{
						"start_time": "2025-03-21T10:00:00+08:00",
						"end_time":   "2025-03-21T11:00:00+08:00",
					},
				},
			},
		},
	})

	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy",
		"--start", "2025-03-21",
		"--end", "2025-03-21",
		"--user-id", "ou_someone",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "start_time") {
		t.Errorf("stdout should contain freebusy data, got: %s", stdout.String())
	}
}

func TestFreebusy_BotWithoutUser_Fails(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy",
		"--start", "2025-03-21",
		"--end", "2025-03-21",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected validation error for bot without --user-id, got nil")
	}
	if !strings.Contains(err.Error(), "--user-id is required") {
		t.Errorf("error should mention --user-id requirement, got: %v", err)
	}
}

func TestFreebusy_APIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/list",
		Body: map[string]interface{}{
			"code": 190001,
			"msg":  "permission denied",
		},
	})

	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy",
		"--start", "2025-03-21",
		"--end", "2025-03-21",
		"--user-id", "ou_someone",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

func TestFreebusy_InvalidParamsWithDetail(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/list",
		Body: map[string]interface{}{
			"code": codeInvalidParamsWithDetail,
			"msg":  "invalid params",
			"error": map[string]interface{}{
				"details": []interface{}{
					map[string]interface{}{"value": "user_id is invalid"},
				},
			},
		},
	})

	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy",
		"--start", "2025-03-21",
		"--end", "2025-03-21",
		"--user-id", "ou_someone",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error for 190014, got nil")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T", err)
	}
	if ae.Subtype != errs.SubtypeInvalidParameters {
		t.Errorf("subtype=%q, want invalid_parameters", ae.Subtype)
	}
	if ae.Code != codeInvalidParamsWithDetail {
		t.Errorf("expected code %d, got %d", codeInvalidParamsWithDetail, ae.Code)
	}
	if !strings.Contains(ae.Hint, "user_id is invalid") {
		t.Errorf("expected detail value in hint, got %q", ae.Hint)
	}
}

// ---------------------------------------------------------------------------
// CalendarSuggestion tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// CalendarRsvp tests
// ---------------------------------------------------------------------------

func TestRsvp_Success(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_rsvp1/reply",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
		},
	})

	err := mountAndRun(t, CalendarRsvp, []string{
		"+rsvp",
		"--event-id", "evt_rsvp1",
		"--rsvp-status", "accept",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{`"event_id": "evt_rsvp1"`, `"rsvp_status": "accept"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout should contain %s, got: %s", want, stdout.String())
		}
	}
}

func TestRsvp_InvalidStatus(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, CalendarRsvp, []string{
		"+rsvp",
		"--event-id", "evt_rsvp1",
		"--rsvp-status", "invalid_status",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected validation error for invalid status, got nil")
	}
	if !strings.Contains(err.Error(), "invalid value") {
		t.Errorf("error should mention invalid value, got: %v", err)
	}
}

func TestRsvp_APIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/evt_rsvp1/reply",
		Body: map[string]interface{}{
			"code": 190001,
			"msg":  "permission denied",
		},
	})

	err := mountAndRun(t, CalendarRsvp, []string{
		"+rsvp",
		"--event-id", "evt_rsvp1",
		"--rsvp-status", "decline",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

func TestRsvp_RejectsDangerousChars(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, CalendarRsvp, []string{
		"+rsvp",
		"--event-id", "evt_rsvp1\u202e",
		"--rsvp-status", "accept",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected validation error for dangerous characters, got nil")
	}
	if !strings.Contains(err.Error(), "dangerous Unicode") && !strings.Contains(err.Error(), "control character") {
		t.Errorf("error should mention dangerous input, got: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--event-id" {
		t.Errorf("param=%q, want --event-id", ve.Param)
	}
}

func TestRsvp_DryRun_TrimmedPrimaryCalendar(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, CalendarRsvp, []string{
		"+rsvp",
		"--calendar-id", " primary ",
		"--event-id", "evt_rsvp1",
		"--rsvp-status", "accept",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"calendar_id": "\u003cprimary\u003e"`) {
		t.Errorf("dry-run should normalize primary calendar, got: %s", stdout.String())
	}
}

func TestSuggestion_Success(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/suggestion",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"suggestions": []interface{}{
					map[string]interface{}{
						"event_start_time": "2025-03-21T10:00:00+08:00",
						"event_end_time":   "2025-03-21T11:00:00+08:00",
						"recommend_reason": "everyone is free",
					},
				},
				"ai_action_guidance": "book it",
			},
		},
	})

	// 正常执行
	err := mountAndRun(t, CalendarSuggestion, []string{
		"+suggestion",
		"--start", "2025-03-21",
		"--end", "2025-03-21",
		"--attendee-ids", "ou_user1,oc_chat1",
		"--event-rrule", "FREQ=DAILY;BYDAY=MO",
		"--duration-minutes", "60",
		"--timezone", "Asia/Shanghai",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "2025-03-21T10:00:00+08:00") {
		t.Errorf("stdout should contain start time, got: %s", out)
	}
	if !strings.Contains(out, "everyone is free") {
		t.Errorf("stdout should contain reason, got: %s", out)
	}
	if !strings.Contains(out, `"ai_action_guidance": "book it"`) {
		t.Errorf("stdout should contain guidance, got: %s", out)
	}
}

func TestSuggestion_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarSuggestion, []string{
		"+suggestion",
		"--start", "2025-03-21",
		"--end", "2025-03-21",
		"--attendee-ids", "ou_user1,oc_chat1",
		"--event-rrule", "FREQ=DAILY;BYDAY=MO",
		"--duration-minutes", "60",
		"--timezone", "Asia/Shanghai",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSuggestion_Pretty(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/suggestion",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"suggestions": []interface{}{
					map[string]interface{}{
						"event_start_time": "2025-03-21T10:00:00+08:00",
						"event_end_time":   "2025-03-21T11:00:00+08:00",
						"recommend_reason": "everyone is free",
					},
				},
				"ai_action_guidance": "book it",
			},
		},
	})

	err := mountAndRun(t, CalendarSuggestion, []string{
		"+suggestion",
		"--start", "2025-03-21",
		"--end", "2025-03-21",
		"--attendee-ids", "ou_user1,oc_chat1",
		"--event-rrule", "FREQ=DAILY;BYDAY=MO",
		"--duration-minutes", "60",
		"--timezone", "Asia/Shanghai",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSuggestion_DefaultTime(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/suggestion",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"suggestions": []interface{}{
					map[string]interface{}{
						"event_start_time": "2025-03-21T10:00:00+08:00",
						"event_end_time":   "2025-03-21T11:00:00+08:00",
						"recommend_reason": "everyone is free",
					},
				},
				"ai_action_guidance": "book it",
			},
		},
	})

	err := mountAndRun(t, CalendarSuggestion, []string{
		"+suggestion",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSuggestion_ExcludeTime(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/suggestion",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"suggestions": []interface{}{
					map[string]interface{}{
						"event_start_time": "2025-03-21T10:00:00+08:00",
						"event_end_time":   "2025-03-21T11:00:00+08:00",
						"recommend_reason": "everyone is free",
					},
				},
				"ai_action_guidance": "book it",
			},
		},
	})

	err := mountAndRun(t, CalendarSuggestion, []string{
		"+suggestion",
		"--start", "2025-03-21T14:00:00+08:00",
		"--end", "2025-03-21T18:00:00+08:00",
		"--duration-minutes", "30",
		"--timezone", "Asia/Shanghai",
		"--exclude", "2025-03-21T14:00:00+08:00~2025-03-21T14:30:00+08:00,2025-03-21T15:00:00+08:00~2025-03-21T15:30:00+08:00",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSuggestion_InvalidAttendee_Fails(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, CalendarSuggestion, []string{
		"+suggestion",
		"--attendee-ids", "invalid_id",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected validation error for invalid attendee id, got nil")
	}
	if !strings.Contains(err.Error(), "invalid attendee id format") {
		t.Errorf("error should mention attendee id format, got: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--attendee-ids" {
		t.Errorf("param=%q, want --attendee-ids", ve.Param)
	}
}

func TestSuggestion_HTTPNon2xx_Typed(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{Method: "POST", URL: suggestionPath, Status: 500, Body: map[string]interface{}{"code": 500, "msg": "server error"}})
	err := mountAndRun(t, CalendarSuggestion, []string{"+suggestion", "--start", "2025-03-21T10:00:00+08:00", "--end", "2025-03-21T11:00:00+08:00", "--as", "bot"}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *errs.APIError, got %T", err)
	}
	if ae.Code != 500 {
		t.Errorf("code=%d, want 500", ae.Code)
	}
}

func TestSuggestion_UnmarshalFail_Typed(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{Method: "POST", URL: suggestionPath, Status: 200, RawBody: []byte("not json")})
	err := mountAndRun(t, CalendarSuggestion, []string{"+suggestion", "--start", "2025-03-21T10:00:00+08:00", "--end", "2025-03-21T11:00:00+08:00", "--as", "bot"}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ie *errs.InternalError
	if !errors.As(err, &ie) {
		t.Fatalf("want *errs.InternalError, got %T", err)
	}
	if ie.Subtype != errs.SubtypeInvalidResponse {
		t.Errorf("subtype=%q, want invalid_response", ie.Subtype)
	}
}

func TestRoomFind_UnmarshalFail_Typed(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{Method: "POST", URL: roomFindPath, Status: 200, RawBody: []byte("not json")})
	err := mountAndRun(t, CalendarRoomFind, []string{"+room-find", "--slot", "2025-03-21T10:00:00+08:00~2025-03-21T11:00:00+08:00", "--as", "bot"}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ie *errs.InternalError
	if !errors.As(err, &ie) {
		t.Fatalf("want *errs.InternalError, got %T", err)
	}
	if ie.Subtype != errs.SubtypeInvalidResponse {
		t.Errorf("subtype=%q, want invalid_response", ie.Subtype)
	}
}

func TestSuggestion_InvalidExclude_Fails(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, CalendarSuggestion, []string{
		"+suggestion",
		"--exclude", "2025-03-21", // missing ~
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected validation error for invalid exclude format, got nil")
	}
	if !strings.Contains(err.Error(), "invalid range format in --exclude") {
		t.Errorf("error should mention exclude format, got: %v", err)
	}
}

func TestSuggestion_APIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/suggestion",
		Body: map[string]interface{}{
			"code": 190001,
			"msg":  "permission denied",
		},
	})

	err := mountAndRun(t, CalendarSuggestion, []string{
		"+suggestion",
		"--start", "2025-03-21",
		"--end", "2025-03-21",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// CalendarRoomFind tests
// ---------------------------------------------------------------------------

func TestRoomFind_MultiSlot_NewEventContext(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	for range 2 {
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/calendar/v4/freebusy/room_find",
			Body: map[string]interface{}{
				"code": 0,
				"msg":  "ok",
				"data": map[string]interface{}{
					"available_rooms": []interface{}{
						map[string]interface{}{
							"room_id":            "omm_room1",
							"room_name":          "F2-02",
							"capacity":           7,
							"reserve_until_time": "2026-04-01T00:00:00Z",
						},
					},
				},
			},
		})
	}

	err := mountAndRun(t, CalendarRoomFind, []string{
		"+room-find",
		"--slot", "2026-03-27T14:00:00+08:00~2026-03-27T15:00:00+08:00",
		"--slot", "2026-03-27T16:00:00+08:00~2026-03-27T17:00:00+08:00",
		"--attendee-ids", "ou_user1,ou_user2",
		"--format", "json",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "\"time_slots\"") {
		t.Fatalf("expected aggregated time_slots output, got: %s", stdout.String())
	}
}

func TestRoomFind_RejectsDangerousChars(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, CalendarRoomFind, []string{
		"+room-find",
		"--slot", "2026-03-27T14:00:00+08:00~2026-03-27T15:00:00+08:00",
		"--room-name", "F2-02\x7f",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected validation error for dangerous characters")
	}
	if !strings.Contains(err.Error(), "--room-name") {
		t.Fatalf("expected dangerous char error for --room-name, got: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--room-name" {
		t.Errorf("param=%q, want --room-name", ve.Param)
	}
}

func TestRoomFind_DryRun_SplitsUserAndChatAttendees(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, CalendarRoomFind, []string{
		"+room-find",
		"--slot", "2026-03-27T14:00:00+08:00~2026-03-27T15:00:00+08:00",
		"--attendee-ids", "ou_user1,oc_group1",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"attendee_user_ids"`) || !strings.Contains(out, `"ou_user1"`) || !strings.Contains(out, `"attendee_chat_ids"`) || !strings.Contains(out, `"oc_group1"`) {
		t.Fatalf("dry-run should split attendee IDs by prefix, got: %s", out)
	}
}

func TestRoomFind_DryRun_IncludesStructuredLocationFields(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, CalendarRoomFind, []string{
		"+room-find",
		"--slot", "2026-03-27T14:00:00+08:00~2026-03-27T15:00:00+08:00",
		"--city", "北京",
		"--building", "学清嘉创大厦B座",
		"--floor", "F2",
		"--room-name", "木星",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{`"city": "北京"`, `"building": "学清嘉创大厦B座"`, `"floor": "F2"`, `"room_name": "木星"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run should include %s, got: %s", want, out)
		}
	}
}

func TestRoomFind_RequestIncludesStructuredLocationFields(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/room_find",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"available_rooms": []interface{}{},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, CalendarRoomFind, []string{
		"+room-find",
		"--slot", "2026-03-27T14:00:00+08:00~2026-03-27T15:00:00+08:00",
		"--city", "北京",
		"--building", "学清嘉创大厦B座",
		"--floor", "F2",
		"--room-name", "木星",
		"--as", "bot",
	}, f, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &got); err != nil {
		t.Fatalf("unmarshal captured request: %v", err)
	}
	for key, want := range map[string]string{
		"city":      "北京",
		"building":  "学清嘉创大厦B座",
		"floor":     "F2",
		"room_name": "木星",
	} {
		if got[key] != want {
			t.Fatalf("expected %s=%q, got %#v", key, want, got[key])
		}
	}
}

func TestRoomFind_RejectsInvertedOrZeroLengthSlots(t *testing.T) {
	cases := []struct {
		name string
		slot string
	}{
		{
			name: "inverted",
			slot: "2026-03-27T15:00:00+08:00~2026-03-27T14:00:00+08:00",
		},
		{
			name: "zero-length",
			slot: "2026-03-27T15:00:00+08:00~2026-03-27T15:00:00+08:00",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

			err := mountAndRun(t, CalendarRoomFind, []string{
				"+room-find",
				"--slot", tc.slot,
				"--as", "bot",
			}, f, nil)
			if err == nil {
				t.Fatal("expected slot validation error")
			}
			if !strings.Contains(err.Error(), "--slot end time must be after start time") {
				t.Fatalf("expected invalid slot range error, got: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// helpers unit tests
// ---------------------------------------------------------------------------

func TestDedupeAndSortItems(t *testing.T) {
	items := []map[string]interface{}{
		{"event_id": "e1", "start_time": map[string]interface{}{"timestamp": "200"}, "end_time": map[string]interface{}{"timestamp": "300"}},
		{"event_id": "e2", "start_time": map[string]interface{}{"timestamp": "100"}, "end_time": map[string]interface{}{"timestamp": "150"}},
		// duplicate of e1
		{"event_id": "e1", "start_time": map[string]interface{}{"timestamp": "200"}, "end_time": map[string]interface{}{"timestamp": "300"}},
	}

	result := dedupeAndSortItems(items)

	if len(result) != 2 {
		t.Fatalf("expected 2 items after dedup, got %d", len(result))
	}
	id0, _ := result[0]["event_id"].(string)
	id1, _ := result[1]["event_id"].(string)
	if id0 != "e2" || id1 != "e1" {
		t.Errorf("expected order [e2, e1], got [%s, %s]", id0, id1)
	}
}

func TestResolveStartEnd_Defaults(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("start", "", "")
	cmd.Flags().String("end", "", "")
	cmd.ParseFlags(nil)

	rt := &common.RuntimeContext{Cmd: cmd}
	start, end := resolveStartEnd(rt)

	if start == "" {
		t.Error("start should not be empty")
	}
	if end != start {
		t.Errorf("end should equal start when both unset, got start=%q end=%q", start, end)
	}
}

func TestResolveStartEnd_ExplicitValues(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("start", "", "")
	cmd.Flags().String("end", "", "")
	cmd.ParseFlags(nil)
	cmd.Flags().Set("start", "2025-03-01")
	cmd.Flags().Set("end", "2025-03-15")

	rt := &common.RuntimeContext{Cmd: cmd}
	start, end := resolveStartEnd(rt)

	if start != "2025-03-01" {
		t.Errorf("start = %q, want 2025-03-01", start)
	}
	if end != "2025-03-15" {
		t.Errorf("end = %q, want 2025-03-15", end)
	}
}

// ---------------------------------------------------------------------------
// Shortcuts() registration test
// ---------------------------------------------------------------------------

func TestShortcuts_Returns10(t *testing.T) {
	shortcuts := Shortcuts()
	if len(shortcuts) != 10 {
		t.Fatalf("expected 10 shortcuts, got %d", len(shortcuts))
	}

	names := map[string]bool{}
	for _, s := range shortcuts {
		names[s.Command] = true
	}
	for _, want := range []string{"+agenda", "+create", "+update", "+freebusy", "+room-find", "+rsvp", "+suggestion", "+get"} {
		if !names[want] {
			t.Errorf("missing shortcut %s", want)
		}
	}
}

func TestShortcuts_AllHaveScopes(t *testing.T) {
	for _, s := range Shortcuts() {
		if s.Scopes == nil {
			t.Errorf("shortcut %s: Scopes is nil", s.Command)
		}
	}
}

// ---------------------------------------------------------------------------
// Typed error shape tests (typed-errs migration pass 1)
// ---------------------------------------------------------------------------

// Task 1: calendar_agenda.go
func TestAgenda_ParseTimeRange_InvalidStart_Typed(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarAgenda, []string{"+agenda", "--start", "not-a-time", "--as", "bot"}, f, nil)
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
	if ve.Param != "--start" {
		t.Errorf("param=%q, want --start", ve.Param)
	}
}

// Task 2: calendar_create.go
func TestCreate_InvalidAttendeeID_Typed(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarCreate, []string{"+create", "--summary", "x", "--start", "2025-03-21T10:00:00+08:00", "--end", "2025-03-21T11:00:00+08:00", "--calendar-id", "cal_test123", "--attendee-ids", "bad_id", "--as", "bot"}, f, nil)
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
}

func TestCreate_NoEventID_TypedInternal(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/calendar/v4/calendars/cal_test123/events", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"event": map[string]interface{}{}}}})
	err := mountAndRun(t, CalendarCreate, []string{"+create", "--summary", "x", "--start", "2025-03-21T10:00:00+08:00", "--end", "2025-03-21T11:00:00+08:00", "--calendar-id", "cal_test123", "--as", "bot"}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ie *errs.InternalError
	if !errors.As(err, &ie) {
		t.Fatalf("want *errs.InternalError, got %T", err)
	}
	if ie.Subtype != errs.SubtypeInvalidResponse {
		t.Errorf("subtype=%q", ie.Subtype)
	}
}

// Task 3: calendar_freebusy.go
func TestFreebusy_InvalidStart_Typed(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarFreebusy, []string{"+freebusy", "--start", "not-a-time", "--user-id", "ou_someone", "--as", "bot"}, f, nil)
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
	if ve.Param != "--start" {
		t.Errorf("param=%q, want --start", ve.Param)
	}
}

// Task 4: calendar_rsvp.go
func TestRsvp_EmptyEventID_Typed(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarRsvp, []string{"+rsvp", "--event-id", "   ", "--rsvp-status", "accept", "--as", "bot"}, f, nil)
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
	if ve.Param != "--event-id" {
		t.Errorf("param=%q, want --event-id", ve.Param)
	}
}

// Task 5: calendar_room_find.go
func TestRoomFind_MissingSlot_Typed(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarRoomFind, []string{"+room-find", "--as", "bot"}, f, nil)
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
	if ve.Param != "--slot" {
		t.Errorf("param=%q, want --slot", ve.Param)
	}
}

func TestRoomFind_APICodeError_Typed(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{Method: "POST", URL: roomFindPath, Body: map[string]interface{}{"code": 99991, "msg": "boom"}})
	err := mountAndRun(t, CalendarRoomFind, []string{"+room-find", "--slot", "2025-03-21T10:00:00+08:00~2025-03-21T11:00:00+08:00", "--as", "bot"}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *errs.APIError, got %T", err)
	}
	if ae.Subtype != errs.SubtypeUnknown {
		t.Errorf("subtype=%q, want unknown", ae.Subtype)
	}
	if ae.Code != 99991 {
		t.Errorf("code=%d, want 99991", ae.Code)
	}
	if output.ExitCodeOf(err) != output.ExitAPI {
		t.Errorf("exit=%d want ExitAPI", output.ExitCodeOf(err))
	}
}

func TestRoomFind_APICodeError_PreservesEnvelopeDetails(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    roomFindPath,
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Tt-Logid":   []string{"log-room-find"},
		},
		Body: map[string]interface{}{
			"code": codeInvalidParamsWithDetail,
			"msg":  "invalid params",
			"error": map[string]interface{}{
				"details": []interface{}{
					map[string]interface{}{"value": "event_start_time is required"},
				},
			},
		},
	})
	err := mountAndRun(t, CalendarRoomFind, []string{"+room-find", "--slot", "2025-03-21T10:00:00+08:00~2025-03-21T11:00:00+08:00", "--as", "bot"}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *errs.APIError, got %T", err)
	}
	if ae.Code != codeInvalidParamsWithDetail {
		t.Errorf("code=%d, want %d", ae.Code, codeInvalidParamsWithDetail)
	}
	if !strings.Contains(ae.Hint, "event_start_time is required") {
		t.Errorf("expected server detail in hint, got %q", ae.Hint)
	}
	if ae.LogID != "log-room-find" {
		t.Errorf("log_id=%q, want log-room-find", ae.LogID)
	}
}

func TestRoomFind_HTTPNon2xx_Typed(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{Method: "POST", URL: roomFindPath, Status: 500, Body: map[string]interface{}{"code": 500, "msg": "server error"}})
	err := mountAndRun(t, CalendarRoomFind, []string{"+room-find", "--slot", "2025-03-21T10:00:00+08:00~2025-03-21T11:00:00+08:00", "--as", "bot"}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *errs.APIError, got %T", err)
	}
	if ae.Subtype != errs.SubtypeUnknown {
		t.Errorf("subtype=%q, want unknown", ae.Subtype)
	}
	if ae.Code != 500 {
		t.Errorf("code=%d, want 500", ae.Code)
	}
	if output.ExitCodeOf(err) != output.ExitAPI {
		t.Errorf("exit=%d want ExitAPI", output.ExitCodeOf(err))
	}
}

// Task 6: calendar_suggestion.go
func TestSuggestion_InvalidExclude_Typed(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarSuggestion, []string{"+suggestion", "--exclude", "not-a-range", "--as", "bot"}, f, nil)
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
	if ve.Param != "--exclude" {
		t.Errorf("param=%q, want --exclude", ve.Param)
	}
}

func TestSuggestion_APICodeError_Typed(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{Method: "POST", URL: suggestionPath, Body: map[string]interface{}{"code": 99991, "msg": "boom"}})
	err := mountAndRun(t, CalendarSuggestion, []string{"+suggestion", "--start", "2025-03-21T10:00:00+08:00", "--end", "2025-03-21T11:00:00+08:00", "--as", "bot"}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *errs.APIError, got %T", err)
	}
	if ae.Subtype != errs.SubtypeUnknown {
		t.Errorf("subtype=%q, want unknown", ae.Subtype)
	}
	if ae.Code != 99991 {
		t.Errorf("code=%d, want 99991", ae.Code)
	}
	if output.ExitCodeOf(err) != output.ExitAPI {
		t.Errorf("exit=%d want ExitAPI", output.ExitCodeOf(err))
	}
}

func TestSuggestion_APICodeError_PreservesEnvelopeDetails(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    suggestionPath,
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Tt-Logid":   []string{"log-suggestion"},
		},
		Body: map[string]interface{}{
			"code": codeInvalidParamsWithDetail,
			"msg":  "invalid params",
			"error": map[string]interface{}{
				"details": []interface{}{
					map[string]interface{}{"value": "search_end_time must be after search_start_time"},
				},
			},
		},
	})
	err := mountAndRun(t, CalendarSuggestion, []string{"+suggestion", "--start", "2025-03-21T10:00:00+08:00", "--end", "2025-03-21T11:00:00+08:00", "--as", "bot"}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *errs.APIError, got %T", err)
	}
	if ae.Code != codeInvalidParamsWithDetail {
		t.Errorf("code=%d, want %d", ae.Code, codeInvalidParamsWithDetail)
	}
	if !strings.Contains(ae.Hint, "search_end_time must be after search_start_time") {
		t.Errorf("expected server detail in hint, got %q", ae.Hint)
	}
	if ae.LogID != "log-suggestion" {
		t.Errorf("log_id=%q, want log-suggestion", ae.LogID)
	}
}

// Task 7: calendar_update.go
func TestUpdate_AttendeeConflict_Typed(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarUpdate, []string{"+update", "--event-id", "evt_1", "--add-attendee-ids", "ou_dup", "--remove-attendee-ids", "ou_dup", "--as", "bot"}, f, nil)
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
	if ve.Param != "" {
		t.Errorf("param=%q, want empty (cross-flag)", ve.Param)
	}
}

// The empty-event-id guard at executeCalendarUpdate is defensive: the Validate
// hook (validateCalendarUpdate) rejects an empty --event-id before Execute runs,
// so the :283 guard is unreachable through the normal CLI flow. Exercise it
// directly to pin the migrated typed shape (ValidationError / invalid_argument /
// --event-id).
func TestUpdate_EmptyEventID_Typed(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("calendar-id", "", "")
	cmd.Flags().String("event-id", "", "")
	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, defaultConfig())
	err := executeCalendarUpdate(context.Background(), runtime)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype=%q, want invalid_argument", ve.Subtype)
	}
	if ve.Param != "--event-id" {
		t.Errorf("param=%q, want --event-id", ve.Param)
	}
}

// Round-1 completeness: FlagErrorf call sites migrated to typed errs.

// calendar_create.go start/end validation block.
func TestCreate_MissingStart_TypedFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	// --start is a Required flag; pass it empty to satisfy cobra's required-flag
	// check and reach the in-builder empty-value guard.
	err := mountAndRun(t, CalendarCreate, []string{"+create", "--summary", "x", "--calendar-id", "cal_test123", "--start", "", "--end", "2025-03-21T11:00:00+08:00", "--as", "bot"}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype=%q, want invalid_argument", ve.Subtype)
	}
	if ve.Param != "--start" {
		t.Errorf("param=%q, want --start", ve.Param)
	}
}

// calendar_freebusy.go bot-identity guard.
func TestFreebusy_BotMissingUserID_TypedFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarFreebusy, []string{"+freebusy", "--start", "2025-03-21T10:00:00+08:00", "--end", "2025-03-21T11:00:00+08:00", "--as", "bot"}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype=%q, want invalid_argument", ve.Subtype)
	}
	if ve.Param != "--user-id" {
		t.Errorf("param=%q, want --user-id", ve.Param)
	}
}

// calendar_update.go buildCalendarUpdateEventData time-pairing guard.
func TestUpdate_StartWithoutEnd_TypedFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarUpdate, []string{"+update", "--event-id", "evt_1", "--start", "2025-03-21T10:00:00+08:00", "--as", "bot"}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype=%q, want invalid_argument", ve.Subtype)
	}
}

// calendar_update.go invalid start-time guard carries the offending flag.
func TestUpdate_InvalidStartTime_TypedFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarUpdate, []string{"+update", "--event-id", "evt_1", "--start", "not-a-time", "--end", "2025-03-21T11:00:00+08:00", "--as", "bot"}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--start" {
		t.Errorf("param=%q, want --start", ve.Param)
	}
}

// ---------------------------------------------------------------------------
// Additional success / branch coverage for the migrated command paths.
// ---------------------------------------------------------------------------

// TestAgenda_TooManyInstances_SplitSucceeds pins the 193104 recovery path: the
// full range trips the too-many-instances limit, the window is halved via
// fetchInstanceViewSplit, and both sub-ranges succeed and aggregate.
func TestAgenda_TooManyInstances_SplitSucceeds(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/events/instance_view",
		Body:   map[string]interface{}{"code": 193104, "msg": "too many instances"},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/events/instance_view",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"event_id":   "evt_left",
						"summary":    "Left",
						"status":     "confirmed",
						"start_time": map[string]interface{}{"timestamp": "1742515200"},
						"end_time":   map[string]interface{}{"timestamp": "1742518800"},
					},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/events/instance_view",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"event_id":   "evt_right",
						"summary":    "Right",
						"status":     "confirmed",
						"start_time": map[string]interface{}{"timestamp": "1745193600"},
						"end_time":   map[string]interface{}{"timestamp": "1745197200"},
					},
				},
			},
		},
	})

	// A 30-day span is above minSplitWindowSeconds (2h), so the 193104 branch
	// halves the window and aggregates the two successful sub-ranges.
	err := mountAndRun(t, CalendarAgenda, []string{
		"+agenda",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-04-20T00:00:00+08:00",
		"--as", "bot",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "evt_left") || !strings.Contains(out, "evt_right") {
		t.Errorf("expected aggregated events from both halves, got: %s", out)
	}
}

// TestAgenda_TimeRangeExceeded_CannotSplit pins the 193103 guard where the
// window is a single point (mid <= startTime), so the range cannot be narrowed
// further and the typed error surfaces.
func TestAgenda_TimeRangeExceeded_CannotSplit(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method:   "GET",
		URL:      "/events/instance_view",
		Reusable: true,
		Body:     map[string]interface{}{"code": 193103, "msg": "time range exceeds limit"},
	})

	// start == end gives a zero-length span; the 193103 branch computes
	// mid == startTime and bails with the typed "narrow the range" error.
	err := mountAndRun(t, CalendarAgenda, []string{
		"+agenda",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-03-21T00:00:00+08:00",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected typed error when 193103 range cannot be split, got nil")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T", err)
	}
	if ae.Code != 193103 {
		t.Errorf("code=%d, want 193103", ae.Code)
	}
	if !strings.Contains(ae.Error(), "narrow the range") {
		t.Errorf("expected narrow-the-range guidance, got: %q", ae.Error())
	}
}

// TestUpdate_PatchStepFails_TypedError pins that a failed event PATCH surfaces
// the typed API error wrapped with completed-step context.
func TestUpdate_PatchStepFails_TypedError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events/evt_patchfail",
		Body:   map[string]interface{}{"code": 190001, "msg": "permission denied"},
	})

	err := mountAndRun(t, CalendarUpdate, []string{
		"+update",
		"--event-id", "evt_patchfail",
		"--calendar-id", "cal_test123",
		"--summary", "New title",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error when PATCH step fails, got nil")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T", err)
	}
}

// TestUpdate_RemoveStepFails_TypedError pins the batch_delete failure path.
func TestUpdate_RemoveStepFails_TypedError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/events/evt_removefail/attendees/batch_delete",
		Body:   map[string]interface{}{"code": 190001, "msg": "permission denied"},
	})

	err := mountAndRun(t, CalendarUpdate, []string{
		"+update",
		"--event-id", "evt_removefail",
		"--remove-attendee-ids", "ou_user1",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error when remove step fails, got nil")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T", err)
	}
}

// TestUpdate_AddStepFails_TypedError pins the add-attendees failure path.
func TestUpdate_AddStepFails_TypedError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/events/evt_addfail/attendees",
		Body:   map[string]interface{}{"code": 190001, "msg": "permission denied"},
	})

	err := mountAndRun(t, CalendarUpdate, []string{
		"+update",
		"--event-id", "evt_addfail",
		"--add-attendee-ids", "ou_user1",
		"--as", "bot",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error when add step fails, got nil")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T", err)
	}
}

// TestUpdate_InvalidEndTime_TypedFlag pins the --end parse error inside
// buildCalendarUpdateEventData (start valid, end malformed).
func TestUpdate_InvalidEndTime_TypedFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarUpdate, []string{
		"+update", "--event-id", "evt_1",
		"--start", "2025-03-21T10:00:00+08:00", "--end", "not-a-time", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--end" {
		t.Errorf("param=%q, want --end", ve.Param)
	}
}

// TestUpdate_RejectsDangerousChars pins the dangerous-character guard.
func TestUpdate_RejectsDangerousChars(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarUpdate, []string{
		"+update", "--event-id", "evt_1", "--summary", "bad\x7ftitle", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error for dangerous chars, got nil")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--summary" {
		t.Errorf("param=%q, want --summary", ve.Param)
	}
}

// TestCreate_InvalidEndTime_TypedFlag pins the --end parse error in Validate.
func TestCreate_InvalidEndTime_TypedFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarCreate, []string{
		"+create", "--summary", "X",
		"--start", "2025-03-21T10:00:00+08:00", "--end", "not-a-time", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--end" {
		t.Errorf("param=%q, want --end", ve.Param)
	}
}

// TestCreate_RejectsDangerousChars pins the dangerous-character guard on
// --summary.
func TestCreate_RejectsDangerousChars(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarCreate, []string{
		"+create", "--summary", "bad\x7ftitle",
		"--start", "2025-03-21T10:00:00+08:00", "--end", "2025-03-21T11:00:00+08:00", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error for dangerous chars, got nil")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--summary" {
		t.Errorf("param=%q, want --summary", ve.Param)
	}
}

// TestFreebusy_InvalidEnd_TypedFlag pins the --end parse error in
// parseFreebusyTimeRange.
func TestFreebusy_InvalidEnd_TypedFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy", "--start", "2025-03-21", "--end", "not-a-time",
		"--user-id", "ou_someone", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--end" {
		t.Errorf("param=%q, want --end", ve.Param)
	}
}

// TestFreebusy_InvalidUserID_TypedFlag pins the --user-id format guard.
func TestFreebusy_InvalidUserID_TypedFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy", "--start", "2025-03-21", "--end", "2025-03-21",
		"--user-id", "not-an-open-id", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--user-id" {
		t.Errorf("param=%q, want --user-id", ve.Param)
	}
}

// TestRoomFind_InvalidCapacity_TypedFlag pins the --min-capacity / --max-capacity
// ordering guard.
func TestRoomFind_InvalidCapacity_TypedFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarRoomFind, []string{
		"+room-find",
		"--slot", "2025-03-21T10:00:00+08:00~2025-03-21T11:00:00+08:00",
		"--min-capacity", "10", "--max-capacity", "5", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--min-capacity" {
		t.Errorf("param=%q, want --min-capacity", ve.Param)
	}
}

// TestFreebusy_NoLoginNoUserID_TypedFlag pins the "cannot determine user ID"
// guard: no --user-id, not bot, and no logged-in user.
func TestFreebusy_NoLoginNoUserID_TypedFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, noLoginConfig())
	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy", "--start", "2025-03-21", "--end", "2025-03-21",
	}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	// May surface as a login/identity guard or the --user-id validation guard;
	// either way it must be a typed error, never a panic or nil.
	if _, ok := errs.ProblemOf(err); !ok {
		t.Fatalf("expected a typed problem error, got %T: %v", err, err)
	}
}

// TestSuggestion_DurationOutOfRange_TypedFlag pins the --duration-minutes range
// guard (must be 1..1440).
func TestSuggestion_DurationOutOfRange_TypedFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarSuggestion, []string{
		"+suggestion",
		"--start", "2025-03-21T10:00:00+08:00", "--end", "2025-03-21T11:00:00+08:00",
		"--duration-minutes", "5000", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--duration-minutes" {
		t.Errorf("param=%q, want --duration-minutes", ve.Param)
	}
}

// TestSuggestion_InvalidStart_TypedFlag pins the --start parse guard in Validate.
func TestSuggestion_InvalidStart_TypedFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarSuggestion, []string{
		"+suggestion", "--start", "not-a-time", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--start" {
		t.Errorf("param=%q, want --start", ve.Param)
	}
}

// TestSuggestion_InvalidEnd_TypedFlag pins the --end parse guard in Validate.
func TestSuggestion_InvalidEnd_TypedFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarSuggestion, []string{
		"+suggestion", "--start", "2025-03-21T10:00:00+08:00", "--end", "not-a-time", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--end" {
		t.Errorf("param=%q, want --end", ve.Param)
	}
}

// TestSuggestion_InvalidExcludeStart_TypedFlag pins the malformed --exclude
// start-time guard in Validate.
func TestSuggestion_InvalidExcludeStart_TypedFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarSuggestion, []string{
		"+suggestion",
		"--start", "2025-03-21T10:00:00+08:00", "--end", "2025-03-21T18:00:00+08:00",
		"--exclude", "not-a-time~2025-03-21T12:00:00+08:00", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--exclude" {
		t.Errorf("param=%q, want --exclude", ve.Param)
	}
}

// TestSuggestion_InvalidExcludeEnd_TypedFlag pins the malformed --exclude
// end-time guard in Validate.
func TestSuggestion_InvalidExcludeEnd_TypedFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarSuggestion, []string{
		"+suggestion",
		"--start", "2025-03-21T10:00:00+08:00", "--end", "2025-03-21T18:00:00+08:00",
		"--exclude", "2025-03-21T11:00:00+08:00~not-a-time", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--exclude" {
		t.Errorf("param=%q, want --exclude", ve.Param)
	}
}

// TestSuggestion_RejectsDangerousTimezone_Typed pins the dangerous-character
// guard on --timezone.
func TestSuggestion_RejectsDangerousTimezone_Typed(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarSuggestion, []string{
		"+suggestion",
		"--start", "2025-03-21T10:00:00+08:00", "--end", "2025-03-21T11:00:00+08:00",
		"--timezone", "Asia/Shanghai\x7f", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--timezone" {
		t.Errorf("param=%q, want --timezone", ve.Param)
	}
}

// ---------------------------------------------------------------------------
// CalendarGet tests
// ---------------------------------------------------------------------------

func TestGet_Success_FlattensAndConvertsTimes(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events/evt_001",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"event": map[string]interface{}{
					"event_id":    "evt_001",
					"summary":     "Daily Sync",
					"create_time": "1602504000",
					"start_time": map[string]interface{}{
						"timestamp": "1742515200",
						"timezone":  "Asia/Shanghai",
					},
					"end_time": map[string]interface{}{
						"timestamp": "1742518800",
						"timezone":  "Asia/Shanghai",
					},
					"status": "confirmed",
				},
			},
		},
	})

	err := mountAndRun(t, CalendarGet, []string{
		"+get",
		"--calendar-id", "cal_test123",
		"--event-id", "evt_001",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	// Expect flattened — fields appear directly under "data", not under "data.event"
	if strings.Contains(out, "\"event\": {") {
		t.Errorf("payload should be flattened (no event wrapper), got: %s", out)
	}
	if !strings.Contains(out, "\"event_id\": \"evt_001\"") {
		t.Errorf("expected event_id in output, got: %s", out)
	}
	// status=confirmed should be dropped
	if strings.Contains(out, "\"status\": \"confirmed\"") {
		t.Errorf("status should be dropped when not cancelled, got: %s", out)
	}
	// timestamp must be replaced with datetime
	if strings.Contains(out, "\"timestamp\":") {
		t.Errorf("timestamp should be replaced with datetime, got: %s", out)
	}
	if !strings.Contains(out, "\"datetime\":") {
		t.Errorf("expected datetime in output, got: %s", out)
	}
	// create_time must be RFC3339 (contain 'T' and timezone)
	if !strings.Contains(out, "\"create_time\": \"2020-10-12T") {
		t.Errorf("expected RFC3339 create_time, got: %s", out)
	}
}

func TestGet_CancelledStatus_PreservesStatus(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events/evt_002",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"event": map[string]interface{}{
					"event_id":    "evt_002",
					"summary":     "Cancelled Meeting",
					"create_time": "1602504000",
					"start_time":  map[string]interface{}{"timestamp": "1742515200"},
					"end_time":    map[string]interface{}{"timestamp": "1742518800"},
					"status":      "cancelled",
				},
			},
		},
	})

	err := mountAndRun(t, CalendarGet, []string{
		"+get",
		"--calendar-id", "cal_test123",
		"--event-id", "evt_002",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "\"status\": \"cancelled\"") {
		t.Errorf("status should be preserved when cancelled, got: %s", out)
	}
}

func TestGet_AllDayEvent_AdjustsEndDate(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	// All-day event: start 2025-03-21, end 2025-03-22 (exclusive in API).
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events/evt_003",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"event": map[string]interface{}{
					"event_id":   "evt_003",
					"summary":    "All-day",
					"start_time": map[string]interface{}{"date": "2025-03-21"},
					"end_time":   map[string]interface{}{"date": "2025-03-22"},
					"status":     "confirmed",
				},
			},
		},
	})

	err := mountAndRun(t, CalendarGet, []string{
		"+get",
		"--calendar-id", "cal_test123",
		"--event-id", "evt_003",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	// end date 2025-03-22 should rewind by 1s -> 2025-03-21
	if !strings.Contains(out, "\"date\": \"2025-03-21\"") {
		t.Errorf("expected end date adjusted to 2025-03-21, got: %s", out)
	}
}

func TestGet_EmptyEventID_Typed(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarGet, []string{
		"+get",
		"--event-id", "   ",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("want error for empty event-id")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--event-id" {
		t.Errorf("param=%q, want --event-id", ve.Param)
	}
}

func TestGet_MissingEventField_TypedInternal(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events/evt_404",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRun(t, CalendarGet, []string{
		"+get",
		"--calendar-id", "cal_test123",
		"--event-id", "evt_404",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("want error when event field is missing")
	}
	var ie *errs.InternalError
	if !errors.As(err, &ie) {
		t.Fatalf("want *errs.InternalError, got %T", err)
	}
	if ie.Subtype != errs.SubtypeInvalidResponse {
		t.Errorf("subtype=%q, want invalid_response", ie.Subtype)
	}
}
