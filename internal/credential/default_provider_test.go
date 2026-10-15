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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/zalando/go-keyring"
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
	provider := NewDefaultTokenProvider(account, func() (*http.Client, error) { return client, nil }, io.Discard)
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

func TestDefaultTokenProviderRetainsClearedUATErrorAcrossRepeatedResolve(t *testing.T) {
	keyring.MockInit()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	const (
		appID      = "cli_offline_repeat"
		userOpenID = "ou_offline_repeat"
	)
	config := &core.MultiAppConfig{Apps: []core.AppConfig{{
		AppId:     appID,
		AppSecret: core.PlainSecret("secret"),
		Brand:     core.BrandFeishu,
		DPoPMode:  core.DPoPModeDisabled,
		Users:     []core.AppUser{{UserOpenId: userOpenID, UserName: "Test User"}},
	}}}
	if err := core.SaveMultiAppConfig(config); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	if err := auth.SetStoredToken(&auth.StoredUAToken{
		AppId:            appID,
		UserOpenId:       userOpenID,
		AccessToken:      "u-expired-no-refresh",
		ExpiresAt:        now - 1000,
		RefreshExpiresAt: now - 1000,
		Scope:            "im:chat:read",
		TokenType:        auth.StoredTokenTypeBearer,
	}); err != nil {
		t.Fatal(err)
	}

	account := NewDefaultAccountProvider(
		func() keychain.KeychainAccess { return &tenantTokenStoreKeychain{} },
		"",
		core.ProfileFromConfig,
	)
	provider := NewDefaultTokenProvider(account, func() (*http.Client, error) { return http.DefaultClient, nil }, io.Discard)

	// First call (simulating shortcut/service scope preflight) clears the expired token and returns the offline_access diagnosis.
	_, firstErr := provider.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: appID})
	firstProblem, ok := errs.ProblemOf(firstErr)
	if !ok || !strings.Contains(firstProblem.Hint, "offline_access") {
		t.Fatalf("first ResolveToken() error = %v (hint=%q), want offline_access diagnosis", firstErr, firstProblem.Hint)
	}

	// A concurrent call that observed hadStored != nil before deletion and later received a generic no-token error
	// must not overwrite the first call's offline_access diagnosis.
	racedErr := provider.recordClearedUATError(appID, userOpenID, auth.NewNeedUserAuthorizationError(userOpenID))
	racedProblem, ok := errs.ProblemOf(racedErr)
	if !ok || !strings.Contains(racedProblem.Hint, "offline_access") {
		t.Fatalf("recordClearedUATError() after race = %v (hint=%q), want preserved offline_access diagnosis", racedErr, racedProblem.Hint)
	}

	// Conversely, if a generic no-token error was recorded first, a subsequent specific diagnosis must upgrade it.
	provider.uatMu.Lock()
	provider.clearCachedUATErrorLocked(appID, userOpenID)
	provider.uatMu.Unlock()
	_ = provider.recordClearedUATError(appID, userOpenID, auth.NewNeedUserAuthorizationError(userOpenID))
	upgradedErr := provider.recordClearedUATError(appID, userOpenID, firstErr)
	upgradedProblem, ok := errs.ProblemOf(upgradedErr)
	if !ok || !strings.Contains(upgradedProblem.Hint, "offline_access") {
		t.Fatalf("recordClearedUATError() when generic recorded first = %v (hint=%q), want upgraded offline_access diagnosis", upgradedErr, upgradedProblem.Hint)
	}

	// Second call in the same CLI invocation (simulating DoAPI / AccessToken after preflight) must retain the offline_access diagnosis.
	_, secondErr := provider.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: appID})
	secondProblem, ok := errs.ProblemOf(secondErr)
	if !ok || !strings.Contains(secondProblem.Hint, "offline_access") {
		t.Fatalf("second ResolveToken() error = %v (hint=%q), want retained offline_access diagnosis", secondErr, secondProblem.Hint)
	}

	// Concurrent ResolveToken calls against a freshly expired access-only token must all receive the offline_access diagnosis.
	if err := auth.SetStoredToken(&auth.StoredUAToken{
		AppId:            appID,
		UserOpenId:       userOpenID,
		AccessToken:      "u-expired-no-refresh-concurrent",
		ExpiresAt:        now - 1000,
		RefreshExpiresAt: now - 1000,
		Scope:            "im:chat:read",
		TokenType:        auth.StoredTokenTypeBearer,
	}); err != nil {
		t.Fatal(err)
	}
	concurrentProvider := NewDefaultTokenProvider(account, func() (*http.Client, error) { return http.DefaultClient, nil }, io.Discard)
	const workers = 8
	errsCh := make(chan error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := concurrentProvider.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: appID})
			errsCh <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errsCh)
	for workerErr := range errsCh {
		workerProblem, ok := errs.ProblemOf(workerErr)
		if !ok || !strings.Contains(workerProblem.Hint, "offline_access") {
			t.Fatalf("concurrent ResolveToken() error = %v (hint=%q), want offline_access diagnosis", workerErr, workerProblem.Hint)
		}
	}

	// A token-store read/corruption error must surface directly instead of being masked by the retained diagnosis.
	if err := keychain.Set(keychain.LarkCliService, appID+":"+userOpenID, "{corrupt-json"); err != nil {
		t.Fatal(err)
	}
	_, corruptErr := provider.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: appID})
	corruptProblem, ok := errs.ProblemOf(corruptErr)
	if !ok || corruptProblem.Category != errs.CategoryInternal || corruptProblem.Subtype != errs.SubtypeStorage {
		t.Fatalf("ResolveToken() with corrupt stored token = %v, want internal/storage error", corruptErr)
	}

	// Once a valid token is stored, ResolveToken clears the retained error and returns the token.
	if err := auth.SetStoredToken(&auth.StoredUAToken{
		AppId:            appID,
		UserOpenId:       userOpenID,
		AccessToken:      "u-fresh-valid",
		ExpiresAt:        now + 3600_000,
		RefreshExpiresAt: now + 3600_000,
		Scope:            "im:chat:read",
		TokenType:        auth.StoredTokenTypeBearer,
	}); err != nil {
		t.Fatal(err)
	}
	resolved, thirdErr := provider.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: appID})
	if thirdErr != nil || resolved == nil || resolved.Token != "u-fresh-valid" {
		t.Fatalf("third ResolveToken() = (%+v, %v), want fresh valid token", resolved, thirdErr)
	}
}
