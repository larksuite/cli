// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/keychain"

	extcred "github.com/larksuite/cli/extension/credential"
)

// classifyTATResponseCode wraps a deterministic (non-transient) failure from the
// unified Token Endpoint into the canonical typed errs.* error. The v3 endpoint
// reports failures using the OAuth 2.0 model — an `error` string plus an
// optional numeric `code` — instead of the legacy `{code, msg}` shape.
//
// invalid_client / unauthorized_client mean the configured app_id/app_secret
// cannot mint a token; from the user's perspective that is the same actionable
// CategoryConfig/InvalidClient failure the legacy 10003/10014 codes produced.
// Every other deterministic error falls through to BuildAPIError, which still
// yields a typed error so probe callers (errs.IsTyped) surface it rather than
// swallowing it. Transient/server-side failures (5xx / server_error) are
// filtered out by FetchTAT before this is called, so they stay untyped.
func classifyTATResponseCode(code int, oauthErr, errDesc, brand, appID string) error {
	msg := errDesc
	if msg == "" {
		msg = oauthErr
	}
	switch oauthErr {
	case "invalid_client", "unauthorized_client":
		typed := errs.NewConfigError(errs.SubtypeInvalidClient, "%s", msg).
			WithCode(code).
			WithHint("%s", errclass.ConfigHint(errs.SubtypeInvalidClient))
		return errclass.AnnotateConfigRecovery(typed, errs.SubtypeInvalidClient)
	}
	if err := errclass.BuildAPIError(map[string]any{
		"code": code,
		"msg":  msg,
	}, errclass.ClassifyContext{
		Brand: brand,
		AppID: appID,
	}); err != nil {
		return err
	}
	// BuildAPIError returns nil for code 0 (Feishu's success convention), but this
	// function is only reached once FetchTAT has ruled out success — a non-credential
	// OAuth error (e.g. invalid_scope) can arrive with code 0 and is still a
	// deterministic rejection. Back it with a typed APIError so callers never receive
	// the ("", nil) "empty token, no error" pair.
	return errs.NewAPIError(errs.SubtypeUnknown, "%s", msg).WithCode(code)
}

// DefaultAccountProvider resolves account from config.json via keychain.
type DefaultAccountProvider struct {
	keychain      func() keychain.KeychainAccess
	profile       string
	profileSource core.ProfileSource
}

func NewDefaultAccountProvider(kc func() keychain.KeychainAccess, profile string, source core.ProfileSource) *DefaultAccountProvider {
	if kc == nil {
		kc = keychain.Default
	}
	return &DefaultAccountProvider{keychain: kc, profile: profile, profileSource: source}
}

func (p *DefaultAccountProvider) ResolveAccount(ctx context.Context) (*Account, error) {
	// Load config once — used for both credentials and strict mode.
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		return nil, core.NotConfiguredError()
	}

	cfg, err := core.ResolveConfigFromMulti(multi, p.keychain(), p.profile, p.profileSource)
	if err != nil {
		return nil, err
	}
	cfg.SupportedIdentities = strictModeToIdentitySupport(multi, p.profile)
	return AccountFromCliConfig(cfg), nil
}

// ResolveLocalDPoPMode reads the selected profile's non-secret local issuance
// policy. Credential source checks decide whether the policy applies.
func (p *DefaultAccountProvider) ResolveLocalDPoPMode(appID string) core.DPoPMode {
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		return core.DPoPModePreferred
	}
	app := multi.CurrentAppConfig(p.profile)
	if app == nil || app.AppId != appID {
		return core.DPoPModePreferred
	}
	mode, err := app.EffectiveDPoPMode()
	if err != nil {
		return core.DPoPModePreferred
	}
	return mode
}

// strictModeToIdentitySupport maps the config-level strict mode to
// the SupportedIdentities bitflag using an already-loaded MultiAppConfig.
func strictModeToIdentitySupport(multi *core.MultiAppConfig, profileOverride string) uint8 {
	app := multi.CurrentAppConfig(profileOverride)
	var mode core.StrictMode
	if app != nil && app.StrictMode != nil {
		mode = *app.StrictMode
	} else {
		mode = multi.StrictMode
	}
	switch mode {
	case core.StrictModeBot:
		return uint8(extcred.SupportsBot)
	case core.StrictModeUser:
		return uint8(extcred.SupportsUser)
	default:
		return 0
	}
}

// DefaultTokenProvider resolves UAT/TAT using keychain + direct HTTP calls.
// No SDK/LarkClient dependency — eliminates circular dependency with Factory.
type DefaultTokenProvider struct {
	defaultAcct *DefaultAccountProvider
	httpClient  func() (*http.Client, error)
	errOut      io.Writer

	tatMu        sync.Mutex
	tatResult    *TokenResult
	tatRefreshAt time.Time
	tatExpiresAt time.Time
	tatRefresh   *tatRefreshCall
	timeNow      func() time.Time
}

