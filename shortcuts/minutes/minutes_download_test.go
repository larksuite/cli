// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

var warmOnce sync.Once

func warmTokenCache(t *testing.T) {
	t.Helper()
	warmOnce.Do(func() {
		f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
		reg.Register(&httpmock.Stub{
			URL:  "/open-apis/test/v1/warm",
			Body: map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
		})
		s := common.Shortcut{
			Service:   "test",
			Command:   "+warm",
			AuthTypes: []string{"bot"},
			Execute: func(_ context.Context, rctx *common.RuntimeContext) error {
				_, err := rctx.CallAPI("GET", "/open-apis/test/v1/warm", nil, nil)
				return err
			},
		}
		parent := &cobra.Command{Use: "test"}
		s.Mount(parent, f)
		parent.SetArgs([]string{"+warm"})
		parent.SilenceErrors = true
		parent.SilenceUsage = true
		parent.Execute()
	})
}

func mountAndRun(t *testing.T, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	warmTokenCache(t)
	parent := &cobra.Command{Use: "minutes"}
	s.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func defaultConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
		UserOpenId: "ou_testuser",
	}
}

func mediaStub(token, downloadURL string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/" + token + "/media",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"download_url": downloadURL},
		},
	}
}

func downloadStub(url string, body []byte, contentType string) *httpmock.Stub {
	return &httpmock.Stub{
		URL:     url,
		RawBody: body,
		Headers: http.Header{"Content-Type": []string{contentType}},
	}
}

// chdir changes the working directory and restores it when the test finishes.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to chdir to %s: %v", dir, err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

// ---------------------------------------------------------------------------
// Unit tests: resolveOutputFromResponse
// ---------------------------------------------------------------------------

func TestResolveFilenameFromResponse_ContentDisposition(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{
			"Content-Disposition": []string{`attachment; filename="meeting_recording.mp4"`},
			"Content-Type":        []string{"video/mp4"},
		},
	}
	got := resolveFilenameFromResponse(resp, "tok001")
	if got != "meeting_recording.mp4" {
		t.Errorf("expected Content-Disposition filename, got %q", got)
	}
}

func TestResolveFilenameFromResponse_ContentType(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{
			"Content-Type": []string{"video/mp4"},
		},
	}
	got := resolveFilenameFromResponse(resp, "tok001")
	if !strings.HasPrefix(got, "tok001") {
		t.Errorf("expected token prefix, got %q", got)
	}
	if ext := got[len("tok001"):]; ext == "" {
		t.Errorf("expected extension after token, got %q", got)
	}
}

func TestResolveFilenameFromResponse_Fallback(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	got := resolveFilenameFromResponse(resp, "tok001")
	if got != "tok001.media" {
		t.Errorf("expected fallback %q, got %q", "tok001.media", got)
	}
}

func TestResolveFilenameFromResponse_InvalidContentDisposition(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{
			"Content-Disposition": []string{"invalid;;;"},
			"Content-Type":        []string{"audio/mpeg"},
		},
	}
	got := resolveFilenameFromResponse(resp, "tok001")
	if !strings.HasPrefix(got, "tok001") {
		t.Errorf("expected token prefix from Content-Type fallback, got %q", got)
	}
}

func TestResolveFilenameFromResponse_RejectsTraversalInDisposition(t *testing.T) {
	tests := []struct {
		disposition string
		wantBase    string
	}{
		{`attachment; filename="../evil.mp4"`, "evil.mp4"},
		{`attachment; filename="../../etc/passwd"`, "passwd"},
		{`attachment; filename="subdir/inner.mp4"`, "inner.mp4"},
		{`attachment; filename=".."`, "tok001.media"},
		{`attachment; filename="."`, "tok001.media"},
	}
	for _, tt := range tests {
		resp := &http.Response{
			Header: http.Header{
				"Content-Disposition": []string{tt.disposition},
			},
		}
		got := resolveFilenameFromResponse(resp, "tok001")
		if got != tt.wantBase {
			t.Errorf("disposition=%q: got %q, want %q", tt.disposition, got, tt.wantBase)
		}
	}
}

