// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/keychain"
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
	restore := keychain.SetAuthLogHooksForTest(log.New(&buf, "", 0), func() time.Time {
		return time.Date(2026, 4, 2, 3, 4, 5, 0, time.UTC)
	}, func() []string {
		return []string{"lark-cli", "auth", "login", "--device-code", "device-code-secret", "--app-secret=top-secret"}
	})
	t.Cleanup(restore)

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
	got := keychain.FormatAuthCmdline([]string{
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
	restore := keychain.SetAuthLogHooksForTest(log.New(&buf, "", 0), nil, nil)
	t.Cleanup(restore)

	var resp *http.Response
	logHTTPResponse(resp)

	if got := buf.String(); got != "" {
		t.Fatalf("expected no log output, got %q", got)
	}
}

// TestLogAuthResponse_HandlesNilSDKResponse verifies that a nil SDK response is handled without panicking.
func TestLogAuthResponse_HandlesNilSDKResponse(t *testing.T) {
	var buf bytes.Buffer
	restore := keychain.SetAuthLogHooksForTest(log.New(&buf, "", 0), func() time.Time {
		return time.Date(2026, 4, 2, 3, 4, 5, 0, time.UTC)
	}, func() []string {
		return []string{"lark-cli", "auth", "status", "--verify"}
	})
	t.Cleanup(restore)

	logSDKResponse(PathUserInfoV1, nil)

	got := buf.String()
	if !strings.Contains(got, "path="+PathUserInfoV1) {
		t.Fatalf("expected sdk path in log, got %q", got)
	}
	if !strings.Contains(got, "status=0") {
		t.Fatalf("expected zero status in log, got %q", got)
	}
}

func TestLogAuthError_RecordsStructuredEntry(t *testing.T) {
	var buf bytes.Buffer
	restore := keychain.SetAuthLogHooksForTest(log.New(&buf, "", 0), func() time.Time {
		return time.Date(2026, 4, 2, 3, 4, 5, 0, time.UTC)
	}, func() []string {
		return []string{"lark-cli", "auth", "login", "--device-code", "secret"}
	})
	t.Cleanup(restore)

	keychain.LogAuthError("keychain", "Set", fmt.Errorf("keychain Set error: %w", http.ErrUseLastResponse))

	got := buf.String()
	if !strings.Contains(got, "auth-error") {
		t.Fatalf("expected auth-error log entry, got %q", got)
	}
	if !strings.Contains(got, "component=keychain") {
		t.Fatalf("expected component in log, got %q", got)
	}
	if !strings.Contains(got, "op=Set") {
		t.Fatalf("expected op in log, got %q", got)
	}
	if !strings.Contains(got, "error=\"keychain Set error: net/http: use last response\"") {
		t.Fatalf("expected quoted error in log, got %q", got)
	}
	if !strings.Contains(got, "cmdline=lark-cli auth login ...") {
		t.Fatalf("expected truncated cmdline in log, got %q", got)
	}
}
