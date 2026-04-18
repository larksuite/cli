// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
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
	body := buildMeetingJoinBody(runtime)

	if body["join_type"] != 1 {
		t.Errorf("join_type = %v, want 1", body["join_type"])
	}
	ji, ok := body["join_identify"].(map[string]interface{})
	if !ok {
		t.Fatalf("join_identify missing or wrong type: %v", body["join_identify"])
	}
	if ji["meeting_no"] != "123456789" {
		t.Errorf("meeting_no = %v, want 123456789", ji["meeting_no"])
	}
	if _, exists := body["password"]; exists {
		t.Errorf("password should be omitted when empty, got %v", body["password"])
	}
}

func TestBuildMeetingJoinBody_WithPassword(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-number", "", "")
	cmd.Flags().String("password", "", "")
	_ = cmd.Flags().Set("meeting-number", "123456789")
	_ = cmd.Flags().Set("password", "secret")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	body := buildMeetingJoinBody(runtime)

	if body["password"] != "secret" {
		t.Errorf("password = %v, want secret", body["password"])
	}
}

func TestBuildMeetingJoinBody_TrimsWhitespace(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-number", "", "")
	cmd.Flags().String("password", "", "")
	_ = cmd.Flags().Set("meeting-number", "  123456789  ")
	_ = cmd.Flags().Set("password", "  pw  ")

	runtime := common.TestNewRuntimeContext(cmd, defaultConfig())
	body := buildMeetingJoinBody(runtime)

	ji, _ := body["join_identify"].(map[string]interface{})
	if ji["meeting_no"] != "123456789" {
		t.Errorf("meeting_no should be trimmed, got %q", ji["meeting_no"])
	}
	if body["password"] != "pw" {
		t.Errorf("password should be trimmed, got %q", body["password"])
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
	if !strings.Contains(err.Error(), "no permission") {
		t.Errorf("error should surface API message, got: %v", err)
	}
}
