// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

func TestMarkdownDiffRejectsUnsupportedFormat(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownDiff, []string{
		"+diff",
		"--file-token", "box_md_diff",
		"--from-version", "7633658129540910621",
		"--format", "table",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "only supports --format json or pretty") {
		t.Fatalf("expected format validation error, got %v", err)
	}
}

func TestMarkdownDiffRejectsToVersionWithoutFromVersion(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownDiff, []string{
		"+diff",
		"--file-token", "box_md_diff",
		"--to-version", "7633658129540910628",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "--to-version requires --from-version") {
		t.Fatalf("expected version validation error, got %v", err)
	}
}

func TestMarkdownDiffMissingVersionAndFileNamesCandidateParams(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownDiff, []string{
		"+diff",
		"--file-token", "box_md_diff",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected validation error when no version or file source is given")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T (%v)", err, err)
	}
	got := map[string]bool{}
	for _, p := range ve.Params {
		got[p.Name] = true
	}
	for _, want := range []string{"--from-version", "--to-version", "--file"} {
		if !got[want] {
			t.Fatalf("params %+v missing candidate %q", ve.Params, want)
		}
	}
}

func TestMarkdownDiffRemoteVsRemoteJSON(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_diff/download?version=7633658129540910621",
		Status:  200,
		RawBody: []byte("# Title\n\n- alpha\n- beta\n"),
		Headers: http.Header{
			"Content-Disposition": []string{`attachment; filename="README.md"`},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_diff/download?version=7633658129540910628",
		Status:  200,
		RawBody: []byte("# Title\n\n- alpha\n- beta updated\n- gamma\n"),
		Headers: http.Header{
			"Content-Disposition": []string{`attachment; filename="README.md"`},
		},
	})

	err := mountAndRunMarkdown(t, MarkdownDiff, []string{
		"+diff",
		"--file-token", "box_md_diff",
		"--from-version", "7633658129540910621",
		"--to-version", "7633658129540910628",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Changed      bool               `json:"changed"`
			Mode         string             `json:"mode"`
			FromVersion  string             `json:"from_version"`
			ToVersion    string             `json:"to_version"`
			AddedLines   int                `json:"added_lines"`
			DeletedLines int                `json:"deleted_lines"`
			Diff         string             `json:"diff"`
			Hunks        []markdownDiffHunk `json:"hunks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json unmarshal error: %v\n%s", err, stdout.String())
	}
	if !env.OK {
		t.Fatalf("expected ok=true, got false: %s", stdout.String())
	}
	if !env.Data.Changed {
		t.Fatalf("expected changed=true: %s", stdout.String())
	}
	if env.Data.Mode != markdownDiffModeRemoteVsRemote {
		t.Fatalf("mode = %q, want %q", env.Data.Mode, markdownDiffModeRemoteVsRemote)
	}
	if env.Data.FromVersion != "7633658129540910621" || env.Data.ToVersion != "7633658129540910628" {
		t.Fatalf("versions = %q -> %q", env.Data.FromVersion, env.Data.ToVersion)
	}
	if env.Data.AddedLines != 2 || env.Data.DeletedLines != 1 {
		t.Fatalf("added/deleted = %d/%d, want 2/1", env.Data.AddedLines, env.Data.DeletedLines)
	}
	if len(env.Data.Hunks) != 1 {
		t.Fatalf("len(hunks) = %d, want 1", len(env.Data.Hunks))
	}
	if !strings.Contains(env.Data.Diff, "@@") || !strings.Contains(env.Data.Diff, "+- gamma") {
		t.Fatalf("diff missing expected content: %s", env.Data.Diff)
	}
}

func TestMarkdownDiffRemoteVsLocalPretty(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_diff/download",
		Status:  200,
		RawBody: []byte("# Title\n\nhello old\n"),
		Headers: http.Header{
			"Content-Disposition": []string{`attachment; filename="README.md"`},
		},
	})

	tmpDir := t.TempDir()
	withMarkdownWorkingDir(t, tmpDir)
	if err := os.WriteFile("local.md", []byte("# Title\n\nhello new\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunMarkdown(t, MarkdownDiff, []string{
		"+diff",
		"--file-token", "box_md_diff",
		"--file", "./local.md",
		"--format", "pretty",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "@@") {
		t.Fatalf("pretty output missing hunk header: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), output.Red+"-hello old"+output.Reset) {
		t.Fatalf("pretty output missing removed line color: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), output.Green+"+hello new"+output.Reset) {
		t.Fatalf("pretty output missing added line color: %q", stdout.String())
	}
}

func TestMarkdownDiffRejectsOversizedRemoteContent(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_diff/download",
		Status:  200,
		RawBody: bytes.Repeat([]byte("x"), markdownDiffMaxContentBytes+1),
	})

	tmpDir := t.TempDir()
	withMarkdownWorkingDir(t, tmpDir)
	if err := os.WriteFile("local.md", []byte("# Title\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunMarkdown(t, MarkdownDiff, []string{
		"+diff",
		"--file-token", "box_md_diff",
		"--file", "./local.md",
		"--as", "bot",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "remote Markdown content exceeds 10.0 MB markdown +diff content limit") {
		t.Fatalf("expected remote content size error, got %v", err)
	}
}

func TestMarkdownDiffRejectsOversizedLocalContent(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_diff/download",
		Status:  200,
		RawBody: []byte("# Title\n"),
	})

	tmpDir := t.TempDir()
	withMarkdownWorkingDir(t, tmpDir)
	if err := os.WriteFile("local.md", bytes.Repeat([]byte("x"), markdownDiffMaxContentBytes+1), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunMarkdown(t, MarkdownDiff, []string{
		"+diff",
		"--file-token", "box_md_diff",
		"--file", "./local.md",
		"--as", "bot",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "local Markdown file exceeds 10.0 MB markdown +diff content limit") {
		t.Fatalf("expected local content size error, got %v", err)
	}
	requireMarkdownValidationParam(t, err, "--file")
}

func TestWrapMarkdownDownloadError(t *testing.T) {
	cause := errors.New("dial tcp timeout")

	tests := []struct {
		name        string
		in          error
		wantSame    bool // result must be the same error value (carrier preserved)
		wantMessage string
		wantCat     errs.Category
		wantSubtype errs.Subtype
		wantCode    int
		wantCause   error // when set, errors.Is(result, wantCause) must hold
	}{
		{
			name:        "non-validation typed error keeps carrier and gains prefix",
			in:          errs.NewAPIError(errs.SubtypePermissionDenied, "permission denied").WithCode(99991663),
			wantSame:    true,
			wantMessage: "download failed: permission denied",
			wantCat:     errs.CategoryAPI,
			wantSubtype: errs.SubtypePermissionDenied,
			wantCode:    99991663,
		},
		{
			name:        "validation error passes through verbatim",
			in:          markdownValidationError("invalid markdown content"),
			wantSame:    true,
			wantMessage: "invalid markdown content",
			wantCat:     errs.CategoryValidation,
			wantSubtype: errs.SubtypeInvalidArgument,
		},
		{
			name:        "untyped error becomes a network transport error",
			in:          cause,
			wantMessage: "download failed: dial tcp timeout",
			wantCat:     errs.CategoryNetwork,
			wantSubtype: errs.SubtypeNetworkTransport,
			wantCause:   cause,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapMarkdownDownloadError(tt.in)
			if tt.wantSame && got != tt.in {
				t.Fatalf("wrapMarkdownDownloadError() returned a new error; want the original carrier preserved")
			}
			if got.Error() != tt.wantMessage {
				t.Fatalf("message = %q, want %q", got.Error(), tt.wantMessage)
			}
			problem, ok := errs.ProblemOf(got)
			if !ok {
				t.Fatalf("wrapMarkdownDownloadError() = %T, want a typed problem", got)
			}
			if problem.Category != tt.wantCat || problem.Subtype != tt.wantSubtype {
				t.Fatalf("classification = %s/%s, want %s/%s", problem.Category, problem.Subtype, tt.wantCat, tt.wantSubtype)
			}
			if tt.wantCode != 0 && problem.Code != tt.wantCode {
				t.Fatalf("code = %d, want %d", problem.Code, tt.wantCode)
			}
			if tt.wantCause != nil && !errors.Is(got, tt.wantCause) {
				t.Fatalf("wrapped error does not unwrap to its cause")
			}
		})
	}
}

func TestMarkdownDiffIncludesNoNewlineMarker(t *testing.T) {
	diffText, changed, added, deleted, hunks := summarizeMarkdownDiff(
		"a/test.md",
		"b/test.md",
		"# Title\n\nhello old",
		"# Title\n\nhello new",
		3,
	)
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if added != 1 || deleted != 1 {
		t.Fatalf("added/deleted = %d/%d, want 1/1", added, deleted)
	}
	if len(hunks) != 1 {
		t.Fatalf("len(hunks) = %d, want 1", len(hunks))
	}
	if strings.Count(diffText, "\\ No newline at end of file") != 2 {
		t.Fatalf("diff should contain two no-newline markers: %q", diffText)
	}
	if !strings.Contains(diffText, "-hello old\n\\ No newline at end of file\n+hello new\n\\ No newline at end of file\n") {
		t.Fatalf("diff missing expected no-newline marker sequence: %q", diffText)
	}
}

func TestMarkdownDiffRemoteVsRemoteJSONMultipleHunks(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_diff/download?version=7633658129540910621",
		Status:  200,
		RawBody: []byte("line1\nline2\nline3\nline4\nline5\nline6\n"),
		Headers: http.Header{
			"Content-Disposition": []string{`attachment; filename="README.md"`},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_diff/download?version=7633658129540910628",
		Status:  200,
		RawBody: []byte("line1\nline2 changed\nline3\nline4\nline5 changed\nline6\n"),
		Headers: http.Header{
			"Content-Disposition": []string{`attachment; filename="README.md"`},
		},
	})

	err := mountAndRunMarkdown(t, MarkdownDiff, []string{
		"+diff",
		"--file-token", "box_md_diff",
		"--from-version", "7633658129540910621",
		"--to-version", "7633658129540910628",
		"--context-lines", "0",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Changed      bool               `json:"changed"`
			AddedLines   int                `json:"added_lines"`
			DeletedLines int                `json:"deleted_lines"`
			Hunks        []markdownDiffHunk `json:"hunks"`
			Diff         string             `json:"diff"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json unmarshal error: %v\n%s", err, stdout.String())
	}
	if !env.OK || !env.Data.Changed {
		t.Fatalf("expected changed=true: %s", stdout.String())
	}
	if env.Data.AddedLines != 2 || env.Data.DeletedLines != 2 {
		t.Fatalf("added/deleted = %d/%d, want 2/2", env.Data.AddedLines, env.Data.DeletedLines)
	}
	if len(env.Data.Hunks) != 2 {
		t.Fatalf("len(hunks) = %d, want 2", len(env.Data.Hunks))
	}
	if !strings.Contains(env.Data.Diff, "-line2") || !strings.Contains(env.Data.Diff, "+line5 changed") {
		t.Fatalf("diff missing expected content: %s", env.Data.Diff)
	}
}