func TestDownload_ServerFilenameTraversalStaysInOutputDir(t *testing.T) {
	// Integration: server returns Content-Disposition with "../evil.mp4";
	// file must land inside minutes/{token}/ not the parent directory.
	chdir(t, t.TempDir())

	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(mediaStub("tok001", "https://example.com/presigned/download"))
	reg.Register(&httpmock.Stub{
		URL:     "example.com/presigned/download",
		RawBody: []byte("content"),
		Headers: http.Header{
			"Content-Type":        []string{"video/mp4"},
			"Content-Disposition": []string{`attachment; filename="../evil.mp4"`},
		},
	})

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001", "--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat("minutes/tok001/evil.mp4"); err != nil {
		t.Errorf("expected file inside per-token subdir, got err: %v", err)
	}
	if _, err := os.Stat("minutes/evil.mp4"); err == nil {
		t.Error("file escaped per-token subdir into parent: minutes/evil.mp4 exists")
	}
}

func TestResolveFilenameFromResponse_EmptyDispositionFilename(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{
			"Content-Disposition": []string{"attachment"},
			"Content-Type":        []string{"video/mp4"},
		},
	}
	got := resolveFilenameFromResponse(resp, "tok001")
	if got == "" {
		t.Error("expected non-empty filename")
	}
	if !strings.HasPrefix(got, "tok001") {
		t.Errorf("expected token prefix, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Validation tests
// ---------------------------------------------------------------------------

func TestDownload_Validation_NoFlags(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, MinutesDownload, []string{"+download", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for no flags")
	}
}

func TestDownload_Validation_InvalidToken(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "obcn***invalid", "--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for invalid token")
	}
	if !strings.Contains(err.Error(), "invalid minute token") {
		t.Errorf("expected 'invalid minute token' error, got: %v", err)
	}
}

func TestDownload_Validation_OutputIsFileInBatchMode(t *testing.T) {
	chdir(t, t.TempDir())
	if err := os.WriteFile("already.mp4", []byte("x"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "t1,t2", "--output", "already.mp4", "--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error for --output pointing at an existing file in batch mode")
	}
	if !strings.Contains(err.Error(), "batch mode expects a directory") {
		t.Errorf("error should mention batch-mode directory expectation, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Integration tests: single mode
// ---------------------------------------------------------------------------

func TestDownload_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001", "--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "media") {
		t.Errorf("dry-run should show media API path, got: %s", out)
	}
	if !strings.Contains(out, "tok001") {
		t.Errorf("dry-run should show minute_token, got: %s", out)
	}
}

func TestDownload_UrlOnly(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(mediaStub("tok001", "https://example.com/presigned/download"))

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001", "--url-only", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "https://example.com/presigned/download") {
		t.Errorf("url-only should output download URL, got: %s", stdout.String())
	}
}

