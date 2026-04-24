// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package env implements a credential.Provider that resolves app credentials
// and access tokens from environment variables. It is the primary auth path
// for headless callers — remote AI agents, CI jobs, and short-lived
// containers — where interactive device flow and OS keychains are unavailable.
//
// See the Provider type for the full list of supported environment variables.
package env

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

// tatSafetyMargin is subtracted from the server-reported expiry so callers
// always see a token with enough remaining life to complete a request.
const tatSafetyMargin = 30 * time.Second

// defaultTATTimeout bounds the token-exchange call when the Provider's
// HTTPClient field is nil. A finite timeout prevents a hung TCP connection
// from indefinitely blocking concurrent callers on the minter lock.
const defaultTATTimeout = 30 * time.Second

// tatFetcherFunc performs the app_id + app_secret → tenant_access_token exchange.
// Production code uses realTATFetcher; tests substitute a stub on Provider.
type tatFetcherFunc func(ctx context.Context, hc *http.Client, brand credential.Brand, appID, appSecret string) (string, int, error)

// realTATFetcher is the production implementation wired by default.
func realTATFetcher(ctx context.Context, hc *http.Client, brand credential.Brand, appID, appSecret string) (string, int, error) {
	ep := core.ResolveEndpoints(core.LarkBrand(brand))
	res, err := auth.FetchTenantAccessToken(ctx, hc, ep.Open, appID, appSecret)
	if err != nil {
		return "", 0, err
	}
	return res.Token, res.ExpiresIn, nil
}

// defaultHTTPClient returns a timeout-protected client used when the caller
// does not supply one via Provider.HTTPClient.
func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: defaultTATTimeout}
}

// Provider resolves credentials from environment variables.
//
// Supported modes:
//
//   - Bot via app secret: LARKSUITE_CLI_APP_ID + LARKSUITE_CLI_APP_SECRET.
//     The provider exchanges these for a tenant access token on demand
//     and caches the result in-process until it expires. This is the
//     preferred mode for ephemeral, non-interactive callers (CI jobs,
//     remote agents) since no keychain or profile setup is required.
//   - Bot via pre-issued token: LARKSUITE_CLI_APP_ID + LARKSUITE_CLI_TENANT_ACCESS_TOKEN.
//   - User via pre-issued token: LARKSUITE_CLI_APP_ID + LARKSUITE_CLI_USER_ACCESS_TOKEN
//     (optionally combined with LARKSUITE_CLI_APP_SECRET for bot fallback).
type Provider struct {
	// HTTPClient is used for the tenant_access_token exchange. A nil value
	// falls back to a dedicated client with a 30-second timeout so a hung
	// exchange cannot wedge the minter lock indefinitely.
	HTTPClient *http.Client

	// fetchTAT overrides the production TAT exchange for tests. Unit tests
	// assign a stub here instead of mutating package state so concurrent
	// tests cannot interfere with each other.
	fetchTAT tatFetcherFunc

	tatMu    sync.Mutex
	tatCache tatCacheEntry
}

type tatCacheEntry struct {
	token     string
	expiresAt time.Time
	appID     string
	brand     credential.Brand
}

func (p *Provider) Name() string { return "env" }

func (p *Provider) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	appID := os.Getenv(envvars.CliAppID)
	appSecret := os.Getenv(envvars.CliAppSecret)
	hasUAT := os.Getenv(envvars.CliUserAccessToken) != ""
	hasTAT := os.Getenv(envvars.CliTenantAccessToken) != ""
	if appID == "" && appSecret == "" {
		switch {
		case hasUAT:
			return nil, &credential.BlockError{Provider: "env", Reason: envvars.CliUserAccessToken + " is set but " + envvars.CliAppID + " is missing"}
		case hasTAT:
			return nil, &credential.BlockError{Provider: "env", Reason: envvars.CliTenantAccessToken + " is set but " + envvars.CliAppID + " is missing"}
		default:
			return nil, nil
		}
	}
	if appID == "" {
		return nil, &credential.BlockError{Provider: "env", Reason: envvars.CliAppSecret + " is set but " + envvars.CliAppID + " is missing"}
	}
	if appSecret == "" && !hasUAT && !hasTAT {
		return nil, &credential.BlockError{
			Provider: "env",
			Reason:   envvars.CliAppID + " is set but no app secret or access token is available",
		}
	}
	brand := credential.Brand(os.Getenv(envvars.CliBrand))
	if brand == "" {
		brand = credential.BrandFeishu
	}
	acct := &credential.Account{AppID: appID, AppSecret: appSecret, Brand: brand}

	switch id := credential.Identity(os.Getenv(envvars.CliDefaultAs)); id {
	case "", credential.IdentityAuto:
		acct.DefaultAs = id
	case credential.IdentityUser, credential.IdentityBot:
		acct.DefaultAs = id
	default:
		return nil, &credential.BlockError{
			Provider: "env",
			Reason:   fmt.Sprintf("invalid %s %q (want user, bot, or auto)", envvars.CliDefaultAs, id),
		}
	}

	// Explicit strict mode policy takes priority
	switch strictMode := os.Getenv(envvars.CliStrictMode); strictMode {
	case "bot":
		acct.SupportedIdentities = credential.SupportsBot
	case "user":
		acct.SupportedIdentities = credential.SupportsUser
	case "off":
		acct.SupportedIdentities = credential.SupportsAll
	case "":
		// Infer from available tokens. An app_secret unlocks bot identity
		// because a tenant access token can be minted on demand.
		if hasUAT {
			acct.SupportedIdentities |= credential.SupportsUser
		}
		if hasTAT || appSecret != "" {
			acct.SupportedIdentities |= credential.SupportsBot
		}
	default:
		return nil, &credential.BlockError{
			Provider: "env",
			Reason:   fmt.Sprintf("invalid %s %q (want bot, user, or off)", envvars.CliStrictMode, strictMode),
		}
	}

	if acct.DefaultAs == "" {
		switch {
		case hasUAT:
			acct.DefaultAs = credential.IdentityUser
		case hasTAT, appSecret != "":
			acct.DefaultAs = credential.IdentityBot
		}
	}

	return acct, nil
}

