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
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
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

func markdownPermissionTestConfig(userOpenID string) *core.CliConfig {
	return &core.CliConfig{
		AppID: "markdown-perm-test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
		UserOpenId: userOpenID,
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
	want := []string{"+create", "+diff", "+fetch", "+patch", "+overwrite"}

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
			name: "wiki token cannot be empty",
			args: []string{
				"+create",
				"--name", "README.md",
				"--content", "# hello",
				"--wiki-token=",
			},
			want: "--wiki-token cannot be empty",
		},
		{
			name: "folder and wiki tokens are mutually exclusive",
			args: []string{
				"+create",
				"--name", "README.md",
				"--content", "# hello",
				"--folder-token", "fld_target",
				"--wiki-token", "wikcn_target",
			},
			want: "--folder-token and --wiki-token are mutually exclusive",
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
			name: "wiki token must be valid",
			args: []string{
				"+create",
				"--name", "README.md",
				"--content", "# hello",
				"--wiki-token", "../bad",
			},
			want: "--wiki-token",
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
	if !strings.Contains(out, "/open-apis/drive/v1/metas/batch_query") || !strings.Contains(out, `"with_url": true`) {
		t.Fatalf("dry-run missing metadata URL lookup: %s", out)
	}
	if !strings.Contains(out, "markdown content") {
		t.Fatalf("dry-run missing content marker: %s", out)
	}
}

func TestMarkdownCreateDryRunWithWikiToken(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--name", "README.md",
		"--content", "# hello",
		"--wiki-token", "wikcn_markdown_dryrun_target",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, `"parent_type": "wiki"`) {
		t.Fatalf("dry-run missing wiki parent_type: %s", out)
	}
	if !strings.Contains(out, `"parent_node": "wikcn_markdown_dryrun_target"`) {
		t.Fatalf("dry-run missing wiki parent_node: %s", out)
	}
	if !strings.Contains(out, "/open-apis/drive/v1/metas/batch_query") || !strings.Contains(out, `"with_url": true`) {
		t.Fatalf("dry-run missing metadata URL lookup: %s", out)
	}
}

func TestMarkdownCreateDryRunNormalizesFolderURL(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--name", "README.md",
		"--content", "# hello",
		"--folder-token", "https://feishu.cn/drive/folder/fldcnMarkdownTarget",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, `"parent_type": "explorer"`) {
		t.Fatalf("dry-run missing explorer parent_type: %s", out)
	}
	if !strings.Contains(out, `"parent_node": "fldcnMarkdownTarget"`) {
		t.Fatalf("dry-run did not normalize folder URL to token: %s", out)
	}
	if strings.Contains(out, "https://feishu.cn/drive/folder/") {
		t.Fatalf("dry-run leaked raw folder URL instead of token: %s", out)
	}
}

func TestMarkdownCreateRejectsWikiURLInFolderToken(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--name", "README.md",
		"--content", "# hello",
		"--folder-token", "https://feishu.cn/wiki/wikcnWrongFlag",
	}, f, stdout)
	if err == nil {
		t.Fatalf("expected folder-token URL type error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf() ok=false for %T: %v", err, err)
	}
	if !strings.Contains(p.Message, "must identify a Drive folder") || !strings.Contains(p.Hint, "Use --wiki-token") {
		t.Fatalf("expected folder-token URL type error, got %v", err)
	}
}

func TestMarkdownCreateRejectsDocURLInWikiToken(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, markdownTestConfig())

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--name", "README.md",
		"--content", "# hello",
		"--wiki-token", "https://feishu.cn/docx/docxWrongFlag",
	}, f, stdout)
	if err == nil {
		t.Fatalf("expected wiki-token URL type error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf() ok=false for %T: %v", err, err)
	}
	if !strings.Contains(p.Message, "must identify a wiki node") || !strings.Contains(p.Hint, "+node-get") {
		t.Fatalf("expected wiki-token URL type error, got %v", err)
	}
}

