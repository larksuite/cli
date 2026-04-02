// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

// TestResolveOAuthEndpoints_Feishu validates endpoints for the Feishu brand.
func TestResolveOAuthEndpoints_Feishu(t *testing.T) {
	ep := ResolveOAuthEndpoints(core.BrandFeishu)
	if ep.DeviceAuthorization != "https://accounts.feishu.cn/oauth/v1/device_authorization" {
		t.Errorf("DeviceAuthorization = %q", ep.DeviceAuthorization)
	}
	if ep.Token != "https://open.feishu.cn/open-apis/authen/v2/oauth/token" {
		t.Errorf("Token = %q", ep.Token)
	}
}

// TestResolveOAuthEndpoints_Lark validates endpoints for the Lark brand.
func TestResolveOAuthEndpoints_Lark(t *testing.T) {
	ep := ResolveOAuthEndpoints(core.BrandLark)
	if ep.DeviceAuthorization != "https://accounts.larksuite.com/oauth/v1/device_authorization" {
		t.Errorf("DeviceAuthorization = %q", ep.DeviceAuthorization)
	}
	if ep.Token != "https://open.larksuite.com/open-apis/authen/v2/oauth/token" {
		t.Errorf("Token = %q", ep.Token)
	}
}

// TestRequestDeviceAuthorization_LogsResponse checks if API responses are logged correctly.
func TestRequestDeviceAuthorization_LogsResponse(t *testing.T) {
	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    PathDeviceAuthorization,
		Body: map[string]interface{}{
			"device_code":               "device-code",
			"user_code":                 "user-code",
			"verification_uri":          "https://example.com/verify",
			"verification_uri_complete": "https://example.com/verify?code=123",
			"expires_in":                240,
			"interval":                  5,
		},
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Tt-Logid":   []string{"device-log-id"},
		},
	})

	var buf bytes.Buffer
	prevWriter := authResponseLogWriter
	prevNow := authResponseLogNow
	prevArgs := authResponseLogArgs
	authResponseLogWriter = &buf
	authResponseLogNow = func() time.Time {
		return time.Date(2026, 4, 2, 3, 4, 5, 0, time.UTC)
	}
	authResponseLogArgs = func() []string {
		return []string{"lark-cli", "auth", "login", "--device-code", "device-code-secret", "--app-secret=top-secret"}
	}
	t.Cleanup(func() {
		authResponseLogWriter = prevWriter
		authResponseLogNow = prevNow
		authResponseLogArgs = prevArgs
	})

	_, err := RequestDeviceAuthorization(httpmock.NewClient(reg), "cli_a", "secret_b", core.BrandFeishu, "", nil)
	if err != nil {
		t.Fatalf("RequestDeviceAuthorization() error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "time=2026-04-02T03:04:05Z") {
		t.Fatalf("expected time in log, got %q", got)
	}
	if !strings.Contains(got, "path=missing") {
		t.Fatalf("expected path in log, got %q", got)
	}
	if !strings.Contains(got, "status=200") {
		t.Fatalf("expected status=200 in log, got %q", got)
	}
	if !strings.Contains(got, "x-tt-logid=device-log-id") {
		t.Fatalf("expected x-tt-logid in log, got %q", got)
	}
	if !strings.Contains(got, "cmdline=lark-cli auth login ...") {
		t.Fatalf("expected cmdline in log, got %q", got)
	}
}

// TestFormatAuthCmdline_TruncatesExtraArgs verifies that long command lines are truncated.
func TestFormatAuthCmdline_TruncatesExtraArgs(t *testing.T) {
	got := formatAuthCmdline([]string{
		"lark-cli",
		"auth",
		"login",
		"--device-code", "device-code-secret",
		"--app-secret=top-secret",
		"--scope", "contact:read",
	})

	want := "lark-cli auth login ..."
	if got != want {
		t.Fatalf("formatAuthCmdline() = %q, want %q", got, want)
	}
}

// TestLogAuthResponse_IgnoresTypedNilHTTPResponse tests that a typed nil HTTP response is ignored gracefully.
func TestLogAuthResponse_IgnoresTypedNilHTTPResponse(t *testing.T) {
	var buf bytes.Buffer
	prevWriter := authResponseLogWriter
	authResponseLogWriter = &buf
	t.Cleanup(func() {
		authResponseLogWriter = prevWriter
	})

	var resp *http.Response
	logHTTPResponse(resp)

	if got := buf.String(); got != "" {
		t.Fatalf("expected no log output, got %q", got)
	}
}

// TestLogAuthResponse_HandlesNilSDKResponse verifies that a nil SDK response is handled without panicking.
func TestLogAuthResponse_HandlesNilSDKResponse(t *testing.T) {
	var buf bytes.Buffer
	prevWriter := authResponseLogWriter
	prevNow := authResponseLogNow
	prevArgs := authResponseLogArgs
	authResponseLogWriter = &buf
	authResponseLogNow = func() time.Time {
		return time.Date(2026, 4, 2, 3, 4, 5, 0, time.UTC)
	}
	authResponseLogArgs = func() []string {
		return []string{"lark-cli", "auth", "status", "--verify"}
	}
	t.Cleanup(func() {
		authResponseLogWriter = prevWriter
		authResponseLogNow = prevNow
		authResponseLogArgs = prevArgs
	})

	logSDKResponse(PathUserInfoV1, nil)

	got := buf.String()
	if !strings.Contains(got, "path="+PathUserInfoV1) {
		t.Fatalf("expected sdk path in log, got %q", got)
	}
	if !strings.Contains(got, "status=0") {
		t.Fatalf("expected zero status in log, got %q", got)
	}
}

// TestDefaultLogWriter_PanicsBeforeLock ensures log code behaves correctly when panic occurs before the lock is acquired.
func TestDefaultLogWriter_PanicsBeforeLock(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	prevNow := authResponseLogNow
	prevCleanup := authResponseLogCleanup
	authResponseLogCleanup = func(_ string, _ time.Time) {}
	t.Cleanup(func() {
		authResponseLogNow = prevNow
		authResponseLogCleanup = prevCleanup
	})

	writer := defaultLogWriter{}
	authResponseLogNow = func() time.Time {
		panic("boom")
	}

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic from authResponseLogNow")
			}
		}()
		_, _ = writer.Write([]byte("first\n"))
	}()

	authResponseLogNow = func() time.Time {
		return time.Date(2026, 4, 2, 3, 4, 5, 0, time.UTC)
	}
	if _, err := writer.Write([]byte("second\n")); err != nil {
		t.Fatalf("second Write() error: %v", err)
	}
}
