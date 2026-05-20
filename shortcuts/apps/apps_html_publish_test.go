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

	"github.com/larksuite/cli/internal/output"
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
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o644)
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
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected ExitError with detail, got %v", err)
	}
	if exitErr.Detail.Type != "validation" {
		t.Fatalf("error.type = %q, want validation", exitErr.Detail.Type)
	}
	if !strings.Contains(exitErr.Detail.Message, "index.html") {
		t.Fatalf("message missing 'index.html': %v", exitErr.Detail.Message)
	}
	if exitErr.Detail.Hint == "" {
		t.Fatalf("expected non-empty hint")
	}
	if len(fake.calls) != 0 {
		t.Fatalf("client should not be called when index.html missing")
	}
}

func TestRunHTMLPublish_DirWithIndexHTMLPasses(t *testing.T) {
	// 目录含 index.html 应该正常走完
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "extra.html"), []byte("<html></html>"), 0o644)
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
	_ = os.WriteFile(single, []byte("<html></html>"), 0o644)
	fake := &fakeAppsHTMLPublishClient{}
	_, err := runHTMLPublish(context.Background(), newTestFIO(), fake, appsHTMLPublishSpec{AppID: "app_x", Path: single})
	if err == nil {
		t.Fatalf("single-file path 'foo.html' should be rejected (not named index.html)")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil || exitErr.Detail.Type != "validation" {
		t.Fatalf("expected ExitError type=validation, got %v", err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("client must not be called when index.html missing")
	}
}

func TestRunHTMLPublish_SingleFileNamedIndexPasses(t *testing.T) {
	// 单文件形态：文件名恰好就是 index.html → 放行
	dir := t.TempDir()
	single := filepath.Join(dir, "index.html")
	_ = os.WriteFile(single, []byte("<html></html>"), 0o644)
	fake := &fakeAppsHTMLPublishClient{resp: &htmlPublishResponse{URL: "https://miaoda/app_x"}}
	if _, err := runHTMLPublish(context.Background(), newTestFIO(), fake, appsHTMLPublishSpec{AppID: "app_x", Path: single}); err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("client should be called for single index.html")
	}
}

func TestRunHTMLPublish_RejectsOversizeTarball(t *testing.T) {
	// 临时把上限调到 100 字节验证拦截，恢复原值避免污染其它测试。
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
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected ExitError with detail, got %v", err)
	}
	if exitErr.Detail.Type != "validation" {
		t.Fatalf("error.type = %q, want validation", exitErr.Detail.Type)
	}
	if !strings.Contains(exitErr.Detail.Message, "exceeds") {
		t.Fatalf("message missing 'exceeds': %v", exitErr.Detail.Message)
	}
	if exitErr.Detail.Hint == "" {
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
	if err := os.WriteFile("index.html", []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsHTMLPublish,
		[]string{"+html-publish", "--app-id", "app_x", "--path", ".", "--dry-run", "--as", "user"},
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
