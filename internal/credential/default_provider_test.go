// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

func TestDefaultTokenProvider_Dispatches(t *testing.T) {
	// Just verify the type implements DefaultTokenResolver
	var _ DefaultTokenResolver = &DefaultTokenProvider{}
}

func TestDefaultAccountProvider_Implements(t *testing.T) {
	var _ DefaultAccountResolver = &DefaultAccountProvider{}
}

// TestClassifyTATResponseCode_InvalidClient_MapsToInvalidClient pins that the
// unified Token Endpoint's OAuth2 invalid_client error surfaces as
// CategoryConfig/InvalidClient — the configured app_id/app_secret cannot mint a
// tenant access token, the same actionable failure the legacy 10003/10014 codes
// produced. The numeric code is intentionally not asserted: the v3 endpoint may
// return invalid_client with no Lark code (code defaults to 0).
func TestClassifyTATResponseCode_InvalidClient_MapsToInvalidClient(t *testing.T) {
	err := classifyTATResponseCode(0, "invalid_client", "client authentication failed", "feishu", "cli_app_x")
	if err == nil {
		t.Fatal("expected non-nil error for invalid_client")
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *errs.ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Category != errs.CategoryConfig {
		t.Errorf("Category = %q, want %q", cfgErr.Category, errs.CategoryConfig)
	}
	if cfgErr.Subtype != errs.SubtypeInvalidClient {
		t.Errorf("Subtype = %q, want %q", cfgErr.Subtype, errs.SubtypeInvalidClient)
	}
	if cfgErr.Hint == "" {
		t.Error("Hint must be non-empty so the user gets a recovery action")
	}
}

// TestClassifyTATResponseCode_UnauthorizedClient_MapsToInvalidClient pins that
// unauthorized_client is treated as the same credential failure as
// invalid_client.
func TestClassifyTATResponseCode_UnauthorizedClient_MapsToInvalidClient(t *testing.T) {
	err := classifyTATResponseCode(0, "unauthorized_client", "client not authorized", "feishu", "cli_app_x")
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *errs.ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Subtype != errs.SubtypeInvalidClient {
		t.Errorf("Subtype = %q, want %q", cfgErr.Subtype, errs.SubtypeInvalidClient)
	}
}

// TestClassifyTATResponseCode_OtherErrorFallsThrough pins that OAuth errors
// outside the credential set fall through to the generic BuildAPIError fallback
// — still typed, but not a ConfigError. The mapping is narrow and intentional.
func TestClassifyTATResponseCode_OtherErrorFallsThrough(t *testing.T) {
	err := classifyTATResponseCode(20068, "invalid_scope", "unauthorized scope", "feishu", "cli_app_x")
	if err == nil {
		t.Fatal("expected non-nil error for invalid_scope")
	}
	var cfgErr *errs.ConfigError
	if errors.As(err, &cfgErr) {
		t.Fatalf("invalid_scope must not be classified as ConfigError, got %T", err)
	}
}

// TestClassifyTATResponseCode_CodeZeroOtherError_StillTyped pins the code-0
// backstop: a non-credential OAuth error (e.g. invalid_scope) that arrives with no
// numeric code (code 0) must still produce a non-nil typed error. BuildAPIError
// returns nil for code 0 (Feishu's success convention); without the backstop,
// FetchTAT would surface this deterministic rejection as ("", nil) — an empty token
// with no error.
func TestClassifyTATResponseCode_CodeZeroOtherError_StillTyped(t *testing.T) {
	err := classifyTATResponseCode(0, "invalid_scope", "the requested scope is not granted", "feishu", "cli_app_x")
	if err == nil {
		t.Fatal("expected non-nil error for code-0 invalid_scope (must not be swallowed as success)")
	}
	if !errs.IsTyped(err) {
		t.Fatalf("expected a typed errs.* error, got %T %v", err, err)
	}
	var cfgErr *errs.ConfigError
	if errors.As(err, &cfgErr) {
		t.Fatalf("code-0 invalid_scope must not be a ConfigError, got %T", err)
	}
}

func TestCheckTokenAppID(t *testing.T) {
	if err := checkTokenAppID(TokenSpec{Type: TokenTypeUAT}, "cli_a"); err == nil {
		t.Fatal("empty requested app must be rejected: it would silently disable the guarantee")
	}
	if err := checkTokenAppID(TokenSpec{AppID: "cli_a"}, "cli_a"); err != nil {
		t.Fatalf("matching app must pass: %v", err)
	}
	err := checkTokenAppID(TokenSpec{AppID: "cli_a"}, "cli_b")
	if err == nil {
		t.Fatal("mismatched app must be refused")
	}
	var ie *errs.InternalError
	if !errors.As(err, &ie) {
		t.Fatalf("error type = %T, want *errs.InternalError", err)
	}
}

// REAL-path regression for review F2: the token provider re-reads the config,
// so a profile edit between account resolution and token resolution must not
// hand a token minted for the new app to a caller that resolved the old one.
// Uses the real DefaultAccountProvider + DefaultTokenProvider; the HTTP stub
// makes the network step unreachable, so reaching it proves the app check ran
// and passed first.
func TestDefaultTokenProvider_RefusesTokenAfterConfigSwap(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	writeCfg := func(appID string) {
		t.Helper()
		multi := &core.MultiAppConfig{CurrentApp: "tenant_a", Apps: []core.AppConfig{{
			Name: "tenant_a", AppId: appID, AppSecret: core.PlainSecret("your-secret"), Brand: core.BrandFeishu,
		}}}
		if err := core.SaveMultiAppConfig(multi); err != nil {
			t.Fatalf("SaveMultiAppConfig: %v", err)
		}
	}
	writeCfg("cli_a")

	httpSentinel := errors.New("http client sentinel: unreachable in test")
	tp := NewDefaultTokenProvider(
		NewDefaultAccountProvider(nil, "tenant_a"),
		func() (*http.Client, error) { return nil, httpSentinel },
		nil,
	)

	// Matching app: the consistency check passes and resolution proceeds to
	// the (stubbed) HTTP step.
	_, err := tp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: "cli_a"})
	if !errors.Is(err, httpSentinel) {
		t.Fatalf("err = %v, want the HTTP sentinel (check must pass for a matching app)", err)
	}

	// The profile now resolves to a different app: the token request that was
	// arbitrated for cli_a must be refused before any token work happens.
	writeCfg("cli_b")
	_, err = tp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: "cli_a"})
	if err == nil || !strings.Contains(err.Error(), "config changed during resolution") {
		t.Fatalf("err = %v, want config-changed refusal", err)
	}
}

