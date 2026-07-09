// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

var calWarmOnce sync.Once

func calWarmTokenCache(t *testing.T) {
	t.Helper()
	calWarmOnce.Do(func() {
		f, _, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
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

func calDefaultConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
		UserOpenId: "ou_testuser",
	}
}

func calMountAndRun(t *testing.T, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	calWarmTokenCache(t)
	parent := &cobra.Command{Use: "calendar"}
	s.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

// ---------------------------------------------------------------------------
// calendar +meeting tests
// ---------------------------------------------------------------------------

func mgetInstanceRelationStub(calendarID, instanceID string, meetingIDs []string, meetingNotes []string, aiMeetingNotes []string) *httpmock.Stub {
	infos := map[string]interface{}{
		"instance_id": instanceID,
	}
	mIDs := make([]interface{}, len(meetingIDs))
	for i, id := range meetingIDs {
		mIDs[i] = id
	}
	infos["meeting_instance_ids"] = mIDs
	if len(meetingNotes) > 0 {
		notes := make([]interface{}, len(meetingNotes))
		for i, n := range meetingNotes {
			notes[i] = n
		}
		infos["meeting_notes"] = notes
	}
	if len(aiMeetingNotes) > 0 {
		notes := make([]interface{}, len(aiMeetingNotes))
		for i, n := range aiMeetingNotes {
			notes[i] = n
		}
		infos["ai_meeting_notes"] = notes
	}
	return &httpmock.Stub{
		Method: "POST",
		URL:    fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/mget_instance_relation_info", calendarID),
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"instance_relation_infos": []interface{}{infos},
			},
		},
	}
}

func mgetInstanceRelationFailedStub(calendarID, instanceID, failMsg string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "POST",
		URL:    fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/mget_instance_relation_info", calendarID),
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"instance_relation_infos": []interface{}{},
				"failed_instance_ids": []interface{}{
					map[string]interface{}{
						"instance_id": instanceID,
						"fail_msg":    failMsg,
					},
				},
			},
		},
	}
}

func TestMeeting_Validation_MissingEventIDs(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, calDefaultConfig())
	err := calMountAndRun(t, CalendarMeeting, []string{"+meeting", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for missing --event-ids")
	}
}

func TestMeeting_Validation_BatchLimit(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, calDefaultConfig())
	ids := make([]string, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("evt%d", i)
	}
	err := calMountAndRun(t, CalendarMeeting, []string{"+meeting", "--event-ids", strings.Join(ids, ","), "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected batch limit error")
	}
	if !strings.Contains(err.Error(), "too many IDs") {
		t.Errorf("expected 'too many IDs' error, got: %v", err)
	}
}

func TestMeeting_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, calDefaultConfig())
	err := calMountAndRun(t, CalendarMeeting, []string{"+meeting", "--event-ids", "evt001", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "mget_instance_relation_info") {
		t.Errorf("dry-run should show mget API path, got: %s", stdout.String())
	}
}

