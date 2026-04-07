// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package env

import (
	"context"
	"fmt"
	"os"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/envvars"
)

// Provider resolves credentials from environment variables.
type Provider struct{}

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
	if token == "" {
		return nil, nil
	}
	return &credential.Token{Value: token, Source: "env:" + envKey}, nil
}

func init() {
	credential.Register(&Provider{})
}