type tatRefreshCall struct {
	done   chan struct{}
	result *TokenResult
	err    error
}

const tatMaxRefreshAhead = 5 * time.Minute

func NewDefaultTokenProvider(defaultAcct *DefaultAccountProvider, httpClient func() (*http.Client, error), errOut io.Writer) *DefaultTokenProvider {
	return &DefaultTokenProvider{
		defaultAcct: defaultAcct,
		httpClient:  httpClient,
		errOut:      errOut,
		timeNow:     time.Now,
	}
}

func (p *DefaultTokenProvider) ResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, error) {
	switch req.Type {
	case TokenTypeUAT:
		return p.resolveUAT(ctx)
	case TokenTypeTAT:
		return p.resolveTAT(ctx)
	default:
		return nil, fmt.Errorf("unsupported token type: %s", req.Type)
	}
}

// resolveUAT resolves a user access token. Not cached (unlike TAT) because UAT
// may be refreshed between calls and GetValidAccessToken handles its own caching.
func (p *DefaultTokenProvider) resolveUAT(ctx context.Context) (*TokenResult, error) {
	acct, err := p.defaultAcct.ResolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	httpClient, err := p.httpClient()
	if err != nil {
		return nil, err
	}
	resolved, err := auth.GetValidAccessToken(ctx, httpClient, auth.NewUATCallOptions(acct.ToCliConfig(), p.errOut))
	if err != nil {
		return nil, err
	}
	stored, _ := auth.GetStoredToken(acct.AppID, acct.UserOpenId)
	scopes := ""
	if stored != nil {
		scopes = stored.Scope
	}
	return &TokenResult{Token: resolved.AccessToken, Scopes: scopes, DPoP: resolved.DPoP}, nil
}

// resolveTAT caches minted tenant tokens in memory and renews them before their
// server-provided expiry. A process shares one in-flight mint while independent
// waiters retain their own cancellation.
func (p *DefaultTokenProvider) resolveTAT(ctx context.Context) (*TokenResult, error) {
	p.tatMu.Lock()
	now := p.currentTimeForToken(p.tatResult)
	if p.tatResult != nil && now.Before(p.tatRefreshAt) && now.Before(p.tatExpiresAt) {
		result := cloneTokenResult(p.tatResult)
		p.tatMu.Unlock()
		return result, nil
	}
	if call := p.tatRefresh; call != nil {
		p.tatMu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-call.done:
			return cloneTokenResult(call.result), call.err
		}
	}
	call := &tatRefreshCall{done: make(chan struct{})}
	p.tatRefresh = call
	p.tatMu.Unlock()

	result, lifetime, err := p.doResolveTAT(ctx)
	p.tatMu.Lock()
	if err == nil {
		now = p.currentTimeForToken(result)
		refreshAhead := tatMaxRefreshAhead
		if proportional := lifetime / 10; proportional < refreshAhead {
			refreshAhead = proportional
		}
		p.tatResult = cloneTokenResult(result)
		p.tatExpiresAt = now.Add(lifetime)
		p.tatRefreshAt = p.tatExpiresAt.Add(-refreshAhead)
	}
	call.result = cloneTokenResult(result)
	call.err = err
	p.tatRefresh = nil
	close(call.done)
	p.tatMu.Unlock()
	return result, err
}

func (p *DefaultTokenProvider) currentTime() time.Time {
	if p.timeNow == nil {
		return time.Now()
	}
	return p.timeNow()
}

func (p *DefaultTokenProvider) currentTimeForToken(result *TokenResult) time.Time {
	if result != nil && result.DPoP != nil && result.DPoP.Key() != nil {
		return result.DPoP.Key().Clock().Now()
	}
	return p.currentTime()
}

func cloneTokenResult(result *TokenResult) *TokenResult {
	if result == nil {
		return nil
	}
	cloned := *result
	return &cloned
}

func (p *DefaultTokenProvider) doResolveTAT(ctx context.Context) (*TokenResult, time.Duration, error) {
	acct, err := p.defaultAcct.ResolveAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	httpClient, err := p.httpClient()
	if err != nil {
		return nil, 0, err
	}
	token, err := FetchTAT(ctx, httpClient, acct.Brand, acct.AppID, acct.AppSecret, acct.DPoPMode)
	if err != nil {
		return nil, 0, err
	}
	lifetime := time.Duration(token.ExpiresIn) * time.Second
	if lifetime <= 0 {
		return nil, 0, fmt.Errorf("TAT response has invalid expires_in %d", token.ExpiresIn)
	}
	return &TokenResult{Token: token.AccessToken, DPoP: token.DPoP}, lifetime, nil
}
