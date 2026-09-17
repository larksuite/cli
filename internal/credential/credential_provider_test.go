// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
)

type mockExtProvider struct {
	name       string
	account    *extcred.Account
	token      *extcred.Token
	err        error
	accountErr error
	tokenErr   error
}

func (m *mockExtProvider) Name() string { return m.name }
func (m *mockExtProvider) ResolveAccount(ctx context.Context) (*extcred.Account, error) {
	if m.accountErr != nil {
		return nil, m.accountErr
	}
	return m.account, m.err
}
func (m *mockExtProvider) ResolveToken(ctx context.Context, req extcred.TokenSpec) (*extcred.Token, error) {
	if m.tokenErr != nil {
		return nil, m.tokenErr
	}
	return m.token, m.err
}

type mockDefaultAcct struct {
	account *Account
	err     error
}

func (m *mockDefaultAcct) ResolveAccount(ctx context.Context) (*Account, error) {
	return m.account, m.err
}

type mockDefaultToken struct {
	result *TokenResult
	err    error
}

func (m *mockDefaultToken) ResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, error) {
	return m.result, m.err
}

func TestCredentialProvider_AccountFromExtension(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "env", account: &extcred.Account{AppID: "ext_app", Brand: "lark"}}},
		&mockDefaultAcct{account: &Account{AppID: "default_app"}},
		&mockDefaultToken{}, nil,
	)
	acct, err := cp.ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct.AppID != "ext_app" {
		t.Errorf("expected ext_app, got %s", acct.AppID)
	}
}

func TestCredentialProvider_AccountFallsToDefault(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "skip"}},
		&mockDefaultAcct{account: &Account{AppID: "default_app", Brand: "feishu"}},
		&mockDefaultToken{}, nil,
	)
	acct, err := cp.ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct.AppID != "default_app" {
		t.Errorf("expected default_app, got %s", acct.AppID)
	}
}

func TestCredentialProvider_AccountBlockStopsChain(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "blocker", err: &extcred.BlockError{Provider: "blocker", Reason: "denied"}}},
		&mockDefaultAcct{account: &Account{AppID: "default_app"}},
		&mockDefaultToken{}, nil,
	)
	_, err := cp.ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var blockErr *extcred.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %T", err)
	}
}

func TestCredentialProvider_AccountCached(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "env", account: &extcred.Account{AppID: "cached"}}},
		nil, nil, nil,
	)
	a1, _ := cp.ResolveAccount(context.Background())
	a2, _ := cp.ResolveAccount(context.Background())
	if a1 != a2 {
		t.Error("expected same pointer (cached)")
	}
}

func TestCredentialProvider_TokenFromExtension(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{
			name:    "env",
			account: &extcred.Account{AppID: "ext_app", Brand: "feishu"},
			token:   &extcred.Token{Value: "ext_tok", Source: "env"},
		}},
		&mockDefaultAcct{}, &mockDefaultToken{result: &TokenResult{Token: "default_tok"}}, nil,
	)
	result, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT})
	if err != nil {
		t.Fatal(err)
	}
	if result.Token != "ext_tok" {
		t.Errorf("expected ext_tok, got %s", result.Token)
	}
	if result.Source != core.CredentialSourceEnv {
		t.Errorf("TokenResult.Source = %q, want env", result.Source)
	}
}

func TestCredentialProvider_TokenFallsToDefault(t *testing.T) {
	defaultResult := &TokenResult{
		Token:  "default_tok",
		Source: core.CredentialSourceExtension,
	}
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "skip"}},
		&mockDefaultAcct{}, &mockDefaultToken{result: defaultResult}, nil,
	)
	result, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT})
	if err != nil {
		t.Fatal(err)
	}
	if result.Token != "default_tok" {
		t.Errorf("expected default_tok, got %s", result.Token)
	}
	if result.Source != core.CredentialSourceLocal {
		t.Errorf("TokenResult.Source = %q, want local", result.Source)
	}
	if defaultResult.Source != core.CredentialSourceExtension {
		t.Errorf("default resolver result Source = %q, want unchanged extension", defaultResult.Source)
	}
}

func TestExtensionCredentialSource(t *testing.T) {
	for _, test := range []struct{ provider, want string }{
		{provider: "sidecar", want: "sidecar"},
		{provider: "custom", want: "extension"},
	} {
		source := extensionTokenSource{provider: &mockExtProvider{name: test.provider}}
		if got := source.CredentialSource(); got != test.want {
			t.Errorf("provider %q source = %q, want %q", test.provider, got, test.want)
		}
	}
}

