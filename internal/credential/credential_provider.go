// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
)

// DefaultAccountResolver is implemented by the default account provider.
type DefaultAccountResolver interface {
	ResolveAccount(ctx context.Context) (*Account, error)
}

// DefaultTokenResolver is implemented by the default token provider.
type DefaultTokenResolver interface {
	ResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, error)
}

var (
	getStoredToken       = auth.GetStoredToken
	getStoredTokenStatus = auth.TokenStatus
)

type credentialSource interface {
	Name() string
	TryResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, bool, error)
	ResolveIdentityHint(ctx context.Context, acct *Account) (*IdentityHint, error)
}

type extensionTokenSource struct {
	provider extcred.Provider
}

func (s extensionTokenSource) Name() string { return s.provider.Name() }

func (s extensionTokenSource) TryResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, bool, error) {
	tok, err := s.provider.ResolveToken(ctx, extcred.TokenSpec{
		Type:  extcred.TokenType(req.Type.String()),
		AppID: req.AppID,
	})
	if err != nil {
		return nil, false, err
	}
	if tok == nil {
		return nil, false, nil
	}
	if tok.Value == "" {
		return nil, false, &MalformedTokenResultError{Source: s.Name(), Type: req.Type, Reason: "empty token"}
	}
	return &TokenResult{Token: tok.Value, Scopes: tok.Scopes}, true, nil
}

func (s extensionTokenSource) ResolveIdentityHint(ctx context.Context, acct *Account) (*IdentityHint, error) {
	hint := &IdentityHint{}
	if acct == nil {
		return hint, nil
	}
	hint.DefaultAs = acct.DefaultAs
	// Extension sources verify user identity via enrichUserInfo, so a resolved
	// UserOpenId is sufficient here; no keychain-backed token status lookup is needed.
	if acct.UserOpenId != "" {
		hint.AutoAs = core.AsUser
		return hint, nil
	}
	ids := extcred.IdentitySupport(acct.SupportedIdentities)
	switch {
	case ids.UserOnly():
		hint.AutoAs = core.AsUser
	case ids.BotOnly():
		hint.AutoAs = core.AsBot
	}
	return hint, nil
}

type defaultTokenSource struct {
	resolver DefaultTokenResolver
}

func (s defaultTokenSource) Name() string { return "default" }

func (s defaultTokenSource) TryResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, bool, error) {
	if s.resolver == nil {
		return nil, false, nil
	}
	result, err := s.resolver.ResolveToken(ctx, req)
	if err != nil {
		return nil, false, err
	}
	if result == nil {
		return nil, false, &MalformedTokenResultError{Source: s.Name(), Type: req.Type, Reason: "nil token result"}
	}
	if result.Token == "" {
		return nil, false, &MalformedTokenResultError{Source: s.Name(), Type: req.Type, Reason: "empty token"}
	}
	return result, true, nil
}

func (s defaultTokenSource) ResolveIdentityHint(ctx context.Context, acct *Account) (*IdentityHint, error) {
	hint := &IdentityHint{}
	if acct == nil {
		return hint, nil
	}
	hint.DefaultAs = acct.DefaultAs
	if acct.UserOpenId == "" {
		hint.AutoAs = core.AsBot
		return hint, nil
	}
	stored := getStoredToken(acct.AppID, acct.UserOpenId)
	if stored == nil {
		hint.AutoAs = core.AsBot
		return hint, nil
	}
	if getStoredTokenStatus(stored) == "expired" {
		hint.AutoAs = core.AsBot
		return hint, nil
	}
	hint.AutoAs = core.AsUser
	return hint, nil
}

// CredentialProvider is the unified entry point for all credential resolution.
type CredentialProvider struct {
	providers    []extcred.Provider
	defaultAcct  DefaultAccountResolver
	defaultToken DefaultTokenResolver
	httpClient   func() (*http.Client, error)
	warnOut      io.Writer

	accountOnce    sync.Once
	account        *Account
	accountErr     error
	selectedSource credentialSource

	hintOnce sync.Once
	hint     *IdentityHint
	hintErr  error
}

// NewCredentialProvider creates a CredentialProvider.
func NewCredentialProvider(providers []extcred.Provider, defaultAcct DefaultAccountResolver, defaultToken DefaultTokenResolver, httpClient func() (*http.Client, error)) *CredentialProvider {
	return &CredentialProvider{
		providers:    providers,
		defaultAcct:  defaultAcct,
		defaultToken: defaultToken,
		httpClient:   httpClient,
	}
}

func (p *CredentialProvider) SetWarnOut(warnOut io.Writer) *CredentialProvider {
	p.warnOut = warnOut
	return p
}

// ResolveAccount resolves app credentials. Result is cached after first call.
// NOTE: Uses sync.Once — only the context from the first call is used for resolution.
// Subsequent calls return the cached result regardless of their context.
// This is acceptable for CLI (single invocation per process) but not for long-running servers.
func (p *CredentialProvider) ResolveAccount(ctx context.Context) (*Account, error) {
	p.accountOnce.Do(func() {
		p.account, p.accountErr = p.doResolveAccount(ctx)
	})
	return p.account, p.accountErr
}

