// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

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

func TestMarkdownPatchValidation(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "pattern is required",
			args: []string{
				"+patch",
				"--file-token", "box_md_patch",
				"--content", "DONE",
			},
			want: "--pattern is required",
		},
		{
			name: "pattern cannot be empty",
			args: []string{
				"+patch",
				"--file-token", "box_md_patch",
				"--pattern", "",
				"--content", "DONE",
			},
			want: "--pattern cannot be empty",
		},
		{
			name: "content is required",
			args: []string{
				"+patch",
				"--file-token", "box_md_patch",
				"--pattern", "TODO",
			},
			want: "--content is required",
		},
		{
			name: "invalid regex",
			args: []string{
				"+patch",
				"--file-token", "box_md_patch",
				"--regex",
				"--pattern", "(",
				"--content", "DONE",
			},
			want: "invalid --pattern regex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mountAndRunMarkdown(t, MarkdownPatch, tt.args, f, stdout)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestMarkdownPatchDryRunLiteral(t *testing.T) {
	dry := decodeMarkdownPatchDryRun(t, "box_md_patch", "TODO", "DONE", false)

	if got := dry.Mode; got != markdownPatchModeLiteral {
		t.Fatalf("mode = %q, want %q", got, markdownPatchModeLiteral)
	}
	if got := len(dry.API); got != 6 {
		t.Fatalf("api steps = %d, want 6", got)
	}
	if got := dry.API[0].URL; got != "/open-apis/drive/v1/files/box_md_patch/download" {
		t.Fatalf("download url = %q", got)
	}
	if got := dry.API[1].URL; got != "/open-apis/drive/v1/metas/batch_query" {
		t.Fatalf("metas url = %q", got)
	}
	if got := dry.API[2].URL; got != "/open-apis/drive/v1/files/upload_all" {
		t.Fatalf("upload_all url = %q", got)
	}
	if got := dry.API[3].URL; got != "/open-apis/drive/v1/files/upload_prepare" {
		t.Fatalf("upload_prepare url = %q", got)
	}
	if got := dry.API[4].URL; got != "/open-apis/drive/v1/files/upload_part" {
		t.Fatalf("upload_part url = %q", got)
	}
	if got := dry.API[5].URL; got != "/open-apis/drive/v1/files/upload_finish" {
		t.Fatalf("upload_finish url = %q", got)
	}
	if got := dry.API[2].Body["file_token"]; got != "box_md_patch" {
		t.Fatalf("upload_all file_token = %#v", got)
	}
	if got := dry.API[3].Body["file_token"]; got != "box_md_patch" {
		t.Fatalf("upload_prepare file_token = %#v", got)
	}
	if got := dry.API[2].Body["file"]; got != "<patched_markdown_content>" {
		t.Fatalf("upload_all file placeholder = %#v", got)
	}
}

func TestMarkdownPatchDryRunRegex(t *testing.T) {
	dry := decodeMarkdownPatchDryRun(t, "box_md_patch", `Version: ([0-9]+)`, `Version: $1`, true)

	if got := dry.Mode; got != markdownPatchModeRegex {
		t.Fatalf("mode = %q, want %q", got, markdownPatchModeRegex)
	}
	if got := dry.API[0].Desc; !strings.Contains(got, "Download the current Markdown content") {
		t.Fatalf("download desc = %q", got)
	}
	if got := dry.API[3].Desc; !strings.Contains(got, "multipart overwrite upload") {
		t.Fatalf("upload_prepare desc = %q", got)
	}
	if got := dry.API[5].Body["block_num"]; got != "<block_num>" {
		t.Fatalf("upload_finish block_num = %#v", got)
	}
}

func TestValidateMarkdownPatchSpecRejectsInvalidFileToken(t *testing.T) {
	runtime := newMarkdownPatchRuntime(t, "../bad", "TODO", "DONE", false)

	err := validateMarkdownPatchSpec(runtime, newMarkdownPatchSpec(runtime))
	if err == nil || !strings.Contains(err.Error(), "--file-token must not contain '..' path traversal") {
		t.Fatalf("expected invalid file-token error, got %v", err)
	}
}

