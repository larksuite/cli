// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/dpop"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
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
	if ep.Token != "https://accounts.feishu.cn/oauth/v3/token" {
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
	if ep.Token != "https://accounts.larksuite.com/oauth/v3/token" {
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

	_, err := RequestDeviceAuthorization(context.Background(), httpmock.NewClient(reg), "cli_a", "secret_b", core.BrandFeishu, "", nil)
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

func TestPollDeviceToken_PreservesStatusMessage(t *testing.T) {
	t.Parallel()

	const statusMessage = "[不可申请，勿重试] 企业管理员禁止申请的权限：mail:user_mailbox.message:send\n" +
		"[待审核，通过后用户需重新授权] 以下权限正在等待管理员审核：offline_access"
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{
					"code": 0,
					"access_token": "access-token",
					"expires_in": 7200,
					"scope": "approval:task:read",
					"status_message": "[不可申请，勿重试] 企业管理员禁止申请的权限：mail:user_mailbox.message:send\n[待审核，通过后用户需重新授权] 以下权限正在等待管理员审核：offline_access"
				}`)),
			}, nil
		}),
	}

	result := PollDeviceToken(context.Background(), client, "cli_a", "secret_b", core.BrandFeishu, "device-code", 1, 3, nil)
	if result == nil || !result.OK || result.Token == nil {
		t.Fatalf("PollDeviceToken() = %#v, want successful token result", result)
	}
	if result.Token.StatusMessage != statusMessage {
		t.Fatalf("StatusMessage = %q, want %q", result.Token.StatusMessage, statusMessage)
	}
}

func TestPollDeviceToken_MissingStatusMessageIsEmpty(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{
					"code": 0,
					"access_token": "access-token",
					"expires_in": 7200,
					"scope": "approval:task:read"
				}`)),
			}, nil
		}),
	}

	result := PollDeviceToken(context.Background(), client, "cli_a", "secret_b", core.BrandFeishu, "device-code", 1, 3, nil)
	if result == nil || !result.OK || result.Token == nil {
		t.Fatalf("PollDeviceToken() = %#v, want successful token result", result)
	}
	if result.Token.StatusMessage != "" {
		t.Fatalf("StatusMessage = %q, want empty string", result.Token.StatusMessage)
	}
}

func TestPollDeviceTokenPolicyWithoutSigner(t *testing.T) {
	for _, mode := range []core.DPoPMode{"", core.DPoPModeDisabled, core.DPoPModePreferred, core.DPoPModeRequired} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			requests := 0
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				if req.URL.Path != core.OAuthTokenV3Path || req.Header.Get(dpop.ProofHeader) != "" {
					t.Fatal("expected a Bearer token request without a proof")
				}
				return refreshHTTPResponse(req, `{"access_token":"synthetic-token","token_type":"Bearer"}`), nil
			})}
			result := pollDeviceTokenWithKeyStore(context.Background(), client, "cli_test", "synthetic-secret",
				core.BrandFeishu, "device-code", 1, 5, nil, mode, nil)
			if mode == core.DPoPModeRequired {
				problem, ok := errs.ProblemOf(result.Err)
				if result.OK || !ok || problem.Subtype != errs.SubtypeDPoPKeyMissing ||
					!errors.Is(result.Err, keysigner.ErrUnavailable) || requests != 0 {
					t.Fatalf("required = %#v, requests = %d; want key-unavailable failure before HTTP", result, requests)
				}
			} else if !result.OK || result.Token == nil || result.Token.DPoP != nil || requests != 1 {
				t.Fatalf("result = %#v, requests = %d; want one successful Bearer exchange", result, requests)
			}
		})
	}
}

type deviceFlowMetadata map[string]string

func (s deviceFlowMetadata) Get(_, account string) (string, error) { return s[account], nil }
func (s deviceFlowMetadata) Set(_, account, value string) error {
	s[account] = value
	return nil
}
func (s deviceFlowMetadata) Remove(_, account string) error { delete(s, account); return nil }

