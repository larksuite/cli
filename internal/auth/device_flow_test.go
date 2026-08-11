// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/keychain"
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
	if ep.Revoke != "https://accounts.feishu.cn/oauth/v1/revoke" {
		t.Errorf("Revoke = %q", ep.Revoke)
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
	if ep.Revoke != "https://accounts.larksuite.com/oauth/v1/revoke" {
		t.Errorf("Revoke = %q", ep.Revoke)
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

	_, err := RequestDeviceAuthorization(context.Background(), httpmock.NewClient(reg), ClientAuth{AppID: "cli_a", AppSecret: "test-secret"}, core.BrandFeishu, "", nil)
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

// captureRT records the last request + body and returns a canned device-auth response.
func captureDeviceAuthClient(gotReq **http.Request, gotBody *string, respJSON string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		*gotReq = req
		if req.Body != nil {
			b, _ := io.ReadAll(req.Body)
			*gotBody = string(b)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(respJSON)),
		}, nil
	})}
}

const deviceAuthRespJSON = `{"device_code":"dc","user_code":"uc","verification_uri":"https://example/verify","expires_in":300,"interval":5}`

func TestRequestDeviceAuthorization_PrivateKeyJWT_UsesAssertionNotBasic(t *testing.T) {
	var req *http.Request
	var body string
	client := captureDeviceAuthClient(&req, &body, deviceAuthRespJSON)

	ca := ClientAuth{AppID: "cli_a", AuthMethod: core.AuthMethodPrivateKeyJWT, Signer: newFakeAuthSigner(t), KeyLabel: "k"}
	if _, err := RequestDeviceAuthorization(context.Background(), client, ca, core.BrandFeishu, "im:message:send", nil); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Errorf("private_key_jwt must NOT send Basic auth, got %q", req.Header.Get("Authorization"))
	}
	form, _ := url.ParseQuery(body)
	if form.Get("client_assertion") == "" {
		t.Error("missing client_assertion")
	}
	if form.Get("client_assertion_type") != "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" {
		t.Errorf("client_assertion_type = %q", form.Get("client_assertion_type"))
	}
	if form.Has("client_secret") {
		t.Error("client_secret must not be present for private_key_jwt")
	}
}

func TestRequestDeviceAuthorization_ClientSecret_UsesBasic(t *testing.T) {
	var req *http.Request
	var body string
	client := captureDeviceAuthClient(&req, &body, deviceAuthRespJSON)

	ca := ClientAuth{AppID: "cli_a", AppSecret: "test-secret"} // client_secret
	if _, err := RequestDeviceAuthorization(context.Background(), client, ca, core.BrandFeishu, "", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(req.Header.Get("Authorization"), "Basic ") {
		t.Errorf("client_secret should use Basic auth, got %q", req.Header.Get("Authorization"))
	}
	form, _ := url.ParseQuery(body)
	if form.Has("client_assertion") {
		t.Error("client_secret must not send a client_assertion")
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

	result := PollDeviceToken(ctx, client, ClientAuth{AppID: "cli_a", AppSecret: "test-secret"}, core.BrandFeishu, "device-code", 0, 10, nil)
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
