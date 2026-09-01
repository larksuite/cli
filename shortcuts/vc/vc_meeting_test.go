// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// ---------------------------------------------------------------------------
// Unit tests: pure functions
// ---------------------------------------------------------------------------

func TestValidMeetingNumber(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"9 digits", "123456789", true},
		{"9 digits leading zero", "012345678", true},
		{"empty", "", false},
		{"8 digits", "12345678", false},
		{"10 digits", "1234567890", false},
		{"with space", "12345 678", false},
		{"letters mixed", "12345678a", false},
		{"pure letters", "abcdefghi", false},
		{"with dash", "123-456-789", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validMeetingNumber(tt.in); got != tt.want {
				t.Errorf("validMeetingNumber(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildMeetingJoinBody_WithoutPassword(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-number", "", "")
	cmd.Flags().String("password", "", "")
	_ = cmd.Flags().Set("meeting-number", "123456789")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	body := buildMeetingJoinBody(runtime, meetingJoinActionJoin)

	if body.JoinType != 1 {
		t.Errorf("join_type = %v, want 1", body.JoinType)
	}
	if body.JoinIdentify.MeetingNo != "123456789" {
		t.Errorf("meeting_no = %v, want 123456789", body.JoinIdentify.MeetingNo)
	}
	if body.Password != "" {
		t.Errorf("password should be empty, got %q", body.Password)
	}
	if body.Action != nil {
		t.Errorf("default join must not include action, got %d", *body.Action)
	}
}

func TestBuildMeetingJoinBody_WithPassword(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-number", "", "")
	cmd.Flags().String("password", "", "")
	_ = cmd.Flags().Set("meeting-number", "123456789")
	_ = cmd.Flags().Set("password", "secret")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	body := buildMeetingJoinBody(runtime, meetingJoinActionJoin)

	if body.Password != "secret" {
		t.Errorf("password = %v, want secret", body.Password)
	}
}

func TestBuildMeetingJoinBody_TrimsWhitespace(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-number", "", "")
	cmd.Flags().String("password", "", "")
	_ = cmd.Flags().Set("meeting-number", "  123456789  ")
	_ = cmd.Flags().Set("password", "  pw  ")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	body := buildMeetingJoinBody(runtime, meetingJoinActionJoin)

	if body.JoinIdentify.MeetingNo != "123456789" {
		t.Errorf("meeting_no should be trimmed, got %q", body.JoinIdentify.MeetingNo)
	}
	if body.Password != "pw" {
		t.Errorf("password should be trimmed, got %q", body.Password)
	}
}

func TestBuildMeetingJoinBody_WithoutCallID(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-number", "", "")
	cmd.Flags().String("password", "", "")
	cmd.Flags().String("call-id", "", "")
	_ = cmd.Flags().Set("meeting-number", "123456789")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	body := buildMeetingJoinBody(runtime, meetingJoinActionJoin)

	if body.CallID != "" {
		t.Errorf("call_id should be empty, got %q", body.CallID)
	}
}

func TestBuildMeetingJoinBody_StartAction(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-number", "", "")
	cmd.Flags().String("password", "", "")
	cmd.Flags().String("call-id", "", "")
	cmd.Flags().String("action", "", "")
	_ = cmd.Flags().Set("meeting-number", "123456789")
	_ = cmd.Flags().Set("action", "start")

	body := buildMeetingJoinBody(common.TestNewRuntimeContext(cmd, defaultConfig()), meetingJoinActionStart)

	if body.Action == nil || *body.Action != meetingJoinStartAPIFlag {
		t.Errorf("action = %v, want %d", body.Action, meetingJoinStartAPIFlag)
	}
}

func TestMeetingJoinAction(t *testing.T) {
	tests := []struct {
		name   string
		action string
		want   string
	}{
		{name: "start", action: "start", want: meetingJoinActionStart},
		{name: "join", action: "join", want: meetingJoinActionJoin},
		{name: "unexpected defaults to join", action: "other", want: meetingJoinActionJoin},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("action", "", "")
			_ = cmd.Flags().Set("action", tt.action)

			if got := meetingJoinAction(common.TestNewRuntimeContext(cmd, defaultConfig())); got != tt.want {
				t.Fatalf("meetingJoinAction() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVCMeetingJoinNormalizesAction(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("action", "", "")
	_ = cmd.Flags().Set("action", " START ")
	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())

	if err := VCMeetingJoin.Normalize(context.Background(), runtime.FlagContext()); err != nil {
		t.Fatalf("VCMeetingJoin.Normalize() error = %v", err)
	}
	if got := meetingJoinAction(runtime); got != meetingJoinActionStart {
		t.Fatalf("meetingJoinAction() = %q, want %q", got, meetingJoinActionStart)
	}
}

func TestBuildMeetingJoinBody_WithCallID(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-number", "", "")
	cmd.Flags().String("password", "", "")
	cmd.Flags().String("call-id", "", "")
	_ = cmd.Flags().Set("meeting-number", "123456789")
	_ = cmd.Flags().Set("call-id", "a08e06bf-9a41-44e4-a89c-a7871899e783")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	body := buildMeetingJoinBody(runtime, meetingJoinActionJoin)

	if body.CallID != "a08e06bf-9a41-44e4-a89c-a7871899e783" {
		t.Errorf("call_id = %v, want a08e06bf-9a41-44e4-a89c-a7871899e783", body.CallID)
	}
}

func TestBuildMeetingJoinBody_TrimsCallIDWhitespace(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-number", "", "")
	cmd.Flags().String("password", "", "")
	cmd.Flags().String("call-id", "", "")
	_ = cmd.Flags().Set("meeting-number", "123456789")
	_ = cmd.Flags().Set("call-id", "  call-xyz  ")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	body := buildMeetingJoinBody(runtime, meetingJoinActionJoin)

	if body.CallID != "call-xyz" {
		t.Errorf("call_id should be trimmed, got %q", body.CallID)
	}
}

// ---------------------------------------------------------------------------
// Validate tests: VCMeetingJoin
// ---------------------------------------------------------------------------

func TestMeetingJoin_Validate_MissingNumber(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	// cobra MarkFlagRequired should reject missing --meeting-number
	err := mountAndRun(t, VCMeetingJoin, []string{"+meeting-join", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected error when --meeting-number is missing")
	}
	if !strings.Contains(err.Error(), "meeting-number") {
		t.Errorf("error should mention meeting-number, got: %v", err)
	}
}

func TestMeetingJoin_Validate_InvalidFormat(t *testing.T) {
	tests := []struct {
		name string
		num  string
	}{
		{"too short", "12345678"},
		{"too long", "1234567890"},
		{"with letters", "12345abcd"},
		{"empty after trim", "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("meeting-number", "", "")
			cmd.Flags().String("password", "", "")
			_ = cmd.Flags().Set("meeting-number", tt.num)

			runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
			err := VCMeetingJoin.Validate(context.Background(), runtime)
			if err == nil {
				t.Fatalf("expected validation error for %q", tt.num)
			}
			if !strings.Contains(err.Error(), "9 digits") {
				t.Errorf("error should mention '9 digits', got: %v", err)
			}
		})
	}
}

func TestMeetingJoin_Validate_Valid(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-number", "", "")
	cmd.Flags().String("password", "", "")
	_ = cmd.Flags().Set("meeting-number", "123456789")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	if err := VCMeetingJoin.Validate(context.Background(), runtime); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func TestMeetingJoin_Validate_StartRequiresBot(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, VCMeetingJoin, []string{
		"+meeting-join", "--as", "user",
		"--meeting-number", "123456789",
		"--action", "start",
		"--dry-run",
	}, f, nil)

	if err == nil || !strings.Contains(err.Error(), "--action start requires --as bot") {
		t.Fatalf("start action error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// DryRun tests: VCMeetingJoin
// ---------------------------------------------------------------------------

func TestMeetingJoin_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCMeetingJoin, []string{
		"+meeting-join", "--meeting-number", "123456789", "--password", "pw123",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/vc/v1/bots/join") {
		t.Errorf("dry-run should include API path, got: %s", out)
	}
	if !strings.Contains(out, "123456789") {
		t.Errorf("dry-run should include meeting number, got: %s", out)
	}
	if !strings.Contains(out, "pw123") {
		t.Errorf("dry-run should include password, got: %s", out)
	}
}

func TestMeetingJoin_StartAction_Bot(t *testing.T) {
	t.Run("dry run normalizes action", func(t *testing.T) {
		f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
		err := mountAndRun(t, VCMeetingJoin, []string{
			"+meeting-join",
			"--meeting-number", "123456789",
			"--action", " START ",
			"--dry-run",
			"--as", "bot",
		}, f, stdout)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, want := range []string{meetingBotJoinPath, "\"action\"", "2"} {
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("dry-run output missing %q: %s", want, stdout.String())
			}
		}
	})

	t.Run("execute", func(t *testing.T) {
		f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
		stub := &httpmock.Stub{
			Method: "POST",
			URL:    meetingBotJoinPath,
			Body: map[string]interface{}{
				"code": 0,
				"msg":  "ok",
				"data": map[string]interface{}{
					"meeting": map[string]interface{}{
						"id":         "69999999",
						"meeting_no": "123456789",
						"topic":      "Calendar meeting",
						"start_time": "1700000000",
					},
				},
			},
		}
		reg.Register(stub)

		err := mountAndRun(t, VCMeetingJoin, []string{
			"+meeting-join",
			"--meeting-number", "123456789",
			"--action", "start",
			"--format", "pretty",
			"--as", "bot",
		}, f, stdout)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		reg.Verify(t)

		var body map[string]interface{}
		if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
			t.Fatalf("decode captured request body: %v", err)
		}
		if body["action"] != float64(meetingJoinStartAPIFlag) {
			t.Fatalf("request action = %#v, want %d", body["action"], meetingJoinStartAPIFlag)
		}
		for _, want := range []string{"Started Calendar meeting.", "69999999", "123456789", "Calendar meeting", "1700000000"} {
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("pretty output missing %q: %s", want, stdout.String())
			}
		}
	})
}

func TestMeetingJoin_StartAction_NoMeetingInfo(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    meetingBotJoinPath,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	})

	err := mountAndRun(t, VCMeetingJoin, []string{
		"+meeting-join",
		"--meeting-number", "123456789",
		"--action", "start",
		"--format", "pretty",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)
	if !strings.Contains(stdout.String(), "Started Calendar meeting (no meeting info returned).") {
		t.Fatalf("pretty output = %s", stdout.String())
	}
}

func TestPrintMeetingSummaryWithoutMeeting(t *testing.T) {
	var out strings.Builder
	printMeetingSummary(&out, map[string]interface{}{})
	if got := out.String(); got != "" {
		t.Fatalf("printMeetingSummary() = %q, want empty output", got)
	}
}

// ---------------------------------------------------------------------------
// Execute tests: VCMeetingJoin
// ---------------------------------------------------------------------------

func TestMeetingJoin_Execute_Success(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/vc/v1/bots/join",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"meeting": map[string]interface{}{
					"id":         "69999999",
					"meeting_no": "123456789",
					"topic":      "Weekly Sync",
					"start_time": "1700000000",
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, VCMeetingJoin, []string{
		"+meeting-join", "--meeting-number", "123456789",
		"--format", "json", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// verify captured request body
	if len(stub.CapturedBody) == 0 {
		t.Fatal("expected request body to be captured")
	}
	var req map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &req); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	if req["join_type"].(float64) != 1 {
		t.Errorf("join_type = %v, want 1", req["join_type"])
	}
	if _, exists := req["action"]; exists {
		t.Errorf("default join must not include action, got %v", req["action"])
	}
	ji, _ := req["join_identify"].(map[string]interface{})
	if ji["meeting_no"] != "123456789" {
		t.Errorf("meeting_no = %v, want 123456789", ji["meeting_no"])
	}
	if _, exists := ji["password"]; exists {
		t.Errorf("password should be omitted when not provided, got %v", ji["password"])
	}

	// verify response envelope carries meeting info under data.meeting
	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse stdout: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	meeting, _ := data["meeting"].(map[string]any)
	if meeting["id"] != "69999999" {
		t.Errorf("meeting.id = %v, want 69999999 (envelope: %s)", meeting["id"], stdout.String())
	}
}

func TestMeetingJoin_Execute_WithPassword_CapturesBody(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/vc/v1/bots/join",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, VCMeetingJoin, []string{
		"+meeting-join", "--meeting-number", "987654321", "--password", "s3cret",
		"--format", "json", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var req map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &req); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	ji, _ := req["join_identify"].(map[string]interface{})
	if req["password"] != "s3cret" {
		t.Errorf("password = %v, want s3cret", req["password"])
	}
	if ji["meeting_no"] != "987654321" {
		t.Errorf("meeting_no = %v, want 987654321", ji["meeting_no"])
	}
}

