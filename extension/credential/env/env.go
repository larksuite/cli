// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package env

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/vfs/localfileio"
)

const (
	maxCredentialFileBytes                 = 64 * 1024
	tenantAccessTokenSourceCredentialStore = "credential-store"
)

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
	appID, _, err := readCredentialValue(envvars.CliAppID, envvars.CliAppIDFile)
	if err != nil {
		return nil, err
	}
	appSecret := os.Getenv(envvars.CliAppSecret)
	tatSource, err := tenantAccessTokenSource()
	if err != nil {
		return nil, err
	}
	storedTATSelected := tatSource == tenantAccessTokenSourceCredentialStore
	if storedTATSelected && appID == "" {
		return nil, &credential.BlockError{
			Provider: "env",
			Reason:   envvars.CliTenantAccessTokenSource + "=credential-store requires " + tokenName(envvars.CliAppID, envvars.CliAppIDFile),
		}
	}
	uat, _, err := readCredentialValue(envvars.CliUserAccessToken, envvars.CliUserAccessTokenFile)
	if err != nil {
		return nil, err
	}
	hasUAT := uat != ""
	hasTAT := storedTATSelected
	if !storedTATSelected {
		tat, _, err := readCredentialValue(envvars.CliTenantAccessToken, envvars.CliTenantAccessTokenFile)
		if err != nil {
			return nil, err
		}
		hasTAT = tat != ""
	}
	if appID == "" && appSecret == "" {
		switch {
		case hasUAT:
			return nil, &credential.BlockError{Provider: "env", Reason: tokenName(envvars.CliUserAccessToken, envvars.CliUserAccessTokenFile) + " is set but " + tokenName(envvars.CliAppID, envvars.CliAppIDFile) + " is missing"}
		case hasTAT:
			return nil, &credential.BlockError{Provider: "env", Reason: tokenName(envvars.CliTenantAccessToken, envvars.CliTenantAccessTokenFile) + " is set but " + tokenName(envvars.CliAppID, envvars.CliAppIDFile) + " is missing"}
		default:
			return nil, nil
		}
	}
	if appID == "" {
		return nil, &credential.BlockError{Provider: "env", Reason: envvars.CliAppSecret + " is set but " + tokenName(envvars.CliAppID, envvars.CliAppIDFile) + " is missing"}
	}
	if appSecret == "" && !hasUAT && !hasTAT {
		return nil, &credential.BlockError{
			Provider: "env",
			Reason:   tokenName(envvars.CliAppID, envvars.CliAppIDFile) + " is set but no app secret or access token is available",
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
		case strictMode == "user":
			acct.DefaultAs = credential.IdentityUser
		case strictMode == "bot":
			acct.DefaultAs = credential.IdentityBot
		case hasUAT:
			acct.DefaultAs = credential.IdentityUser
		case hasTAT:
			acct.DefaultAs = credential.IdentityBot
		}
	}

	return acct, nil
}

func (p *Provider) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.Token, error) {
	var envKey, fileEnvKey string
	switch req.Type {
	case credential.TokenTypeUAT:
		envKey = envvars.CliUserAccessToken
		fileEnvKey = envvars.CliUserAccessTokenFile
	case credential.TokenTypeTAT:
		tatSource, err := tenantAccessTokenSource()
		if err != nil {
			return nil, err
		}
		if tatSource == tenantAccessTokenSourceCredentialStore {
			if req.AppID == "" {
				return nil, nil
			}
			appID, _, err := readCredentialValue(envvars.CliAppID, envvars.CliAppIDFile)
			if err != nil {
				return nil, err
			}
			if req.AppID != appID {
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
		fileEnvKey = envvars.CliTenantAccessTokenFile
	default:
		return nil, nil
	}
	token, source, err := readCredentialValue(envKey, fileEnvKey)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, nil
	}
	return &credential.Token{Value: token, Source: source}, nil
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

func readCredentialValue(envKey, fileEnvKey string) (string, string, error) {
	envValue := os.Getenv(envKey)
	filePath := os.Getenv(fileEnvKey)
	if envValue != "" && filePath != "" {
		return "", "", &credential.BlockError{Provider: "env", Reason: "set only one of " + envKey + " or " + fileEnvKey}
	}
	if envValue != "" {
		return envValue, "env:" + envKey, nil
	}
	if filePath == "" {
		return "", "", nil
	}

	file, err := localfileio.OpenEnvFile(filePath, fileEnvKey)
	if err != nil {
		return "", "", &credential.BlockError{Provider: "env", Reason: err.Error()}
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", "", &credential.BlockError{Provider: "env", Reason: "cannot stat " + fileEnvKey + " file"}
	}
	if info.Size() > maxCredentialFileBytes {
		return "", "", &credential.BlockError{Provider: "env", Reason: fileEnvKey + " file is too large"}
	}
	data, err := io.ReadAll(io.LimitReader(file, maxCredentialFileBytes+1))
	if err != nil {
		return "", "", &credential.BlockError{Provider: "env", Reason: "cannot read " + fileEnvKey + " file"}
	}
	if len(data) > maxCredentialFileBytes {
		return "", "", &credential.BlockError{Provider: "env", Reason: fileEnvKey + " file is too large"}
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", "", &credential.BlockError{Provider: "env", Reason: fileEnvKey + " file is empty"}
	}
	if strings.ContainsAny(value, "\r\n") {
		return "", "", &credential.BlockError{Provider: "env", Reason: fileEnvKey + " file must contain exactly one credential value"}
	}
	return value, "file:" + fileEnvKey, nil
}

func tokenName(envKey, fileEnvKey string) string {
	return envKey + " or " + fileEnvKey
}
