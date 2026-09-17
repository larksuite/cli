// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package env resolves credentials handed to the CLI through the environment,
// for callers that already hold a token and do not want the CLI to manage one.
//
// The app is named by LARKSUITE_CLI_APP_ID, with LARKSUITE_CLI_BRAND and
// LARKSUITE_CLI_APP_SECRET alongside it. Exactly one identity may be injected:
//
//	LARKSUITE_CLI_USER_ACCESS_TOKEN    a human's token; the profile reads as uat
//	LARKSUITE_CLI_TENANT_ACCESS_TOKEN  an app's token; the profile reads as tat
//	LARKSUITE_CLI_AGENT_ACCESS_TOKEN   an Agent Employee's token; reads as aat
//
// Setting the Agent variable together with either of the others is rejected:
// the two would disagree about who the profile is, on every command.
//
// An injected Agent token is provider-managed. The CLI does not learn its
// principal or its expiry, does not cache or renew it, and never writes it
// anywhere — whoever exports it owns replacing it before it expires. Because
// the principal is unknown, an Agent Employee's local scope pre-check
// (LARKSUITE_CLI_AGENT_SCOPE_PRECHECK, off by default) has nothing to check
// against; the open platform enforces scopes either way.
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
	hasAAT := os.Getenv(envvars.CliAgentAccessToken) != ""
	tatSource, err := tenantAccessTokenSource()
	if err != nil {
		return nil, err
	}
	// An injected Agent token decides the whole profile's identity, so it
	// cannot coexist with a user or tenant token: ResolveAccount would classify
	// the account from one variable while ResolveAccess hands back the other,
	// and the contract check would report that disagreement as an internal
	// error on every command. Name the conflict here, where it is still a
	// configuration mistake with an obvious fix.
	if hasAAT && (hasUAT || hasTAT) {
		conflicting := envvars.CliUserAccessToken
		if hasTAT {
			conflicting = envvars.CliTenantAccessToken
		}
		return nil, &credential.BlockError{
			Provider: "env",
			Reason: envvars.CliAgentAccessToken + " cannot be combined with " + conflicting +
				"; unset whichever identity this run should not use",
		}
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
		case hasAAT:
			return nil, &credential.BlockError{Provider: "env", Reason: envvars.CliAgentAccessToken + " is set but " + envvars.CliAppID + " is missing"}
		default:
			return nil, nil
		}
	}
	if appID == "" {
		return nil, &credential.BlockError{Provider: "env", Reason: envvars.CliAppSecret + " is set but " + envvars.CliAppID + " is missing"}
	}
	if appSecret == "" && !hasUAT && !hasTAT && !hasAAT && !storedTATSelected {
		return nil, &credential.BlockError{
			Provider: "env",
			Reason:   envvars.CliAppID + " is set but no app secret or access token is available",
		}
	}
	brand := credential.Brand(core.ParseBrand(os.Getenv(envvars.CliBrand)))
	acct := &credential.Account{AppID: appID, AppSecret: appSecret, Brand: brand}
	switch {
	case hasUAT:
		acct.CredentialKind = credential.CredentialUAT
	case hasTAT:
		acct.CredentialKind = credential.CredentialTAT
	case hasAAT:
		acct.CredentialKind = credential.CredentialAAT
	}

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
		if hasAAT {
			return nil, &credential.BlockError{
				Provider: "env",
				Reason: envvars.CliStrictMode + "=bot cannot be used with " + envvars.CliAgentAccessToken +
					": an Agent Employee has no tenant lane, so no command would be available. Set it to user or off",
			}
		}
		acct.SupportedIdentities = credential.SupportsBot
	case "user":
		acct.SupportedIdentities = credential.SupportsUser
	case "off":
		acct.SupportedIdentities = credential.SupportsAll
	case "":
		// Infer from available tokens
		if hasUAT || hasAAT {
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
		case hasUAT || hasAAT:
			acct.DefaultAs = credential.IdentityUser
		case hasTAT || storedTATSelected:
			acct.DefaultAs = credential.IdentityBot
		}
	}

	return acct, nil
}

// ResolveAccess forwards the injected Agent Employee credential; user and tenant
// tokens keep travelling ResolveToken. Only Kind is stated: the provider has not
// verified whose token this is, so OpenID, ExpiresAt and Scopes stay empty.
func (p *Provider) ResolveAccess(ctx context.Context, req credential.AccessSpec) (*credential.Access, error) {
	token := os.Getenv(envvars.CliAgentAccessToken)
	if token == "" {
		return nil, nil
	}
	if req.AppID != "" && req.AppID != os.Getenv(envvars.CliAppID) {
		return nil, nil
	}
	return &credential.Access{Value: token, Kind: credential.CredentialAAT}, nil
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