func TestMeetingJoin_Execute_PrettyOutput(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/vc/v1/bots/join",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"meeting": map[string]interface{}{
					"id":         "69999999",
					"meeting_no": "123456789",
					"topic":      "Weekly Sync",
					"start_time": "1700000000",
				},
			},
		},
	})

	err := mountAndRun(t, VCMeetingJoin, []string{
		"+meeting-join", "--meeting-number", "123456789",
		"--format", "pretty", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"Joined meeting successfully", "69999999", "123456789", "Weekly Sync", "1700000000"} {
		if !strings.Contains(out, want) {
			t.Errorf("pretty output missing %q, got: %s", want, out)
		}
	}
}

func TestMeetingJoin_Execute_PrettyOutput_NoMeetingInfo(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/vc/v1/bots/join",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRun(t, VCMeetingJoin, []string{
		"+meeting-join", "--meeting-number", "123456789",
		"--format", "pretty", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "no meeting info returned") {
		t.Errorf("pretty output should fall back to 'no meeting info' notice, got: %s", stdout.String())
	}
}

func TestMeetingLeave_Execute_PrettyOutput(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/vc/v1/bots/leave",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRun(t, VCMeetingLeave, []string{
		"+meeting-leave", "--meeting-id", "69999999",
		"--format", "pretty", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Left meeting 69999999 successfully") {
		t.Errorf("pretty output should confirm leave, got: %s", out)
	}
}

