// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// ---------------------------------------------------------------------------
// Validation tests
// ---------------------------------------------------------------------------

func TestDetail_Validation_MissingMeetingIDs(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCDetail, []string{"+detail", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for missing --meeting-ids")
	}
	if !strings.Contains(err.Error(), "meeting-ids") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDetail_Validation_BatchLimit(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	ids := make([]string, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("m%d", i)
	}
	err := mountAndRun(t, VCDetail, []string{"+detail", "--meeting-ids", strings.Join(ids, ","), "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected batch limit error")
	}
	if !strings.Contains(err.Error(), "too many IDs") {
		t.Errorf("expected 'too many IDs' error, got: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want SubtypeInvalidArgument", ve.Subtype)
	}
}

// ---------------------------------------------------------------------------
// DryRun tests
// ---------------------------------------------------------------------------

func TestDetail_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCDetail, []string{"+detail", "--meeting-ids", "m001", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "/open-apis/vc/v1/meetings/") {
		t.Errorf("dry-run should show meeting API path, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "recording") {
		t.Errorf("dry-run should show recording API path, got: %s", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// Execute tests with mocked HTTP
// ---------------------------------------------------------------------------

func TestDetail_Execute_Success(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingGetStub("m_detail1", "note_001"))
	reg.Register(recordingOKStub("m_detail1", "https://meetings.feishu.cn/minutes/obc_detail1"))

	err := mountAndRun(t, VCDetail, []string{"+detail", "--meeting-ids", "m_detail1", "--as", "user"}, f, stdout)
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
	if m["meeting_id"] != "m_detail1" {
		t.Errorf("meeting_id = %v, want m_detail1", m["meeting_id"])
	}
	if m["note_id"] != "note_001" {
		t.Errorf("note_id = %v, want note_001", m["note_id"])
	}
	if m["minute_token"] != "obc_detail1" {
		t.Errorf("minute_token = %v, want obc_detail1", m["minute_token"])
	}
}

func TestDetail_Execute_NoNoteNoMinute(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingGetStub("m_nonote", ""))
	reg.Register(recordingErrStub("m_nonote", 121004, "not found"))

	err := mountAndRun(t, VCDetail, []string{"+detail", "--meeting-ids", "m_nonote", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify hint is present for empty note_id and missing recording
	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	meetings, _ := data["meetings"].([]any)
	m, _ := meetings[0].(map[string]any)
	if hint, _ := m["hint"].(string); !strings.Contains(hint, "note_id") || !strings.Contains(hint, "no minute file for this meeting") {
		t.Errorf("hint should mention note_id and minute file missing, got: %v", hint)
	}
	if errMsg, _ := m["error"].(string); errMsg != "" {
		t.Errorf("error should be empty, got: %v", errMsg)
	}
}

func TestDetail_Execute_MeetingNotFound(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m_bad",
		Body:   map[string]interface{}{"code": 121004, "msg": "data not found"},
	})

	err := mountAndRun(t, VCDetail, []string{"+detail", "--meeting-ids", "m_bad", "--as", "user"}, f, stdout)
	if err == nil {
		t.Fatal("expected partial failure error")
	}
}

func TestDetail_Execute_Batch(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	// m1 succeeds with note and minute
	reg.Register(meetingGetStub("m_batch1", "note_b1"))
	reg.Register(recordingOKStub("m_batch1", "https://meetings.feishu.cn/minutes/obc_b1"))
	// m2 has no note_id but has minute
	reg.Register(meetingGetStub("m_batch2", ""))
	reg.Register(recordingOKStub("m_batch2", "https://meetings.feishu.cn/minutes/obc_b2"))

	err := mountAndRun(t, VCDetail, []string{"+detail", "--meeting-ids", "m_batch1,m_batch2", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	meetings, _ := data["meetings"].([]any)
	if len(meetings) != 2 {
		t.Fatalf("expected 2 meetings, got %d", len(meetings))
	}
}

// ---------------------------------------------------------------------------
// Pure function tests
// ---------------------------------------------------------------------------

func TestFetchMeetingDetail_MeetingWithNoteAndMinute(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingGetStub("m_fn", "note_fn"))
	reg.Register(recordingOKStub("m_fn", "https://meetings.feishu.cn/minutes/obc_fn"))

	if err := botExec(t, "detail-fn", f, func(_ context.Context, rctx *common.RuntimeContext) error {
		result := fetchMeetingDetail(context.Background(), rctx, "m_fn")
		if result.MeetingID != "m_fn" {
			t.Errorf("meeting_id = %v, want m_fn", result.MeetingID)
		}
		if result.NoteID != "note_fn" {
			t.Errorf("note_id = %v, want note_fn", result.NoteID)
		}
		if result.MinuteToken != "obc_fn" {
			t.Errorf("minute_token = %v, want obc_fn", result.MinuteToken)
		}
		if result.Error != "" {
			t.Errorf("unexpected error: %v", result.Error)
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchMeetingDetail_MeetingNotFound(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m_nf",
		Body:   map[string]interface{}{"code": 121004, "msg": "data not found"},
	})

	if err := botExec(t, "detail-nf", f, func(_ context.Context, rctx *common.RuntimeContext) error {
		result := fetchMeetingDetail(context.Background(), rctx, "m_nf")
		if result.Error == "" {
			t.Error("expected error for meeting not found")
		}
		// note_id and minute_token should still be present (empty)
		if result.NoteID != "" {
			t.Errorf("note_id = %q, want empty", result.NoteID)
		}
		if result.MinuteToken != "" {
			t.Errorf("minute_token = %q, want empty", result.MinuteToken)
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchMeetingDetail_RecordingFailsButNoteOK(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingGetStub("m_partial", "note_partial"))
	reg.Register(recordingErrStub("m_partial", 121004, "not found"))

	if err := botExec(t, "detail-partial", f, func(_ context.Context, rctx *common.RuntimeContext) error {
		result := fetchMeetingDetail(context.Background(), rctx, "m_partial")
		if result.NoteID != "note_partial" {
			t.Errorf("note_id = %v, want note_partial", result.NoteID)
		}
		if result.MinuteToken != "" {
			t.Errorf("minute_token = %q, want empty", result.MinuteToken)
		}
		if result.Error != "" {
			t.Errorf("error = %q, want empty", result.Error)
		}
		if !strings.Contains(result.Hint, "no minute file for this meeting") {
			t.Errorf("hint = %q, want contains 'no minute file for this meeting'", result.Hint)
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchMeetingDetail_RecordingAPIErrorButNoteOK(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingGetStub("m_api_err", "note_apierr"))
	reg.Register(recordingErrStub("m_api_err", 99999, "weird API error"))

	if err := botExec(t, "detail-apierr", f, func(_ context.Context, rctx *common.RuntimeContext) error {
		result := fetchMeetingDetail(context.Background(), rctx, "m_api_err")
		if result.NoteID != "note_apierr" {
			t.Errorf("note_id = %v, want note_apierr", result.NoteID)
		}
		if result.MinuteToken != "" {
			t.Errorf("minute_token = %q, want empty", result.MinuteToken)
		}
		if result.Error != "" {
			t.Errorf("error = %q, want empty: a recording lookup failure must not fail the command", result.Error)
		}
		if !strings.Contains(result.Hint, "failed to query minutes") || !strings.Contains(result.Hint, "weird API error") {
			t.Errorf("hint = %q, want contains 'failed to query minutes' and 'weird API error'", result.Hint)
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestFetchMeetingDetail_MeetingInProgress pins the in-progress behavior: when a
// meeting is still ongoing (end_time not after start_time), +detail must not
// call the recording API at all — it returns meeting metadata with an
// informational hint and no error. Deliberately register NO recording stub so
// that any recording call would fail on an unmatched request.
func TestFetchMeetingDetail_MeetingInProgress(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m_live",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"meeting": map[string]interface{}{
				"id":         "m_live",
				"topic":      "Live Meeting",
				"meeting_no": "912052453",
				// end_time == start_time signals an ongoing meeting.
				"start_time": "1752000000",
				"end_time":   "1752000000",
			}},
		},
	})

	if err := botExec(t, "detail-live", f, func(_ context.Context, rctx *common.RuntimeContext) error {
		result := fetchMeetingDetail(context.Background(), rctx, "m_live")
		if result.Topic != "Live Meeting" {
			t.Errorf("topic = %q, want 'Live Meeting'", result.Topic)
		}
		if result.Error != "" {
			t.Errorf("error = %q, want empty for an in-progress meeting", result.Error)
		}
		if result.MinuteToken != "" {
			t.Errorf("minute_token = %q, want empty for an in-progress meeting", result.MinuteToken)
		}
		if !strings.Contains(result.Hint, "in progress") {
			t.Errorf("hint = %q, want to mention the meeting is in progress", result.Hint)
		}
		if strings.Contains(result.Hint, "not found for this meeting") {
			t.Errorf("hint = %q, should not emit not-found noise for an in-progress meeting", result.Hint)
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
