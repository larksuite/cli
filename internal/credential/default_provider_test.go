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
	type requestContextKey struct{}
	const requestContextValue = "exec-123"
	var calls atomic.Int32
	started, release := make(chan any, 1), make(chan struct{})
	client := &http.Client{Transport: tatRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		call := calls.Add(1)
		if call == 1 {
			started <- req.Context().Value(requestContextKey{})
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
	provider := NewDefaultTokenProvider(account, func() (*http.Client, error) { return client, nil }, io.Discard, nil)
	now := time.Unix(1700000000, 0)
	provider.timeNow = func() time.Time { return now }

	type tokenResult struct {
		token *TokenResult
		err   error
	}
	leaderCtx, cancelLeader := context.WithCancel(
		context.WithValue(context.Background(), requestContextKey{}, requestContextValue),
	)
	defer cancelLeader()
	leader := make(chan tokenResult, 1)
	go func() {
		token, err := provider.ResolveToken(leaderCtx, TokenSpec{Type: TokenTypeTAT})
		leader <- tokenResult{token: token, err: err}
	}()
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	select {
	case got := <-started:
		if got != requestContextValue {
			t.Fatalf("mint context value = %v, want %q", got, requestContextValue)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("initial token request did not start")
	}
	cancelLeader()
	if result := <-leader; result.token != nil || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("canceled leader = (%+v, %v)", result.token, result.err)
	}
	close(release)
	released = true
	initial, err := provider.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeTAT})
	if err != nil || initial == nil || initial.Token != "token-1" {
		t.Fatalf("initial token = (%+v, %v)", initial, err)
	}
	initial.Token = "mutated"
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

func TestClassifyTATResponseCode_OAuthErrors(t *testing.T) {
	for _, tc := range []struct {
		oauthError string
		category   errs.Category
		subtype    errs.Subtype
	}{
		{"invalid_client", errs.CategoryConfig, errs.SubtypeInvalidClient},
		{"unauthorized_client", errs.CategoryAuthorization, errs.SubtypeUnauthorizedClient},
		{"invalid_grant", errs.CategoryAuthentication, errs.SubtypeInvalidGrant},
		{"invalid_request", errs.CategoryAPI, errs.SubtypeInvalidParameters},
		{"unsupported_grant_type", errs.CategoryAPI, errs.SubtypeUnsupportedGrantType},
		{"invalid_scope", errs.CategoryAPI, errs.SubtypeInvalidScope},
		{"vendor_error", errs.CategoryAPI, errs.SubtypeUnknown},
	} {
		for _, code := range []int{0, 99999} {
			for _, description := range []string{"server detail", ""} {
				t.Run(fmt.Sprintf("%s/%d/description=%t", tc.oauthError, code, description != ""), func(t *testing.T) {
					err := classifyTATResponseCode(code, tc.oauthError, description, "feishu", "cli_app_x")
					problem, ok := errs.ProblemOf(err)
					wantMessage := description
					if wantMessage == "" {
						wantMessage = tc.oauthError
					}
					if !ok || problem.Category != tc.category || problem.Subtype != tc.subtype || problem.Code != code || problem.Retryable || problem.Message != wantMessage {
						t.Fatalf("error = %+v, want %s/%s code=%d message=%q without retry", problem, tc.category, tc.subtype, code, wantMessage)
					}
				})
			}
		}
	}
}

func TestClassifyTATResponseCode_NumericPrecedence(t *testing.T) {
	for _, tc := range []struct {
		code       int
		oauthError string
		category   errs.Category
		subtype    errs.Subtype
		retryable  bool
	}{
		{21000, "invalid_client", errs.CategoryPolicy, errs.SubtypeChallengeRequired, false},
		{21001, "unauthorized_client", errs.CategoryPolicy, errs.SubtypeAccessDenied, false},
		{20037, "invalid_grant", errs.CategoryAuthentication, errs.SubtypeRefreshTokenExpired, false},
		{99991672, "invalid_scope", errs.CategoryAuthorization, errs.SubtypeAppScopeNotApplied, false},
		{99991400, "invalid_request", errs.CategoryAPI, errs.SubtypeRateLimit, true},
		{10014, "unauthorized_client", errs.CategoryConfig, errs.SubtypeInvalidClient, false},
		{20002, "invalid_client", errs.CategoryConfig, errs.SubtypeInvalidClient, false},
		{20048, "invalid_client", errs.CategoryConfig, errs.SubtypeInvalidClient, false},
		{20068, "invalid_scope", errs.CategoryAPI, errs.SubtypeInvalidScope, false},
		{800070003, "invalid_scope", errs.CategoryAPI, errs.SubtypeInvalidScope, false},
		{800100112, "invalid_scope", errs.CategoryInternal, errs.SubtypeUnknown, false},
	} {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			err := classifyTATResponseCode(tc.code, tc.oauthError, "server detail", "feishu", "cli_app_x")
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != tc.category || problem.Subtype != tc.subtype || problem.Code != tc.code || problem.Retryable != tc.retryable {
				t.Fatalf("error = %+v, want %s/%s code=%d retryable=%t", problem, tc.category, tc.subtype, tc.code, tc.retryable)
			}
			if tc.category == errs.CategoryConfig && problem.Hint == "" {
				t.Fatal("numeric configuration errors must retain recovery guidance")
			}
		})
	}
}