func TestDownload_FullDownload(t *testing.T) {
	chdir(t, t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(mediaStub("tok001", "https://example.com/presigned/download"))
	reg.Register(downloadStub("example.com/presigned/download", []byte("fake-video-content"), "video/mp4"))

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001", "--output", "output.mp4", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile("output.mp4")
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if string(data) != "fake-video-content" {
		t.Errorf("file content = %q, want %q", string(data), "fake-video-content")
	}
}

func TestDownload_OverwriteProtection(t *testing.T) {
	chdir(t, t.TempDir())
	if err := os.WriteFile("existing.mp4", []byte("old"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(mediaStub("tok001", "https://example.com/presigned/download"))
	reg.Register(downloadStub("example.com/presigned/download", []byte("new-content"), "video/mp4"))

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001", "--output", "existing.mp4", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error for existing file without --overwrite")
	}
	if !strings.Contains(err.Error(), "exists") {
		t.Errorf("error should mention file exists, got: %v", err)
	}

	data, _ := os.ReadFile("existing.mp4")
	if string(data) != "old" {
		t.Errorf("original file should be preserved, got %q", string(data))
	}
}

func TestDownload_HttpError(t *testing.T) {
	chdir(t, t.TempDir())

	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(mediaStub("tok001", "https://example.com/presigned/download"))
	reg.Register(&httpmock.Stub{
		URL:     "example.com/presigned/download",
		Status:  403,
		RawBody: []byte("Forbidden"),
	})

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001", "--output", "output.mp4", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error for HTTP 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should contain status code, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Integration tests: batch mode
// ---------------------------------------------------------------------------

func TestDownload_Batch_UrlOnly(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(mediaStub("tok001", "https://example.com/download/1"))
	reg.Register(mediaStub("tok002", "https://example.com/download/2"))

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001,tok002", "--url-only", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "download/1") || !strings.Contains(out, "download/2") {
		t.Errorf("batch url-only should show both URLs, got: %s", out)
	}
}

func TestDownload_Batch_Download(t *testing.T) {
	chdir(t, t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(mediaStub("tok001", "https://example.com/download/1"))
	reg.Register(mediaStub("tok002", "https://example.com/download/2"))
	reg.Register(downloadStub("example.com/download/1", []byte("content-1"), "video/mp4"))
	reg.Register(downloadStub("example.com/download/2", []byte("content-2"), "video/mp4"))

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001,tok002", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Data struct {
			Downloads []struct {
				MinuteToken  string `json:"minute_token"`
				ArtifactType string `json:"artifact_type"`
				SavedPath    string `json:"saved_path"`
			} `json:"downloads"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse output: %v\nraw: %s", err, stdout.String())
	}
	if len(result.Data.Downloads) != 2 {
		t.Fatalf("expected 2 downloads, got %d", len(result.Data.Downloads))
	}
	for _, d := range result.Data.Downloads {
		if d.ArtifactType != "recording" {
			t.Errorf("token=%s: artifact_type=%q, want recording", d.MinuteToken, d.ArtifactType)
		}
		wantPrefix := "minutes/" + d.MinuteToken + "/"
		if !strings.Contains(d.SavedPath, wantPrefix) {
			t.Errorf("token=%s: saved_path=%q, want contain %q", d.MinuteToken, d.SavedPath, wantPrefix)
		}
	}
}

func TestDownload_Batch_PartialFailure(t *testing.T) {
	chdir(t, t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(mediaStub("tok001", "https://example.com/download/1"))
	reg.Register(downloadStub("example.com/download/1", []byte("content-1"), "video/mp4"))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/tok002/media",
		Status: 200,
		Body: map[string]interface{}{
			"code": 99999, "msg": "permission denied",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001,tok002", "--as", "bot",
	}, f, stdout)
	// partial failure should not cause an overall error
	if err != nil {
		t.Fatalf("partial failure should not return error, got: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "tok001") || !strings.Contains(out, "tok002") {
		t.Errorf("output should contain both tokens, got: %s", out)
	}
}

func TestDownload_Batch_DuplicateToken(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	// register media stub only once — dedup means only one API call
	reg.Register(mediaStub("tok001", "https://example.com/download/1"))

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001,tok001", "--url-only", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "duplicate") {
		t.Errorf("second token should report duplicate, got: %s", out)
	}
}

// ---------------------------------------------------------------------------
// Integration tests: unified default layout (./minutes/{token}/)
// ---------------------------------------------------------------------------

func TestDownload_DefaultLayout_Single(t *testing.T) {
	chdir(t, t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(mediaStub("tok001", "https://example.com/presigned/download"))
	reg.Register(downloadStub("example.com/presigned/download", []byte("fake-video"), "video/mp4"))

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The stub omits Content-Disposition, so filename resolution falls back
	// to {token}{ext} derived from Content-Type.
	wantPath := "minutes/tok001/tok001.mp4"
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("expected file at %s: %v", wantPath, err)
	}
	if string(data) != "fake-video" {
		t.Errorf("content mismatch: %q", string(data))
	}

	var result struct {
		Data struct {
			MinuteToken  string `json:"minute_token"`
			ArtifactType string `json:"artifact_type"`
			SavedPath    string `json:"saved_path"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse output: %v\nraw: %s", err, stdout.String())
	}
	if result.Data.MinuteToken != "tok001" {
		t.Errorf("minute_token = %q, want tok001", result.Data.MinuteToken)
	}
	if result.Data.ArtifactType != "recording" {
		t.Errorf("artifact_type = %q, want recording", result.Data.ArtifactType)
	}
	if !strings.Contains(result.Data.SavedPath, "minutes/tok001/tok001.mp4") {
		t.Errorf("saved_path = %q, want contain minutes/tok001/tok001.mp4", result.Data.SavedPath)
	}
}

func TestDownload_DefaultLayout_Batch(t *testing.T) {
	chdir(t, t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(mediaStub("tok001", "https://example.com/download/1"))
	reg.Register(mediaStub("tok002", "https://example.com/download/2"))
	reg.Register(downloadStub("example.com/download/1", []byte("content-1"), "video/mp4"))
	reg.Register(downloadStub("example.com/download/2", []byte("content-2"), "video/mp4"))

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001,tok002", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, tok := range []string{"tok001", "tok002"} {
		p := "minutes/" + tok + "/" + tok + ".mp4"
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected file %s: %v", p, err)
		}
	}
}

