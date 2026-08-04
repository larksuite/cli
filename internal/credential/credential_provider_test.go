// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
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
	tokenCalls int
}

func (m *mockExtProvider) Name() string { return m.name }
func (m *mockExtProvider) ResolveAccount(ctx context.Context) (*extcred.Account, error) {
	if m.accountErr != nil {
		return nil, m.accountErr
	}
	return m.account, m.err
}
func (m *mockExtProvider) ResolveToken(ctx context.Context, req extcred.TokenSpec) (*extcred.Token, error) {
	m.tokenCalls++
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
	result     *TokenResult
	err        error
	tokenCalls int
}

func (m *mockDefaultToken) ResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, error) {
	m.tokenCalls++
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
	for _, sourceName := range []string{"env", "authsidecar"} {
		t.Run(sourceName, func(t *testing.T) {
			cp := NewCredentialProvider(
				[]extcred.Provider{&mockExtProvider{
					name:    sourceName,
					account: &extcred.Account{AppID: "ext_app", Brand: "feishu"},
					token:   &extcred.Token{Value: "ext_tok", Source: sourceName},
				}},
				&mockDefaultAcct{account: &Account{AppID: "default_app"}},
				&mockDefaultToken{result: &TokenResult{Token: "default_tok"}}, nil,
			)
			result, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: "ext_app"})
			if err != nil {
				t.Fatal(err)
			}
			if result.Token != "ext_tok" {
				t.Errorf("expected ext_tok, got %s", result.Token)
			}
		})
	}
}

func TestCredentialProvider_TokenFallsToDefault(t *testing.T) {
	defaultToken := &mockDefaultToken{result: &TokenResult{Token: "default_tok"}}
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "skip"}},
		&mockDefaultAcct{account: &Account{AppID: "default_app"}},
		defaultToken, nil,
	)
	result, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: "default_app"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Token != "default_tok" {
		t.Errorf("expected default_tok, got %s", result.Token)
	}
	if defaultToken.tokenCalls != 1 {
		t.Fatalf("default ResolveToken() calls = %d, want 1", defaultToken.tokenCalls)
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

	result, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: "default_app"})
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

	_, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: "ext_app"})
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

	_, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: "ext_app"})
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
		&mockDefaultAcct{account: &Account{AppID: "default_app"}},
		&mockDefaultToken{result: &TokenResult{Token: ""}},
		nil,
	)

	_, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: "default_app"})
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
	defaultToken := &mockDefaultToken{result: &TokenResult{Token: "default_tok"}}
	cp := NewCredentialProvider(
		nil,
		&mockDefaultAcct{err: errors.New("config unavailable")},
		defaultToken,
		nil,
	)

	_, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: "default_app"})
	if err == nil || err.Error() != "config unavailable" {
		t.Fatalf("ResolveToken() error = %v, want config unavailable", err)
	}
	if defaultToken.tokenCalls != 0 {
		t.Fatalf("default ResolveToken() calls = %d, want 0", defaultToken.tokenCalls)
	}
}

func TestCredentialProvider_ResolveTokenRejectsUnboundAppBeforeExtensionIO(t *testing.T) {
	tests := []struct {
		name  string
		appID string
	}{
		{name: "empty app id"},
		{name: "different app id", appID: "other_app"},
	}

	for _, tt := range tests {
		for _, sourceName := range []string{"env", "authsidecar"} {
			t.Run(tt.name+"/"+sourceName, func(t *testing.T) {
				provider := &mockExtProvider{
					name:    sourceName,
					account: &extcred.Account{AppID: "ext_app", Brand: "feishu"},
					token:   &extcred.Token{Value: "ext_tok", Source: sourceName},
				}
				httpClientCalls := 0
				cp := NewCredentialProvider(
					[]extcred.Provider{provider},
					&mockDefaultAcct{account: &Account{AppID: "default_app"}},
					&mockDefaultToken{result: &TokenResult{Token: "default_tok"}},
					func() (*http.Client, error) {
						httpClientCalls++
						return nil, errors.New("unexpected user_info call")
					},
				)

				_, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: tt.appID})
				if err == nil {
					t.Fatal("ResolveToken() error = nil, want app binding error")
				}
				assertInternalUnknownWithRetryHint(t, err)
				if provider.tokenCalls != 0 {
					t.Fatalf("extension ResolveToken() calls = %d, want 0", provider.tokenCalls)
				}
				if httpClientCalls != 0 {
					t.Fatalf("httpClient() calls = %d, want 0", httpClientCalls)
				}
			})
		}
	}
}

