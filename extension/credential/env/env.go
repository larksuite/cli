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
type Provider struct{}

func (p *Provider) Name() string { return "env" }

func (p *Provider) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	appID := os.Getenv(envvars.CliAppID)
	appSecret := os.Getenv(envvars.CliAppSecret)
	hasUAT := os.Getenv(envvars.CliUserAccessToken) != ""
	hasTAT := os.Getenv(envvars.CliTenantAccessToken) != ""
	presentKeys := presentCredentialEnvKeys(appID, appSecret, hasUAT, hasTAT)
	if len(presentKeys) == 0 {
		return nil, nil
	}

	// Identity policy variables are validated whenever a direct credential
	// input is present. Their errors must not be hidden by a later credential
	// completeness check or profile arbitration.
	defaultAs := credential.Identity(os.Getenv(envvars.CliDefaultAs))
	switch defaultAs {
	case "", credential.IdentityAuto, credential.IdentityUser, credential.IdentityBot:
	default:
		return nil, &credential.BlockError{
			Provider: "env",
			Reason:   fmt.Sprintf("invalid %s %q (want user, bot, or auto)", envvars.CliDefaultAs, defaultAs),
			Code:     credential.BlockReasonInvalidPolicy,
			Param:    envvars.CliDefaultAs,
		}
	}

	strictMode := os.Getenv(envvars.CliStrictMode)
	var supported credential.IdentitySupport
	switch strictMode {
	case "bot":
		supported = credential.SupportsBot
	case "user":
		supported = credential.SupportsUser
	case "off":
		supported = credential.SupportsAll
	case "":
		if hasUAT {
			supported |= credential.SupportsUser
		}
		if hasTAT {
			supported |= credential.SupportsBot
		}
	default:
		return nil, &credential.BlockError{
			Provider: "env",
			Reason:   fmt.Sprintf("invalid %s %q (want bot, user, or off)", envvars.CliStrictMode, strictMode),
			Code:     credential.BlockReasonInvalidPolicy,
			Param:    envvars.CliStrictMode,
		}
	}

	if appID == "" && appSecret == "" {
		switch {
		case hasUAT:
			return nil, incompleteCredentialError(
				appID,
				envvars.CliUserAccessToken+" is set but "+envvars.CliAppID+" is missing",
				[]string{envvars.CliAppID}, nil, presentKeys)
		case hasTAT:
			return nil, incompleteCredentialError(
				appID,
				envvars.CliTenantAccessToken+" is set but "+envvars.CliAppID+" is missing",
				[]string{envvars.CliAppID}, nil, presentKeys)
		}
	}
	if appID == "" {
		return nil, incompleteCredentialError(
			appID,
			envvars.CliAppSecret+" is set but "+envvars.CliAppID+" is missing",
			[]string{envvars.CliAppID}, nil, presentKeys)
	}
	if appSecret == "" && !hasUAT && !hasTAT {
		return nil, incompleteCredentialError(
			appID,
			envvars.CliAppID+" is set but no app secret or access token is available",
			nil,
			[]string{envvars.CliAppSecret, envvars.CliUserAccessToken, envvars.CliTenantAccessToken},
			presentKeys)
	}
	brand := credential.Brand(core.ParseBrand(os.Getenv(envvars.CliBrand)))
	acct := &credential.Account{
		AppID:               appID,
		AppSecret:           appSecret,
		Brand:               brand,
		DefaultAs:           defaultAs,
		SupportedIdentities: supported,
		Kind:                credential.AccountDirect,
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

func incompleteCredentialError(appID, reason string, missingKeys, requiredAnyOf, presentKeys []string) *credential.BlockError {
	return &credential.BlockError{
		Provider:      "env",
		Reason:        reason,
		Code:          credential.BlockReasonCredentialIncomplete,
		MissingKeys:   missingKeys,
		RequiredAnyOf: requiredAnyOf,
		PresentKeys:   presentKeys,
		AppID:         appID,
	}
}

func presentCredentialEnvKeys(appID, appSecret string, hasUAT, hasTAT bool) []string {
	var keys []string
	if appID != "" {
		keys = append(keys, envvars.CliAppID)
	}
	if appSecret != "" {
		keys = append(keys, envvars.CliAppSecret)
	}
	if hasUAT {
		keys = append(keys, envvars.CliUserAccessToken)
	}
	if hasTAT {
		keys = append(keys, envvars.CliTenantAccessToken)
	}
	return keys
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