func TestDownload_OutputDirFlag_SingleToken(t *testing.T) {
	chdir(t, t.TempDir())
	if err := os.MkdirAll("dl", 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(mediaStub("tok001", "https://example.com/download/1"))
	reg.Register(downloadStub("example.com/download/1", []byte("content-1"), "video/mp4"))

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001", "--output-dir", "dl", "--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat("dl/tok001.mp4"); err != nil {
		t.Errorf("expected dl/tok001.mp4, got err: %v", err)
	}
	if _, err := os.Stat("minutes"); err == nil {
		t.Errorf("minutes/ should not be created when --output-dir is explicit")
	}
}

func TestDownload_Batch_OutputNonExistentPath(t *testing.T) {
	// Batch mode with --output pointing at a path that doesn't exist yet:
	// auto-upgrade to --output-dir semantics and create the directory.
	chdir(t, t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(mediaStub("tok001", "https://example.com/download/1"))
	reg.Register(mediaStub("tok002", "https://example.com/download/2"))
	reg.Register(downloadStub("example.com/download/1", []byte("c1"), "video/mp4"))
	reg.Register(downloadStub("example.com/download/2", []byte("c2"), "video/mp4"))

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001,tok002", "--output", "new_dir", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, tok := range []string{"tok001", "tok002"} {
		p := "new_dir/" + tok + ".mp4"
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}
}

func TestDownload_Validation_RejectsTraversalPath(t *testing.T) {
	// --output / --output-dir escaping the working directory must be blocked
	// at Validate time, before any API call or file write.
	chdir(t, t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	for _, flag := range []string{"--output", "--output-dir"} {
		err := mountAndRun(t, MinutesDownload, []string{
			"+download", "--minute-tokens", "tok001", flag, "../escape", "--as", "bot",
		}, f, nil)
		if err == nil {
			t.Errorf("%s ../escape: expected validation error, got nil", flag)
		}
	}
}

func TestDownload_Bug_OutputIsExistingDir(t *testing.T) {
	chdir(t, t.TempDir())
	if err := os.MkdirAll("existing", 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(mediaStub("tok001", "https://example.com/download/1"))
	reg.Register(downloadStub("example.com/download/1", []byte("x"), "video/mp4"))

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001", "--output", "existing", "--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat("existing/tok001.mp4"); err != nil {
		t.Errorf("expected existing/tok001.mp4, got err: %v", err)
	}
}

func TestDownload_Validation_OutputAndOutputDirBothSet(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001", "--output", "a.mp4", "--output-dir", "b", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error when both --output and --output-dir are set")
	}
	if !strings.Contains(err.Error(), "output-dir") {
		t.Errorf("error should mention output-dir, got: %v", err)
	}
}

func TestDownload_ExplicitOutputFile_PreservesPath(t *testing.T) {
	chdir(t, t.TempDir())

	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(mediaStub("tok001", "https://example.com/download/1"))
	reg.Register(downloadStub("example.com/download/1", []byte("x"), "video/mp4"))

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001", "--output", "my.mp4", "--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat("my.mp4"); err != nil {
		t.Errorf("expected my.mp4, got err: %v", err)
	}
	if _, err := os.Stat("minutes"); err == nil {
		t.Errorf("minutes/ should not be created when --output is explicit file path")
	}
}

func TestDownload_Batch_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001,tok002", "--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "tok001") || !strings.Contains(out, "tok002") {
		t.Errorf("dry-run should show tokens, got: %s", out)
	}
}

// ---------------------------------------------------------------------------
// Typed-error lock tests
// ---------------------------------------------------------------------------

// TestDownload_TypedErr_ValidationInvalidArgument verifies that an invalid
// minute token format (passing cobra's required check but failing our regex)
// returns a *errs.ValidationError with SubtypeInvalidArgument and the expected
// Param.  This locks site :64 (invalid minute token %q).
func TestDownload_TypedErr_ValidationInvalidArgument(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "INVALID***TOKEN", "--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != "--minute-tokens" {
		t.Errorf("Param = %q, want %q", ve.Param, "--minute-tokens")
	}
}