func (p *Provider) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.Token, error) {
	switch req.Type {
	case credential.TokenTypeUAT:
		token := os.Getenv(envvars.CliUserAccessToken)
		if token == "" {
			return nil, nil
		}
		return &credential.Token{Value: token, Source: "env:" + envvars.CliUserAccessToken}, nil

	case credential.TokenTypeTAT:
		if token := os.Getenv(envvars.CliTenantAccessToken); token != "" {
			return &credential.Token{Value: token, Source: "env:" + envvars.CliTenantAccessToken}, nil
		}
		return p.mintTenantAccessToken(ctx, req.AppID)

	default:
		return nil, nil
	}
}

// mintTenantAccessToken exchanges app_id + app_secret for a tenant access token.
// Returns nil, nil when app_secret is absent so the credential layer surfaces a
// clean TokenUnavailableError rather than a harder-to-debug exchange failure.
//
// The lock is intentionally held across the HTTP exchange: concurrent callers
// share the same result and do not stampede the auth endpoint. The default
// HTTP client carries a 30-second timeout (see defaultTATTimeout) so a hung
// exchange cannot wedge the mutex indefinitely; callers needing a different
// budget should set Provider.HTTPClient or pass a context with a deadline.
func (p *Provider) mintTenantAccessToken(ctx context.Context, requestedAppID string) (*credential.Token, error) {
	appID := os.Getenv(envvars.CliAppID)
	appSecret := os.Getenv(envvars.CliAppSecret)
	if appID == "" || appSecret == "" {
		return nil, nil
	}
	if requestedAppID != "" && requestedAppID != appID {
		return nil, &credential.BlockError{
			Provider: "env",
			Reason:   fmt.Sprintf("requested app_id %q does not match %s=%q", requestedAppID, envvars.CliAppID, appID),
		}
	}
	brand := credential.Brand(os.Getenv(envvars.CliBrand))
	if brand == "" {
		brand = credential.BrandFeishu
	}

	p.tatMu.Lock()
	defer p.tatMu.Unlock()
	if token := p.cachedTAT(appID, brand); token != "" {
		return &credential.Token{Value: token, Source: "env:" + envvars.CliAppSecret + "(cached)"}, nil
	}

	hc := p.HTTPClient
	if hc == nil {
		hc = defaultHTTPClient()
	}
	fetch := p.fetchTAT
	if fetch == nil {
		fetch = realTATFetcher
	}
	token, expiresIn, err := fetch(ctx, hc, brand, appID, appSecret)
	if err != nil {
		return nil, fmt.Errorf("env provider: tenant_access_token exchange failed: %w", err)
	}
	if token == "" {
		return nil, fmt.Errorf("env provider: tenant_access_token exchange returned empty token")
	}
	ttl := time.Duration(expiresIn)*time.Second - tatSafetyMargin
	if ttl > 0 {
		p.tatCache = tatCacheEntry{
			token:     token,
			expiresAt: time.Now().Add(ttl),
			appID:     appID,
			brand:     brand,
		}
	}
	// If ttl <= 0 (server-reported life too short to honour the safety
	// margin, or expiresIn==0), surface the token once without caching so
	// the next call re-mints rather than returning a token that already
	// violates the margin invariant.
	return &credential.Token{Value: token, Source: "env:" + envvars.CliAppSecret}, nil
}

// cachedTAT returns a still-valid cached token for (appID, brand) or "".
// Caller must hold p.tatMu.
func (p *Provider) cachedTAT(appID string, brand credential.Brand) string {
	if p.tatCache.token == "" {
		return ""
	}
	if p.tatCache.appID != appID || p.tatCache.brand != brand {
		return ""
	}
	if time.Now().After(p.tatCache.expiresAt) {
		return ""
	}
	return p.tatCache.token
}

func init() {
	credential.Register(&Provider{})
}
