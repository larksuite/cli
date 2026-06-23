// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeAppsHTMLPublishClient struct {
	resp  *htmlPublishResponse
	err   error
	calls []string
}

func (f *fakeAppsHTMLPublishClient) HTMLPublish(ctx context.Context, appID string, tarball *htmlPublishTarball) (*htmlPublishResponse, error) {
	f.calls = append(f.calls, appID)
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func writeAppsSampleSite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return dir
}

func TestRunHTMLPublish_HappyPath(t *testing.T) {
	site := writeAppsSampleSite(t)
	fake := &fakeAppsHTMLPublishClient{
		resp: &htmlPublishResponse{URL: "https://miaoda/app_x"},
	}
	out, err := runHTMLPublish(context.Background(), newTestFIO(), fake, appsHTMLPublishSpec{AppID: "app_x", Path: site})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if out["url"] != "https://miaoda/app_x" {
		t.Fatalf("url=%v", out["url"])
	}
	if len(fake.calls) != 1 || fake.calls[0] != "app_x" {
		t.Fatalf("calls=%v", fake.calls)
	}
}

func TestRunHTMLPublish_OnlyURLInEnvelope(t *testing.T) {
	// Pin 概要设计 §5.3 不变量 4 "同步语义不会变成异步":
	// envelope 只含 url，未来若有人加 status / release_id 字段会被这个测试拦截。
	site := writeAppsSampleSite(t)
	fake := &fakeAppsHTMLPublishClient{
		resp: &htmlPublishResponse{URL: "https://miaoda/app_x"},
	}
	out, err := runHTMLPublish(context.Background(), newTestFIO(), fake, appsHTMLPublishSpec{AppID: "app_x", Path: site})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(out) != 1 {
		t.Fatalf("envelope should only contain 'url', got %d keys: %v", len(out), out)
	}
	if _, ok := out["url"]; !ok {
		t.Fatalf("envelope missing 'url': %v", out)
	}
}

func TestRunHTMLPublish_ClientErrorPropagated(t *testing.T) {
	site := writeAppsSampleSite(t)
	wantErr := errors.New("server timeout")
	fake := &fakeAppsHTMLPublishClient{err: wantErr}
	_, err := runHTMLPublish(context.Background(), newTestFIO(), fake, appsHTMLPublishSpec{AppID: "app_x", Path: site})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err=%v", err)
	}
}

