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
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const maxCredentialFileBytes = 64 * 1024

// Provider resolves credentials from environment variables.
type Provider struct{}

func (p *Provider) Name() string { return "env" }

func (p *Provider) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	appID, _, err := readCredentialValue(envvars.CliAppID, envvars.CliAppIDFile)
	if err != nil {
		return nil, err
	}
	appSecret := os.Getenv(envvars.CliAppSecret)
	uat, _, err := readCredentialValue(envvars.CliUserAccessToken, envvars.CliUserAccessTokenFile)
	if err != nil {
		return nil, err
	}
	tat, _, err := readCredentialValue(envvars.CliTenantAccessToken, envvars.CliTenantAccessTokenFile)
	if err != nil {
		return nil, err
	}
	hasUAT := uat != ""
	hasTAT := tat != ""
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
	var envKey, fileEnvKey string
	switch req.Type {
	case credential.TokenTypeUAT:
		envKey = envvars.CliUserAccessToken
		fileEnvKey = envvars.CliUserAccessTokenFile
	case credential.TokenTypeTAT:
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

	safePath, err := validate.SafeEnvFilePath(filePath, fileEnvKey)
	if err != nil {
		return "", "", &credential.BlockError{Provider: "env", Reason: err.Error()}
	}
	info, err := vfs.Stat(safePath)
	if err != nil {
		return "", "", &credential.BlockError{Provider: "env", Reason: "cannot stat " + fileEnvKey + ": " + err.Error()}
	}
	if info.Size() > maxCredentialFileBytes {
		return "", "", &credential.BlockError{Provider: "env", Reason: fileEnvKey + " file is too large"}
	}
	data, err := vfs.ReadFile(safePath)
	if err != nil {
		return "", "", &credential.BlockError{Provider: "env", Reason: "cannot read " + fileEnvKey + ": " + err.Error()}
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