func TestNormalizeMarkdownTargetTokensRejectAmbiguousInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		run      func() (string, error)
		wantMsg  string
		wantHint string
	}{
		{
			name:     "wiki token passed as folder token",
			run:      func() (string, error) { return normalizeMarkdownFolderToken("wik_placeholder_wrong") },
			wantMsg:  "--folder-token looks like a wiki node token",
			wantHint: "--wiki-token",
		},
		{
			name:     "folder token path fragment",
			run:      func() (string, error) { return normalizeMarkdownFolderToken("folder_token/child") },
			wantMsg:  "--folder-token must be a raw token",
			wantHint: "full Lark URL",
		},
		{
			name:     "doc token passed as wiki token",
			run:      func() (string, error) { return normalizeMarkdownWikiToken("docx_placeholder_wrong") },
			wantMsg:  "--wiki-token must be a wiki node token",
			wantHint: "",
		},
		{
			name:     "wiki token query fragment",
			run:      func() (string, error) { return normalizeMarkdownWikiToken("wik_placeholder?from=copy") },
			wantMsg:  "--wiki-token must be a raw token",
			wantHint: "path/query/fragment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.run()
			if err == nil {
				t.Fatalf("expected validation error")
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("ProblemOf() ok=false for %T: %v", err, err)
			}
			if !strings.Contains(p.Message, tt.wantMsg) {
				t.Fatalf("message = %q, want substring %q", p.Message, tt.wantMsg)
			}
			if tt.wantHint != "" && !strings.Contains(p.Hint, tt.wantHint) {
				t.Fatalf("hint = %q, want substring %q", p.Hint, tt.wantHint)
			}
		})
	}
}

func TestNormalizeMarkdownTargetTokensAcceptRawTokens(t *testing.T) {
	t.Parallel()

	folderToken, err := normalizeMarkdownFolderToken("folder_token_raw")
	if err != nil {
		t.Fatalf("normalizeMarkdownFolderToken() error = %v", err)
	}
	if folderToken != "folder_token_raw" {
		t.Fatalf("folder token = %q", folderToken)
	}

	wikiToken, err := normalizeMarkdownWikiToken("wik_placeholder_raw")
	if err != nil {
		t.Fatalf("normalizeMarkdownWikiToken() error = %v", err)
	}
	if wikiToken != "wik_placeholder_raw" {
		t.Fatalf("wiki token = %q", wikiToken)
	}
}

func TestMarkdownUploadProblemAddsQuotaAndServerHints(t *testing.T) {
	t.Parallel()

	quotaErr := errs.NewAPIError(errs.SubtypeQuotaExceeded, "file quota exceeded").WithCode(1061101)
	got := markdownUploadProblem(quotaErr, markdownUploadAllAction)
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("ProblemOf(quotaErr) ok=false")
	}
	if !strings.Contains(p.Hint, "storage quota is exhausted") {
		t.Fatalf("quota hint = %q", p.Hint)
	}

	serverErr := errs.NewAPIError(errs.SubtypeServerError, "NA").WithCode(233523001).WithRetryable()
	got = markdownUploadProblem(serverErr, markdownUploadAllAction)
	p, ok = errs.ProblemOf(got)
	if !ok {
		t.Fatalf("ProblemOf(serverErr) ok=false")
	}
	if !p.Retryable || !strings.Contains(p.Hint, "transient server error") {
		t.Fatalf("server retryable=%v hint=%q", p.Retryable, p.Hint)
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
	if !strings.Contains(out, "/open-apis/drive/v1/metas/batch_query") || !strings.Contains(out, `"with_url": true`) {
		t.Fatalf("dry-run missing metadata URL lookup: %s", out)
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
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "box_md_create", "doc_type": "file", "url": "https://tenant.example.com/file/box_md_create"},
				},
			},
		},
	})

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
	if !strings.Contains(stdout.String(), `"url": "https://tenant.example.com/file/box_md_create"`) {
		t.Fatalf("stdout missing url: %s", stdout.String())
	}
}