func TestMarkdownDiffNoChangesPretty(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_diff/download?version=7633658129540910621",
		Status:  200,
		RawBody: []byte("# Title\n"),
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_diff/download",
		Status:  200,
		RawBody: []byte("# Title\n"),
	})

	err := mountAndRunMarkdown(t, MarkdownDiff, []string{
		"+diff",
		"--file-token", "box_md_diff",
		"--from-version", "7633658129540910621",
		"--format", "pretty",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "No differences." {
		t.Fatalf("pretty no-change output = %q, want %q", got, "No differences.")
	}
}

func TestMarkdownDiffDryRunRemoteVsLocal(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	tmpDir := t.TempDir()
	withMarkdownWorkingDir(t, tmpDir)
	localPath := filepath.Join(".", "local.md")
	if err := os.WriteFile(localPath, []byte("# local\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunMarkdown(t, MarkdownDiff, []string{
		"+diff",
		"--file-token", "box_md_diff",
		"--file", localPath,
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "/open-apis/drive/v1/files/:file_token/download") && !strings.Contains(stdout.String(), "/open-apis/drive/v1/files/box_md_diff/download") {
		t.Fatalf("dry-run missing download call: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"local_file": "local.md"`) && !strings.Contains(stdout.String(), `"local_file": "./local.md"`) {
		t.Fatalf("dry-run missing local file metadata: %s", stdout.String())
	}
}
