// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	parent := &cobra.Command{Use: "vc"}
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

func meetingGetStub(meetingID, noteID string) *httpmock.Stub {
	meeting := map[string]interface{}{
		"id":    meetingID,
		"topic": "Test Meeting",
	}
	if noteID != "" {
		meeting["note_id"] = noteID
	}
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/" + meetingID,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"meeting": meeting},
		},
	}
}

func noteDetailStub(noteID string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/notes/" + noteID,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"note": map[string]interface{}{
					"creator_id":  "ou_creator",
					"create_time": "1700000000",
					"artifacts": []interface{}{
						map[string]interface{}{"doc_token": "doc_main", "artifact_type": 1},
						map[string]interface{}{"doc_token": "doc_verbatim", "artifact_type": 2},
					},
					"references": []interface{}{
						map[string]interface{}{"doc_token": "doc_shared1"},
					},
				},
			},
		},
	}
}

func artifactsStub(token string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/" + token + "/artifacts",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"summary":         "Test summary content",
				"minute_todos":    []interface{}{map[string]interface{}{"content": "Buy milk"}},
				"minute_chapters": []interface{}{map[string]interface{}{"title": "Intro", "summary_content": "Opening"}},
			},
		},
	}
}

func emptyArtifactsStub(token string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/" + token + "/artifacts",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	}
}

func transcriptStub(token string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/" + token + "/transcript",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	}
}

func minuteGetStub(token, noteID, title string) *httpmock.Stub {
	minute := map[string]interface{}{"title": title}
	if noteID != "" {
		minute["note_id"] = noteID
	}
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/" + token,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"minute": minute},
		},
	}
}

// ---------------------------------------------------------------------------
// Unit tests for pure functions
// ---------------------------------------------------------------------------

func TestSanitizeDirName(t *testing.T) {
	tests := []struct {
		title, token, want string
	}{
		{"", "abc123", "artifact-abc123"},
		{"会议纪要", "abc", "artifact-会议纪要-abc"},
		{"a/b\\c:d", "tok", "artifact-a_b_c_d-tok"},
		{"   ", "tok", "artifact-tok"},
		{"ok title", "tok", "artifact-ok title-tok"},
		{"..hidden", "tok", "artifact-hidden-tok"},
		{"a\nb", "tok", "artifact-a_b-tok"},
	}
	for _, tt := range tests {
		got := sanitizeDirName(tt.title, tt.token)
		if got != tt.want {
			t.Errorf("sanitizeDirName(%q, %q) = %q, want %q", tt.title, tt.token, got, tt.want)
		}
	}
}

