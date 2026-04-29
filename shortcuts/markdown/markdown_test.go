// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func markdownTestConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: "markdown-test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
}

func mountAndRunMarkdown(t *testing.T, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "markdown"}
	s.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func withMarkdownWorkingDir(t *testing.T, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q) error: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd error: %v", err)
		}
	})
}

type capturedMultipartBody struct {
	Fields map[string]string
	Files  map[string][]byte
}

func decodeCapturedMultipartBody(t *testing.T, stub *httpmock.Stub) capturedMultipartBody {
	t.Helper()

	contentType := stub.CapturedHeaders.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("parse multipart content type: %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("content type = %q, want multipart/form-data", mediaType)
	}

	reader := multipart.NewReader(bytes.NewReader(stub.CapturedBody), params["boundary"])
	body := capturedMultipartBody{
		Fields: map[string]string{},
		Files:  map[string][]byte{},
	}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read multipart part: %v", err)
		}

		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read multipart data: %v", err)
		}
		if part.FileName() != "" {
			body.Files[part.FormName()] = data
			continue
		}
		body.Fields[part.FormName()] = string(data)
	}
	return body
}

func TestShortcutsIncludesExpectedCommands(t *testing.T) {
	t.Parallel()

	got := Shortcuts()
	want := []string{"+create", "+fetch", "+overwrite"}

	if len(got) != len(want) {
		t.Fatalf("len(Shortcuts()) = %d, want %d", len(got), len(want))
	}

	seen := make(map[string]bool, len(got))
	for _, shortcut := range got {
		if seen[shortcut.Command] {
			t.Fatalf("duplicate shortcut command: %s", shortcut.Command)
		}
		seen[shortcut.Command] = true
	}

	for _, command := range want {
		if !seen[command] {
			t.Fatalf("missing shortcut command %q in Shortcuts()", command)
		}
	}
}

func TestMarkdownCreateRequiresNameWithContent(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--content", "# hello",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "--name is required when using --content") {
		t.Fatalf("expected name validation error, got %v", err)
	}
}

func TestMarkdownCreateRejectsNonMarkdownFile(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	tmpDir := t.TempDir()
	withMarkdownWorkingDir(t, tmpDir)
	if err := os.WriteFile("note.txt", []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--file", "note.txt",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "--file must end with .md") {
		t.Fatalf("expected .md validation error, got %v", err)
	}
}

func TestMarkdownCreateAllowsEmptyInlineContent(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "box_md_empty_inline",
				"version":    "1002",
			},
		},
	}
	reg.Register(uploadStub)

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--name", "empty.md",
		"--content", "",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedMultipartBody(t, uploadStub)
	if got := body.Fields["size"]; got != "0" {
		t.Fatalf("size = %q, want 0", got)
	}
	if got := string(body.Files["file"]); got != "" {
		t.Fatalf("uploaded file content = %q, want empty string", got)
	}
	if !strings.Contains(stdout.String(), `"size_bytes": 0`) {
		t.Fatalf("stdout missing zero size_bytes: %s", stdout.String())
	}
}

func TestMarkdownCreateAllowsEmptyContentFromFileInput(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "box_md_empty_file_input",
				"version":    "1003",
			},
		},
	}
	reg.Register(uploadStub)

	tmpDir := t.TempDir()
	withMarkdownWorkingDir(t, tmpDir)
	if err := os.WriteFile("empty.md", []byte{}, 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--name", "empty.md",
		"--content", "@./empty.md",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedMultipartBody(t, uploadStub)
	if got := body.Fields["size"]; got != "0" {
		t.Fatalf("size = %q, want 0", got)
	}
	if got := string(body.Files["file"]); got != "" {
		t.Fatalf("uploaded file content = %q, want empty string", got)
	}
}

func TestMarkdownCreateSuccessUploadAll(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "box_md_create",
				"version":    "1001",
			},
		},
	}
	reg.Register(uploadStub)

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--name", "README.md",
		"--content", "# hello\n",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedMultipartBody(t, uploadStub)
	if got := body.Fields["file_name"]; got != "README.md" {
		t.Fatalf("file_name = %q, want README.md", got)
	}
	if got := body.Fields["parent_type"]; got != "explorer" {
		t.Fatalf("parent_type = %q, want explorer", got)
	}
	if got := body.Fields["parent_node"]; got != "" {
		t.Fatalf("parent_node = %q, want empty root folder", got)
	}
	if _, exists := body.Fields["file_token"]; exists {
		t.Fatalf("did not expect file_token on create upload_all body")
	}
	if got := string(body.Files["file"]); got != "# hello\n" {
		t.Fatalf("uploaded file content = %q", got)
	}
	if !strings.Contains(stdout.String(), `"file_token": "box_md_create"`) {
		t.Fatalf("stdout missing file_token: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"file_name": "README.md"`) {
		t.Fatalf("stdout missing file_name: %s", stdout.String())
	}
}

