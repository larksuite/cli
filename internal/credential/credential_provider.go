// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	extcred "github.com/larksuite/cli/extension/credential"
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

// CredentialProvider is the unified entry point for all credential resolution.
type CredentialProvider struct {
	providers    []extcred.Provider
	defaultAcct  DefaultAccountResolver
	defaultToken DefaultTokenResolver
	httpClient   func() (*http.Client, error)

	accountOnce sync.Once
	account     *Account
	accountErr  error
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
			if err := p.enrichUserInfo(ctx, internal); err != nil {
				return nil, err
			}
			return internal, nil
		}
	}
	if p.defaultAcct != nil {
		return p.defaultAcct.ResolveAccount(ctx)
	}
	return nil, fmt.Errorf("no credential provider returned an account; run 'lark-cli config' to set up")
}

// enrichUserInfo resolves user identity when extension provides a UAT.
// If UAT is available, user_info API call is mandatory (security: verify token validity).
// If no UAT from extension, falls back to provider-supplied OpenID.
func (p *CredentialProvider) enrichUserInfo(ctx context.Context, acct *Account) error {
	if p.httpClient == nil {
		return nil
	}
	for _, prov := range p.providers {
		tok, err := prov.ResolveToken(ctx, extcred.TokenSpec{Type: extcred.TokenTypeUAT})
		if err != nil {
			var blockErr *extcred.BlockError
			if errors.As(err, &blockErr) {
				return nil // provider explicitly blocks UAT; skip enrichment
			}
			continue
		}
		if tok == nil {
			continue
		}
		// Have UAT — must verify and resolve identity
		hc, err := p.httpClient()
		if err != nil {
			return fmt.Errorf("failed to get HTTP client for user_info: %w", err)
		}
		info, err := fetchUserInfo(ctx, hc, acct.Brand, tok.Value)
		if err != nil {
			return fmt.Errorf("failed to verify user identity: %w", err)
		}
		acct.UserOpenId = info.OpenID
		acct.UserName = info.Name
		return nil
	}
	return nil
}

// ResolveToken resolves an access token.
func (p *CredentialProvider) ResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, error) {
	for _, prov := range p.providers {
		tok, err := prov.ResolveToken(ctx, extcred.TokenSpec{
			Type:  extcred.TokenType(req.Type.String()),
			AppID: req.AppID,
		})
		if err != nil {
			return nil, err
		}
		if tok != nil {
			return &TokenResult{Token: tok.Value, Scopes: tok.Scopes}, nil
		}
	}
	if p.defaultToken != nil {
		return p.defaultToken.ResolveToken(ctx, req)
	}
	return nil, fmt.Errorf("no credential provider returned a token for %s", req.Type)
}

func convertAccount(ext *extcred.Account) *Account {
	return &Account{
		AppID:               ext.AppID,
		AppSecret:           ext.AppSecret,
		Brand:               core.LarkBrand(ext.Brand),
		DefaultAs:           ext.DefaultAs,
		ProfileName:         ext.ProfileName,
		UserOpenId:          ext.OpenID,
		SupportedIdentities: uint8(ext.SupportedIdentities),
	}
}
