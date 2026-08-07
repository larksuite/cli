// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package env

import (
	"context"
	"fmt"
	"os"

	"github.com/larksuite/cli/envnames"
	"github.com/larksuite/cli/extension/credential"
)

// Provider resolves credentials from environment variables.
type Provider struct{}

func (p *Provider) Name() string { return "env" }

func (p *Provider) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	appID := os.Getenv(envnames.CliAppID)
	appSecret := os.Getenv(envnames.CliAppSecret)
	hasUAT := os.Getenv(envnames.CliUserAccessToken) != ""
	hasTAT := os.Getenv(envnames.CliTenantAccessToken) != ""
	if appID == "" && appSecret == "" {
		switch {
		case hasUAT:
			return nil, &credential.BlockError{Provider: "env", Reason: envnames.CliUserAccessToken + " is set but " + envnames.CliAppID + " is missing"}
		case hasTAT:
			return nil, &credential.BlockError{Provider: "env", Reason: envnames.CliTenantAccessToken + " is set but " + envnames.CliAppID + " is missing"}
		default:
			return nil, nil
		}
	}
	if appID == "" {
		return nil, &credential.BlockError{Provider: "env", Reason: envnames.CliAppSecret + " is set but " + envnames.CliAppID + " is missing"}
	}
	if appSecret == "" && !hasUAT && !hasTAT {
		return nil, &credential.BlockError{
			Provider: "env",
			Reason:   envnames.CliAppID + " is set but no app secret or access token is available",
		}
	}
	brand := credential.ParseBrand(os.Getenv(envnames.CliBrand))
	acct := &credential.Account{AppID: appID, AppSecret: appSecret, Brand: brand}

	switch id := credential.Identity(os.Getenv(envnames.CliDefaultAs)); id {
	case "", credential.IdentityAuto:
		acct.DefaultAs = id
	case credential.IdentityUser, credential.IdentityBot:
		acct.DefaultAs = id
	default:
		return nil, &credential.BlockError{
			Provider: "env",
			Reason:   fmt.Sprintf("invalid %s %q (want user, bot, or auto)", envnames.CliDefaultAs, id),
		}
	}

	// Explicit strict mode policy takes priority
	switch strictMode := os.Getenv(envnames.CliStrictMode); strictMode {
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
			Reason:   fmt.Sprintf("invalid %s %q (want bot, user, or off)", envnames.CliStrictMode, strictMode),
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
		envKey = envnames.CliUserAccessToken
	case credential.TokenTypeTAT:
		envKey = envnames.CliTenantAccessToken
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