func TestCredentialProvider_ResolveTokenRejectsUnboundAppBeforeDefaultIO(t *testing.T) {
	tests := []struct {
		name  string
		appID string
	}{
		{name: "empty app id"},
		{name: "different app id", appID: "other_app"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defaultToken := &mockDefaultToken{result: &TokenResult{Token: "default_tok"}}
			cp := NewCredentialProvider(
				nil,
				&mockDefaultAcct{account: &Account{AppID: "default_app"}},
				defaultToken,
				nil,
			)

			_, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: tt.appID})
			if err == nil {
				t.Fatal("ResolveToken() error = nil, want app binding error")
			}
			assertInternalUnknownWithRetryHint(t, err)
			if defaultToken.tokenCalls != 0 {
				t.Fatalf("default ResolveToken() calls = %d, want 0", defaultToken.tokenCalls)
			}
		})
	}
}

func TestCredentialProvider_ResolveTokenRejectsNilAccountBeforeTokenIO(t *testing.T) {
	defaultToken := &mockDefaultToken{result: &TokenResult{Token: "default_tok"}}
	cp := NewCredentialProvider(
		nil,
		&mockDefaultAcct{},
		defaultToken,
		nil,
	)

	_, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: "requested_app"})
	if err == nil {
		t.Fatal("ResolveToken() error = nil, want nil account error")
	}
	assertInternalUnknownWithRetryHint(t, err)
	if defaultToken.tokenCalls != 0 {
		t.Fatalf("default ResolveToken() calls = %d, want 0", defaultToken.tokenCalls)
	}
}

func TestCredentialProvider_ResolveTokenRejectsMissingSelectedSourceWithoutFallback(t *testing.T) {
	extension := &mockExtProvider{
		name:  "env",
		token: &extcred.Token{Value: "ext_tok", Source: "env"},
	}
	defaultToken := &mockDefaultToken{result: &TokenResult{Token: "default_tok"}}
	cp := NewCredentialProvider(
		[]extcred.Provider{extension},
		&mockDefaultAcct{account: &Account{AppID: "default_app"}},
		defaultToken,
		nil,
	)
	cp.account = &Account{AppID: "selected_app"}
	cp.accountOnce.Do(func() {})

	_, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: "selected_app"})
	if err == nil {
		t.Fatal("ResolveToken() error = nil, want missing selected source error")
	}
	assertInternalUnknownWithRetryHint(t, err)
	if extension.tokenCalls != 0 {
		t.Fatalf("extension ResolveToken() calls = %d, want 0", extension.tokenCalls)
	}
	if defaultToken.tokenCalls != 0 {
		t.Fatalf("default ResolveToken() calls = %d, want 0", defaultToken.tokenCalls)
	}
}

func TestCredentialProvider_ResolveTokenMatchingExtensionDoesNotEnrichIdentity(t *testing.T) {
	provider := &mockExtProvider{
		name:    "env",
		account: &extcred.Account{AppID: "ext_app", Brand: "feishu"},
		token:   &extcred.Token{Value: "ext_tok", Source: "env"},
	}
	httpClientCalls := 0
	cp := NewCredentialProvider(
		[]extcred.Provider{provider},
		nil,
		nil,
		func() (*http.Client, error) {
			httpClientCalls++
			return nil, errors.New("unexpected user_info call")
		},
	)

	result, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: "ext_app"})
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}
	if result.Token != "ext_tok" {
		t.Fatalf("ResolveToken() token = %q, want %q", result.Token, "ext_tok")
	}
	if provider.tokenCalls != 1 {
		t.Fatalf("extension ResolveToken() calls = %d, want 1", provider.tokenCalls)
	}
	if httpClientCalls != 0 {
		t.Fatalf("httpClient() calls = %d, want 0", httpClientCalls)
	}
}

func assertInternalUnknownWithRetryHint(t *testing.T, err error) {
	t.Helper()
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error type = %T, want typed internal error", err)
	}
	if problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeUnknown {
		t.Fatalf("error problem = %+v, want internal/unknown", problem)
	}
	if problem.Hint != "retry the command." {
		t.Fatalf("error hint = %q, want retry hint", problem.Hint)
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