func TestMeetingJoin_Execute_APIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/vc/v1/bots/join",
		Body:   map[string]interface{}{"code": 190001, "msg": "invalid meeting number"},
	})

	err := mountAndRun(t, VCMeetingJoin, []string{
		"+meeting-join", "--meeting-number", "123456789",
		"--as", "user",
	}, f, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
	if !strings.Contains(err.Error(), "invalid meeting number") {
		t.Errorf("error should surface API message, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Validate tests: VCMeetingLeave
// ---------------------------------------------------------------------------

func TestMeetingLeave_Validate_MissingID(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCMeetingLeave, []string{"+meeting-leave", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected error when --meeting-id is missing")
	}
	if !strings.Contains(err.Error(), "meeting-id") {
		t.Errorf("error should mention meeting-id, got: %v", err)
	}
}

func TestMeetingLeave_Validate_WhitespaceOnly(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-id", "", "")
	_ = cmd.Flags().Set("meeting-id", "   ")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	err := VCMeetingLeave.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected error for whitespace-only meeting-id")
	}
	if !strings.Contains(err.Error(), "meeting-id") {
		t.Errorf("error should mention meeting-id, got: %v", err)
	}
}

func TestMeetingLeave_Validate_Valid(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-id", "", "")
	_ = cmd.Flags().Set("meeting-id", "69999999")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	if err := VCMeetingLeave.Validate(context.Background(), runtime); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DryRun tests: VCMeetingLeave
// ---------------------------------------------------------------------------

func TestMeetingLeave_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCMeetingLeave, []string{
		"+meeting-leave", "--meeting-id", "69999999",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/vc/v1/bots/leave") {
		t.Errorf("dry-run should include API path, got: %s", out)
	}
	if !strings.Contains(out, "69999999") {
		t.Errorf("dry-run should include meeting-id, got: %s", out)
	}
}

// ---------------------------------------------------------------------------
// Execute tests: VCMeetingLeave
// ---------------------------------------------------------------------------

func TestMeetingLeave_Execute_Success(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/vc/v1/bots/leave",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, VCMeetingLeave, []string{
		"+meeting-leave", "--meeting-id", "69999999",
		"--format", "json", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// verify captured request body
	var req map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &req); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	if req["meeting_id"] != "69999999" {
		t.Errorf("meeting_id = %v, want 69999999", req["meeting_id"])
	}
}

func TestMeetingLeave_Execute_TrimsMeetingID(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/vc/v1/bots/leave",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, VCMeetingLeave, []string{
		"+meeting-leave", "--meeting-id", "  69999999  ",
		"--format", "json", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var req map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &req); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	if req["meeting_id"] != "69999999" {
		t.Errorf("meeting_id should be trimmed, got %q", req["meeting_id"])
	}
}

