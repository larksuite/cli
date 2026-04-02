// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"bytes"
	"context"
	"encoding/json"
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
	keychain keychain.KeychainAccess
	profile  func() string
}

func NewDefaultAccountProvider(kc keychain.KeychainAccess, profile func() string) *DefaultAccountProvider {
	return &DefaultAccountProvider{keychain: kc, profile: profile}
}

func (p *DefaultAccountProvider) ResolveAccount(ctx context.Context) (*Account, error) {
	cfg, err := core.RequireConfigForProfile(p.keychain, p.profile())
	if err != nil {
		return nil, err
	}
	cfg.SupportedIdentities = configStrictModeToIdentitySupport(p.profile())
	return cfg, nil
}

// configStrictModeToIdentitySupport reads strictMode from config file
// and maps it to the SupportedIdentities bitflag.
func configStrictModeToIdentitySupport(profileOverride string) uint8 {
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		return 0
	}
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

func (p *DefaultTokenProvider) resolveUAT(ctx context.Context) (*TokenResult, error) {
	acct, err := p.defaultAcct.ResolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	httpClient, err := p.httpClient()
	if err != nil {
		return nil, err
	}
	token, err := auth.GetValidAccessToken(httpClient, auth.NewUATCallOptions(acct, p.errOut))
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
	url := ep.Open + "/open-apis/auth/v3/tenant_access_token/internal"

	body, _ := json.Marshal(map[string]string{
		"app_id":     acct.AppID,
		"app_secret": acct.AppSecret,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TAT API returned HTTP %d", resp.StatusCode)
	}

	var result struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse TAT response: %w", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("TAT API error: [%d] %s", result.Code, result.Msg)
	}
	return &TokenResult{Token: result.TenantAccessToken}, nil
}