func TestRunHTMLPublish_PathNotFound(t *testing.T) {
	fake := &fakeAppsHTMLPublishClient{}
	_, err := runHTMLPublish(context.Background(), newTestFIO(), fake, appsHTMLPublishSpec{AppID: "app_x", Path: "/nonexistent"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if len(fake.calls) != 0 {
		t.Fatalf("client should not be called when path invalid")
	}
}

func TestRunHTMLPublish_DirRequiresIndexHTML(t *testing.T) {
	// 目录形态：缺 index.html 应该被拦
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	fake := &fakeAppsHTMLPublishClient{}
	_, err := runHTMLPublish(context.Background(), newTestFIO(), fake, appsHTMLPublishSpec{AppID: "app_x", Path: dir})
	if err == nil {
		t.Fatalf("expected error for missing index.html")
	}
	problem := requireAppsValidationProblem(t, err)
	if !strings.Contains(problem.Message, "index.html") {
		t.Fatalf("message missing 'index.html': %v", problem.Message)
	}
	if problem.Hint == "" {
		t.Fatalf("expected non-empty hint")
	}
	if len(fake.calls) != 0 {
		t.Fatalf("client should not be called when index.html missing")
	}
}

func TestRunHTMLPublish_DirWithIndexHTMLPasses(t *testing.T) {
	// 目录含 index.html 应该正常走完
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extra.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	fake := &fakeAppsHTMLPublishClient{resp: &htmlPublishResponse{URL: "https://miaoda/app_x"}}
	if _, err := runHTMLPublish(context.Background(), newTestFIO(), fake, appsHTMLPublishSpec{AppID: "app_x", Path: dir}); err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("client should be called when index.html present")
	}
}

func TestRunHTMLPublish_SingleFileRejectedIfNotNamedIndex(t *testing.T) {
	// 单文件形态：文件名不是 index.html 也要拦
	dir := t.TempDir()
	single := filepath.Join(dir, "foo.html")
	if err := os.WriteFile(single, []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	fake := &fakeAppsHTMLPublishClient{}
	_, err := runHTMLPublish(context.Background(), newTestFIO(), fake, appsHTMLPublishSpec{AppID: "app_x", Path: single})
	if err == nil {
		t.Fatalf("single-file path 'foo.html' should be rejected (not named index.html)")
	}
	requireAppsValidationProblem(t, err)
	if len(fake.calls) != 0 {
		t.Fatalf("client must not be called when index.html missing")
	}
}

func TestRunHTMLPublish_SingleFileNamedIndexPasses(t *testing.T) {
	// 单文件形态：文件名恰好就是 index.html → 放行
	dir := t.TempDir()
	single := filepath.Join(dir, "index.html")
	if err := os.WriteFile(single, []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	fake := &fakeAppsHTMLPublishClient{resp: &htmlPublishResponse{URL: "https://miaoda/app_x"}}
	if _, err := runHTMLPublish(context.Background(), newTestFIO(), fake, appsHTMLPublishSpec{AppID: "app_x", Path: single}); err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("client should be called for single index.html")
	}
}

func TestRunHTMLPublish_RejectsOversizeTarball(t *testing.T) {
	// 把上限调到 100 字节验证拦截，defer 恢复原值避免污染其它测试。
	orig := maxHTMLPublishTarballBytes
	maxHTMLPublishTarballBytes = 100
	defer func() { maxHTMLPublishTarballBytes = orig }()

	dir := t.TempDir()
	// 写 index.html（满足新加的 index 校验）+ 大文件超 100 字节上限。
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "big.html"),
		[]byte(strings.Repeat("x", 4096)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fake := &fakeAppsHTMLPublishClient{}
	_, err := runHTMLPublish(context.Background(), newTestFIO(), fake, appsHTMLPublishSpec{AppID: "app_x", Path: dir})
	if err == nil {
		t.Fatalf("expected oversize error")
	}
	problem := requireAppsValidationProblem(t, err)
	if !strings.Contains(problem.Message, "exceeds") {
		t.Fatalf("message missing 'exceeds': %v", problem.Message)
	}
	if problem.Hint == "" {
		t.Fatalf("expected non-empty hint")
	}
	if len(fake.calls) != 0 {
		t.Fatalf("client should not be called when tarball oversize")
	}
}

func TestMaxHTMLPublishTarballBytes_Default(t *testing.T) {
	// Pin 20MB 常量值，typo 到 20*1000*1024 之类会被拦截。
	if maxHTMLPublishTarballBytes != 20*1024*1024 {
		t.Fatalf("default = %d, want %d (20MiB)", maxHTMLPublishTarballBytes, 20*1024*1024)
	}
}

func TestAppsHTMLPublish_RequiresAppID(t *testing.T) {
	site := writeAppsSampleSite(t)
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsHTMLPublish,
		[]string{"+html-publish", "--path", site}, factory, stdout)
	// cobra Required:true may report flag name without "--" prefix
	if err == nil || !strings.Contains(err.Error(), "app-id") {
		t.Fatalf("expected --app-id required, got %v", err)
	}
}

func TestAppsHTMLPublish_RequiresPath(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsHTMLPublish,
		[]string{"+html-publish", "--app-id", "app_x"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("expected --path required, got %v", err)
	}
}

func TestAppsHTMLPublish_DryRunPrintsManifest(t *testing.T) {
	// 这个用例走真实 shortcut → 真实 LocalFileIO（cwd-bounded）。
	// 必须 chdir 进 tmp 用相对路径，否则 SafeInputPath 会拒绝绝对 --path。
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.MkdirAll(filepath.Join(dir, "dist"), 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dist", "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsHTMLPublish,
		[]string{"+html-publish", "--app-id", "app_x", "--path", "./dist", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "/open-apis/spark/v1/apps/app_x/upload_and_release_html_code") {
		t.Fatalf("dry-run missing endpoint: %s", got)
	}
	if !strings.Contains(got, "index.html") {
		t.Fatalf("dry-run missing file list: %s", got)
	}
}

// TestAppsHTMLPublish_CleanCwdIsAllowed pins the post-PR behavior change:
// --path "." is no longer hard-rejected by Validate. A clean cwd (no
// credential files) is a valid publish target.
func TestAppsHTMLPublish_CleanCwdIsAllowed(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsHTMLPublish,
		[]string{"+html-publish", "--app-id", "app_x", "--path", ".", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run with --path . should pass when cwd is clean, got err=%v", err)
	}
}

// TestAppsHTMLPublish_SensitiveBlocksValidate pins the new behavior: a credential
// file under --path causes Validate to reject before either DryRun or Execute
// runs, so dry-run also returns non-zero (unlike the previous advisory-warning
// model).
func TestAppsHTMLPublish_SensitiveBlocksValidate(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.MkdirAll(filepath.Join(dir, "dist"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dist", "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dist", ".env"), []byte("API_KEY=secret"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	// Dry-run path: must also fail (this is the whole point of moving the
	// check into Validate — dry-run can no longer say "OK" when Execute would
	// reject).
	factory, stdout, _ := newAppsExecuteFactory(t)
	err = runAppsShortcut(t, AppsHTMLPublish,
		[]string{"+html-publish", "--app-id", "app_x", "--path", "./dist", "--dry-run", "--as", "user"},
		factory, stdout)
	if err == nil {
		t.Fatalf("dry-run with sensitive file should fail")
	}
	problem := requireAppsValidationProblem(t, err)
	if !strings.Contains(problem.Message, ".env") {
		t.Fatalf("error message should list the offending file, got %q", problem.Message)
	}
	if !strings.Contains(problem.Hint, "--allow-sensitive") {
		t.Fatalf("error hint should mention --allow-sensitive escape hatch, got %q", problem.Hint)
	}
}

// TestAppsHTMLPublish_AllowSensitiveOverride pins that --allow-sensitive
// bypasses the credential-file check (legitimate cases like a docs site
// shipping an example .env on purpose).
func TestAppsHTMLPublish_AllowSensitiveOverride(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.MkdirAll(filepath.Join(dir, "dist"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dist", "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dist", ".env.example"), []byte("API_KEY=replace-me"), 0o644); err != nil {
		t.Fatalf("write .env.example: %v", err)
	}

	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsHTMLPublish,
		[]string{"+html-publish", "--app-id", "app_x", "--path", "./dist", "--dry-run", "--allow-sensitive", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("--allow-sensitive should bypass the credential scan, got err=%v", err)
	}
	got := stdout.String()
	// Dry-run output surfaces the waived list so the caller still sees what
	// was let through.
	if !strings.Contains(got, "sensitive_waived") {
		t.Fatalf("dry-run output should record the waived credential file under --allow-sensitive, got: %s", got)
	}
	if !strings.Contains(got, ".env.example") {
		t.Fatalf("waived list should name the file, got: %s", got)
	}
}

// TestAppsHTMLPublish_SensitiveBlocksWhenPathIsCredentialParentDir pins that
// the credential-file scan still rejects when --path itself is the
// conventional parent dir (e.g. ./.aws, ./.docker, ./.kube). Without joining
// the candidate back to its absolute path, walker would strip the parent
// segment via filepath.Rel and the cloud-SDK matchers — which anchor on
// parent/file pairs — would silently pass.
func TestAppsHTMLPublish_SensitiveBlocksWhenPathIsCredentialParentDir(t *testing.T) {
	cases := []struct {
		name       string
		parent     string
		fileName   string
		wantSubstr string
	}{
		{"aws_credentials", ".aws", "credentials", "credentials"},
		{"docker_config_json", ".docker", "config.json", "config.json"},
		{"kube_config", ".kube", "config", "config"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cwd, err := os.Getwd()
			if err != nil {
				t.Fatalf("getwd: %v", err)
			}
			if err := os.Chdir(dir); err != nil {
				t.Fatalf("chdir: %v", err)
			}
			t.Cleanup(func() { _ = os.Chdir(cwd) })
			root := filepath.Join(dir, tc.parent)
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(root, tc.fileName), []byte("fake credential"), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<html></html>"), 0o644); err != nil {
				t.Fatalf("write index: %v", err)
			}

			factory, stdout, _ := newAppsExecuteFactory(t)
			err = runAppsShortcut(t, AppsHTMLPublish,
				[]string{"+html-publish", "--app-id", "app_x", "--path", "./" + tc.parent, "--dry-run", "--as", "user"},
				factory, stdout)
			if err == nil {
				t.Fatalf("expected rejection when --path is %s/ (would leak %s), got success", tc.parent, tc.fileName)
			}
			problem := requireAppsValidationProblem(t, err)
			if !strings.Contains(problem.Message, tc.wantSubstr) {
				t.Fatalf("error message should name the leaked file, got %q", problem.Message)
			}
		})
	}
}

// TestAppsHTMLPublish_SensitiveBlocksWhenPathIsCredentialFileItself pins the
// single-file form: --path pointing directly at a credential file (e.g.
// ./.aws/credentials) must also reject. Walker's single-file branch sets
// RelPath = filepath.Base(rootPath), so the .aws segment is lost the same way.
func TestAppsHTMLPublish_SensitiveBlocksWhenPathIsCredentialFileItself(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.MkdirAll(filepath.Join(dir, ".aws"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".aws", "credentials"), []byte("fake credential"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	factory, stdout, _ := newAppsExecuteFactory(t)
	err = runAppsShortcut(t, AppsHTMLPublish,
		[]string{"+html-publish", "--app-id", "app_x", "--path", "./.aws/credentials", "--dry-run", "--as", "user"},
		factory, stdout)
	if err == nil {
		t.Fatalf("expected rejection when --path points directly at .aws/credentials, got success")
	}
	problem := requireAppsValidationProblem(t, err)
	if !strings.Contains(problem.Message, "credentials") {
		t.Fatalf("error message should name the leaked file, got %q", problem.Message)
	}
}

// TestSensitiveCandidatesError_Truncation pins the inline-list truncation so a
// payload with many credential files (e.g. an accidentally-copied tree of
// per-stage .env.* files) produces a readable, length-bounded error.
func TestSensitiveCandidatesError_Truncation(t *testing.T) {
	hits := []string{"a.env", "b.env", "c.env", "d.env", "e.env", "f.env", "g.env"}
	err := sensitiveCandidatesError(hits)
	msg := requireAppsValidationProblem(t, err).Message
	if !strings.Contains(msg, "7 credential file(s)") {
		t.Fatalf("message should report the full count, got %q", msg)
	}
	if !strings.Contains(msg, "and 2 more") {
		t.Fatalf("message should truncate beyond %d entries, got %q", maxSensitiveListInError, msg)
	}
	// Pin: the truncated tail is NOT spelled out.
	if strings.Contains(msg, "g.env") {
		t.Fatalf("message should not list entries past the truncation, got %q", msg)
	}
}

func TestRunHTMLPublish_RejectsOversizeRawCandidates(t *testing.T) {
	orig := maxHTMLPublishRawBytes
	maxHTMLPublishRawBytes = 100
	defer func() { maxHTMLPublishRawBytes = orig }()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "big.html"), []byte(strings.Repeat("x", 4096)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fake := &fakeAppsHTMLPublishClient{}
	_, err := runHTMLPublish(context.Background(), newTestFIO(), fake,
		appsHTMLPublishSpec{AppID: "app_x", Path: dir})
	if err == nil {
		t.Fatalf("expected raw-size cap to fire")
	}
	problem := requireAppsValidationProblem(t, err)
	if !strings.Contains(problem.Message, "raw") || !strings.Contains(problem.Message, "bytes") {
		t.Fatalf("expected message to explain raw-byte cap, got %q", problem.Message)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("client must not be called when raw cap hit")
	}
}