func TestMeetingLeave_Execute_APIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/vc/v1/bots/leave",
		Body:   map[string]interface{}{"code": 121005, "msg": "no permission"},
	})

	err := mountAndRun(t, VCMeetingLeave, []string{
		"+meeting-leave", "--meeting-id", "69999999", "--as", "user",
	}, f, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
	// code 121005 classifies to a typed permission error (no edit/view rights).
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed errs.* error, got %T: %v", err, err)
	}
	if p.Subtype != errs.SubtypePermissionDenied {
		t.Errorf("subtype = %q, want %q", p.Subtype, errs.SubtypePermissionDenied)
	}
}

func TestMeetingListActive_DryRun_UserIdentity(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCMeetingListActive, []string{
		"+meeting-list-active", "--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/vc/v1/bots/user_active_meeting") {
		t.Errorf("dry-run should include API path, got: %s", out)
	}
	if strings.Contains(out, "user_id") {
		t.Errorf("user identity should not send user_id by default, got: %s", out)
	}
}

func TestMeetingListActive_UsesUserScopePreflightAndBotScopeHint(t *testing.T) {
	if got := VCMeetingListActive.ScopesForIdentity("user"); len(got) != 1 || got[0] != meetingQueryUserScope {
		t.Fatalf("ScopesForIdentity(user) = %v, want [%s]", got, meetingQueryUserScope)
	}
	if got := VCMeetingListActive.ScopesForIdentity("bot"); len(got) != 0 {
		t.Fatalf("ScopesForIdentity(bot) = %v, want no bot preflight scopes", got)
	}
	if got := VCMeetingListActive.DeclaredScopesForIdentity("user"); len(got) != 1 || got[0] != meetingQueryUserScope {
		t.Fatalf("DeclaredScopesForIdentity(user) = %v, want [%s]", got, meetingQueryUserScope)
	}
	if got := VCMeetingListActive.DeclaredScopesForIdentity("bot"); len(got) != 1 || got[0] != meetingQueryBotScope {
		t.Fatalf("DeclaredScopesForIdentity(bot) = %v, want [%s]", got, meetingQueryBotScope)
	}
}

func TestMeetingListActive_DryRun_UserIdentityIgnoresUserID(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCMeetingListActive, []string{
		"+meeting-list-active", "--dry-run", "--as", "user", "--user-id", "not-open-id",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stdout.String(), "user_id") {
		t.Errorf("user identity should not send user_id, got: %s", stdout.String())
	}
}

func TestMeetingListActive_Execute_UserIdentityIgnoresInvalidUserID(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	var gotUserID string
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/bots/user_active_meeting",
		OnMatch: func(req *http.Request) {
			gotUserID = req.URL.Query().Get("user_id")
		},
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"meetings": []interface{}{}},
		},
	})

	err := mountAndRun(t, VCMeetingListActive, []string{
		"+meeting-list-active", "--as", "user", "--user-id", "not-open-id", "--format", "json",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUserID != "" {
		t.Fatalf("user identity should not send user_id, got %q", gotUserID)
	}
}

func TestMeetingListActive_Validate_BotRequiresUserID(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCMeetingListActive, []string{"+meeting-list-active", "--as", "bot"}, f, nil)
	if err == nil {
		t.Fatal("expected error when --as bot omits --user-id")
	}
	assertMeetingListActiveUserIDValidationError(t, err)
}

func TestMeetingListActive_Validate_UserIDOpenIDFormat(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCMeetingListActive, []string{
		"+meeting-list-active", "--as", "bot", "--user-id", "300",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error for non-open_id user-id")
	}
	assertMeetingListActiveUserIDValidationError(t, err)
}

func TestMeetingListActive_Execute_BotPassesUserID(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	var gotUserID string
	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/bots/user_active_meeting",
		OnMatch: func(req *http.Request) {
			gotUserID = req.URL.Query().Get("user_id")
		},
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"meetings": []interface{}{
					map[string]interface{}{
						"meeting_id":    "9001",
						"meeting_no":    "123456789",
						"meeting_title": "Standup",
					},
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, VCMeetingListActive, []string{
		"+meeting-list-active", "--user-id", "ou_300",
		"--format", "json", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUserID != "ou_300" {
		t.Fatalf("user_id query = %q, want ou_300", gotUserID)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse stdout: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	meetings, _ := data["meetings"].([]any)
	if len(meetings) != 1 {
		t.Fatalf("meetings = %d, want 1 (envelope: %s)", len(meetings), stdout.String())
	}
}

func TestMeetingListActive_DryRun_BotValidationErrorEnvelope(t *testing.T) {
	cmd := &cobra.Command{Use: "+meeting-list-active"}
	cmd.Flags().String("user-id", "", "")
	runtime := common.TestNewRuntimeContextWithIdentity(cmd, defaultConfig(), core.AsBot)

	dry := VCMeetingListActive.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}
	raw, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("failed to marshal dry-run output: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "--user-id") {
		t.Fatalf("dry-run error = %q, want user-id validation", got)
	}
}

func TestMeetingListActive_DryRun_BotSendsUserID(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCMeetingListActive, []string{
		"+meeting-list-active", "--dry-run", "--as", "bot", "--user-id", "ou_300",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "user_id") || !strings.Contains(stdout.String(), "ou_300") {
		t.Fatalf("dry-run should include user_id=ou_300, got: %s", stdout.String())
	}
}

func TestMeetingListActive_Execute_ValidationError(t *testing.T) {
	cmd := &cobra.Command{Use: "+meeting-list-active"}
	cmd.Flags().String("user-id", "", "")
	runtime := common.TestNewRuntimeContextWithIdentity(cmd, defaultConfig(), core.AsBot)

	err := VCMeetingListActive.Execute(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error")
	}
	assertMeetingListActiveUserIDValidationError(t, err)
}

func TestMeetingListActive_ExecutePretty_Empty(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/bots/user_active_meeting",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})

	err := mountAndRun(t, VCMeetingListActive, []string{
		"+meeting-list-active", "--format", "pretty", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "No active meetings.") {
		t.Fatalf("pretty output = %q, want empty-state message", stdout.String())
	}
}

func TestMeetingListActive_ExecutePretty_SingleMeetingNoSelectionPrompt(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/bots/user_active_meeting",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"meetings": []interface{}{
					map[string]interface{}{
						"meeting_id":    "9001",
						"meeting_no":    "123456789",
						"meeting_title": "Standup",
					},
				},
			},
		},
	})

	err := mountAndRun(t, VCMeetingListActive, []string{
		"+meeting-list-active", "--format", "pretty", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"Standup", "Meeting ID:  9001", "Meeting No:  123456789"} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty output missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "Multiple active meetings found") {
		t.Fatalf("single meeting should not show selection prompt: %s", out)
	}
}