func TestMarkdownCreateFailsWhenMultipartPlanIsTooSmall(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_prepare",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"upload_id":  "upload_markdown_bad_plan",
				"block_size": float64(markdownSinglePartSizeLimit),
				"block_num":  float64(1),
			},
		},
	})

	tmpDir := t.TempDir()
	withMarkdownWorkingDir(t, tmpDir)
	fh, err := os.Create("large.md")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := fh.Truncate(markdownSinglePartSizeLimit + 1); err != nil {
		fh.Close()
		t.Fatalf("Truncate() error: %v", err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	err = mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--file", "large.md",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "inconsistent chunk plan") {
		t.Fatalf("expected inconsistent chunk plan error, got %v", err)
	}
}

func TestMarkdownCreateFailsWhenMultipartPlanIsTooLarge(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_prepare",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"upload_id":  "upload_markdown_bad_plan_large",
				"block_size": float64(markdownSinglePartSizeLimit),
				"block_num":  float64(3),
			},
		},
	})

	tmpDir := t.TempDir()
	withMarkdownWorkingDir(t, tmpDir)
	fh, err := os.Create("large.md")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := fh.Truncate(markdownSinglePartSizeLimit + 1); err != nil {
		fh.Close()
		t.Fatalf("Truncate() error: %v", err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	err = mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--file", "large.md",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "inconsistent chunk plan") {
		t.Fatalf("expected inconsistent chunk plan error, got %v", err)
	}
}

func TestMarkdownOverwriteUploadAllIncludesFileTokenAndVersion(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
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
				"file_token": "box_md_existing",
				"version":    "7633658129540910621",
			},
		},
	}
	reg.Register(uploadStub)

	err := mountAndRunMarkdown(t, MarkdownOverwrite, []string{
		"+overwrite",
		"--file-token", "box_md_existing",
		"--content", "# updated\n",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedMultipartBody(t, uploadStub)
	if got := body.Fields["file_token"]; got != "box_md_existing" {
		t.Fatalf("file_token = %q, want box_md_existing", got)
	}
	if got := body.Fields["file_name"]; got != "README.md" {
		t.Fatalf("file_name = %q, want README.md", got)
	}
	if got := string(body.Files["file"]); got != "# updated\n" {
		t.Fatalf("uploaded file content = %q", got)
	}
	if !strings.Contains(stdout.String(), `"version": "7633658129540910621"`) {
		t.Fatalf("stdout missing version: %s", stdout.String())
	}
}

func TestMarkdownOverwriteUsesExplicitNameWhenProvided(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "box_md_existing",
				"version":    "7633658129540910622",
			},
		},
	}
	reg.Register(uploadStub)

	err := mountAndRunMarkdown(t, MarkdownOverwrite, []string{
		"+overwrite",
		"--file-token", "box_md_existing",
		"--name", "Renamed.md",
		"--content", "# updated\n",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedMultipartBody(t, uploadStub)
	if got := body.Fields["file_name"]; got != "Renamed.md" {
		t.Fatalf("file_name = %q, want Renamed.md", got)
	}
	if !strings.Contains(stdout.String(), `"file_name": "Renamed.md"`) {
		t.Fatalf("stdout missing overridden file_name: %s", stdout.String())
	}
}

func TestMarkdownOverwriteUsesLocalFileNameByDefault(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "box_md_existing",
				"version":    "7633658129540910623",
			},
		},
	}
	reg.Register(uploadStub)

	tmpDir := t.TempDir()
	withMarkdownWorkingDir(t, tmpDir)
	if err := os.WriteFile("local-name.md", []byte("# local\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunMarkdown(t, MarkdownOverwrite, []string{
		"+overwrite",
		"--file-token", "box_md_existing",
		"--file", "local-name.md",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedMultipartBody(t, uploadStub)
	if got := body.Fields["file_name"]; got != "local-name.md" {
		t.Fatalf("file_name = %q, want local-name.md", got)
	}
}

func TestMarkdownOverwriteFailsWithoutVersion(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
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
				"file_token": "box_md_existing",
			},
		},
	})

	err := mountAndRunMarkdown(t, MarkdownOverwrite, []string{
		"+overwrite",
		"--file-token", "box_md_existing",
		"--content", "# updated\n",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "overwrite failed: no version returned") {
		t.Fatalf("expected version error, got %v", err)
	}
}