func TestParseArtifactType(t *testing.T) {
	tests := []struct {
		input any
		want  int
	}{
		{float64(1), 1},
		{float64(2), 2},
		{json.Number("3"), 3},
		{"unknown", 0},
		{nil, 0},
	}
	for _, tt := range tests {
		got := parseArtifactType(tt.input)
		if got != tt.want {
			t.Errorf("parseArtifactType(%v) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestExtractArtifactTokens(t *testing.T) {
	artifacts := []any{
		map[string]any{"doc_token": "main_doc", "artifact_type": float64(1)},
		map[string]any{"doc_token": "verbatim_doc", "artifact_type": float64(2)},
		map[string]any{"doc_token": "unknown_doc", "artifact_type": float64(99)},
		nil,
	}
	noteDoc, verbatimDoc := extractArtifactTokens(artifacts)
	if noteDoc != "main_doc" {
		t.Errorf("noteDoc = %q, want %q", noteDoc, "main_doc")
	}
	if verbatimDoc != "verbatim_doc" {
		t.Errorf("verbatimDoc = %q, want %q", verbatimDoc, "verbatim_doc")
	}
}

func TestExtractArtifactTokens_Empty(t *testing.T) {
	noteDoc, verbatimDoc := extractArtifactTokens(nil)
	if noteDoc != "" || verbatimDoc != "" {
		t.Errorf("expected empty tokens for nil input, got %q, %q", noteDoc, verbatimDoc)
	}
}

func TestExtractDocTokens(t *testing.T) {
	refs := []any{
		map[string]any{"doc_token": "shared1"},
		map[string]any{"doc_token": "shared2"},
		map[string]any{"doc_token": ""},
		map[string]any{},
		nil,
	}
	tokens := extractDocTokens(refs)
	if len(tokens) != 2 || tokens[0] != "shared1" || tokens[1] != "shared2" {
		t.Errorf("extractDocTokens = %v, want [shared1 shared2]", tokens)
	}
}

func TestExtractDocTokens_Empty(t *testing.T) {
	tokens := extractDocTokens(nil)
	if tokens != nil {
		t.Errorf("expected nil for nil input, got %v", tokens)
	}
}

// ---------------------------------------------------------------------------
// Integration tests: +notes with mocked HTTP
// ---------------------------------------------------------------------------

func TestNotes_Validation_ExactlyOne(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, VCNotes, []string{"+notes", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for no flags")
	}

	err = mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", "m1", "--minute-tokens", "t1", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for two flags")
	}
}

func TestNotes_DryRun_MeetingIDs(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", "m001", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "meeting.get") {
		t.Errorf("dry-run should show meeting.get step, got: %s", stdout.String())
	}
}

func TestNotes_DryRun_MinuteTokens(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCNotes, []string{"+notes", "--minute-tokens", "tok001", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "minutes API") {
		t.Errorf("dry-run should show minutes API step, got: %s", stdout.String())
	}
}

func TestNotes_DryRun_CalendarEventIDs(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCNotes, []string{"+notes", "--calendar-event-ids", "evt001", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "mget_instance_relation_info") {
		t.Errorf("dry-run should show mget step, got: %s", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// Additional unit tests for coverage
// ---------------------------------------------------------------------------

func TestSanitizeDirName_Truncate(t *testing.T) {
	long := strings.Repeat("a", 300)
	got := sanitizeDirName(long, "tok")
	if len(got) > 250 { // artifact- prefix + 200 chars + - + tok
		t.Errorf("expected truncated dir name, got len=%d", len(got))
	}
	if !strings.Contains(got, "tok") {
		t.Errorf("expected minute_token in dir name, got %q", got)
	}
}

func TestSanitizeDirName_LeadingDots(t *testing.T) {
	got := sanitizeDirName("...hidden", "tok")
	if strings.Contains(got, "artifact-...") {
		t.Errorf("expected dots stripped, got %q", got)
	}
}

func TestSanitizeLogValue(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"normal", "normal"},
		{"line1\nline2", "line1 line2"},
		{"has\rCR", "has CR"},
		{"ansi\x1b[31mred\x1b[0m", "ansired"},
		{"", ""},
	}
	for _, tt := range tests {
		got := sanitizeLogValue(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeLogValue(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNotes_BatchLimit(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	// generate 51 IDs (over limit of 50)
	ids := make([]string, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("m%d", i)
	}
	err := mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", strings.Join(ids, ","), "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected batch limit error")
	}
	if !strings.Contains(err.Error(), "too many IDs") {
		t.Errorf("expected 'too many IDs' error, got: %v", err)
	}
}

func TestParseArtifactType_AllBranches(t *testing.T) {
	// cover json.Number branch
	if got := parseArtifactType(json.Number("1")); got != 1 {
		t.Errorf("json.Number: got %d, want 1", got)
	}
	// cover float64 branch
	if got := parseArtifactType(float64(2)); got != 2 {
		t.Errorf("float64: got %d, want 2", got)
	}
	// cover default branch
	if got := parseArtifactType("str"); got != 0 {
		t.Errorf("default: got %d, want 0", got)
	}
	// cover nil
	if got := parseArtifactType(nil); got != 0 {
		t.Errorf("nil: got %d, want 0", got)
	}
}