func TestMeetingListActive_ExecutePretty_MultipleMeetings(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/bots/user_active_meeting",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"meetings": []interface{}{
					map[string]interface{}{
						"meeting_id":    "9001",
						"meeting_no":    "123456789",
						"meeting_title": "Standup",
					},
					"ignored",
					map[string]interface{}{
						"meeting_id":    "9002",
						"meeting_no":    "987654321",
						"meeting_title": "Planning",
					},
					map[string]interface{}{
						"meeting_id": "9003",
					},
				},
			},
		},
	})

	err := mountAndRun(t, VCMeetingListActive, []string{
		"+meeting-list-active", "--format", "pretty", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"Standup",
		"Meeting ID:  9001",
		"Meeting No:  123456789",
		"Planning",
		"Meeting ID:  9002",
		"Meeting No:  987654321",
		"Untitled meeting",
		"Meeting ID:  9003",
		"Multiple active meetings found. Ask the user to choose one meeting_id before calling +meeting-events.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty output missing %q: %s", want, out)
		}
	}
}

func TestMeetingListActive_Execute_APIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/bots/user_active_meeting",
		Body:   map[string]interface{}{"code": 121005, "msg": "no permission"},
	})

	err := mountAndRun(t, VCMeetingListActive, []string{
		"+meeting-list-active", "--format", "json", "--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected API error")
	}
	if p, ok := errs.ProblemOf(err); !ok || p.Category != errs.CategoryAuthorization {
		t.Fatalf("error problem = (%+v, %t), want authorization problem", p, ok)
	} else if p.Subtype != errs.SubtypePermissionDenied {
		t.Fatalf("error subtype = %q, want %q", p.Subtype, errs.SubtypePermissionDenied)
	} else if p.Code != 121005 {
		t.Fatalf("error code = %d, want 121005", p.Code)
	}
	var pe *errs.PermissionError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *errs.PermissionError, got %T: %v", err, err)
	}
}

func assertMeetingListActiveUserIDValidationError(t *testing.T, err error) {
	t.Helper()
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryValidation {
		t.Errorf("Category = %q, want %q", p.Category, errs.CategoryValidation)
	}
	if p.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidArgument)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Param != "--user-id" {
		t.Errorf("Param = %q, want %q", ve.Param, "--user-id")
	}
}

// ---------------------------------------------------------------------------
// Typed error lock assertions
// ---------------------------------------------------------------------------

func TestMeetingJoin_Validate_InvalidFormat_TypedError(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-number", "", "")
	cmd.Flags().String("password", "", "")
	_ = cmd.Flags().Set("meeting-number", "12345678") // 8 digits — invalid

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	err := VCMeetingJoin.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "9 digits") {
		t.Errorf("message mismatch: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != "--meeting-number" {
		t.Errorf("Param = %q, want %q", ve.Param, "--meeting-number")
	}
}

func TestMeetingLeave_Validate_WhitespaceOnly_TypedError(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-id", "", "")
	_ = cmd.Flags().Set("meeting-id", "   ")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	err := VCMeetingLeave.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "meeting-id") {
		t.Errorf("message mismatch: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != "--meeting-id" {
		t.Errorf("Param = %q, want %q", ve.Param, "--meeting-id")
	}
}

// ---------------------------------------------------------------------------
// Agent bot actions: +meeting-invite / +meeting-end
// ---------------------------------------------------------------------------

func TestVCMeetingInviteNormalizesTypeBeforeEnumValidation(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("type", "", "")
	_ = cmd.Flags().Set("type", " selected ")
	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())

	if err := VCMeetingInvite.Normalize(context.Background(), runtime.FlagContext()); err != nil {
		t.Fatalf("VCMeetingInvite.Normalize() error = %v", err)
	}
	if got := runtime.Str("type"); got != meetingInviteTypeSelected {
		t.Fatalf("normalized type = %q, want %q", got, meetingInviteTypeSelected)
	}
}

