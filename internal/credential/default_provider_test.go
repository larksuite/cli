// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
)

var _ DefaultTokenResolver = (*DefaultTokenProvider)(nil)
var _ DefaultAccountResolver = (*DefaultAccountProvider)(nil)

func TestDefaultTokenProviderSharesRefreshAndCachesDefensiveCopies(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	config := &core.MultiAppConfig{Apps: []core.AppConfig{{
		AppId:     "cli-cache",
		AppSecret: core.PlainSecret("secret"),
		Brand:     core.BrandFeishu,
		DPoPMode:  core.DPoPModeDisabled,
	}}}
	if err := core.SaveMultiAppConfig(config); err != nil {
		t.Fatal(err)
	}
	account := NewDefaultAccountProvider(
		func() keychain.KeychainAccess { return &tenantTokenStoreKeychain{} },
		"",
		core.ProfileFromConfig,
	)
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	client := &http.Client{Transport: tatRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		call := calls.Add(1)
		if call == 1 {
			close(started)
			<-release
		}
		body := fmt.Sprintf(`{"code":0,"access_token":"token-%d","token_type":"Bearer","expires_in":3600}`, call)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}
	provider := NewDefaultTokenProvider(account, func() (*http.Client, error) { return client, nil }, io.Discard)
	now := time.Unix(1700000000, 0)
	provider.timeNow = func() time.Time { return now }

	type tokenResult struct {
		token *TokenResult
		err   error
	}
	first := make(chan tokenResult, 1)
	go func() {
		token, err := provider.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeTAT})
		first <- tokenResult{token: token, err: err}
	}()
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("initial token request did not start")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if token, err := provider.ResolveToken(canceled, TokenSpec{Type: TokenTypeTAT}); token != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter = (%+v, %v)", token, err)
	}
	close(release)
	released = true
	initial := <-first
	if initial.err != nil || initial.token == nil || initial.token.Token != "token-1" {
		t.Fatalf("initial token = (%+v, %v)", initial.token, initial.err)
	}
	initial.token.Token = "mutated"
	cached, err := provider.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeTAT})
	if err != nil || cached == nil || cached.Token != "token-1" || calls.Load() != 1 {
		t.Fatalf("cached token = (%+v, %v), calls=%d", cached, err, calls.Load())
	}
	now = now.Add(time.Hour)
	refreshed, err := provider.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeTAT})
	if err != nil || refreshed == nil || refreshed.Token != "token-2" || calls.Load() != 2 {
		t.Fatalf("refreshed token = (%+v, %v), calls=%d", refreshed, err, calls.Load())
	}
}

func TestDefaultAccountProviderResolvesSelectedDPoPPolicy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	config := &core.MultiAppConfig{
		CurrentApp: "required",
		Apps: []core.AppConfig{
			{Name: "required", AppId: "cli-required", DPoPMode: core.DPoPModeRequired},
			{Name: "disabled", AppId: "cli-disabled", DPoPMode: core.DPoPModeDisabled},
		},
	}
	if err := core.SaveMultiAppConfig(config); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		profile, appID string
		want           core.DPoPMode
	}{
		{profile: "", appID: "cli-required", want: core.DPoPModeRequired},
		{profile: "disabled", appID: "cli-disabled", want: core.DPoPModeDisabled},
		{profile: "disabled", appID: "other", want: core.DPoPModePreferred},
		{profile: "missing", appID: "cli-required", want: core.DPoPModePreferred},
	} {
		provider := NewDefaultAccountProvider(nil, tc.profile, core.ProfileFromFlag)
		if got := provider.ResolveLocalDPoPMode(tc.appID); got != tc.want {
			t.Errorf("ResolveLocalDPoPMode(%q, %q) = %q, want %q", tc.profile, tc.appID, got, tc.want)
		}
	}
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
// FetchTAT would surface this deterministic rejection as (nil, nil) — no token
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