func TestMeeting_Execute_Success(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
	reg.Register(mgetInstanceRelationStub("primary", "evt_m1", []string{"123456"}, []string{"doc_note1"}, []string{"doc_ai1"}))

	err := calMountAndRun(t, CalendarMeeting, []string{"+meeting", "--event-ids", "evt_m1", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	meetings, _ := data["meetings"].([]any)
	if len(meetings) != 1 {
		t.Fatalf("expected 1 meeting, got %d", len(meetings))
	}
	m, _ := meetings[0].(map[string]any)
	if m["meeting_id"] != "123456" {
		t.Errorf("meeting_id = %v, want 123456", m["meeting_id"])
	}
	if m["meeting_note"] != "doc_note1" {
		t.Errorf("meeting_note = %v, want doc_note1", m["meeting_note"])
	}
	if _, hasAI := m["ai_meeting_note"]; hasAI {
		t.Error("ai_meeting_note should not be present in output")
	}
}

func TestMeeting_Execute_FailedInstance(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
	reg.Register(mgetInstanceRelationFailedStub("primary", "evt_fail", "No Permission"))

	err := calMountAndRun(t, CalendarMeeting, []string{"+meeting", "--event-ids", "evt_fail", "--as", "user"}, f, stdout)
	if err == nil {
		t.Fatal("expected partial failure error")
	}
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected *output.PartialFailureError, got %T: %v", err, err)
	}
	// Verify translated fail_msg appears in output
	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err == nil {
		data, _ := resp["data"].(map[string]any)
		meetings, _ := data["meetings"].([]any)
		if len(meetings) > 0 {
			m, _ := meetings[0].(map[string]any)
			if errMsg, _ := m["error"].(string); !strings.Contains(errMsg, "no read permission") {
				t.Errorf("expected translated fail_msg, got: %v", errMsg)
			}
		}
	}
}

func TestMeeting_Execute_NoMeeting(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
	reg.Register(mgetInstanceRelationStub("primary", "evt_nomeet", []string{}, nil, nil))

	err := calMountAndRun(t, CalendarMeeting, []string{"+meeting", "--event-ids", "evt_nomeet", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	meetings, _ := data["meetings"].([]any)
	if len(meetings) != 1 {
		t.Fatalf("expected 1 meeting, got %d", len(meetings))
	}
	m, _ := meetings[0].(map[string]any)
	if hint, _ := m["hint"].(string); !strings.Contains(hint, "meeting_id") {
		t.Errorf("expected hint about meeting_id, got: %v", hint)
	}
}

// ---------------------------------------------------------------------------
// calendar +search-event tests
// ---------------------------------------------------------------------------

func TestSearchEvent_Validation_InvalidTimeRange(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, calDefaultConfig())
	err := calMountAndRun(t, CalendarSearchEvent, []string{"+search-event", "--start", "bad-format", "--end", "2026-04-27", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for invalid --start")
	}
	if !strings.Contains(err.Error(), "--start") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSearchEvent_Validation_TimeRangeStartAfterEnd(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, calDefaultConfig())
	err := calMountAndRun(t, CalendarSearchEvent, []string{"+search-event", "--start", "2026-04-27", "--end", "2026-04-20", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for start after end")
	}
}

func TestSearchEvent_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, calDefaultConfig())
	err := calMountAndRun(t, CalendarSearchEvent, []string{"+search-event", "--query", "周会", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "search_event") {
		t.Errorf("dry-run should show search_event API path, got: %s", stdout.String())
	}
}

func TestSearchEvent_Execute_Success(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/search_event",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"display_info": "Q2 周会\n2026-04-23 15:00-16:00",
						"meta_data": map[string]interface{}{
							"event_id": "evt_search1",
							"summary":  "Q2 周会",
							"start": map[string]interface{}{
								"date_time": "2026-04-23T15:00:00+08:00",
								"timezone":  "Asia/Shanghai",
							},
							"end": map[string]interface{}{
								"date_time": "2026-04-23T16:00:00+08:00",
								"timezone":  "Asia/Shanghai",
							},
							"is_all_day": false,
							"app_link":   "https://applink.feishu.cn/...",
						},
					},
				},
				"has_more":   false,
				"page_token": "",
			},
		},
	})

	err := calMountAndRun(t, CalendarSearchEvent, []string{"+search-event", "--query", "周会", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["calendar_id"] != "primary" {
		t.Errorf("calendar_id = %v, want primary", data["calendar_id"])
	}
	items, _ := data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item, _ := items[0].(map[string]any)
	if item["event_id"] != "evt_search1" {
		t.Errorf("event_id = %v, want evt_search1", item["event_id"])
	}
	if item["summary"] != "Q2 周会" {
		t.Errorf("summary = %v, want 'Q2 周会'", item["summary"])
	}
}

func TestSearchEvent_Execute_Empty(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/primary/events/search_event",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{},
				"has_more": false,
			},
		},
	})

	err := calMountAndRun(t, CalendarSearchEvent, []string{"+search-event", "--query", "nonexistent", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Pure function tests
// ---------------------------------------------------------------------------

func TestParseSearchEventTimeRange(t *testing.T) {
	tests := []struct {
		name    string
		start   string
		end     string
		wantErr bool
	}{
		{"empty", "", "", false},
		{"valid", "2026-04-20", "2026-04-27", false},
		{"start only defaults end", "2026-04-20", "", false},
		{"end only defaults start", "", "2026-04-27", false},
		{"invalid start format", "not-a-date", "2026-04-27", true},
		{"start after end", "2026-04-27", "2026-04-20", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("start", "", "")
			cmd.Flags().String("end", "", "")
			if tt.start != "" {
				_ = cmd.Flags().Set("start", tt.start)
			}
			if tt.end != "" {
				_ = cmd.Flags().Set("end", tt.end)
			}
			runtime := common.TestNewRuntimeContext(cmd, calDefaultConfig())
			_, _, err := parseSearchEventTimeRange(runtime)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSearchEventTimeRange() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	t.Run("start only fills end with end-of-day", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("start", "", "")
		cmd.Flags().String("end", "", "")
		_ = cmd.Flags().Set("start", "2026-04-20")
		runtime := common.TestNewRuntimeContext(cmd, calDefaultConfig())
		startRFC, endRFC, err := parseSearchEventTimeRange(runtime)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(startRFC, "2026-04-20T00:00:00") {
			t.Errorf("start = %s, want 2026-04-20T00:00:00...", startRFC)
		}
		if !strings.HasPrefix(endRFC, "2026-04-20T23:59:59") {
			t.Errorf("end = %s, want 2026-04-20T23:59:59...", endRFC)
		}
	})

	t.Run("end only fills start with start-of-day", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("start", "", "")
		cmd.Flags().String("end", "", "")
		_ = cmd.Flags().Set("end", "2026-04-27")
		runtime := common.TestNewRuntimeContext(cmd, calDefaultConfig())
		startRFC, endRFC, err := parseSearchEventTimeRange(runtime)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(startRFC, "2026-04-27T00:00:00") {
			t.Errorf("start = %s, want 2026-04-27T00:00:00...", startRFC)
		}
		if !strings.HasPrefix(endRFC, "2026-04-27T23:59:59") {
			t.Errorf("end = %s, want 2026-04-27T23:59:59...", endRFC)
		}
	})
}

func TestBuildSearchEventFilter(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("attendee-ids", "", "")
	_ = cmd.Flags().Set("attendee-ids", "ou_user1,oc_chat1,omm_room1")
	runtime := common.TestNewRuntimeContext(cmd, calDefaultConfig())

	filter := buildSearchEventFilter(runtime, "", "")
	if filter == nil {
		t.Fatal("expected filter to be non-nil")
	}
	if len(filter.AttendeeUserIDs) != 1 || filter.AttendeeUserIDs[0] != "ou_user1" {
		t.Errorf("attendee_user_ids = %v, want [ou_user1]", filter.AttendeeUserIDs)
	}
	if len(filter.AttendeeChatIDs) != 1 || filter.AttendeeChatIDs[0] != "oc_chat1" {
		t.Errorf("attendee_chat_ids = %v, want [oc_chat1]", filter.AttendeeChatIDs)
	}
	if len(filter.MeetingRoomIDs) != 1 || filter.MeetingRoomIDs[0] != "omm_room1" {
		t.Errorf("meeting_room_ids = %v, want [omm_room1]", filter.MeetingRoomIDs)
	}
}

func TestBuildSearchEventFilter_Empty(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("attendee-ids", "", "")
	runtime := common.TestNewRuntimeContext(cmd, calDefaultConfig())

	filter := buildSearchEventFilter(runtime, "", "")
	if filter != nil {
		t.Errorf("expected nil for empty filter, got %v", filter)
	}
}

func TestBuildSearchEventFilter_TimeRange(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("attendee-ids", "", "")
	runtime := common.TestNewRuntimeContext(cmd, calDefaultConfig())

	filter := buildSearchEventFilter(runtime, "2026-04-20T00:00:00+08:00", "2026-04-27T23:59:59+08:00")
	if filter == nil {
		t.Fatal("expected filter to be non-nil")
	}
	if filter.TimeRange == nil {
		t.Fatal("expected time_range in filter")
	}
	if filter.TimeRange.StartTime != "2026-04-20T00:00:00+08:00" {
		t.Errorf("start_time = %v, want 2026-04-20T00:00:00+08:00", filter.TimeRange.StartTime)
	}
}
