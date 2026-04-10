// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

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
				_, err := rctx.CallAPI("GET", "/open-apis/test/v1/warm", nil, nil)
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
	if !strings.Contains(err.Error(), "rolled back successfully") && !strings.Contains(err.Error(), "auto-rolled back") {
		t.Fatalf("error should mention rollback, got: %v", err)
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
			name:     "freebusy",
			shortcut: CalendarFreebusy,
			args:     []string{"+freebusy", "--start", "2025-03-21", "--end", "2025-03-21"},
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

func TestShortcuts_Returns5(t *testing.T) {
	shortcuts := Shortcuts()
	if len(shortcuts) != 5 {
		t.Fatalf("expected 5 shortcuts, got %d", len(shortcuts))
	}

	names := map[string]bool{}
	for _, s := range shortcuts {
		names[s.Command] = true
	}
	for _, want := range []string{"+agenda", "+create", "+freebusy", "+rsvp", "+suggestion"} {
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