func (p *CredentialProvider) doResolveAccount(ctx context.Context) (*Account, error) {
	for _, prov := range p.providers {
		acct, err := prov.ResolveAccount(ctx)
		if err != nil {
			return nil, err
		}
		if acct != nil {
			internal := convertAccount(acct)
			source := extensionTokenSource{provider: prov}
			if err := p.enrichUserInfo(ctx, internal, source); err != nil {
				if p.warnOut != nil {
					_, _ = fmt.Fprintf(p.warnOut, "warning: unable to verify user identity from credential source %q: %v\n", source.Name(), err)
				}
				// enrichUserInfo failure is non-fatal: SupportedIdentities
				// (used for strict mode) is already set by the provider.
				// Clear unverified user identity for safety.
				internal.UserOpenId = ""
				internal.UserName = ""
			}
			p.selectedSource = source
			return internal, nil
		}
	}
	if p.defaultAcct != nil {
		acct, err := p.defaultAcct.ResolveAccount(ctx)
		if err != nil {
			return nil, err
		}
		p.selectedSource = defaultTokenSource{resolver: p.defaultToken}
		return acct, nil
	}
	return nil, fmt.Errorf("no credential provider returned an account; run 'lark-cli config' to set up")
}

// enrichUserInfo resolves user identity when extension provides a UAT.
// If UAT is available, user_info API call is mandatory (security: verify token validity).
// If no UAT from extension, falls back to provider-supplied OpenID.
func (p *CredentialProvider) enrichUserInfo(ctx context.Context, acct *Account, source credentialSource) error {
	if p.httpClient == nil || source == nil {
		return nil
	}
	tok, found, err := source.TryResolveToken(ctx, TokenSpec{Type: TokenTypeUAT, AppID: acct.AppID})
	if err != nil {
		var blockErr *extcred.BlockError
		if errors.As(err, &blockErr) {
			return nil // provider explicitly blocks UAT; skip enrichment
		}
		return fmt.Errorf("failed to resolve UAT for user identity verification: %w", err)
	}
	if !found {
		return nil
	}
	// Have UAT — must verify and resolve identity
	hc, err := p.httpClient()
	if err != nil {
		return fmt.Errorf("failed to get HTTP client for user_info: %w", err)
	}
	info, err := fetchUserInfo(ctx, hc, acct.Brand, tok.Token)
	if err != nil {
		return fmt.Errorf("failed to verify user identity: %w", err)
	}
	acct.UserOpenId = info.OpenID
	acct.UserName = info.Name
	return nil
}

func (p *CredentialProvider) selectedCredentialSource(ctx context.Context) (credentialSource, error) {
	if p.selectedSource != nil {
		return p.selectedSource, nil
	}
	if p.defaultAcct == nil {
		return nil, nil
	}
	if _, err := p.ResolveAccount(ctx); err != nil {
		return nil, err
	}
	if p.selectedSource == nil {
		return nil, fmt.Errorf("credential provider resolved an account without selecting a token source")
	}
	return p.selectedSource, nil
}

func resolveTokenFromSource(ctx context.Context, source credentialSource, req TokenSpec) (*TokenResult, error) {
	result, found, err := source.TryResolveToken(ctx, req)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, &TokenUnavailableError{Source: source.Name(), Type: req.Type}
	}
	return result, nil
}

// ResolveIdentityHint resolves default/auto identity guidance from the selected source.
// NOTE: Uses sync.Once — only the context from the first call is used for resolution.
// This matches ResolveAccount and keeps identity decisions stable within one CLI invocation.
func (p *CredentialProvider) ResolveIdentityHint(ctx context.Context) (*IdentityHint, error) {
	p.hintOnce.Do(func() {
		p.hint, p.hintErr = p.doResolveIdentityHint(ctx)
	})
	return p.hint, p.hintErr
}

func (p *CredentialProvider) doResolveIdentityHint(ctx context.Context) (*IdentityHint, error) {
	acct, err := p.ResolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		return &IdentityHint{}, nil
	}
	source, err := p.selectedCredentialSource(ctx)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return &IdentityHint{}, nil
	}
	hint, err := source.ResolveIdentityHint(ctx, acct)
	if err != nil {
		return nil, err
	}
	if hint == nil {
		return &IdentityHint{}, nil
	}
	return hint, nil
}

// ResolveToken resolves an access token.
func (p *CredentialProvider) ResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, error) {
	source, err := p.selectedCredentialSource(ctx)
	if err != nil {
		return nil, err
	}
	if source != nil {
		return resolveTokenFromSource(ctx, source, req)
	}

	for _, prov := range p.providers {
		source := extensionTokenSource{provider: prov}
		result, found, err := source.TryResolveToken(ctx, req)
		if err != nil {
			return nil, err
		}
		if found {
			return result, nil
		}
	}
	source = defaultTokenSource{resolver: p.defaultToken}
	result, found, err := source.TryResolveToken(ctx, req)
	if err != nil {
		return nil, err
	}
	if found {
		return result, nil
	}
	return nil, &TokenUnavailableError{Type: req.Type}
}

func convertAccount(ext *extcred.Account) *Account {
	return &Account{
		AppID:               ext.AppID,
		AppSecret:           ext.AppSecret,
		Brand:               core.LarkBrand(ext.Brand),
		DefaultAs:           core.Identity(ext.DefaultAs),
		ProfileName:         ext.ProfileName,
		UserOpenId:          ext.OpenID,
		SupportedIdentities: uint8(ext.SupportedIdentities),
	}
}
