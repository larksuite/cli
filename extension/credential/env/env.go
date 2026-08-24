// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package env

import (
	"context"
	"fmt"
	"os"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

// Provider resolves credentials from environment variables.
type Provider struct {
	tokenFallback credential.Provider
}

// WithTokenFallback returns an invocation-scoped copy that consults fallback
// only when the requested token is absent from the environment. It is a public
// compatibility hook for built-in CLI wiring; third-party providers are not
// configured through it. The registered zero-value Provider remains
// environment-only for compatibility.
func (p *Provider) WithTokenFallback(fallback credential.Provider) credential.Provider {
	clone := *p
	clone.tokenFallback = fallback
	return &clone
}

func (p *Provider) Name() string { return "env" }

func (p *Provider) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	appID := os.Getenv(envvars.CliAppID)
	appSecret := os.Getenv(envvars.CliAppSecret)
	hasUAT := os.Getenv(envvars.CliUserAccessToken) != ""
	hasTAT := os.Getenv(envvars.CliTenantAccessToken) != ""
	if appID != "" && appSecret == "" && !hasUAT && !hasTAT {
		tok, err := p.resolveTokenFallback(ctx, credential.TokenSpec{Type: credential.TokenTypeTAT, AppID: appID})
		if err != nil {
			return nil, err
		}
		hasTAT = tok != nil && tok.Value != ""
	}
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
	brand := credential.Brand(core.ParseBrand(os.Getenv(envvars.CliBrand)))
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
		// Infer from available tokens
		if hasUAT {
			acct.SupportedIdentities |= credential.SupportsUser
		}
		if hasTAT {
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
		case hasTAT:
			acct.DefaultAs = credential.IdentityBot
		}
	}

	return acct, nil
}

func (p *Provider) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.Token, error) {
	var envKey string
	switch req.Type {
	case credential.TokenTypeUAT:
		envKey = envvars.CliUserAccessToken
	case credential.TokenTypeTAT:
		envKey = envvars.CliTenantAccessToken
	default:
		return nil, nil
	}
	token := os.Getenv(envKey)
	if token != "" {
		return &credential.Token{Value: token, Source: "env:" + envKey}, nil
	}
	return p.resolveTokenFallback(ctx, req)
}

func (p *Provider) resolveTokenFallback(ctx context.Context, req credential.TokenSpec) (*credential.Token, error) {
	if p.tokenFallback == nil || req.Type != credential.TokenTypeTAT || req.AppID == "" {
		return nil, nil
	}
	if req.AppID != os.Getenv(envvars.CliAppID) {
		return nil, nil
	}
	// Injected TATs are a fallback for APP_ID-only token contexts. Existing
	// app-secret and user-token accounts keep their pre-existing behavior and
	// must not become dependent on local injected-token storage.
	if os.Getenv(envvars.CliAppSecret) != "" || os.Getenv(envvars.CliUserAccessToken) != "" {
		return nil, nil
	}
	// Storage errors intentionally propagate so an APP_ID-only environment
	// fails closed instead of silently selecting another credential source.
	return p.tokenFallback.ResolveToken(ctx, req)
}

func init() {
	credential.Register(&Provider{})
}