func TestCredentialProvider_TokenDoesNotMixSourcesAfterDefaultAccountSelection(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "env", token: &extcred.Token{Value: "ext_tok", Source: "env"}}},
		&mockDefaultAcct{account: &Account{AppID: "default_app", Brand: core.BrandFeishu}},
		&mockDefaultToken{result: &TokenResult{Token: "default_tok"}},
		nil,
	)

	if _, err := cp.ResolveAccount(context.Background()); err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}

	result, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT})
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}
	if result.Token != "default_tok" {
		t.Fatalf("ResolveToken() token = %q, want %q", result.Token, "default_tok")
	}
}

func TestCredentialProvider_SelectedSourceWithoutTokenReturnsUnavailableError(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{
			name:    "env",
			account: &extcred.Account{AppID: "ext_app", Brand: "feishu"},
		}},
		nil, nil, nil,
	)

	if _, err := cp.ResolveAccount(context.Background()); err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}

	_, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT})
	if err == nil {
		t.Fatal("ResolveToken() error = nil, want unavailable error")
	}
	var unavailableErr *TokenUnavailableError
	if !errors.As(err, &unavailableErr) {
		t.Fatalf("ResolveToken() error type = %T, want *TokenUnavailableError", err)
	}
	if unavailableErr.Source != "env" || unavailableErr.Type != TokenTypeUAT {
		t.Fatalf("ResolveToken() unavailable error = %+v, want source env and type uat", unavailableErr)
	}
}

func TestCredentialProvider_ResolveTokenPropagatesNonBlockExtensionError(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "env", err: errors.New("provider exploded")}},
		nil,
		&mockDefaultToken{result: &TokenResult{Token: "default_tok"}},
		nil,
	)

	_, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT})
	if err == nil || err.Error() != "provider exploded" {
		t.Fatalf("ResolveToken() error = %v, want provider exploded", err)
	}
}

func TestCredentialProvider_ResolveIdentityHint_FromExtensionAccount(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "env", account: &extcred.Account{
			AppID:               "ext_app",
			Brand:               "feishu",
			DefaultAs:           extcred.IdentityUser,
			SupportedIdentities: extcred.SupportsUser,
		}}},
		nil, nil, nil,
	)

	hint, err := cp.ResolveIdentityHint(context.Background())
	if err != nil {
		t.Fatalf("ResolveIdentityHint() error = %v", err)
	}
	if hint.DefaultAs != core.AsUser {
		t.Fatalf("ResolveIdentityHint() defaultAs = %q, want %q", hint.DefaultAs, core.AsUser)
	}
	if hint.AutoAs != core.AsUser {
		t.Fatalf("ResolveIdentityHint() autoAs = %q, want %q", hint.AutoAs, core.AsUser)
	}
}

func TestCredentialProvider_ResolveIdentityHint_DefaultSourceUsesStoredTokenState(t *testing.T) {
	origGetStoredToken := getStoredToken
	origTokenStatus := getStoredTokenStatus
	t.Cleanup(func() {
		getStoredToken = origGetStoredToken
		getStoredTokenStatus = origTokenStatus
	})

	getStoredToken = func(appID, userOpenID string) (*auth.StoredUAToken, error) {
		if appID != "default_app" || userOpenID != "ou_default" {
			t.Fatalf("GetStoredToken() args = (%q, %q), want (%q, %q)", appID, userOpenID, "default_app", "ou_default")
		}
		return &auth.StoredUAToken{AppId: appID, UserOpenId: userOpenID}, nil
	}
	getStoredTokenStatus = func(token *auth.StoredUAToken) string {
		return "valid"
	}

	cp := NewCredentialProvider(
		nil,
		&mockDefaultAcct{account: &Account{AppID: "default_app", Brand: core.BrandFeishu, UserOpenId: "ou_default"}},
		&mockDefaultToken{result: &TokenResult{Token: "default_tok"}},
		nil,
	)

	hint, err := cp.ResolveIdentityHint(context.Background())
	if err != nil {
		t.Fatalf("ResolveIdentityHint() error = %v", err)
	}
	if hint.AutoAs != core.AsUser {
		t.Fatalf("ResolveIdentityHint() autoAs = %q, want %q", hint.AutoAs, core.AsUser)
	}
}