func TestMarkdownPatchReturnsSuccessWhenNothingMatches(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_patch/download",
		Status:  200,
		RawBody: []byte("# hello\n"),
	})

	err := mountAndRunMarkdown(t, MarkdownPatch, []string{
		"+patch",
		"--file-token", "box_md_patch",
		"--pattern", "TODO",
		"--content", "DONE",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeMarkdownEnvelope(t, stdout)
	if common.GetBool(data, "updated") {
		t.Fatalf("updated = true, want false")
	}
	if got := common.GetString(data, "mode"); got != markdownPatchModeLiteral {
		t.Fatalf("mode = %q, want %q", got, markdownPatchModeLiteral)
	}
	if got := common.GetInt(data, "match_count"); got != 0 {
		t.Fatalf("match_count = %d, want 0", got)
	}
	if got := common.GetString(data, "version"); got != "" {
		t.Fatalf("version = %q, want empty", got)
	}
	if got := common.GetInt(data, "size_bytes_before"); got != len("# hello\n") {
		t.Fatalf("size_bytes_before = %d, want %d", got, len("# hello\n"))
	}
	if got := common.GetInt(data, "size_bytes_after"); got != len("# hello\n") {
		t.Fatalf("size_bytes_after = %d, want %d", got, len("# hello\n"))
	}
	if strings.Contains(stdout.String(), `"matches"`) {
		t.Fatalf("stdout should not include matches field: %s", stdout.String())
	}
}

func TestMarkdownPatchPrettyOutputWhenNothingMatches(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_patch/download",
		Status:  200,
		RawBody: []byte("# hello\n"),
	})

	err := mountAndRunMarkdown(t, MarkdownPatch, []string{
		"+patch",
		"--file-token", "box_md_patch",
		"--pattern", "TODO",
		"--content", "DONE",
		"--format", "pretty",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"updated: false",
		"mode: literal",
		"match_count: 0",
		"size_bytes_before: 8",
		"size_bytes_after: 8",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "version:") {
		t.Fatalf("pretty output should omit version when unchanged:\n%s", out)
	}
}

func TestMarkdownPatchLiteralOverwrite(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_patch/download",
		Status:  200,
		RawBody: []byte("# TODO\nTODO\n"),
		Headers: map[string][]string{
			"Content-Disposition": {`attachment; filename="README.md"`},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"title": "README.md"},
				},
			},
		},
	})
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "box_md_patch",
				"version":    "7633658129540910626",
			},
		},
	}
	reg.Register(uploadStub)

	err := mountAndRunMarkdown(t, MarkdownPatch, []string{
		"+patch",
		"--file-token", "box_md_patch",
		"--pattern", "TODO",
		"--content", "DONE",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedMultipartBody(t, uploadStub)
	if got := body.Fields["file_token"]; got != "box_md_patch" {
		t.Fatalf("file_token = %q, want box_md_patch", got)
	}
	if got := body.Fields["file_name"]; got != "README.md" {
		t.Fatalf("file_name = %q, want README.md", got)
	}
	if got := string(body.Files["file"]); got != "# DONE\nDONE\n" {
		t.Fatalf("uploaded file content = %q", got)
	}

	data := decodeMarkdownEnvelope(t, stdout)
	if !common.GetBool(data, "updated") {
		t.Fatalf("updated = false, want true")
	}
	if got := common.GetInt(data, "match_count"); got != 2 {
		t.Fatalf("match_count = %d, want 2", got)
	}
	if got := common.GetString(data, "version"); got != "7633658129540910626" {
		t.Fatalf("version = %q, want 7633658129540910626", got)
	}
	if got := common.GetInt(data, "size_bytes_before"); got != len("# TODO\nTODO\n") {
		t.Fatalf("size_bytes_before = %d, want %d", got, len("# TODO\nTODO\n"))
	}
	if got := common.GetInt(data, "size_bytes_after"); got != len("# DONE\nDONE\n") {
		t.Fatalf("size_bytes_after = %d, want %d", got, len("# DONE\nDONE\n"))
	}
}

func TestMarkdownPatchPrettyOutputWhenUpdated(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_patch/download",
		Status:  200,
		RawBody: []byte("# TODO\n"),
		Headers: map[string][]string{
			"Content-Disposition": {`attachment; filename="README.md"`},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"title": "README.md"},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "box_md_patch",
				"version":    "9001",
			},
		},
	})

	err := mountAndRunMarkdown(t, MarkdownPatch, []string{
		"+patch",
		"--file-token", "box_md_patch",
		"--pattern", "TODO",
		"--content", "DONE",
		"--format", "pretty",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"updated: true",
		"mode: literal",
		"match_count: 1",
		"version: 9001",
		"size_bytes_before: 7",
		"size_bytes_after: 7",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty output missing %q:\n%s", want, out)
		}
	}
}

