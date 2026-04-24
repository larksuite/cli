// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"

	extcred "github.com/larksuite/cli/extension/credential"
)

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
		return nil, &core.ConfigError{Code: 2, Type: "config", Message: "not configured", Hint: "run `lark-cli config init --new` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete setup."}
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

// resolveTAT resolves a tenant access token. Result is cached after first call.
// NOTE: Uses sync.Once — only the context from the first call is used.
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
	ep := core.ResolveEndpoints(acct.Brand)
	res, err := auth.FetchTenantAccessToken(ctx, httpClient, ep.Open, acct.AppID, acct.AppSecret)
	if err != nil {
		return nil, fmt.Errorf("fetch tenant access token: %w", err)
	}
	return &TokenResult{Token: res.Token}, nil
}