func TestPollDeviceTokenPolicyAndKeyLifetime(t *testing.T) {
	heartbeat := fmt.Sprintf(`{"data":{"now":"%d"}}`, time.Now().Unix())
	const bearer = `{"access_token":"synthetic-token","token_type":"Bearer"}`
	const bound = `{"access_token":"synthetic-token","token_type":"DPoP"}`
	type step struct {
		path, body string
		proof      bool
		cancel     bool
	}
	for _, tc := range []struct {
		name    string
		mode    core.DPoPMode
		steps   []step
		subtype errs.Subtype
	}{
		{"preferred_clock_failure_before_request", core.DPoPModePreferred, []step{
			{path: dpop.HeartbeatPath, body: `{}`},
			{path: core.OAuthTokenV3Path, body: bearer},
		}, ""},
		{"required_clock_failure", core.DPoPModeRequired, []step{
			{path: dpop.HeartbeatPath, body: `{}`},
		}, errs.SubtypeDPoPClockSyncFailed},
		{"preferred_clock_failure_after_request", core.DPoPModePreferred, []step{
			{path: dpop.HeartbeatPath, body: heartbeat},
			{path: core.OAuthTokenV3Path, body: `{"error":"authorization_pending"}`, proof: true},
			{path: dpop.HeartbeatPath, body: `{}`},
		}, errs.SubtypeDPoPClockSyncFailed},
		{"preferred_rejects_bearer_response", core.DPoPModePreferred, []step{
			{path: dpop.HeartbeatPath, body: heartbeat},
			{path: core.OAuthTokenV3Path, body: bearer, proof: true},
		}, errs.SubtypeDPoPRequired},
		{"preferred_cancellation", core.DPoPModePreferred, []step{
			{path: dpop.HeartbeatPath, cancel: true},
		}, errs.SubtypeDPoPClockSyncFailed},
		{"required_pending_then_success", core.DPoPModeRequired, []step{
			{path: dpop.HeartbeatPath, body: heartbeat},
			{path: core.OAuthTokenV3Path, body: `{"error":"authorization_pending"}`, proof: true},
			{path: dpop.HeartbeatPath, body: heartbeat},
			{path: core.OAuthTokenV3Path, body: bound, proof: true},
		}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			directory := t.TempDir()
			signer, err := keysigner.NewSoftwareSigner(directory, func(context.Context) ([]byte, error) {
				return []byte("synthetic-unlock-secret"), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			store := dpop.NewKeyStoreWithSigner(deviceFlowMetadata{}, signer)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			requests := 0
			var proofs []string
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if requests >= len(tc.steps) {
					t.Fatalf("unexpected request to %s", req.URL.Path)
				}
				step := tc.steps[requests]
				requests++
				proof := req.Header.Get(dpop.ProofHeader)
				if req.URL.Path != step.path || (proof != "") != step.proof {
					t.Fatalf("request %d: path = %s, proof present = %v", requests, req.URL.Path, proof != "")
				}
				if proof != "" {
					proofs = append(proofs, proof)
				}
				if step.cancel {
					cancel()
					return nil, ctx.Err()
				}
				return refreshHTTPResponse(req, step.body), nil
			})}
			result := pollDeviceTokenWithKeyStore(ctx, client, "cli_test", "synthetic-secret",
				core.BrandFeishu, "device-code", 1, 10, nil, tc.mode, store)
			if requests != len(tc.steps) {
				t.Fatalf("requests = %d, want %d", requests, len(tc.steps))
			}
			if tc.subtype != "" {
				problem, ok := errs.ProblemOf(result.Err)
				if result.OK || !ok || problem.Category != errs.CategoryAuthentication || problem.Subtype != tc.subtype {
					t.Fatalf("result = %#v, want authentication/%s", result, tc.subtype)
				}
				if ctx.Err() != nil && !errors.Is(result.Err, ctx.Err()) {
					t.Fatalf("cancellation cause lost: %v", result.Err)
				}
			} else if !result.OK || result.Token == nil {
				t.Fatalf("expected successful exchange: %#v", result)
			}
			wantKeys := 0
			if result.Token != nil && result.Token.DPoP != nil {
				wantKeys = 1
				defer func() {
					if err := store.DeleteKeyContext(context.Background(), result.Token.DPoP.Key()); err != nil {
						t.Error(err)
					}
				}()
				if len(proofs) != 2 || proofs[0] == proofs[1] || strings.Split(proofs[0], ".")[0] != strings.Split(proofs[1], ".")[0] {
					t.Fatal("polls must use fresh proofs with the same public key")
				}
			} else if tc.subtype == "" && len(proofs) != 0 {
				t.Fatal("DPoP exchange succeeded without a key binding")
			}
			files, err := filepath.Glob(filepath.Join(directory, "*.json"))
			if err != nil || len(files) != wantKeys {
				t.Fatalf("key files = %v, err = %v; want %d retained keys", files, err, wantKeys)
			}
		})
	}
}
