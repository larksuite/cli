// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package note

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestNoteTranscriptRequiresUnifiedNote(t *testing.T) {
	factory, stdout, _, reg := noteShortcutTestFactory(t)
	reg.Register(noteDetailStub("note_normal", displayTypeNormal))

	err := runNoteShortcut(t, NoteTranscript, []string{"+transcript", "--note-id", "note_normal", "--output", "out.md", "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatal("expected non-unified note to fail")
	}
	if got := err.Error(); !strings.Contains(got, "not a unified note") || !strings.Contains(got, "note_display_type=normal") || !strings.Contains(got, "verbatim_doc_token=doc_verbatim") {
		t.Fatalf("err = %q, want non-unified message", got)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if problem.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("subtype = %v, want FailedPrecondition", problem.Subtype)
	}
	if !strings.Contains(problem.Hint, "docs +fetch --doc doc_verbatim") {
		t.Fatalf("hint = %q, want docs +fetch guidance", problem.Hint)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestNoteTranscriptFetchesUnifiedNote(t *testing.T) {
	factory, stdout, _, reg := noteShortcutTestFactory(t)
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	reg.Register(noteDetailStub("note_unified", displayTypeUnified))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/notes/note_unified/unified_note_transcript?format=markdown&locale=zh_cn&page_size=200",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more": false,
				"transcript": map[string]interface{}{
					"markdown": "# transcript\n",
				},
			},
		},
	})

	err := runNoteShortcut(t, NoteTranscript, []string{"+transcript", "--note-id", "note_unified", "--as", "user"}, factory, stdout)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "notes", "note_unified", "unified_transcript.md"))
	if err != nil {
		t.Fatalf("ReadFile transcript err=%v", err)
	}
	if string(content) != "# transcript\n" {
		t.Fatalf("transcript = %q, want %q", string(content), "# transcript\n")
	}
	data := decodeNoteEnvelope(t, stdout)
	if data["note_id"] != "note_unified" || data["size_bytes"] != float64(len(content)) {
		t.Fatalf("unexpected output: %#v", data)
	}
}

func TestNoteTranscriptFormatFlagDoesNotShadowOutputFormat(t *testing.T) {
	factory, stdout, _, reg := noteShortcutTestFactory(t)
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	reg.Register(noteDetailStub("note_plain", displayTypeUnified))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/notes/note_plain/unified_note_transcript?format=plain_text&locale=zh_cn&page_size=200",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more": false,
				"transcript": map[string]interface{}{
					"plain_text": "plain transcript\n",
				},
			},
		},
	})

	err := runNoteShortcut(t, NoteTranscript, []string{
		"+transcript",
		"--note-id", "note_plain",
		"--transcript-format", "plain_text",
		"--format", "json",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "notes", "note_plain", "unified_transcript.txt"))
	if err != nil {
		t.Fatalf("ReadFile transcript err=%v", err)
	}
	if string(content) != "plain transcript\n" {
		t.Fatalf("transcript = %q, want plain transcript", string(content))
	}
	data := decodeNoteEnvelope(t, stdout)
	if data["transcript_format"] != "plain_text" {
		t.Fatalf("transcript_format = %#v, want plain_text; output=%s", data["transcript_format"], stdout.String())
	}
	if _, ok := data["format"]; ok {
		t.Fatalf("output should not expose ambiguous format field: %#v", data)
	}
}

func TestNoteTranscriptPassesLocaleThrough(t *testing.T) {
	factory, stdout, _, reg := noteShortcutTestFactory(t)
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	reg.Register(noteDetailStub("note_locale", displayTypeUnified))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/notes/note_locale/unified_note_transcript?format=markdown&locale=en_us&page_size=200",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more": false,
				"transcript": map[string]interface{}{
					"markdown": "# en transcript\n",
				},
			},
		},
	})

	err := runNoteShortcut(t, NoteTranscript, []string{"+transcript", "--note-id", "note_locale", "--locale", "en_us", "--as", "user"}, factory, stdout)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "notes", "note_locale", "unified_transcript.md"))
	if err != nil {
		t.Fatalf("ReadFile transcript err=%v", err)
	}
	if string(content) != "# en transcript\n" {
		t.Fatalf("transcript = %q, want en transcript", string(content))
	}
}

