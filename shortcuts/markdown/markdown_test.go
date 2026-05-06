// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/extension/fileio"
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type errReadCloser struct{ err error }

func (r *errReadCloser) Read(_ []byte) (int, error) { return 0, r.err }
func (r *errReadCloser) Close() error               { return nil }

type staticFileIOProvider struct {
	fileIO fileio.FileIO
}

func (p *staticFileIOProvider) Name() string { return "static" }

func (p *staticFileIOProvider) ResolveFileIO(context.Context) fileio.FileIO {
	return p.fileIO
}

type failingSaveFileIO struct {
	base fileio.FileIO
	err  error
}

func (f *failingSaveFileIO) Open(name string) (fileio.File, error) {
	return f.base.Open(name)
}

func (f *failingSaveFileIO) Stat(name string) (fileio.FileInfo, error) {
	return f.base.Stat(name)
}

func (f *failingSaveFileIO) ResolvePath(path string) (string, error) {
	return f.base.ResolvePath(path)
}

func (f *failingSaveFileIO) Save(string, fileio.SaveOptions, io.Reader) (fileio.SaveResult, error) {
	return nil, &fileio.WriteError{Err: f.err}
}

type stubFileInfo struct {
	size int64
}

func (i stubFileInfo) Size() int64       { return i.size }
func (i stubFileInfo) IsDir() bool       { return false }
func (i stubFileInfo) Mode() fs.FileMode { return 0 }

type statOnlyFileIO struct {
	base    fileio.FileIO
	size    int64
	openErr error
}

func (f *statOnlyFileIO) Open(string) (fileio.File, error) {
	return nil, f.openErr
}

func (f *statOnlyFileIO) Stat(string) (fileio.FileInfo, error) {
	return stubFileInfo{size: f.size}, nil
}

func (f *statOnlyFileIO) ResolvePath(path string) (string, error) {
	return f.base.ResolvePath(path)
}

func (f *statOnlyFileIO) Save(path string, opts fileio.SaveOptions, body io.Reader) (fileio.SaveResult, error) {
	return f.base.Save(path, opts, body)
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

func TestMarkdownCreateValidationBranches(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "content and file are mutually exclusive",
			args: []string{
				"+create",
				"--name", "README.md",
				"--content", "# hello",
				"--file", "note.md",
			},
			want: "--content and --file are mutually exclusive",
		},
		{
			name: "exactly one source is required",
			args: []string{
				"+create",
				"--name", "README.md",
			},
			want: "specify exactly one of --content or --file",
		},
		{
			name: "folder token cannot be empty",
			args: []string{
				"+create",
				"--name", "README.md",
				"--content", "# hello",
				"--folder-token=",
			},
			want: "--folder-token cannot be empty",
		},
		{
			name: "folder token must be valid",
			args: []string{
				"+create",
				"--name", "README.md",
				"--content", "# hello",
				"--folder-token", "../bad",
			},
			want: "--folder-token",
		},
		{
			name: "content mode still validates markdown file name",
			args: []string{
				"+create",
				"--name", "README.txt",
				"--content", "# hello",
			},
			want: "--name must end with .md",
		},
		{
			name: "file flag cannot be empty",
			args: []string{
				"+create",
				"--file=",
			},
			want: "--file cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mountAndRunMarkdown(t, MarkdownCreate, tt.args, f, stdout)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestMarkdownCreateRejectsEmptyInlineContent(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())
	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--name", "empty.md",
		"--content", "",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "empty markdown content is not supported") {
		t.Fatalf("expected empty content validation error, got %v", err)
	}
}

func TestMarkdownCreateRejectsEmptyContentFromFileInput(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())
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
	if err == nil || !strings.Contains(err.Error(), "empty markdown content is not supported") {
		t.Fatalf("expected empty content validation error, got %v", err)
	}
}

func TestMarkdownCreateRejectsEmptyLocalFile(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())
	tmpDir := t.TempDir()
	withMarkdownWorkingDir(t, tmpDir)
	if err := os.WriteFile("empty.md", []byte{}, 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--file", "empty.md",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "empty markdown content is not supported") {
		t.Fatalf("expected empty content validation error, got %v", err)
	}
}

func TestMarkdownCreateDryRunWithInlineContent(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--name", "README.md",
		"--content", "# hello",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "/open-apis/drive/v1/files/upload_all") {
		t.Fatalf("dry-run missing upload_all: %s", out)
	}
	if !strings.Contains(out, "markdown content") {
		t.Fatalf("dry-run missing content marker: %s", out)
	}
}

