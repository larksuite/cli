// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

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
		return errs.NewConfigError(errs.SubtypeInvalidClient, "%s", msg).
			WithCode(code).
			WithHint("%s", errclass.ConfigHint(errs.SubtypeInvalidClient))
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
	// function is only reached once fetchTAT has ruled out success — a non-credential
	// OAuth error (e.g. invalid_scope) can arrive with code 0 and is still a
	// deterministic rejection. Back it with a typed APIError so callers never receive
	// the ("", nil) "empty token, no error" pair.
	return errs.NewAPIError(errs.SubtypeUnknown, "%s", msg).WithCode(code)
}

// DefaultAccountProvider resolves account from config.json via keychain.
type DefaultAccountProvider struct {
	keychain func() keychain.KeychainAccess
	profile  string
}

func NewDefaultAccountProvider(kc func() keychain.KeychainAccess, profile string) *DefaultAccountProvider {
	if kc == nil {
		kc = keychain.Default
	}
	return &DefaultAccountProvider{keychain: kc, profile: profile}
}

func (p *DefaultAccountProvider) ResolveAccount(ctx context.Context) (*Account, error) {
	// Load config once — used for both credentials and strict mode.
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		return nil, core.NotConfiguredError()
	}

	cfg, err := core.ResolveConfigFromMulti(multi, p.keychain(), p.profile)
	if err != nil {
		return nil, err
	}
	cfg.SupportedIdentities = strictModeToIdentitySupport(multi, p.profile)
	return AccountFromCliConfig(cfg), nil
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

	tatOnce   sync.Once
	tatResult *TokenResult
	tatErr    error
}

func NewDefaultTokenProvider(defaultAcct *DefaultAccountProvider, httpClient func() (*http.Client, error), errOut io.Writer) *DefaultTokenProvider {
	return &DefaultTokenProvider{defaultAcct: defaultAcct, httpClient: httpClient, errOut: errOut}
}

func (p *DefaultTokenProvider) ResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, error) {
	switch req.Type {
	case TokenTypeUAT:
		return p.resolveUAT(ctx)
	case TokenTypeTAT:
		return p.resolveTAT(ctx, req.Scopes)
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
	token, err := auth.GetValidAccessToken(httpClient, auth.NewUATCallOptions(acct.ToCliConfig(), p.errOut))
	if err != nil {
		return nil, err
	}
	stored := auth.GetStoredToken(acct.AppID, acct.UserOpenId)
	scopes := ""
	if stored != nil {
		scopes = stored.Scope
	}
	return &TokenResult{Token: token, Scopes: scopes}, nil
}

// resolveTAT resolves a tenant access token. The default (unscoped) result is
// cached after first call via sync.Once — only the context from the first call
// is used. A scoped request (scopes != "") always mints fresh and is NOT cached:
// it is the rare missing-scope auto-retry path, and the caller (APIClient) holds
// its own cache for the resulting full-scope token.
func (p *DefaultTokenProvider) resolveTAT(ctx context.Context, scopes string) (*TokenResult, error) {
	if scopes != "" {
		return p.doResolveTAT(ctx, scopes)
	}
	p.tatOnce.Do(func() {
		p.tatResult, p.tatErr = p.doResolveTAT(ctx, "")
	})
	return p.tatResult, p.tatErr
}

func (p *DefaultTokenProvider) doResolveTAT(ctx context.Context, scopes string) (*TokenResult, error) {
	acct, err := p.defaultAcct.ResolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	httpClient, err := p.httpClient()
	if err != nil {
		return nil, err
	}
	token, err := fetchTAT(ctx, httpClient, acct.Brand, acct.AppID, acct.AppSecret, scopes)
	if err != nil {
		return nil, err
	}
	return &TokenResult{Token: token, Scopes: scopes}, nil
}
