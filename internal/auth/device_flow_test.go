// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/tracking"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

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

// TestRequestDeviceAuthorization_LogsResponse checks that responses with
// unresolvable paths (path="missing") are not logged.
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
	restore := tracking.SetAuthLogHooksForTest(log.New(&buf, "", 0), func() time.Time {
		return time.Date(2026, 4, 2, 3, 4, 5, 0, time.UTC)
	}, func() []string {
		return []string{"lark-cli", "auth", "login", "--device-code", "device-code-secret", "--app-secret=top-secret"}
	})
	t.Cleanup(restore)

	_, err := RequestDeviceAuthorization(httpmock.NewClient(reg), "cli_a", "secret_b", core.BrandFeishu, "", nil)
	if err != nil {
		t.Fatalf("RequestDeviceAuthorization() error: %v", err)
	}

	if got := buf.String(); got != "" {
		t.Fatalf("expected no log for missing path, got %q", got)
	}
}

// TestLogAuthResponse_IgnoresNilHTTPResponse tests that a nil HTTP response produces no log output.
func TestLogAuthResponse_IgnoresNilHTTPResponse(t *testing.T) {
	var buf bytes.Buffer
	restore := tracking.SetAuthLogHooksForTest(log.New(&buf, "", 0), nil, nil)
	t.Cleanup(restore)

	if got := responsePath(nil); got != "missing" {
		t.Fatalf("responsePath(nil) = %q, want %q", got, "missing")
	}
	if got := buf.String(); got != "" {
		t.Fatalf("expected no log output, got %q", got)
	}
}

// TestLogAuthResponse_LogsNilSDKResponseWithZeroStatus verifies that a nil SDK response is logged with status=0.
func TestLogAuthResponse_LogsNilSDKResponseWithZeroStatus(t *testing.T) {
	var buf bytes.Buffer
	restore := tracking.SetAuthLogHooksForTest(log.New(&buf, "", 0), func() time.Time {
		return time.Date(2026, 4, 2, 3, 4, 5, 0, time.UTC)
	}, func() []string {
		return []string{"lark-cli", "auth", "status", "--verify"}
	})
	t.Cleanup(restore)

	logAuthResponse(PathUserInfoV1, 0, "")

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
	restoreLocal := tracking.SetAuthLogHooksForTest(log.New(&buf, "", 0), func() time.Time {
		return time.Date(2026, 4, 2, 3, 4, 5, 0, time.UTC)
	}, func() []string {
		return []string{"lark-cli", "auth", "login", "--device-code", "secret"}
	})
	t.Cleanup(restoreLocal)

	restoreRemote := tracking.SetAuthLogRemoteHooksForTest(nil, "", nil, false)
	t.Cleanup(restoreRemote)

	tracking.LogAuthError(tracking.AuthComponentKeychain, tracking.AuthOpKeychainSet, fmt.Errorf("keychain Set error: %w", http.ErrUseLastResponse))

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

func TestShouldSkipAuthResponseLog(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		statusCode int
		want       bool
	}{
		{
			name:       "oauth token 400 skipped",
			path:       PathOAuthTokenV2,
			statusCode: 400,
			want:       true,
		},
		{
			name:       "oauth token 200 not skipped",
			path:       PathOAuthTokenV2,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "oauth token 401 not skipped",
			path:       PathOAuthTokenV2,
			statusCode: 401,
			want:       false,
		},
		{
			name:       "oauth token 500 not skipped",
			path:       PathOAuthTokenV2,
			statusCode: 500,
			want:       false,
		},
		{
			name:       "other path 400 skipped",
			path:       PathUserInfoV1,
			statusCode: 400,
			want:       true,
		},
		{
			name:       "other path 200 not skipped",
			path:       PathUserInfoV1,
			statusCode: 200,
			want:       false,
		},
		{
			name:       "missing path skipped",
			path:       "missing",
			statusCode: 200,
			want:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSkipAuthResponseLog(tt.path, tt.statusCode); got != tt.want {
				t.Errorf("shouldSkipAuthResponseLog(%q, %d) = %v, want %v", tt.path, tt.statusCode, got, tt.want)
			}
		})
	}
}

func TestPollDeviceToken_DefaultsZeroIntervalToFiveSeconds(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests.Add(1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       http.NoBody,
			}, nil
		}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	t.Cleanup(cancel)

	result := PollDeviceToken(ctx, client, "cli_a", "secret_b", core.BrandFeishu, "device-code", 0, 10, nil)
	if result == nil {
		t.Fatal("PollDeviceToken() returned nil result")
	}
	if result.Message != "Polling was cancelled" {
		t.Fatalf("PollDeviceToken() message = %q, want polling cancellation", result.Message)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("PollDeviceToken() sent %d requests before context cancellation, want 0", got)
	}
}