// TestDownload_TypedErr_NetworkTransport_HttpError verifies that a non-2xx
// download response from downloadMediaFile returns a *errs.NetworkError with
// SubtypeNetworkTransport.
//
// In the end-to-end single-token Execute path the typed error is now passed
// through directly via r.err (single-mode passthrough).  We call downloadMediaFile
// directly via a probe shortcut to assert the typed shape at the source.
func TestDownload_TypedErr_NetworkTransport_HttpError(t *testing.T) {
	chdir(t, t.TempDir())

	var capturedErr error
	probe := common.Shortcut{
		Service:   "minutes",
		Command:   "+probe-dl",
		AuthTypes: []string{"bot"},
		Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
			client, err := rctx.Factory.HttpClient()
			if err != nil {
				return err
			}
			_, capturedErr = downloadMediaFile(ctx, client,
				"https://example.com/presigned/download", "tok001",
				downloadOpts{fio: rctx.FileIO(), outputPath: "out.mp4"})
			return nil
		},
	}

	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		URL:     "example.com/presigned/download",
		Status:  503,
		RawBody: []byte("Service Unavailable"),
	})

	if err := mountAndRun(t, probe, []string{"+probe-dl", "--as", "bot"}, f, nil); err != nil {
		t.Fatalf("probe shortcut should not error: %v", err)
	}
	if capturedErr == nil {
		t.Fatal("expected downloadMediaFile to return an error for HTTP 503, got nil")
	}
	var ne *errs.NetworkError
	if !errors.As(capturedErr, &ne) {
		t.Fatalf("expected *errs.NetworkError, got %T: %v", capturedErr, capturedErr)
	}
	if ne.Subtype != errs.SubtypeNetworkTransport {
		t.Errorf("Subtype = %q, want %q", ne.Subtype, errs.SubtypeNetworkTransport)
	}
	if !strings.Contains(ne.Error(), "503") {
		t.Errorf("error message should contain status code 503, got: %v", ne)
	}
}

// TestDownload_TypedErr_InternalInvalidResponse verifies that fetchDownloadURL
// returns *errs.InternalError with SubtypeInvalidResponse when the API
// response contains an empty download_url field.
//
// In the end-to-end single-token Execute path the typed error is now passed
// through directly via r.err (single-mode passthrough).  The typed assertion
// is also made at the fetchDownloadURL call site directly via a probe shortcut.
func TestDownload_TypedErr_InternalInvalidResponse(t *testing.T) {
	var capturedErr error
	probe := common.Shortcut{
		Service:   "minutes",
		Command:   "+probe-download-url",
		AuthTypes: []string{"bot"},
		Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
			_, capturedErr = fetchDownloadURL(ctx, rctx, "tok001")
			// Always return nil so mountAndRun doesn't swallow the error type.
			return nil
		},
	}

	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/tok001/media",
		Status: 200,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"download_url": ""},
		},
	})

	if err := mountAndRun(t, probe, []string{"+probe-download-url", "--as", "bot"}, f, nil); err != nil {
		t.Fatalf("probe shortcut should not error: %v", err)
	}
	if capturedErr == nil {
		t.Fatal("expected fetchDownloadURL to return an error for empty download_url, got nil")
	}
	var ie *errs.InternalError
	if !errors.As(capturedErr, &ie) {
		t.Fatalf("expected *errs.InternalError, got %T: %v", capturedErr, capturedErr)
	}
	if ie.Subtype != errs.SubtypeInvalidResponse {
		t.Errorf("Subtype = %q, want %q", ie.Subtype, errs.SubtypeInvalidResponse)
	}
	if !strings.Contains(ie.Error(), "download_url") {
		t.Errorf("error message should mention download_url, got: %v", ie)
	}
}

