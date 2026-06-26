// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

var detailWarmOnce sync.Once

func detailWarmTokenCache(t *testing.T) {
	t.Helper()
	detailWarmOnce.Do(func() {
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

func detailMountAndRun(t *testing.T, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	detailWarmTokenCache(t)
	parent := &cobra.Command{Use: "minutes"}
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
// Validation tests
// ---------------------------------------------------------------------------

func detailMinuteGetStub(token, noteID, title string) *httpmock.Stub {
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

func detailArtifactsStub(token, transcript string) *httpmock.Stub {
	data := map[string]interface{}{
		"summary":         "Test summary content",
		"minute_todos":    []interface{}{map[string]interface{}{"content": "Buy milk"}},
		"minute_chapters": []interface{}{map[string]interface{}{"title": "Intro", "summary_content": "Opening"}},
		"keywords":        []interface{}{"budget", "roadmap"},
	}
	if transcript != "" {
		data["transcript"] = transcript
	}
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/" + token + "/artifacts",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": data,
		},
	}
}

func TestDetail_Validation_MissingMinuteTokens(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := detailMountAndRun(t, MinutesDetail, []string{"+detail", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for missing --minute-tokens")
	}
}

func TestDetail_Validation_InvalidToken(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := detailMountAndRun(t, MinutesDetail, []string{"+detail", "--minute-tokens", "INVALID!", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for invalid token")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Param != "--minute-tokens" {
		t.Errorf("Param = %q, want --minute-tokens", ve.Param)
	}
}

func TestDetail_Validation_BatchLimit(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	tokens := make([]string, 51)
	for i := range tokens {
		tokens[i] = fmt.Sprintf("tok%d", i)
	}
	err := detailMountAndRun(t, MinutesDetail, []string{"+detail", "--minute-tokens", strings.Join(tokens, ","), "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected batch limit error")
	}
	if !strings.Contains(err.Error(), "too many tokens") {
		t.Errorf("expected 'too many tokens' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DryRun tests
// ---------------------------------------------------------------------------

func TestDetail_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := detailMountAndRun(t, MinutesDetail, []string{"+detail", "--minute-tokens", "tok001", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "/open-apis/minutes/v1/minutes/") {
		t.Errorf("dry-run should show minutes API path, got: %s", stdout.String())
	}
}

func TestDetail_DryRun_WithArtifactFlags(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := detailMountAndRun(t, MinutesDetail, []string{"+detail", "--minute-tokens", "tok001", "--summary", "--todo", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "artifacts") {
		t.Errorf("dry-run should show artifacts API path when artifact flags are set, got: %s", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// Execute tests with mocked HTTP
// ---------------------------------------------------------------------------

func TestDetail_Execute_BasicInfo(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(detailMinuteGetStub("tokbasic", "", "Test Meeting"))

	err := detailMountAndRun(t, MinutesDetail, []string{"+detail", "--minute-tokens", "tokbasic", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	minutes, _ := data["minutes"].([]any)
	if len(minutes) != 1 {
		t.Fatalf("expected 1 minute, got %d", len(minutes))
	}
	m, _ := minutes[0].(map[string]any)
	if m["minute_token"] != "tokbasic" {
		t.Errorf("minute_token = %v, want tokbasic", m["minute_token"])
	}
	if m["title"] != "Test Meeting" {
		t.Errorf("title = %v, want Test Meeting", m["title"])
	}
	noteID, hasNoteID := m["note_id"]
	if !hasNoteID {
		t.Error("note_id should always be present in output (even when empty)")
	}
	if noteID != "" {
		t.Errorf("note_id should be empty string when minute has no note_id, got %v", noteID)
	}
}

func TestDetail_Execute_WithSummaryAndTodo(t *testing.T) {
	chdirForDetailTest(t)

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(detailMinuteGetStub("tokart", "note_art", "Artifact Meeting"))
	reg.Register(detailArtifactsStub("tokart", ""))

	err := detailMountAndRun(t, MinutesDetail, []string{"+detail", "--minute-tokens", "tokart", "--summary", "--todo", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	minutes, _ := data["minutes"].([]any)
	if len(minutes) != 1 {
		t.Fatalf("expected 1 minute, got %d", len(minutes))
	}
	m, _ := minutes[0].(map[string]any)
	if m["note_id"] != "note_art" {
		t.Errorf("note_id = %v, want note_art", m["note_id"])
	}
	arts, _ := m["artifacts"].(map[string]any)
	if arts == nil {
		t.Fatal("expected artifacts to be present")
	}
	if _, ok := arts["summary"]; !ok {
		t.Error("expected summary in artifacts")
	}
	if _, ok := arts["todos"]; !ok {
		t.Error("expected todos in artifacts")
	}
	// chapter and keywords should NOT be present since flags not set
	if _, ok := arts["chapters"]; ok {
		t.Error("chapters should not be present when --chapter not set")
	}
	if _, ok := arts["keywords"]; ok {
		t.Error("keywords should not be present when --keyword not set")
	}
}

func TestDetail_Execute_NoArtifactFlags(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(detailMinuteGetStub("toknoart", "", "No Artifacts"))

	err := detailMountAndRun(t, MinutesDetail, []string{"+detail", "--minute-tokens", "toknoart", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	minutes, _ := data["minutes"].([]any)
	if len(minutes) != 1 {
		t.Fatalf("expected 1 minute, got %d", len(minutes))
	}
	m, _ := minutes[0].(map[string]any)
	if _, ok := m["artifacts"]; ok {
		t.Error("artifacts should not be present when no artifact flags set")
	}
}

func TestDetail_Execute_Transcript(t *testing.T) {
	chdirForDetailTest(t)

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(detailMinuteGetStub("toktrans", "", "Transcript Meeting"))
	reg.Register(detailArtifactsStub("toktrans", "speaker1: hello world\n"))

	err := detailMountAndRun(t, MinutesDetail, []string{"+detail", "--minute-tokens", "toktrans", "--transcript", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check transcript file was saved
	wantPath := "minutes/toktrans/transcript.txt"
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("expected file at %s: %v", wantPath, err)
	}
	if string(data) != "speaker1: hello world\n" {
		t.Errorf("content mismatch: %q", string(data))
	}
}

func TestDetail_Execute_Transcript_OutputDir(t *testing.T) {
	chdirForDetailTest(t)

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(detailMinuteGetStub("tokod", "", "Output Dir Meeting"))
	reg.Register(detailArtifactsStub("tokod", "alice: hi\n"))

	err := detailMountAndRun(t, MinutesDetail, []string{"+detail", "--minute-tokens", "tokod", "--transcript", "--output-dir", "custom_out", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mirrors `minutes +detail --output-dir` layout: artifact-<title>-<token>/transcript.txt
	wantPath := "custom_out/artifact-Output Dir Meeting-tokod/transcript.txt"
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("expected file at %s: %v", wantPath, err)
	}
	if string(data) != "alice: hi\n" {
		t.Errorf("content mismatch: %q", string(data))
	}
}

func TestDetail_Validation_OutputDirEscape(t *testing.T) {
	chdirForDetailTest(t)
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := detailMountAndRun(t, MinutesDetail, []string{"+detail", "--minute-tokens", "tok001", "--output-dir", "../escape", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for escaping output-dir")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
}

func TestDetail_Execute_MinuteNotFound(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/tokbad",
		Body:   map[string]interface{}{"code": 2091004, "msg": "not found"},
	})

	err := detailMountAndRun(t, MinutesDetail, []string{"+detail", "--minute-tokens", "tokbad", "--as", "user"}, f, stdout)
	if err == nil {
		t.Fatal("expected partial failure error")
	}
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected *output.PartialFailureError, got %T: %v", err, err)
	}
}

// ---------------------------------------------------------------------------
// Pure function tests
// ---------------------------------------------------------------------------

func TestValidMinuteTokenDetail(t *testing.T) {
	tests := []struct {
		token string
		valid bool
	}{
		{"abc123", true},
		{"obcnmgn1429t5xt9j82i1p3h", true},
		{"INVALID!", false},
		{"has-space", false},
		{"", false},
	}
	for _, tt := range tests {
		got := validMinuteTokenDetail.MatchString(tt.token)
		if got != tt.valid {
			t.Errorf("validMinuteTokenDetail(%q) = %v, want %v", tt.token, got, tt.valid)
		}
	}
}

// chdirForDetailTest switches cwd to a temp dir for the test.
func chdirForDetailTest(t *testing.T) string {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
	return dir
}
