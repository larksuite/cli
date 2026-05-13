// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

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
	if got := int(common.GetFloat(data, "match_count")); got != 0 {
		t.Fatalf("match_count = %d, want 0", got)
	}
	if got := common.GetString(data, "version"); got != "" {
		t.Fatalf("version = %q, want empty", got)
	}
	if got := int(common.GetFloat(data, "size_bytes_before")); got != len("# hello\n") {
		t.Fatalf("size_bytes_before = %d, want %d", got, len("# hello\n"))
	}
	if got := int(common.GetFloat(data, "size_bytes_after")); got != len("# hello\n") {
		t.Fatalf("size_bytes_after = %d, want %d", got, len("# hello\n"))
	}
	if strings.Contains(stdout.String(), `"matches"`) {
		t.Fatalf("stdout should not include matches field: %s", stdout.String())
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
	if got := int(common.GetFloat(data, "match_count")); got != 2 {
		t.Fatalf("match_count = %d, want 2", got)
	}
	if got := common.GetString(data, "version"); got != "7633658129540910626" {
		t.Fatalf("version = %q, want 7633658129540910626", got)
	}
	if got := int(common.GetFloat(data, "size_bytes_before")); got != len("# TODO\nTODO\n") {
		t.Fatalf("size_bytes_before = %d, want %d", got, len("# TODO\nTODO\n"))
	}
	if got := int(common.GetFloat(data, "size_bytes_after")); got != len("# DONE\nDONE\n") {
		t.Fatalf("size_bytes_after = %d, want %d", got, len("# DONE\nDONE\n"))
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
	if got := int(common.GetFloat(data, "match_count")); got != 2 {
		t.Fatalf("match_count = %d, want 2", got)
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