func TestVCMeetingInviteNormalizeRejectsBlankType(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("type", "", "")
	_ = cmd.Flags().Set("type", " ")

	err := VCMeetingInvite.Normalize(context.Background(), common.TestNewRuntimeContext(cmd, defaultConfig()).FlagContext())
	if err == nil || !strings.Contains(err.Error(), "--type is required") {
		t.Fatalf("Normalize() error = %v, want --type is required", err)
	}
}

func TestBuildMeetingInviteBodySelectedUsesInvitees(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-id", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().StringSlice("open-ids", nil, "")
	_ = cmd.Flags().Set("meeting-id", " 7628568141510692381 ")
	_ = cmd.Flags().Set("type", meetingInviteTypeSelected)
	_ = cmd.Flags().Set("open-ids", "ou_a,ou_b,ou_a")

	body := buildMeetingInviteBody(common.TestNewRuntimeContext(cmd, defaultConfig()))
	if body.MeetingID != "7628568141510692381" || body.InviteType != meetingInviteTypeSelectedValue {
		t.Fatalf("body = %#v", body)
	}
	wantInvitees := []meetingInvitee{
		{ID: "ou_a", UserType: meetingInviteeUserType},
		{ID: "ou_b", UserType: meetingInviteeUserType},
	}
	if !reflect.DeepEqual(body.Invitees, wantInvitees) {
		t.Fatalf("invitees = %#v", body.Invitees)
	}
	if !reflect.DeepEqual(buildMeetingInviteParams(), map[string]interface{}{"user_id_type": "open_id"}) {
		t.Fatalf("invite params = %#v", buildMeetingInviteParams())
	}
}

func TestBuildMeetingInviteBodyAllSuggestedOmitsOpenIDs(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-id", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().StringSlice("open-ids", nil, "")
	_ = cmd.Flags().Set("meeting-id", "7628568141510692381")
	_ = cmd.Flags().Set("type", meetingInviteTypeAllSuggested)

	body := buildMeetingInviteBody(common.TestNewRuntimeContext(cmd, defaultConfig()))
	if body.InviteType != meetingInviteTypeAllValue {
		t.Fatalf("invite_type = %#v", body.InviteType)
	}
	if len(body.Invitees) != 0 {
		t.Fatalf("ALL_SUGGESTED body must omit invitees: %#v", body)
	}
}

func TestBuildMeetingInviteBodyRejectsNonUserOpenID(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-id", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().StringSlice("open-ids", nil, "")
	_ = cmd.Flags().Set("meeting-id", "7628568141510692381")
	_ = cmd.Flags().Set("type", meetingInviteTypeSelected)
	_ = cmd.Flags().Set("open-ids", "oc_chat")

	err := validateMeetingInviteFlags(common.TestNewRuntimeContext(cmd, defaultConfig()))

	if err == nil || !strings.Contains(err.Error(), "ou_xxx") {
		t.Fatalf("error = %v, want user open_id validation", err)
	}
}