// F1 regression: a TAT request for a mismatched app must be refused BEFORE
// any token work starts — no HTTP client construction, no mint, no cache —
// otherwise the CLI mints (and caches) a token for the wrong app and only
// then refuses to return it, leaving auth audit/quota side effects behind.
func TestDefaultTokenProvider_TATChecksAppBeforeAnyTokenWork(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	multi := &core.MultiAppConfig{CurrentApp: "tenant_a", Apps: []core.AppConfig{{
		Name: "tenant_a", AppId: "cli_b", AppSecret: core.PlainSecret("your-secret"), Brand: core.BrandFeishu,
	}}}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	httpCalled := false
	tp := NewDefaultTokenProvider(
		NewDefaultAccountProvider(nil, "tenant_a"),
		func() (*http.Client, error) { httpCalled = true; return nil, errors.New("http sentinel") },
		nil,
	)

	// The profile resolves to cli_b, but the caller arbitrated cli_a.
	_, err := tp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeTAT, AppID: "cli_a"})
	if err == nil || !strings.Contains(err.Error(), "config changed during resolution") {
		t.Fatalf("err = %v, want config-changed refusal", err)
	}
	if httpCalled {
		t.Fatal("token work started for a mismatched app: the check must run before any HTTP client is built")
	}
}

// countingTATTripper serves a canned successful TAT response and counts calls.
type countingTATTripper struct{ calls int }

func (c *countingTATTripper) RoundTrip(*http.Request) (*http.Response, error) {
	c.calls++
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"code":0,"access_token":"your-access-token"}`)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

// TAT happy path: the first request mints the token over HTTP, the second is
// served from the sync.Once cache without another HTTP call.
func TestDefaultTokenProvider_TATSuccessAndCacheHit(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	multi := &core.MultiAppConfig{CurrentApp: "tenant_a", Apps: []core.AppConfig{{
		Name: "tenant_a", AppId: "cli_a", AppSecret: core.PlainSecret("your-secret"), Brand: core.BrandFeishu,
	}}}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	tripper := &countingTATTripper{}
	tp := NewDefaultTokenProvider(
		NewDefaultAccountProvider(nil, "tenant_a"),
		func() (*http.Client, error) { return &http.Client{Transport: tripper}, nil },
		nil,
	)

	req := TokenSpec{Type: TokenTypeTAT, AppID: "cli_a"}
	first, err := tp.ResolveToken(context.Background(), req)
	if err != nil || first.Token != "your-access-token" {
		t.Fatalf("first resolve = %+v, %v; want minted token", first, err)
	}
	second, err := tp.ResolveToken(context.Background(), req)
	if err != nil || second.Token != "your-access-token" {
		t.Fatalf("second resolve = %+v, %v; want cached token", second, err)
	}
	if tripper.calls != 1 {
		t.Fatalf("HTTP calls = %d, want exactly 1 (second resolve must hit the cache)", tripper.calls)
	}
}