func TestMarkdownCreateSuccessUploadAllToWikiReturnsMetaURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "box_md_create_wiki",
				"version":    "1002",
			},
		},
	}
	reg.Register(uploadStub)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "box_md_create_wiki", "doc_type": "file", "url": "https://tenant.example.com/file/box_md_create_wiki"},
				},
			},
		},
	})

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--name", "README.md",
		"--content", "# hello\n",
		"--wiki-token", "wikcn_markdown_create_target",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedMultipartBody(t, uploadStub)
	if got := body.Fields["parent_type"]; got != markdownUploadParentTypeWiki {
		t.Fatalf("parent_type = %q, want %q", got, markdownUploadParentTypeWiki)
	}
	if got := body.Fields["parent_node"]; got != "wikcn_markdown_create_target" {
		t.Fatalf("parent_node = %q, want %q", got, "wikcn_markdown_create_target")
	}
	if !strings.Contains(stdout.String(), `"url": "https://tenant.example.com/file/box_md_create_wiki"`) {
		t.Fatalf("stdout missing metadata url for wiki-hosted markdown file: %s", stdout.String())
	}
}

func TestMarkdownCreateUploadAllReturnsTypedScopeError(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 99991672,
			"msg":  "Access denied. One of the following scopes is required: [drive:file:upload]",
			"error": map[string]interface{}{
				"log_id": "log-md-upload-scope",
			},
		},
	})

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--name", "README.md",
		"--content", "# hello\n",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected scope error")
	}

	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T (%v)", err, err)
	}
	if p.Code != 99991672 {
		t.Fatalf("code = %d, want 99991672", p.Code)
	}
	if p.Subtype != errs.SubtypeAppScopeNotApplied {
		t.Fatalf("subtype = %s, want %s", p.Subtype, errs.SubtypeAppScopeNotApplied)
	}
	if !strings.HasPrefix(p.Message, markdownUploadAllAction+": ") {
		t.Fatalf("message = %q, want %q prefix", p.Message, markdownUploadAllAction+": ")
	}
	if !strings.Contains(p.Hint, "lacks the required document upload scope") {
		t.Fatalf("hint = %q, want upload scope guidance", p.Hint)
	}
}

