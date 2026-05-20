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
	out, err := runHTMLPublish(context.Background(), fake, appsHTMLPublishSpec{AppID: "app_x", Path: site})
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
	out, err := runHTMLPublish(context.Background(), fake, appsHTMLPublishSpec{AppID: "app_x", Path: site})
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
	_, err := runHTMLPublish(context.Background(), fake, appsHTMLPublishSpec{AppID: "app_x", Path: site})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err=%v", err)
	}
}

func TestRunHTMLPublish_PathNotFound(t *testing.T) {
	fake := &fakeAppsHTMLPublishClient{}
	_, err := runHTMLPublish(context.Background(), fake, appsHTMLPublishSpec{AppID: "app_x", Path: "/nonexistent"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if len(fake.calls) != 0 {
		t.Fatalf("client should not be called when path invalid")
	}
}

func TestRunHTMLPublish_RejectsOversizeTarball(t *testing.T) {
	// 临时把上限调到 100 字节验证拦截，恢复原值避免污染其它测试。
	orig := maxHTMLPublishTarballBytes
	maxHTMLPublishTarballBytes = 100
	defer func() { maxHTMLPublishTarballBytes = orig }()

	dir := t.TempDir()
	// 写约 4KB 内容，远超 100 字节上限。
	if err := os.WriteFile(filepath.Join(dir, "big.html"),
		[]byte(strings.Repeat("x", 4096)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// 记录走 reject 前的残留 tarball 数；走完应该不增加。
	tarballPattern := filepath.Join(os.TempDir(), "apps-html-publish-*.tar.gz")
	before, _ := filepath.Glob(tarballPattern)

	fake := &fakeAppsHTMLPublishClient{}
	_, err := runHTMLPublish(context.Background(), fake, appsHTMLPublishSpec{AppID: "app_x", Path: dir})
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

	// 验证 reject 路径仍然清理临时文件
	after, _ := filepath.Glob(tarballPattern)
	if len(after) > len(before) {
		t.Fatalf("oversize reject path leaked tarball: before=%d after=%d", len(before), len(after))
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
	site := writeAppsSampleSite(t)
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsHTMLPublish,
		[]string{"+html-publish", "--app-id", "app_x", "--path", site, "--dry-run", "--as", "user"},
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