func TestMarkdownPatchRegexOverwrite(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_patch/download",
		Status:  200,
		RawBody: []byte("Version: 12\nVersion: 34\n"),
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"title": "version.md"},
				},
			},
		},
	})
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "box_md_patch",
				"version":    "7633658129540910627",
			},
		},
	}
	reg.Register(uploadStub)

	err := mountAndRunMarkdown(t, MarkdownPatch, []string{
		"+patch",
		"--file-token", "box_md_patch",
		"--regex",
		"--pattern", `Version: ([0-9]+)`,
		"--content", `Version: $1 (patched)`,
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedMultipartBody(t, uploadStub)
	if got := string(body.Files["file"]); got != "Version: 12 (patched)\nVersion: 34 (patched)\n" {
		t.Fatalf("uploaded file content = %q", got)
	}

	data := decodeMarkdownEnvelope(t, stdout)
	if got := common.GetString(data, "mode"); got != markdownPatchModeRegex {
		t.Fatalf("mode = %q, want %q", got, markdownPatchModeRegex)
	}
	if got := common.GetInt(data, "match_count"); got != 2 {
		t.Fatalf("match_count = %d, want 2", got)
	}
}

func TestApplyMarkdownPatchRejectsInvalidRegex(t *testing.T) {
	_, _, err := applyMarkdownPatch("hello", markdownPatchSpec{
		Pattern: "(",
		Content: "DONE",
		Regex:   true,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid --pattern regex") {
		t.Fatalf("expected invalid regex error, got %v", err)
	}
}

func TestMarkdownPatchAllowsEmptyReplacement(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_patch/download",
		Status:  200,
		RawBody: []byte("hello world\n"),
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"title": "hello.md"},
				},
			},
		},
	})
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "box_md_patch",
				"version":    "7633658129540910628",
			},
		},
	}
	reg.Register(uploadStub)

	err := mountAndRunMarkdown(t, MarkdownPatch, []string{
		"+patch",
		"--file-token", "box_md_patch",
		"--pattern", " world",
		"--content", "",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedMultipartBody(t, uploadStub)
	if got := string(body.Files["file"]); got != "hello\n" {
		t.Fatalf("uploaded file content = %q", got)
	}
}

func TestMarkdownPatchRejectsEmptyPatchedContent(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_patch/download",
		Status:  200,
		RawBody: []byte("hello\n"),
	})

	err := mountAndRunMarkdown(t, MarkdownPatch, []string{
		"+patch",
		"--file-token", "box_md_patch",
		"--pattern", "hello\n",
		"--content", "",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "empty markdown content is not supported") {
		t.Fatalf("expected empty content validation error, got %v", err)
	}
}

func decodeMarkdownEnvelope(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()

	var envelope struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v\nstdout:\n%s", err, stdout.String())
	}
	return envelope.Data
}

type markdownPatchDryRunOutput struct {
	Mode string `json:"mode"`
	API  []struct {
		Desc string                 `json:"desc"`
		URL  string                 `json:"url"`
		Body map[string]interface{} `json:"body"`
	} `json:"api"`
}

func newMarkdownPatchRuntime(t *testing.T, fileToken, pattern, content string, regex bool) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "markdown +patch"}
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("pattern", "", "")
	cmd.Flags().String("content", "", "")
	cmd.Flags().Bool("regex", false, "")

	for name, value := range map[string]string{
		"file-token": fileToken,
		"pattern":    pattern,
		"content":    content,
	} {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	if regex {
		if err := cmd.Flags().Set("regex", "true"); err != nil {
			t.Fatalf("set --regex: %v", err)
		}
	}

	return common.TestNewRuntimeContext(cmd, markdownTestConfig())
}

func decodeMarkdownPatchDryRun(t *testing.T, fileToken, pattern, content string, regex bool) markdownPatchDryRunOutput {
	t.Helper()

	runtime := newMarkdownPatchRuntime(t, fileToken, pattern, content, regex)
	dry := MarkdownPatch.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry-run json: %v", err)
	}

	var out markdownPatchDryRunOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal dry-run json: %v\njson=%s", err, string(data))
	}
	return out
}