func TestMarkdownCreateUploadAllRetriesRateLimit(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 99991400,
			"msg":  "request frequency limit exceeded",
			"error": map[string]interface{}{
				"log_id": "log-md-upload-ratelimit-1",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "box_md_retry_success",
				"version":    "1003",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "box_md_retry_success", "doc_type": "file", "url": "https://tenant.example.com/file/box_md_retry_success"},
				},
			},
		},
	})

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--name", "README.md",
		"--content", "# hello\n",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr.String(), "retrying (attempt 1/2)") {
		t.Fatalf("stderr = %q, want retry log", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"file_token": "box_md_retry_success"`) {
		t.Fatalf("stdout missing retried upload token: %s", stdout.String())
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
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "box_md_create_pretty", "doc_type": "file", "url": "https://tenant.example.com/file/box_md_create_pretty"},
				},
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
	if !strings.Contains(out, "url: https://tenant.example.com/file/box_md_create_pretty") {
		t.Fatalf("pretty output missing url: %s", out)
	}
	if !strings.Contains(out, "permission_grant.status: skipped") {
		t.Fatalf("pretty output missing permission_grant.status: %s", out)
	}
	if !strings.Contains(out, "permission_grant.perm: full_access") {
		t.Fatalf("pretty output missing permission_grant.perm: %s", out)
	}
}

func TestMarkdownCreateBotAutoGrantSkippedNoUser(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownPermissionTestConfig(""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "box_md_skipped",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "box_md_skipped", "doc_type": "file", "url": "https://example.feishu.cn/file/box_md_skipped"},
				},
			},
		},
	})

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--name", "README.md",
		"--content", "# hello\n",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var envelope struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	grant, _ := envelope.Data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantSkipped {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantSkipped)
	}
	if hint, ok := grant["hint"].(string); !ok || !strings.Contains(hint, "auth login") {
		t.Fatalf("hint = %#v, want string containing 'auth login'", grant["hint"])
	}
}

func TestMarkdownCreateBotAutoGrantFailed(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownPermissionTestConfig("ou_current_user"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"file_token": "box_md_grant_fail",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "box_md_grant_fail", "doc_type": "file", "url": "https://example.feishu.cn/file/box_md_grant_fail"},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/box_md_grant_fail/members",
		Body: map[string]interface{}{
			"code": 230001,
			"msg":  "no permission",
		},
	})

	err := mountAndRunMarkdown(t, MarkdownCreate, []string{
		"+create",
		"--name", "README.md",
		"--content", "# hello\n",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var envelope struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	grant, _ := envelope.Data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantFailed {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantFailed)
	}
	if hint, ok := grant["hint"].(string); !ok || !strings.Contains(hint, "Retry later") {
		t.Fatalf("hint = %#v, want string containing 'Retry later'", grant["hint"])
	}
}

// requireMarkdownValidationParam asserts err is a typed validation envelope
// (category + subtype) whose recoverable Param names the expected flag. It does
// not assert a cause: Param-tagged validation failures such as the +diff content
// limit carry no underlying error.
func requireMarkdownValidationParam(t *testing.T, err error, want string) {
	t.Helper()
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T (%v)", err, err)
	}
	if p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("classification = %s/%s, want %s/%s", p.Category, p.Subtype, errs.CategoryValidation, errs.SubtypeInvalidArgument)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T (%v)", err, err)
	}
	if ve.Param != want {
		t.Fatalf("validation param = %q, want %q", ve.Param, want)
	}
}

// requireMarkdownValidationParamWithCause is requireMarkdownValidationParam for
// file open/read failures, which wrap the underlying os error via WithCause. It
// additionally enforces that the cause is preserved. Validation failures that
// carry no underlying error (e.g. the +diff content limit) use the plain helper.
func requireMarkdownValidationParamWithCause(t *testing.T, err error, want string) {
	t.Helper()
	requireMarkdownValidationParam(t, err, want)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Cause == nil {
		t.Fatalf("expected validation cause to be preserved, got %T (%v)", err, err)
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
	requireMarkdownValidationParamWithCause(t, err, "--file")
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
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "box_md_multipart", "doc_type": "file", "url": "https://tenant.example.com/file/box_md_multipart"},
				},
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
	if !strings.Contains(stdout.String(), `"url": "https://tenant.example.com/file/box_md_multipart"`) {
		t.Fatalf("stdout missing multipart metadata url: %s", stdout.String())
	}
}

func TestMarkdownCreateMultipartUploadToWikiUsesWikiParent(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	prepareStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_prepare",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"upload_id":  "upload_markdown_wiki_ok",
				"block_size": float64(markdownSinglePartSizeLimit),
				"block_num":  float64(2),
			},
		},
	}
	reg.Register(prepareStub)
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
				"file_token": "box_md_multipart_wiki",
				"version":    "1005",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": "box_md_multipart_wiki", "doc_type": "file", "url": "https://tenant.example.com/file/box_md_multipart_wiki"},
				},
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
		"--wiki-token", "wikcn_markdown_multipart_target",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(prepareStub.CapturedBody, &body); err != nil {
		t.Fatalf("decode upload_prepare body: %v\nraw=%s", err, string(prepareStub.CapturedBody))
	}
	if got := body["parent_type"]; got != markdownUploadParentTypeWiki {
		t.Fatalf("parent_type = %#v, want %q", got, markdownUploadParentTypeWiki)
	}
	if got := body["parent_node"]; got != "wikcn_markdown_multipart_target" {
		t.Fatalf("parent_node = %#v, want %q", got, "wikcn_markdown_multipart_target")
	}
	if !strings.Contains(stdout.String(), `"url": "https://tenant.example.com/file/box_md_multipart_wiki"`) {
		t.Fatalf("stdout missing metadata url for wiki-hosted multipart markdown file: %s", stdout.String())
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

func TestWithMarkdownUploadRetryDataDoesNotRetryNonRetryable(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, markdownTestConfig())
	rt := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+create"}, markdownTestConfig(), f, core.AsUser)

	attempts := 0
	expected := errs.NewAPIError(errs.SubtypePermissionDenied, "permission denied").WithCode(1061004)
	_, err := withMarkdownUploadRetryData(rt, markdownUploadAllAction, func() (map[string]interface{}, error) {
		attempts++
		return nil, expected
	})
	if err != expected {
		t.Fatalf("err = %v, want original error", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want no retry log", stderr.String())
	}
}

func TestWithMarkdownUploadRetryVoidExhaustedAppendsHint(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, markdownTestConfig())
	rt := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+create"}, markdownTestConfig(), f, core.AsUser)

	orig := markdownUploadRetryBackoffs
	markdownUploadRetryBackoffs = []time.Duration{0, 0}
	t.Cleanup(func() { markdownUploadRetryBackoffs = orig })

	attempts := 0
	err := withMarkdownUploadRetryVoid(rt, markdownUploadFinishAction, func() error {
		attempts++
		return errs.NewAPIError(errs.SubtypeRateLimit, "too many requests").WithCode(99991400).WithRetryable()
	})
	if err == nil {
		t.Fatal("expected retryable error")
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T (%v)", err, err)
	}
	if !strings.Contains(p.Hint, "remained retryable after 3 attempts") {
		t.Fatalf("hint = %q, want retry exhaustion guidance", p.Hint)
	}
	if strings.Count(stderr.String(), "retrying (attempt") != 2 {
		t.Fatalf("stderr = %q, want 2 retry logs", stderr.String())
	}
}

func TestMarkdownUploadShouldRetryBranches(t *testing.T) {
	if markdownUploadShouldRetry(errors.New("plain")) {
		t.Fatal("plain error should not be retryable")
	}
	if !markdownUploadShouldRetry(errs.NewAPIError(errs.SubtypeRateLimit, "slow down").WithRetryable()) {
		t.Fatal("retryable API error should be retryable")
	}
	if !markdownUploadShouldRetry(errs.NewNetworkError(errs.SubtypeNetworkServer, "gateway").WithCode(502)) {
		t.Fatal("network error should be retryable by category")
	}
}

func TestMarkdownUploadRetryExhaustedZeroRetriesKeepsOriginal(t *testing.T) {
	original := errs.NewAPIError(errs.SubtypeRateLimit, "slow down").WithRetryable()
	got := markdownUploadRetryExhausted(original, markdownUploadAllAction, 0)
	if got != original {
		t.Fatalf("got = %v, want original error", got)
	}
}

func TestMarkdownUploadProblemAppendsCodeSpecificHints(t *testing.T) {
	tests := []struct {
		name string
		code int
		want string
	}{
		{
			name: "missing scope",
			code: 99991672,
			want: "lacks the required document upload scope",
		},
		{
			name: "version limit",
			code: 10071,
			want: "reached its version limit",
		},
		{
			name: "document capability",
			code: 90003087,
			want: "document capabilities enabled",
		},
		{
			name: "target not found",
			code: 1061044,
			want: "target folder or wiki node still exists",
		},
		{
			name: "no write access",
			code: 1062501,
			want: "has write access",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errs.NewAPIError(errs.SubtypeUnknown, "boom").WithCode(tt.code)
			got := markdownUploadProblem(err, markdownUploadAllAction)
			p, ok := errs.ProblemOf(got)
			if !ok {
				t.Fatalf("expected typed problem, got %T (%v)", got, got)
			}
			if !strings.HasPrefix(p.Message, markdownUploadAllAction+": ") {
				t.Fatalf("message = %q, want action prefix", p.Message)
			}
			if !strings.Contains(p.Hint, tt.want) {
				t.Fatalf("hint = %q, want substring %q", p.Hint, tt.want)
			}
		})
	}
}

func TestUploadMarkdownFileAllMissingFileTokenGetsActionPrefix(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"version": "1001",
			},
		},
	})

	_, err := uploadMarkdownFileAll(
		common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+create"}, markdownTestConfig(), f, core.AsUser),
		markdownUploadSpec{ContentSet: true},
		"README.md",
		int64(len("# hello\n")),
		func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("# hello\n")), nil
		},
	)
	if err == nil {
		t.Fatal("expected parse error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T (%v)", err, err)
	}
	if !strings.HasPrefix(p.Message, markdownUploadAllAction+": ") {
		t.Fatalf("message = %q, want %q prefix", p.Message, markdownUploadAllAction+": ")
	}
}

func TestUploadMarkdownFileMultipartOpenFailureNamesFileParam(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_prepare",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"upload_id":  "upload_123",
				"block_size": 4194304,
				"block_num":  1,
			},
		},
	})

	_, err := uploadMarkdownFileMultipart(
		common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+create"}, markdownTestConfig(), f, core.AsUser),
		markdownUploadSpec{FileSet: true, FilePath: "missing.md"},
		"missing.md",
		int64(1),
		func() (io.ReadCloser, error) {
			return nil, errors.New("open missing.md: no such file")
		},
	)
	if err == nil {
		t.Fatal("expected open failure after prepare, got nil")
	}
	requireMarkdownValidationParamWithCause(t, err, "--file")
}

func TestUploadMarkdownFileMultipartPrepareAndFinishParseErrorsGetActionPrefix(t *testing.T) {
	t.Run("prepare", func(t *testing.T) {
		f, _, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/files/upload_prepare",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"upload_id": "upload_123",
					"block_num": 1,
				},
			},
		})

		_, err := uploadMarkdownFileMultipart(
			common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+create"}, markdownTestConfig(), f, core.AsUser),
			markdownUploadSpec{ContentSet: true},
			"README.md",
			int64(len("# hello\n")),
			func() (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader("# hello\n")), nil
			},
		)
		if err == nil {
			t.Fatal("expected prepare parse error")
		}
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("expected typed problem, got %T (%v)", err, err)
		}
		if !strings.HasPrefix(p.Message, markdownUploadPrepareAction+": ") {
			t.Fatalf("message = %q, want %q prefix", p.Message, markdownUploadPrepareAction+": ")
		}
	})

	t.Run("finish", func(t *testing.T) {
		f, _, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/files/upload_prepare",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"upload_id":  "upload_123",
					"block_size": float64(8),
					"block_num":  float64(1),
				},
			},
		})
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/files/upload_part",
			Body:   map[string]interface{}{"code": 0, "msg": "ok"},
		})
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/files/upload_finish",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"version": "1001",
				},
			},
		})

		_, err := uploadMarkdownFileMultipart(
			common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+create"}, markdownTestConfig(), f, core.AsUser),
			markdownUploadSpec{ContentSet: true},
			"README.md",
			int64(len("# hello\n")),
			func() (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader("# hello\n")), nil
			},
		)
		if err == nil {
			t.Fatal("expected finish parse error")
		}
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("expected typed problem, got %T (%v)", err, err)
		}
		if !strings.HasPrefix(p.Message, markdownUploadFinishAction+": ") {
			t.Fatalf("message = %q, want %q prefix", p.Message, markdownUploadFinishAction+": ")
		}
	})
}

func TestAppendMarkdownProblemHintAppendsAndIgnoresBlank(t *testing.T) {
	err := errs.NewAPIError(errs.SubtypeUnknown, "boom").WithHint("first")
	appendMarkdownProblemHint(err, "second")
	appendMarkdownProblemHint(err, "   ")

	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T (%v)", err, err)
	}
	if p.Hint != "first\nsecond" {
		t.Fatalf("hint = %q, want newline-joined hints", p.Hint)
	}

	plain := errors.New("plain")
	if got := appendMarkdownProblemHint(plain, "ignored"); got != plain {
		t.Fatalf("plain error should pass through unchanged")
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
	f, stdout, _, reg := cmdutil.TestFactory(t, markdownTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 1061044,
			"msg":  "parent node not exist",
			"error": map[string]interface{}{
				"log_id": "log-md-meta-notfound",
			},
		},
	})

	err := mountAndRunMarkdown(t, MarkdownOverwrite, []string{
		"+overwrite",
		"--file-token", "box_md_existing",
		"--content", "# updated\n",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected metadata lookup failure")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T (%v)", err, err)
	}
	if p.Code != 1061044 {
		t.Fatalf("code = %d, want 1061044", p.Code)
	}
	if !strings.HasPrefix(p.Message, markdownFetchNameAction+": ") {
		t.Fatalf("message = %q, want %q prefix", p.Message, markdownFetchNameAction+": ")
	}
	if !strings.Contains(p.Hint, "target folder or wiki node still exists") {
		t.Fatalf("hint = %q, want target guidance", p.Hint)
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
	requireMarkdownValidationParamWithCause(t, err, "--file")
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
