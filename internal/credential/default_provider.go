// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/keychain"

	extcred "github.com/larksuite/cli/extension/credential"
)

// classifyTATResponseCode wraps a non-zero TAT endpoint response code into the
// canonical typed error. 10003 (bad/missing app_id) is overridden locally
// because in other Lark endpoints the same code means permission denied;
// 10014 (invalid app_secret) is handled via the shared codemeta table.
func classifyTATResponseCode(code int, msg, brand, appID string) error {
	if code == 10003 {
		return errs.NewConfigError(errs.SubtypeInvalidClient, "%s", msg).
			WithCode(code).
			WithHint("%s", errclass.ConfigHint(errs.SubtypeInvalidClient))
	}
	return errclass.BuildAPIError(map[string]any{
		"code": code,
		"msg":  msg,
	}, errclass.ClassifyContext{
		Brand: brand,
		AppID: appID,
	})
}

// DefaultAccountProvider resolves account from config.json via keychain.
type DefaultAccountProvider struct {
	keychain     func() keychain.KeychainAccess
	profile      string
	userOverride string
	userSource   string // "flag", "env", or "" — drives error-hint copy on miss
}

func NewDefaultAccountProvider(kc func() keychain.KeychainAccess, profile, userOverride, userSource string) *DefaultAccountProvider {
	if kc == nil {
		kc = keychain.Default
	}
	return &DefaultAccountProvider{
		keychain:     kc,
		profile:      profile,
		userOverride: userOverride,
		userSource:   userSource,
	}
}

func (p *DefaultAccountProvider) ResolveAccount(ctx context.Context) (*Account, error) {
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		return nil, core.PassThroughOrNotConfigured(err)
	}

	cfg, err := core.ResolveConfigFromMulti(multi, p.keychain(), p.profile, p.userOverride)
	if err != nil {
		// Source-tag the user-resolution error here; the resolver layer is env-agnostic.
		return nil, decorateUserResolutionError(err, p.userSource)
	}
	cfg.SupportedIdentities = strictModeToIdentitySupport(multi, p.profile)
	return AccountFromCliConfig(cfg), nil
}

// decorateUserResolutionError appends a source-aware remediation suffix to a
// user-rung *core.ConfigError. Pass-through on any non-ConfigError, empty
// source, or non-user rung (which has its own remediation).
//
// Gating is structural via core.ConfigError.Rung — substring-matching the
// Message was fragile: it false-matched a profile-rung error that
// happened to contain "user" (e.g. "available users in this profile: ..."
// rendered into a profile-resolution failure if the copy ever drifted),
// and silently dropped the env-source hint on legitimately user-rung
// errors when the wording changed in any direction.
func decorateUserResolutionError(err error, source string) error {
	if err == nil || source == "" {
		return err
	}
	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		return err
	}
	if cfgErr.Rung != core.RungUser {
		return err
	}
	switch source {
	case "env":
		cfgErr.Hint = cfgErr.Hint + "; this value came from " + envvars.CliOpenID + " — unset it or pass --user explicitly to override"
	case "flag":
		// --user already named in the resolver's hint copy; no-op.
	}
	return cfgErr
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

// DefaultTokenProvider resolves UAT/TAT using keychain + direct HTTP calls
// (no SDK/LarkClient dep — avoids a circular dependency with Factory).
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
		return p.resolveTAT(ctx)
	default:
		return nil, fmt.Errorf("unsupported token type: %s", req.Type)
	}
}

// resolveUAT resolves a user access token. Not cached — GetValidAccessToken
// handles its own refresh/caching.
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

// resolveTAT resolves a tenant access token, cached after the first call via
// sync.Once — only the first caller's context is used.
func (p *DefaultTokenProvider) resolveTAT(ctx context.Context) (*TokenResult, error) {
	p.tatOnce.Do(func() {
		p.tatResult, p.tatErr = p.doResolveTAT(ctx)
	})
	return p.tatResult, p.tatErr
}

func (p *DefaultTokenProvider) doResolveTAT(ctx context.Context) (*TokenResult, error) {
	acct, err := p.defaultAcct.ResolveAccount(ctx)
	if err != nil {
		return nil, err
	}
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