func TestNoteTranscriptDefaultsLocaleFromLarkBrand(t *testing.T) {
	config := &core.CliConfig{
		AppID:      "test-app-lark-locale",
		AppSecret:  "test-secret",
		Brand:      core.BrandLark,
		UserOpenId: "ou_testuser",
	}
	factory, stdout, _, reg := noteShortcutTestFactoryWithConfig(t, config)
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	reg.Register(noteDetailStub("note_lark", displayTypeUnified))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/notes/note_lark/unified_note_transcript?format=markdown&locale=en_us&page_size=200",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more": false,
				"transcript": map[string]interface{}{
					"markdown": "# en transcript\n",
				},
			},
		},
	})

	err := runNoteShortcut(t, NoteTranscript, []string{"+transcript", "--note-id", "note_lark", "--as", "user"}, factory, stdout)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestNoteTranscriptRejectsExistingOutputBeforeFetch(t *testing.T) {
	factory, stdout, _, _ := noteShortcutTestFactory(t)
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	outPath := filepath.Join("notes", "note_exists", "unified_transcript.md")
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		t.Fatalf("MkdirAll err=%v", err)
	}
	if err := os.WriteFile(outPath, []byte("old"), 0644); err != nil {
		t.Fatalf("WriteFile err=%v", err)
	}

	err := runNoteShortcut(t, NoteTranscript, []string{"+transcript", "--note-id", "note_exists", "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatal("expected existing output to fail")
	}
	if got := err.Error(); !strings.Contains(got, "output file already exists") {
		t.Fatalf("err = %q, want existing output error", got)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("err = %T, want ValidationError", err)
	}
	if validationErr.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("subtype = %v, want FailedPrecondition", validationErr.Subtype)
	}
	if !strings.Contains(validationErr.Hint, "--overwrite") {
		t.Fatalf("hint = %q, want --overwrite guidance", validationErr.Hint)
	}
	// The CLI picked the default path itself, so no input param is at fault.
	if validationErr.Param != "" {
		t.Fatalf("param = %q, want empty for default output path", validationErr.Param)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestNoteTranscriptRejectsEmptyTranscript(t *testing.T) {
	factory, stdout, _, reg := noteShortcutTestFactory(t)
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	reg.Register(noteDetailStub("note_empty", displayTypeUnified))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/notes/note_empty/unified_note_transcript?format=markdown&locale=zh_cn&page_size=200",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more": false,
				"transcript": map[string]interface{}{
					"markdown": "",
				},
			},
		},
	})

	err := runNoteShortcut(t, NoteTranscript, []string{"+transcript", "--note-id", "note_empty", "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatal("expected empty transcript to fail")
	}
	if got := err.Error(); !strings.Contains(got, "transcript is empty") || !strings.Contains(got, "note_empty") {
		t.Fatalf("err = %q, want empty transcript error", got)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("category/subtype = %v/%v, want Internal/InvalidResponse", problem.Category, problem.Subtype)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "notes", "note_empty", "unified_transcript.md")); !os.IsNotExist(statErr) {
		t.Fatalf("transcript file should not exist, statErr=%v", statErr)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestNoteTranscriptRejectsCursorCycle(t *testing.T) {
	factory, stdout, _, reg := noteShortcutTestFactory(t)
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	reg.Register(noteDetailStub("note_cycle", displayTypeUnified))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/notes/note_cycle/unified_note_transcript?format=markdown&locale=zh_cn&page_size=200",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more":       true,
				"next_cursor_id": "A",
				"transcript": map[string]interface{}{
					"markdown": "page1\n",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "cursor_id=A",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more":       true,
				"next_cursor_id": "B",
				"transcript": map[string]interface{}{
					"markdown": "page2\n",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "cursor_id=B",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more":       true,
				"next_cursor_id": "A",
				"transcript": map[string]interface{}{
					"markdown": "page3\n",
				},
			},
		},
	})

	err := runNoteShortcut(t, NoteTranscript, []string{"+transcript", "--note-id", "note_cycle", "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatal("expected cursor cycle to fail")
	}
	if got := err.Error(); !strings.Contains(got, "pagination cursor did not advance") {
		t.Fatalf("err = %q, want cursor advance error", got)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("category/subtype = %v/%v, want Internal/InvalidResponse", problem.Category, problem.Subtype)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "notes", "note_cycle", "unified_transcript.md")); !os.IsNotExist(statErr) {
		t.Fatalf("transcript file should not exist, statErr=%v", statErr)
	}
}

func TestTranscriptContextErrorPreservesCause(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		subtype errs.Subtype
	}{
		{
			name:    "canceled",
			err:     context.Canceled,
			subtype: errs.SubtypeNetworkTransport,
		},
		{
			name:    "deadline",
			err:     context.DeadlineExceeded,
			subtype: errs.SubtypeNetworkTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := transcriptContextError(tt.err)
			if !errors.Is(err, tt.err) {
				t.Fatalf("errors.Is(%v) = false", tt.err)
			}
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected typed error, got %T", err)
			}
			if problem.Category != errs.CategoryNetwork || problem.Subtype != tt.subtype {
				t.Fatalf("category/subtype = %v/%v, want Network/%v", problem.Category, problem.Subtype, tt.subtype)
			}
		})
	}
}

func noteShortcutTestFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	config := &core.CliConfig{
		AppID:      "test-app-" + strings.ReplaceAll(strings.ToLower(t.Name()), "/", "-"),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_testuser",
	}
	return noteShortcutTestFactoryWithConfig(t, config)
}

func noteShortcutTestFactoryWithConfig(t *testing.T, config *core.CliConfig) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	return cmdutil.TestFactory(t, config)
}

func runNoteShortcut(t *testing.T, shortcut common.Shortcut, args []string, factory *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "note"}
	shortcut.Mount(parent, factory)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	stdout.Reset()
	if stderr, ok := factory.IOStreams.ErrOut.(*bytes.Buffer); ok {
		stderr.Reset()
	}
	return parent.ExecuteContext(context.Background())
}

func noteDetailStub(noteID string, displayType int) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/notes/" + noteID,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"note": map[string]interface{}{
					"note_display_type": displayType,
					"artifacts": []interface{}{
						map[string]interface{}{"artifact_type": artifactTypeVerbatim, "doc_token": "doc_verbatim"},
					},
				},
			},
		},
	}
}

func decodeNoteEnvelope(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()
	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\nstdout=%s", err, stdout.String())
	}
	if data, _ := envelope["data"].(map[string]interface{}); data != nil {
		return data
	}
	return envelope
}