func TestCredentialProvider_ResolveIdentityHint_CachesResult(t *testing.T) {
	origGetStoredToken := getStoredToken
	origTokenStatus := getStoredTokenStatus
	t.Cleanup(func() {
		getStoredToken = origGetStoredToken
		getStoredTokenStatus = origTokenStatus
	})

	storedCalls := 0
	statusCalls := 0
	getStoredToken = func(appID, userOpenID string) (*auth.StoredUAToken, error) {
		storedCalls++
		return &auth.StoredUAToken{AppId: appID, UserOpenId: userOpenID}, nil
	}
	getStoredTokenStatus = func(token *auth.StoredUAToken) string {
		statusCalls++
		return "valid"
	}

	cp := NewCredentialProvider(
		nil,
		&mockDefaultAcct{account: &Account{AppID: "default_app", Brand: core.BrandFeishu, UserOpenId: "ou_default"}},
		&mockDefaultToken{result: &TokenResult{Token: "default_tok"}},
		nil,
	)

	for i := 0; i < 2; i++ {
		hint, err := cp.ResolveIdentityHint(context.Background())
		if err != nil {
			t.Fatalf("ResolveIdentityHint() error = %v", err)
		}
		if hint.AutoAs != core.AsUser {
			t.Fatalf("ResolveIdentityHint() autoAs = %q, want %q", hint.AutoAs, core.AsUser)
		}
	}

	if storedCalls != 1 {
		t.Fatalf("GetStoredToken() calls = %d, want 1", storedCalls)
	}
	if statusCalls != 1 {
		t.Fatalf("TokenStatus() calls = %d, want 1", statusCalls)
	}
}

func TestCredentialProvider_ResolveTokenTreatsEmptyDefaultTokenAsMalformed(t *testing.T) {
	cp := NewCredentialProvider(
		nil,
		nil,
		&mockDefaultToken{result: &TokenResult{Token: ""}},
		nil,
	)

	_, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT})
	if err == nil || !strings.Contains(err.Error(), "empty token") {
		t.Fatalf("ResolveToken() error = %v, want malformed empty token error", err)
	}
}

func TestCredentialProvider_ResolveAccountDoesNotEnrichWithTokenFromDifferentProvider(t *testing.T) {
	httpClientCalls := 0
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "env", token: &extcred.Token{Value: "ext_tok", Source: "env"}}},
		&mockDefaultAcct{account: &Account{
			AppID:      "default_app",
			Brand:      core.BrandFeishu,
			UserOpenId: "ou_default",
			UserName:   "Default User",
		}},
		&mockDefaultToken{},
		func() (*http.Client, error) {
			httpClientCalls++
			return nil, errors.New("unexpected enrich call")
		},
	)

	acct, err := cp.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if httpClientCalls != 0 {
		t.Fatalf("httpClient() called %d times, want 0", httpClientCalls)
	}
	if acct.UserOpenId != "ou_default" || acct.UserName != "Default User" {
		t.Fatalf("resolved user = (%q, %q), want (%q, %q)", acct.UserOpenId, acct.UserName, "ou_default", "Default User")
	}
}

func TestCredentialProvider_ResolveAccountClearsUnverifiedExtensionIdentityOnTokenError(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "env", account: &extcred.Account{
			AppID:  "ext_app",
			Brand:  "feishu",
			OpenID: "ou_ext",
		}, tokenErr: errors.New("token lookup failed")}},
		nil,
		nil,
		func() (*http.Client, error) {
			t.Fatal("httpClient() should not be called when token lookup fails")
			return nil, nil
		},
	)

	acct, err := cp.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if acct.UserOpenId != "" || acct.UserName != "" {
		t.Fatalf("resolved user = (%q, %q), want cleared unverified identity", acct.UserOpenId, acct.UserName)
	}
}