// TestDownload_TypedErr_OverwriteProtection verifies that the overwrite guard
// in downloadMediaFile returns *errs.ValidationError with SubtypeFailedPrecondition.
//
// In the end-to-end single-token Execute path this typed error is now passed
// through directly via r.err (single-mode passthrough), so the typed shape is
// also asserted end-to-end via the probe shortcut.
func TestDownload_TypedErr_OverwriteProtection(t *testing.T) {
	chdir(t, t.TempDir())
	if err := os.WriteFile("existing.mp4", []byte("old"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	var capturedErr error
	probe := common.Shortcut{
		Service:   "minutes",
		Command:   "+probe-overwrite",
		AuthTypes: []string{"bot"},
		Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
			client, err := rctx.Factory.HttpClient()
			if err != nil {
				return err
			}
			_, capturedErr = downloadMediaFile(ctx, client,
				"https://example.com/presigned/download", "tok001",
				downloadOpts{fio: rctx.FileIO(), outputPath: "existing.mp4", overwrite: false})
			return nil
		},
	}

	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(downloadStub("example.com/presigned/download", []byte("new-content"), "video/mp4"))

	if err := mountAndRun(t, probe, []string{"+probe-overwrite", "--as", "bot"}, f, nil); err != nil {
		t.Fatalf("probe shortcut should not error: %v", err)
	}
	if capturedErr == nil {
		t.Fatal("expected downloadMediaFile to return an error for existing file without overwrite, got nil")
	}
	var ve *errs.ValidationError
	if !errors.As(capturedErr, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", capturedErr, capturedErr)
	}
	if ve.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeFailedPrecondition)
	}
	if !strings.Contains(ve.Error(), "exists") {
		t.Errorf("error message should mention exists, got: %v", ve)
	}
}

// TestDownload_TypedErr_SingleMode_PassthroughTyped verifies that in single-token
// mode a typed error from fetchDownloadURL or downloadMediaFile is returned
// directly to the caller with its Problem shape intact (exit code preserved).
func TestDownload_TypedErr_SingleMode_PassthroughTyped(t *testing.T) {
	chdir(t, t.TempDir())

	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	// API returns non-zero code → CallAPITyped yields a typed APIError.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/tok001/media",
		Status: 200,
		Body: map[string]interface{}{
			"code": 99991, "msg": "permission denied",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001", "--output", "out.mp4", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}

	// The error must carry a Problem (typed envelope).
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error (ProblemOf ok), got %T: %v", err, err)
	}
	if p == nil {
		t.Fatal("ProblemOf returned nil Problem")
	}

	// Exit code must be non-zero and come from the typed error, not a generic 1.
	code := output.ExitCodeOf(err)
	if code == 0 {
		t.Errorf("ExitCodeOf typed error = 0, want non-zero")
	}
}

// TestDownload_TypedErr_Batch_AllFail_OutPartialFailure verifies that when every
// token in a batch fails, Execute emits an ok:false stdout envelope (carrying the
// full downloads array) and returns *output.PartialFailureError with Code==ExitAPI.
// This locks the double-emit fix: the old code called OutFormat then returned ErrAPI;
// the new code calls OutPartialFailure once.
func TestDownload_TypedErr_Batch_AllFail_OutPartialFailure(t *testing.T) {
	chdir(t, t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	// Both tokens fail at the API level.
	for _, tok := range []string{"tok001", "tok002"} {
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/minutes/v1/minutes/" + tok + "/media",
			Status: 200,
			Body: map[string]interface{}{
				"code": 99991, "msg": "permission denied",
				"data": map[string]interface{}{},
			},
		})
	}

	err := mountAndRun(t, MinutesDownload, []string{
		"+download", "--minute-tokens", "tok001,tok002", "--as", "bot",
	}, f, stdout)

	// Must return *output.PartialFailureError with ExitAPI.
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected *output.PartialFailureError, got %T: %v", err, err)
	}
	if pfErr.Code != output.ExitAPI {
		t.Errorf("PartialFailureError.Code = %d, want %d (ExitAPI)", pfErr.Code, output.ExitAPI)
	}

	// stdout must carry ok:false with the downloads array (both failed entries).
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Downloads []struct {
				MinuteToken string `json:"minute_token"`
				Error       string `json:"error"`
			} `json:"downloads"`
		} `json:"data"`
	}
	if jsonErr := json.Unmarshal(stdout.Bytes(), &env); jsonErr != nil {
		t.Fatalf("failed to parse stdout: %v\nraw: %s", jsonErr, stdout.String())
	}
	if env.OK {
		t.Errorf("ok must be false on all-fail batch, got ok:true\nstdout: %s", stdout.String())
	}
	if len(env.Data.Downloads) != 2 {
		t.Fatalf("expected 2 download entries, got %d\nstdout: %s", len(env.Data.Downloads), stdout.String())
	}
	for _, d := range env.Data.Downloads {
		if d.Error == "" {
			t.Errorf("token %s: expected non-empty error field in all-fail batch", d.MinuteToken)
		}
	}
}