func TestMarkdownOverwriteFallsBackToFileTokenNameWhenMetadataMissing(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{},
			},
		},
	})
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "box_md_existing",
				"version":    "7633658129540910624",
			},
		},
	}
	reg.Register(uploadStub)

	err := mountAndRunMarkdown(t, MarkdownOverwrite, []string{
		"+overwrite",
		"--file-token", "box_md_existing",
		"--content", "# updated\n",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedMultipartBody(t, uploadStub)
	if got := body.Fields["file_name"]; got != "box_md_existing.md" {
		t.Fatalf("file_name = %q, want box_md_existing.md", got)
	}
}

func TestMarkdownFetchReturnsContent(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_fetch/download",
		Status:  200,
		RawBody: []byte("# hello\n"),
		Headers: map[string][]string{
			"Content-Type":        {"text/plain"},
			"Content-Disposition": {`attachment; filename="README.md"`},
		},
	})

	err := mountAndRunMarkdown(t, MarkdownFetch, []string{
		"+fetch",
		"--file-token", "box_md_fetch",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"file_name": "README.md"`) {
		t.Fatalf("stdout missing file_name: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"content": "# hello\n"`) {
		t.Fatalf("stdout missing content: %s", stdout.String())
	}
}

func TestMarkdownFetchSavesFile(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_fetch/download",
		Status:  200,
		RawBody: []byte("# hello\n"),
		Headers: map[string][]string{
			"Content-Type":        {"text/plain"},
			"Content-Disposition": {`attachment; filename="README.md"`},
		},
	})

	tmpDir := t.TempDir()
	withMarkdownWorkingDir(t, tmpDir)

	err := mountAndRunMarkdown(t, MarkdownFetch, []string{
		"+fetch",
		"--file-token", "box_md_fetch",
		"--output", "copy.md",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile("copy.md")
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "# hello\n" {
		t.Fatalf("saved content = %q", string(data))
	}

	var envelope struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if got := common.GetString(envelope.Data, "saved_path"); !strings.HasSuffix(got, "copy.md") {
		t.Fatalf("saved_path = %q, want suffix copy.md", got)
	}
}

func TestMarkdownFetchRejectsExistingFileWithoutOverwrite(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_fetch/download",
		Status:  200,
		RawBody: []byte("# hello\n"),
		Headers: map[string][]string{
			"Content-Type":        {"text/plain"},
			"Content-Disposition": {`attachment; filename="README.md"`},
		},
	})

	tmpDir := t.TempDir()
	withMarkdownWorkingDir(t, tmpDir)
	if err := os.WriteFile("copy.md", []byte("existing"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunMarkdown(t, MarkdownFetch, []string{
		"+fetch",
		"--file-token", "box_md_fetch",
		"--output", "copy.md",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "output file already exists") {
		t.Fatalf("expected output exists error, got %v", err)
	}
}

func TestMarkdownFetchOverwritesExistingFileWhenRequested(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_fetch/download",
		Status:  200,
		RawBody: []byte("# hello\n"),
		Headers: map[string][]string{
			"Content-Type":        {"text/plain"},
			"Content-Disposition": {`attachment; filename="README.md"`},
		},
	})

	tmpDir := t.TempDir()
	withMarkdownWorkingDir(t, tmpDir)
	if err := os.WriteFile("copy.md", []byte("existing"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunMarkdown(t, MarkdownFetch, []string{
		"+fetch",
		"--file-token", "box_md_fetch",
		"--output", "copy.md",
		"--overwrite",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile("copy.md")
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "# hello\n" {
		t.Fatalf("saved content = %q", string(data))
	}
}

func TestMarkdownFetchSavesUsingRemoteNameWhenOutputIsExistingDirectory(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_fetch/download",
		Status:  200,
		RawBody: []byte("# hello\n"),
		Headers: map[string][]string{
			"Content-Type":        {"text/plain"},
			"Content-Disposition": {`attachment; filename="README.md"`},
		},
	})

	tmpDir := t.TempDir()
	withMarkdownWorkingDir(t, tmpDir)
	if err := os.MkdirAll("downloads", 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	err := mountAndRunMarkdown(t, MarkdownFetch, []string{
		"+fetch",
		"--file-token", "box_md_fetch",
		"--output", "downloads",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join("downloads", "README.md"))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "# hello\n" {
		t.Fatalf("saved content = %q", string(data))
	}
}

func TestMarkdownFetchSavesUsingRemoteNameWhenOutputUsesDirectorySyntax(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_md_fetch/download",
		Status:  200,
		RawBody: []byte("# hello\n"),
		Headers: map[string][]string{
			"Content-Type":        {"text/plain"},
			"Content-Disposition": {`attachment; filename="README.md"`},
		},
	})

	tmpDir := t.TempDir()
	withMarkdownWorkingDir(t, tmpDir)

	err := mountAndRunMarkdown(t, MarkdownFetch, []string{
		"+fetch",
		"--file-token", "box_md_fetch",
		"--output", "downloads/",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join("downloads", "README.md"))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "# hello\n" {
		t.Fatalf("saved content = %q", string(data))
	}
}