func TestMarkdownCreateDryRunReportsSourceFileError(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--file", "missing.md",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected dry-run error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"error"`) || !strings.Contains(stdout.String(), "cannot read file") {
		t.Fatalf("dry-run output missing file error: %s", stdout.String())
	}
}

func TestMarkdownCreateDryRunWithFileUsesStatOnly(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())
	f.FileIOProvider = &staticFileIOProvider{
		fileIO: &statOnlyFileIO{
			base:    fileio.GetProvider().ResolveFileIO(context.Background()),
			size:    markdownSinglePartSizeLimit + 1,
			openErr: errors.New("open should not be called in dry-run"),
		},
	}

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--file", "large.md",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected dry-run error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "/open-apis/drive/v1/files/upload_prepare") {
		t.Fatalf("dry-run missing multipart prepare step: %s", out)
	}
	if strings.Contains(out, "open should not be called in dry-run") {
		t.Fatalf("dry-run unexpectedly tried to open the source file: %s", out)
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
	if !strings.Contains(stdout.String(), `"url": "https://www.feishu.cn/file/box_md_create"`) {
		t.Fatalf("stdout missing url: %s", stdout.String())
	}
}

func TestMarkdownCreatePrettyOutputIncludesPermissionGrant(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "box_md_create_pretty",
			},
		},
	})

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--name", "README.md",
		"--content", "# hello\n",
		"--as", "bot",
		"--format", "pretty",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "file_token: box_md_create_pretty") {
		t.Fatalf("pretty output missing file_token: %s", out)
	}
	if !strings.Contains(out, "url: https://www.feishu.cn/file/box_md_create_pretty") {
		t.Fatalf("pretty output missing url: %s", out)
	}
	if !strings.Contains(out, "permission_grant.status: skipped") {
		t.Fatalf("pretty output missing permission_grant.status: %s", out)
	}
	if !strings.Contains(out, "permission_grant.perm: full_access") {
		t.Fatalf("pretty output missing permission_grant.perm: %s", out)
	}
}

func TestMarkdownCreateMissingFileReturnsReadError(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--file", "missing.md",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "cannot read file") {
		t.Fatalf("expected cannot read file error, got %v", err)
	}
}

func TestMarkdownCreateMultipartUploadSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_prepare",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"upload_id":  "upload_markdown_ok",
				"block_size": float64(markdownSinglePartSizeLimit),
				"block_num":  float64(2),
			},
		},
	})
	uploadPartStub := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/drive/v1/files/upload_part",
		Reusable: true,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	}
	reg.Register(uploadPartStub)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_finish",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "box_md_multipart",
				"version":    "1004",
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(uploadPartStub.CapturedBodies) != 2 {
		t.Fatalf("upload_part call count = %d, want 2", len(uploadPartStub.CapturedBodies))
	}
	if !strings.Contains(stdout.String(), `"file_token": "box_md_multipart"`) {
		t.Fatalf("stdout missing multipart file_token: %s", stdout.String())
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

func TestUploadMarkdownMultipartPartsRejectsOversizedBlockSize(t *testing.T) {
	maxBufferSize := int64(^uint(0) >> 1)
	if maxBufferSize == int64(^uint64(0)>>1) {
		t.Skip("oversized block_size guard is only reachable on platforms where int is narrower than int64")
	}

	err := uploadMarkdownMultipartParts(nil, bytes.NewReader([]byte("x")), 1, markdownMultipartSession{
		UploadID:  "upload_markdown_bad_block_size",
		BlockSize: maxBufferSize + 1,
		BlockNum:  1,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid block_size returned") {
		t.Fatalf("expected invalid block_size error, got %v", err)
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

func TestMarkdownOverwriteDryRunWithInlineContent(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownOverwrite, []string{
		"+overwrite",
		"--file-token", "box_md_existing",
		"--content", "# updated\n",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "/open-apis/drive/v1/metas/batch_query") {
		t.Fatalf("dry-run missing metas lookup: %s", out)
	}
	if !strings.Contains(out, "/open-apis/drive/v1/files/upload_all") {
		t.Fatalf("dry-run missing upload_all: %s", out)
	}
	if !strings.Contains(out, `"file_token":"box_md_existing"`) && !strings.Contains(out, `"file_token": "box_md_existing"`) {
		t.Fatalf("dry-run missing file_token: %s", out)
	}
}

func TestMarkdownOverwriteDryRunReportsSourceFileError(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownOverwrite, []string{
		"+overwrite",
		"--file-token", "box_md_existing",
		"--file", "missing.md",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected dry-run error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"error"`) || !strings.Contains(stdout.String(), "cannot read file") {
		t.Fatalf("dry-run output missing file error: %s", stdout.String())
	}
}

func TestMarkdownOverwriteRejectsEmptyInlineContent(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownOverwrite, []string{
		"+overwrite",
		"--file-token", "box_md_existing",
		"--content", "",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "empty markdown content is not supported") {
		t.Fatalf("expected empty content validation error, got %v", err)
	}
}

func TestMarkdownOverwriteRejectsEmptyLocalFile(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())
	tmpDir := t.TempDir()
	withMarkdownWorkingDir(t, tmpDir)
	if err := os.WriteFile("empty.md", []byte{}, 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunMarkdown(t, MarkdownOverwrite, []string{
		"+overwrite",
		"--file-token", "box_md_existing",
		"--file", "empty.md",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "empty markdown content is not supported") {
		t.Fatalf("expected empty content validation error, got %v", err)
	}
}

func TestMarkdownOverwriteMetadataLookupFailure(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownOverwrite, []string{
		"+overwrite",
		"--file-token", "box_md_existing",
		"--content", "# updated\n",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected metadata lookup failure")
	}
}

func TestMarkdownOverwriteMissingFileReturnsReadError(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownOverwrite, []string{
		"+overwrite",
		"--file-token", "box_md_existing",
		"--file", "missing.md",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "cannot read file") {
		t.Fatalf("expected cannot read file error, got %v", err)
	}
}

func TestMarkdownOverwritePrettyOutputUsesDataVersionFallback(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token":   "box_md_existing",
				"data_version": "7633658129540910625",
			},
		},
	})

	err := mountAndRunMarkdown(t, MarkdownOverwrite, []string{
		"+overwrite",
		"--file-token", "box_md_existing",
		"--name", "README.md",
		"--content", "# updated\n",
		"--format", "pretty",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "file_name: README.md") {
		t.Fatalf("pretty output missing file_name: %s", out)
	}
	if !strings.Contains(out, "version: 7633658129540910625") {
		t.Fatalf("pretty output missing fallback version: %s", out)
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

func TestMarkdownFetchDownloadNetworkError(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownFetch, []string{
		"+fetch",
		"--file-token", "box_md_fetch",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("expected download failed error, got %v", err)
	}
}

func TestMarkdownFetchReadFailure(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())
	f.HttpClient = func() (*http.Client, error) {
		return &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Header: http.Header{
						"Content-Type":        {"text/plain"},
						"Content-Disposition": {`attachment; filename="README.md"`},
					},
					Body: &errReadCloser{err: errors.New("stream broke")},
				}, nil
			}),
		}, nil
	}

	err := mountAndRunMarkdown(t, MarkdownFetch, []string{
		"+fetch",
		"--file-token", "box_md_fetch",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("expected read failure error, got %v", err)
	}
}

func TestMarkdownFetchPrettyReturnsContent(t *testing.T) {
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
		"--format", "pretty",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "# hello\n") {
		t.Fatalf("pretty output missing content: %s", out)
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

func TestMarkdownFetchPrettySavesFile(t *testing.T) {
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
		"--format", "pretty",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "saved_path:") {
		t.Fatalf("pretty output missing saved_path: %s", out)
	}
	if !strings.Contains(out, "file_name: README.md") {
		t.Fatalf("pretty output missing file_name: %s", out)
	}
}

func TestMarkdownFetchSaveFailure(t *testing.T) {
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
	f.FileIOProvider = &staticFileIOProvider{
		fileIO: &failingSaveFileIO{
			base: fileio.GetProvider().ResolveFileIO(context.Background()),
			err:  errors.New("disk full"),
		},
	}

	err := mountAndRunMarkdown(t, MarkdownFetch, []string{
		"+fetch",
		"--file-token", "box_md_fetch",
		"--output", "copy.md",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "cannot create file") {
		t.Fatalf("expected save failure error, got %v", err)
	}
}
