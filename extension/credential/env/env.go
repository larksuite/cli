// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package env

import (
	"context"
	"fmt"
	"os"

	"github.com/larksuite/cli/extension/credential"
)

// Provider resolves credentials from environment variables.
type Provider struct{}

func (p *Provider) Name() string { return "env" }

func (p *Provider) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	appID := os.Getenv("LARK_APP_ID")
	appSecret := os.Getenv("LARK_APP_SECRET")
	if appID == "" && appSecret == "" {
		return nil, nil
	}
	if appID == "" {
		return nil, &credential.BlockError{Provider: "env", Reason: "LARK_APP_SECRET is set but LARK_APP_ID is missing"}
	}
	if appSecret == "" {
		return nil, &credential.BlockError{Provider: "env", Reason: "LARK_APP_ID is set but LARK_APP_SECRET is missing"}
	}
	brand := os.Getenv("LARK_BRAND")
	if brand == "" {
		brand = "feishu"
	}
	acct := &credential.Account{AppID: appID, AppSecret: appSecret, Brand: brand}
	hasUAT := os.Getenv("LARK_USER_ACCESS_TOKEN") != ""
	hasTAT := os.Getenv("LARK_TENANT_ACCESS_TOKEN") != ""

	switch defaultAs := os.Getenv("LARKSUITE_CLI_DEFAULT_AS"); defaultAs {
	case "", credential.IdentityAuto:
		acct.DefaultAs = defaultAs
	case credential.IdentityUser, credential.IdentityBot:
		acct.DefaultAs = defaultAs
	default:
		return nil, &credential.BlockError{
			Provider: "env",
			Reason:   fmt.Sprintf("invalid LARKSUITE_CLI_DEFAULT_AS %q (want user, bot, or auto)", defaultAs),
		}
	}

	// Explicit strict mode policy takes priority
	switch strictMode := os.Getenv("LARKSUITE_CLI_STRICT_MODE"); strictMode {
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
			Reason:   fmt.Sprintf("invalid LARKSUITE_CLI_STRICT_MODE %q (want bot, user, or off)", strictMode),
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
		envKey = "LARK_USER_ACCESS_TOKEN"
	case credential.TokenTypeTAT:
		envKey = "LARK_TENANT_ACCESS_TOKEN"
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
