// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package env

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

const tenantAccessTokenSourceCredentialStore = "credential-store"

// Provider resolves account selection from environment variables. A configured
// TAT lookup is consulted only when the dedicated source variable selects it.
type Provider struct {
	tenantAccessTokenLookup func(context.Context, string) (*credential.Token, error)
}

// WithTenantAccessTokenLookup returns an invocation-scoped provider copy with
// the CLI-owned stored-TAT lookup. The narrow callback keeps account resolution
// independent from secret storage and avoids exposing a second full Provider.
func (p *Provider) WithTenantAccessTokenLookup(lookup func(context.Context, string) (*credential.Token, error)) credential.Provider {
	clone := *p
	clone.tenantAccessTokenLookup = lookup
	return &clone
}

func (p *Provider) Name() string { return "env" }

func (p *Provider) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	appID := os.Getenv(envvars.CliAppID)
	appSecret := os.Getenv(envvars.CliAppSecret)
	hasUAT := os.Getenv(envvars.CliUserAccessToken) != ""
	hasTAT := os.Getenv(envvars.CliTenantAccessToken) != ""
	tatSource, err := tenantAccessTokenSource()
	if err != nil {
		return nil, err
	}
	storedTATSelected := tatSource == tenantAccessTokenSourceCredentialStore
	if storedTATSelected && appID == "" {
		return nil, &credential.BlockError{
			Provider: "env",
			Reason:   envvars.CliTenantAccessTokenSource + "=credential-store requires " + envvars.CliAppID,
		}
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
	if appSecret == "" && !hasUAT && !hasTAT && !storedTATSelected {
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

	// Explicit strict mode policy takes priority.
	strictMode := os.Getenv(envvars.CliStrictMode)
	switch strictMode {
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
		if hasTAT || storedTATSelected {
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
		case strictMode == "user":
			acct.DefaultAs = credential.IdentityUser
		case strictMode == "bot":
			acct.DefaultAs = credential.IdentityBot
		case hasUAT:
			acct.DefaultAs = credential.IdentityUser
		case hasTAT || storedTATSelected:
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
		tatSource, err := tenantAccessTokenSource()
		if err != nil {
			return nil, err
		}
		if tatSource == tenantAccessTokenSourceCredentialStore {
			if req.AppID == "" || req.AppID != os.Getenv(envvars.CliAppID) {
				return nil, nil
			}
			if p.tenantAccessTokenLookup == nil {
				return nil, &credential.BlockError{
					Provider: "env",
					Reason:   "credential-store tenant access token source is unavailable in this CLI distribution",
				}
			}
			return p.tenantAccessTokenLookup(ctx, req.AppID)
		}
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

func tenantAccessTokenSource() (string, error) {
	source := strings.ToLower(strings.TrimSpace(os.Getenv(envvars.CliTenantAccessTokenSource)))
	switch source {
	case "", tenantAccessTokenSourceCredentialStore:
		return source, nil
	default:
		return "", &credential.BlockError{
			Provider: "env",
			Reason: fmt.Sprintf("invalid %s %q (want credential-store or empty)",
				envvars.CliTenantAccessTokenSource, source),
		}
	}
}

func init() {
	credential.Register(&Provider{})
}
