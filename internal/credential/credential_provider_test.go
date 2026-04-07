package credential

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
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
}

func TestCredentialProvider_TokenFallsToDefault(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "skip"}},
		&mockDefaultAcct{}, &mockDefaultToken{result: &TokenResult{Token: "default_tok"}}, nil,
	)
	result, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT})
	if err != nil {
		t.Fatal(err)
	}
	if result.Token != "default_tok" {
		t.Errorf("expected default_tok, got %s", result.Token)
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

	getStoredToken = func(appID, userOpenID string) *auth.StoredUAToken {
		if appID != "default_app" || userOpenID != "ou_default" {
			t.Fatalf("GetStoredToken() args = (%q, %q), want (%q, %q)", appID, userOpenID, "default_app", "ou_default")
		}
		return &auth.StoredUAToken{AppId: appID, UserOpenId: userOpenID}
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
	getStoredToken = func(appID, userOpenID string) *auth.StoredUAToken {
		storedCalls++
		return &auth.StoredUAToken{AppId: appID, UserOpenId: userOpenID}
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