func TestCredentialProvider_ResolveAccountWarnsWhenExtensionIdentityVerificationFails(t *testing.T) {
	var warnBuf bytes.Buffer

	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "env", account: &extcred.Account{
			AppID:  "ext_app",
			Brand:  "feishu",
			OpenID: "ou_ext",
		}, tokenErr: errors.New("token lookup failed")}},
		nil,
		nil,
		func() (*http.Client, error) {
			t.Fatal("httpClient() should not be called when token lookup fails")
			return nil, nil
		},
	)
	cp.SetWarnOut(&warnBuf)

	acct, err := cp.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if acct.UserOpenId != "" || acct.UserName != "" {
		t.Fatalf("resolved user = (%q, %q), want cleared unverified identity", acct.UserOpenId, acct.UserName)
	}
	if !strings.Contains(warnBuf.String(), "unable to verify user identity from credential source \"env\"") {
		t.Fatalf("warning output = %q, want source-specific verification warning", warnBuf.String())
	}
	if !strings.Contains(warnBuf.String(), "token lookup failed") {
		t.Fatalf("warning output = %q, want underlying error", warnBuf.String())
	}
}

func TestCredentialProvider_ResolveTokenDoesNotBypassFailedDefaultAccountResolution(t *testing.T) {
	cp := NewCredentialProvider(
		nil,
		&mockDefaultAcct{err: errors.New("config unavailable")},
		&mockDefaultToken{result: &TokenResult{Token: "default_tok"}},
		nil,
	)

	_, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT})
	if err == nil || err.Error() != "config unavailable" {
		t.Fatalf("ResolveToken() error = %v, want config unavailable", err)
	}
}

func TestActiveExtensionProviderName_ExtActive(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "env", account: &extcred.Account{AppID: "app"}}},
		nil, nil, nil,
	)
	name, err := cp.ActiveExtensionProviderName(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "env" {
		t.Errorf("got %q, want %q", name, "env")
	}
}

func TestActiveExtensionProviderName_BlockError(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{
			name:       "env",
			accountErr: &extcred.BlockError{Provider: "env", Reason: "APP_ID missing"},
		}},
		nil, nil, nil,
	)
	name, err := cp.ActiveExtensionProviderName(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "env" {
		t.Errorf("got %q, want %q", name, "env")
	}
}

func TestActiveExtensionProviderName_NoExtProvider(t *testing.T) {
	cp := NewCredentialProvider(nil, nil, nil, nil)
	name, err := cp.ActiveExtensionProviderName(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "" {
		t.Errorf("got %q, want empty string", name)
	}
}

func TestActiveExtensionProviderName_UnexpectedError(t *testing.T) {
	sentinel := errors.New("network timeout")
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "env", accountErr: sentinel}},
		nil, nil, nil,
	)
	_, err := cp.ActiveExtensionProviderName(context.Background())
	if !errors.Is(err, sentinel) {
		t.Errorf("got %v, want sentinel error", err)
	}
}

func TestActiveExtensionProviderName_SkipsNilProvider(t *testing.T) {
	// nil account + nil error = provider not applicable; fallback returns ""
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "sidecar"}}, // no account set → returns nil, nil
		nil, nil, nil,
	)
	name, err := cp.ActiveExtensionProviderName(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "" {
		t.Errorf("got %q, want empty string", name)
	}
}

// agentFixture describes one Agent Employee profile shape: the verified
// principal, the keychain record, and the config.json snapshot.
type agentFixture struct {
	principalOpenID string
	// keychainOpenID overrides the seeded record's principal; empty means principalOpenID.
	keychainOpenID string
	keychainScope  string
	configScope    string
	// onHTTP fires when the HTTP client factory is invoked; scope resolution must never reach it.
	onHTTP func()
	// warnOut is the token provider's stderr stream; defaults to io.Discard.
	warnOut io.Writer
}

// newAgentCredentialProvider builds a CredentialProvider bound to the Agent
// Employee profile f describes, with a fake keychain seeded from it.
func newAgentCredentialProvider(t *testing.T, f agentFixture) *CredentialProvider {
	t.Helper()
	// Empty config dir: only the mocked Account below can resolve.
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	kc := newCredentialMemoryKeychain()
	keychainOpenID := f.keychainOpenID
	if keychainOpenID == "" {
		keychainOpenID = f.principalOpenID
	}
	// Seeded expired on purpose: a path that mints instead of peeking would reach for HTTP.
	stored := &auth.StoredAAToken{
		AppID:       "cli_a",
		UserOpenID:  keychainOpenID,
		AccessToken: "tok",
		GrantedAt:   time.Now().Add(-2 * time.Hour).UnixMilli(),
		ExpiresAt:   time.Now().Add(-time.Hour).UnixMilli(),
		Scope:       f.keychainScope,
	}
	data, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("marshal stored AAT: %v", err)
	}
	if err := kc.Set(keychain.LarkCliService, auth.AATAccountKey("cli_a"), string(data)); err != nil {
		t.Fatalf("seed keychain: %v", err)
	}

	httpClient := func() (*http.Client, error) {
		if f.onHTTP != nil {
			f.onHTTP()
		}
		return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatalf("unexpected HTTP call to %s", req.URL)
			return nil, nil
		})}, nil
	}

	warnOut := f.warnOut
	if warnOut == nil {
		warnOut = io.Discard
	}
	defaultAcct := NewDefaultAccountProvider(func() keychain.KeychainAccess { return kc }, "", core.ProfileFromConfig)
	defaultToken := NewDefaultTokenProvider(defaultAcct, httpClient, warnOut)

	acct := &Account{
		AppID:           "cli_a",
		CredentialKind:  core.CredentialAAT,
		PrincipalOpenID: f.principalOpenID,
		AgentScope:      f.configScope,
	}
	return NewCredentialProvider(nil, &mockDefaultAcct{account: acct}, defaultToken, httpClient)
}

