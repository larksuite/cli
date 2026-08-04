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
	// function is only reached once FetchTAT has ruled out success — a non-credential
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
	// LoadOrNotConfigured distinguishes an absent config (→ not_configured)
	// from a malformed/unreadable one (→ invalid_config with cause), so a
	// broken config is never masked as "run config init" — matching the
	// explicit-profile path in doResolveAccount.
	multi, err := core.LoadOrNotConfigured()
	if err != nil {
		return nil, err
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
	tatAppID  string
	tatErr    error
}

func NewDefaultTokenProvider(defaultAcct *DefaultAccountProvider, httpClient func() (*http.Client, error), errOut io.Writer) *DefaultTokenProvider {
	return &DefaultTokenProvider{defaultAcct: defaultAcct, httpClient: httpClient, errOut: errOut}
}

func (p *DefaultTokenProvider) ResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, error) {
	switch req.Type {
	case TokenTypeUAT:
		return p.resolveUAT(ctx, req)
	case TokenTypeTAT:
		return p.resolveTAT(ctx, req)
	default:
		return nil, fmt.Errorf("unsupported token type: %s", req.Type)
	}
}

// checkTokenAppID refuses to hand out a token for a different app than the
// caller resolved. The token provider re-reads the config, so a concurrent
// profile edit between account resolution and token resolution could otherwise
// cross tokens between apps. TokenSpec.AppID is REQUIRED here: an empty value
// would silently disable the guarantee, so it is rejected rather than skipped.
func checkTokenAppID(req TokenSpec, resolvedAppID string) error {
	if req.AppID == "" {
		return errs.NewInternalError(errs.SubtypeUnknown,
			"TokenSpec.AppID is required for %s token resolution", req.Type)
	}
	if req.AppID == resolvedAppID {
		return nil
	}
	return errs.NewInternalError(errs.SubtypeUnknown,
		"config changed during resolution: token requested for app %q but the saved profile now resolves to a different app", req.AppID).
		WithHint("retry the command.")
}

// resolveUAT resolves a user access token. Not cached (unlike TAT) because UAT
// may be refreshed between calls and GetValidAccessToken handles its own caching.
func (p *DefaultTokenProvider) resolveUAT(ctx context.Context, req TokenSpec) (*TokenResult, error) {
	acct, err := p.defaultAcct.ResolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkTokenAppID(req, acct.AppID); err != nil {
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

// resolveTAT resolves a tenant access token. The result is cached after the
// first mint via sync.Once — only the context from that call is used.
//
// The account is resolved and checked against the request BEFORE any token
// work: a mismatched request must not trigger a token mint (network call,
// quota, audit trail) for the wrong app. The cached result is additionally
// re-checked on every hit, so a token minted for one app is never served to
// a request that resolved another.
func (p *DefaultTokenProvider) resolveTAT(ctx context.Context, req TokenSpec) (*TokenResult, error) {
	acct, err := p.defaultAcct.ResolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkTokenAppID(req, acct.AppID); err != nil {
		return nil, err
	}
	p.tatOnce.Do(func() {
		p.tatResult, p.tatErr = p.doResolveTAT(ctx, acct)
		p.tatAppID = acct.AppID
	})
	if p.tatErr != nil {
		return nil, p.tatErr
	}
	if err := checkTokenAppID(req, p.tatAppID); err != nil {
		return nil, err
	}
	return p.tatResult, nil
}

func (p *DefaultTokenProvider) doResolveTAT(ctx context.Context, acct *Account) (*TokenResult, error) {
	httpClient, err := p.httpClient()
	if err != nil {
		return nil, err
	}
	token, err := FetchTAT(ctx, httpClient, acct.Brand, acct.AppID, acct.AppSecret)
	if err != nil {
		return nil, err
	}
	return &TokenResult{Token: token}, nil
}