func TestBuildMeetingInviteBodyRejectsInvalidCombinations(t *testing.T) {
	tooManyOpenIDs := make([]string, meetingInviteeLimit+1)
	for i := range tooManyOpenIDs {
		tooManyOpenIDs[i] = fmt.Sprintf("ou_%d", i)
	}

	tests := []struct {
		name       string
		meetingID  string
		inviteType string
		openIDs    []string
		wantErr    string
	}{
		{
			name:       "missing meeting ID",
			meetingID:  "  ",
			inviteType: meetingInviteTypeSelected,
			openIDs:    []string{"ou_a"},
			wantErr:    "--meeting-id is required",
		},
		{
			name:       "non-numeric meeting ID",
			meetingID:  "6999a999",
			inviteType: meetingInviteTypeSelected,
			openIDs:    []string{"ou_a"},
			wantErr:    "--meeting-id must be a positive integer",
		},
		{
			name:       "selected without open IDs",
			meetingID:  "7628568141510692381",
			inviteType: meetingInviteTypeSelected,
			wantErr:    "--open-ids is required",
		},
		{
			name:       "selected with too many open IDs",
			meetingID:  "7628568141510692381",
			inviteType: meetingInviteTypeSelected,
			openIDs:    tooManyOpenIDs,
			wantErr:    "at most 200 users",
		},
		{
			name:       "all suggested with open IDs",
			meetingID:  "7628568141510692381",
			inviteType: meetingInviteTypeAllSuggested,
			openIDs:    []string{"ou_a"},
			wantErr:    "must not be set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("meeting-id", "", "")
			cmd.Flags().String("type", "", "")
			cmd.Flags().StringSlice("open-ids", nil, "")
			_ = cmd.Flags().Set("meeting-id", tt.meetingID)
			_ = cmd.Flags().Set("type", tt.inviteType)
			if tt.openIDs != nil {
				_ = cmd.Flags().Set("open-ids", strings.Join(tt.openIDs, ","))
			}

			runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
			err := validateMeetingIDFlag(runtime.Str("meeting-id"))
			if err == nil {
				err = validateMeetingInviteFlags(runtime)
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validation error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeMeetingInviteOpenIDs(t *testing.T) {
	got := normalizeMeetingInviteOpenIDs([]string{"", " ou_a ", "ou_a", "ou_b", "  "})
	want := []string{"ou_a", "ou_b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeMeetingInviteOpenIDs() = %#v, want %#v", got, want)
	}
}

func TestMeetingInvite_DryRun(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	apiCalled := false
	reg.Register(&httpmock.Stub{
		Method:   http.MethodPost,
		URL:      meetingBotInvitePath,
		Optional: true,
		OnMatch: func(*http.Request) {
			apiCalled = true
		},
	})

	err := mountAndRun(t, VCMeetingInvite, []string{
		"+meeting-invite",
		"--meeting-id", "7628568141510692381",
		"--type", " selected ",
		"--open-ids", "ou_a,ou_b,ou_a",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if apiCalled {
		t.Fatal("dry-run must not call the invite API")
	}

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-id", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().StringSlice("open-ids", nil, "")
	_ = cmd.Flags().Set("meeting-id", "7628568141510692381")
	_ = cmd.Flags().Set("type", meetingInviteTypeSelected)
	_ = cmd.Flags().Set("open-ids", "ou_a,ou_b")
	raw, marshalErr := json.Marshal(VCMeetingInvite.DryRun(context.Background(), common.TestNewRuntimeContext(cmd, defaultConfig())))
	if marshalErr != nil {
		t.Fatalf("marshal dry-run: %v", marshalErr)
	}
	var payload struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Params map[string]interface{} `json:"params"`
			Body   meetingInviteRequest   `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode dry-run: %v", err)
	}
	if len(payload.API) != 1 {
		t.Fatalf("dry-run API count = %d, want 1", len(payload.API))
	}
	call := payload.API[0]
	if call.Method != http.MethodPost || call.URL != meetingBotInvitePath {
		t.Fatalf("dry-run endpoint = %s %s", call.Method, call.URL)
	}
	if call.Params["user_id_type"] != "open_id" {
		t.Fatalf("dry-run params = %#v", call.Params)
	}
	if call.Body.MeetingID != "7628568141510692381" || call.Body.InviteType != meetingInviteTypeSelectedValue || len(call.Body.Invitees) != 2 {
		t.Fatalf("dry-run body = %#v", call.Body)
	}

	out := stdout.String()
	for _, want := range []string{meetingBotInvitePath, "user_id_type", "open_id", "7628568141510692381", "invite_type"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q: %s", want, out)
		}
	}
}

func TestMeetingInvite_ExecuteSelectedPrettyOutput(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	var gotUserIDType string
	stub := &httpmock.Stub{
		Method: http.MethodPost,
		URL:    meetingBotInvitePath,
		OnMatch: func(req *http.Request) {
			gotUserIDType = req.URL.Query().Get("user_id_type")
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"failed_count":  1,
				"invited_count": 2,
				"has_more":      true,
				"invite_results": []interface{}{
					map[string]interface{}{"id": "ou_a", "status": 1},
					map[string]interface{}{"id": "ou_b", "status": 2},
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, VCMeetingInvite, []string{
		"+meeting-invite",
		"--meeting-id", "7628568141510692381",
		"--type", "selected",
		"--open-ids", "ou_a,ou_b,ou_a",
		"--format", "pretty",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	if gotUserIDType != "open_id" {
		t.Fatalf("user_id_type = %q, want open_id", gotUserIDType)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode captured request body: %v", err)
	}
	if body["meeting_id"] != "7628568141510692381" || body["invite_type"] != float64(meetingInviteTypeSelectedValue) {
		t.Fatalf("request body = %#v", body)
	}
	invitees, ok := body["invitees"].([]interface{})
	if !ok || len(invitees) != 2 {
		t.Fatalf("request invitees = %#v, want two deduplicated users", body["invitees"])
	}
	for _, want := range []string{
		"Invite request sent.",
		"Failed:   1",
		"Invited:  2",
		meetingInviteCandidateLimitNotice,
		"ou_a: invited",
		"ou_b: failed",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("pretty output missing %q: %s", want, stdout.String())
		}
	}
}

func TestBuildMeetingInviteOutputReplacesHasMoreWithLimitNotice(t *testing.T) {
	data := map[string]interface{}{
		"failed_count":  1,
		"has_more":      true,
		"invited_count": 2,
	}

	output := buildMeetingInviteOutput(data)
	if _, ok := output["has_more"]; ok {
		t.Fatalf("output must not expose has_more: %#v", output)
	}
	if got := common.GetString(output, "notice"); got != meetingInviteCandidateLimitNotice {
		t.Fatalf("output notice = %q, want %q", got, meetingInviteCandidateLimitNotice)
	}
	if !common.GetBool(data, "has_more") {
		t.Fatalf("source data must not be mutated: %#v", data)
	}
}

func TestPrintMeetingInviteResultWithoutOptionalFields(t *testing.T) {
	var out strings.Builder
	printMeetingInviteResult(&out, map[string]interface{}{})
	if got, want := out.String(), "Invite request sent.\n"; got != want {
		t.Fatalf("printMeetingInviteResult() = %q, want %q", got, want)
	}
}

func TestMeetingInviteValidateRejectsInvalidMeetingID(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-id", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().StringSlice("open-ids", nil, "")
	_ = cmd.Flags().Set("meeting-id", "invalid")
	_ = cmd.Flags().Set("type", meetingInviteTypeSelected)
	_ = cmd.Flags().Set("open-ids", "ou_a")
	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())

	if err := VCMeetingInvite.Validate(context.Background(), runtime); err == nil || !strings.Contains(err.Error(), "--meeting-id must be a positive integer") {
		t.Fatalf("validate error = %v", err)
	}
}

func TestMeetingInvite_ExecuteHandlesAPIErrorAndEmptyData(t *testing.T) {
	t.Run("API error", func(t *testing.T) {
		f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
		reg.Register(&httpmock.Stub{
			Method: http.MethodPost,
			URL:    meetingBotInvitePath,
			Body:   map[string]interface{}{"code": 121005, "msg": "no permission"},
		})

		err := mountAndRun(t, VCMeetingInvite, []string{
			"+meeting-invite",
			"--meeting-id", "7628568141510692381",
			"--type", meetingInviteTypeAllSuggested,
			"--as", "bot",
		}, f, stdout)
		if err == nil {
			t.Fatalf("execute error = %v", err)
		}
		reg.Verify(t)
	})

	t.Run("empty data", func(t *testing.T) {
		f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
		reg.Register(&httpmock.Stub{
			Method: http.MethodPost,
			URL:    meetingBotInvitePath,
			Body:   map[string]interface{}{"code": 0, "msg": "ok"},
		})

		err := mountAndRun(t, VCMeetingInvite, []string{
			"+meeting-invite",
			"--meeting-id", "7628568141510692381",
			"--type", meetingInviteTypeAllSuggested,
			"--format", "pretty",
			"--as", "bot",
		}, f, stdout)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		reg.Verify(t)
		if !strings.Contains(stdout.String(), "Invite request sent.") {
			t.Fatalf("pretty output = %s", stdout.String())
		}
	})
}

func TestBuildMeetingEndBody(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-id", "", "")
	_ = cmd.Flags().Set("meeting-id", " 7628568141510692381 ")

	body := buildMeetingEndBody(common.TestNewRuntimeContext(cmd, defaultConfig()))
	if body.MeetingID != "7628568141510692381" {
		t.Fatalf("body = %#v", body)
	}
}

func TestValidateMeetingIDFlag(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr string
	}{
		{name: "positive integer", value: "7628568141510692381"},
		{name: "leading zero long meeting ID", value: "0007628568141510692381"},
		{name: "missing", value: " ", wantErr: "--meeting-id is required"},
		{name: "zero", value: "0", wantErr: "--meeting-id must be a positive integer"},
		{name: "non-numeric", value: "699a9999", wantErr: "--meeting-id must be a positive integer"},
		{name: "overflow", value: "9223372036854775808", wantErr: "--meeting-id must be a positive integer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMeetingIDFlag(tt.value)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateMeetingIDFlag(%q) error = %v", tt.value, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateMeetingIDFlag(%q) error = %v, want %q", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestMeetingEndValidateRejectsInvalidMeetingID(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-id", "", "")
	_ = cmd.Flags().Set("meeting-id", "invalid")
	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())

	if err := VCMeetingEnd.Validate(context.Background(), runtime); err == nil || !strings.Contains(err.Error(), "--meeting-id must be a positive integer") {
		t.Fatalf("validate error = %v", err)
	}
}

func TestMeetingEnd_DryRunAndExecutePrettyOutput(t *testing.T) {
	t.Run("dry run", func(t *testing.T) {
		f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
		err := mountAndRun(t, VCMeetingEnd, []string{
			"+meeting-end",
			"--meeting-id", "7628568141510692381",
			"--dry-run",
			"--as", "bot",
		}, f, stdout)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, want := range []string{meetingBotEndPath, "7628568141510692381"} {
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("dry-run output missing %q: %s", want, stdout.String())
			}
		}
	})

	t.Run("execute", func(t *testing.T) {
		f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
		stub := &httpmock.Stub{
			Method: http.MethodPost,
			URL:    meetingBotEndPath,
			Body: map[string]interface{}{
				"code": 0,
				"msg":  "ok",
				"data": map[string]interface{}{},
			},
		}
		reg.Register(stub)

		err := mountAndRun(t, VCMeetingEnd, []string{
			"+meeting-end",
			"--meeting-id", " 7628568141510692381 ",
			"--format", "pretty",
			"--as", "bot",
			"--yes",
		}, f, stdout)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		reg.Verify(t)

		var body map[string]interface{}
		if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
			t.Fatalf("decode captured request body: %v", err)
		}
		if !reflect.DeepEqual(body, map[string]interface{}{"meeting_id": "7628568141510692381"}) {
			t.Fatalf("request body = %#v", body)
		}
		if !strings.Contains(stdout.String(), "Ended meeting 7628568141510692381.") {
			t.Fatalf("pretty output = %s", stdout.String())
		}
	})
}

func TestMeetingEnd_ExecuteHandlesAPIErrorAndEmptyData(t *testing.T) {
	t.Run("API error", func(t *testing.T) {
		f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
		reg.Register(&httpmock.Stub{
			Method: http.MethodPost,
			URL:    meetingBotEndPath,
			Body:   map[string]interface{}{"code": 121005, "msg": "no permission"},
		})

		err := mountAndRun(t, VCMeetingEnd, []string{
			"+meeting-end",
			"--meeting-id", "7628568141510692381",
			"--as", "bot",
			"--yes",
		}, f, stdout)
		if err == nil {
			t.Fatalf("execute error = %v", err)
		}
		reg.Verify(t)
	})

	t.Run("empty data", func(t *testing.T) {
		f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
		reg.Register(&httpmock.Stub{
			Method: http.MethodPost,
			URL:    meetingBotEndPath,
			Body:   map[string]interface{}{"code": 0, "msg": "ok"},
		})

		err := mountAndRun(t, VCMeetingEnd, []string{
			"+meeting-end",
			"--meeting-id", "7628568141510692381",
			"--format", "pretty",
			"--as", "bot",
			"--yes",
		}, f, stdout)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		reg.Verify(t)
		if !strings.Contains(stdout.String(), "Ended meeting 7628568141510692381.") {
			t.Fatalf("pretty output = %s", stdout.String())
		}
	})
}

func TestVCMeetingEndUsesManageScope(t *testing.T) {
	if !reflect.DeepEqual(VCMeetingEnd.Scopes, []string{"vc:meeting.bot.manage:write"}) {
		t.Fatalf("VCMeetingEnd.Scopes = %v", VCMeetingEnd.Scopes)
	}
}