// TestResolveScopeMetadataForAgentPrefersKeychain pins that the keychain scope wins over the bind-time snapshot.
func TestResolveScopeMetadataForAgentPrefersKeychain(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK", "on")
	p := newAgentCredentialProvider(t, agentFixture{
		principalOpenID: "ou_x",
		keychainScope:   "docs:doc",
		configScope:     "bitable:app",
	})
	got, err := p.ResolveScopeMetadata(context.Background(),
		AccessSpec{Identity: core.AsUser, AppID: "cli_a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Available || got.Scopes != "docs:doc" {
		t.Errorf("got %+v, want the keychain scope to win", got)
	}
}

// TestResolveScopeMetadataForAgentFallsBackToConfigSnapshot pins the fallback when the keychain record has no scope.
func TestResolveScopeMetadataForAgentFallsBackToConfigSnapshot(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK", "on")
	p := newAgentCredentialProvider(t, agentFixture{
		principalOpenID: "ou_x",
		keychainScope:   "", // legacy record, or written before the Scope field
		configScope:     "bitable:app",
	})
	got, err := p.ResolveScopeMetadata(context.Background(),
		AccessSpec{Identity: core.AsUser, AppID: "cli_a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Available || got.Scopes != "bitable:app" {
		t.Errorf("got %+v, want the config snapshot", got)
	}
}

// TestResolveScopeMetadataForAgentSkipsWhenNoFacts pins that no scope fact
// anywhere reads as "nothing to compare against", not as a failure.
func TestResolveScopeMetadataForAgentSkipsWhenNoFacts(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK", "on")
	p := newAgentCredentialProvider(t, agentFixture{principalOpenID: "ou_x"})
	got, err := p.ResolveScopeMetadata(context.Background(),
		AccessSpec{Identity: core.AsUser, AppID: "cli_a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Available || !got.NotApplicable {
		t.Errorf("got %+v, want Available=false NotApplicable=true", got)
	}
}

// TestResolveScopeMetadataForAgentMakesNoNetworkCall guards the --dry-run
// constraint: scope resolution reads cached facts and never mints or renews.
func TestResolveScopeMetadataForAgentMakesNoNetworkCall(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK", "on")
	var calls int32
	p := newAgentCredentialProvider(t, agentFixture{
		principalOpenID: "ou_x",
		keychainScope:   "docs:doc",
		onHTTP:          func() { atomic.AddInt32(&calls, 1) },
	})
	if _, err := p.ResolveScopeMetadata(context.Background(),
		AccessSpec{Identity: core.AsUser, AppID: "cli_a"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Errorf("made %d HTTP call(s); scope metadata resolution must stay side-effect free", n)
	}
}

// TestResolveScopeMetadataForAgentRejectsPrincipalMismatch pins that a record for another principal is ignored.
func TestResolveScopeMetadataForAgentRejectsPrincipalMismatch(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK", "on")
	p := newAgentCredentialProvider(t, agentFixture{
		principalOpenID: "ou_x",
		keychainOpenID:  "ou_other",
		keychainScope:   "docs:doc",
		configScope:     "bitable:app",
	})
	got, err := p.ResolveScopeMetadata(context.Background(),
		AccessSpec{Identity: core.AsUser, AppID: "cli_a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Scopes != "bitable:app" {
		t.Errorf("got %+v, want the keychain record for a different principal to be ignored", got)
	}
}
