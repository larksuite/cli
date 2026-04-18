// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar

// Package sidecar provides a noop credential provider for the auth sidecar
// proxy mode. When LARKSUITE_CLI_AUTH_PROXY is set, this provider supplies
// placeholder credentials so the CLI's auth pipeline can proceed normally.
// Real tokens are never present in the sandbox; the sidecar transport
// interceptor routes requests to the trusted sidecar process instead.
package sidecar

import (
	"context"
	"fmt"
	"os"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/sidecar"
)

// Provider is the noop credential provider for sidecar mode.
type Provider struct{}

func (p *Provider) Name() string  { return "sidecar" }
func (p *Provider) Priority() int { return 0 }

// ResolveAccount returns a minimal Account when sidecar mode is active.
// The account contains AppID and Brand from environment variables, a
// placeholder secret, and SupportedIdentities derived from STRICT_MODE.
// Returns nil, nil when sidecar mode is not active (AUTH_PROXY not set).
func (p *Provider) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	proxyAddr := os.Getenv(envvars.CliAuthProxy)
	if proxyAddr == "" {
		return nil, nil // not in sidecar mode, skip
	}

	if err := sidecar.ValidateProxyAddr(proxyAddr); err != nil {
		return nil, &credential.BlockError{
			Provider: "sidecar",
			Reason:   fmt.Sprintf("invalid %s %q: %v", envvars.CliAuthProxy, proxyAddr, err),
		}
	}

	appID := os.Getenv(envvars.CliAppID)
	if appID == "" {
		return nil, &credential.BlockError{
			Provider: "sidecar",
			Reason:   envvars.CliAuthProxy + " is set but " + envvars.CliAppID + " is missing",
		}
	}

	if os.Getenv(envvars.CliProxyKey) == "" {
		return nil, &credential.BlockError{
			Provider: "sidecar",
			Reason:   envvars.CliAuthProxy + " is set but " + envvars.CliProxyKey + " is missing",
		}
	}

	brand := credential.Brand(os.Getenv(envvars.CliBrand))
	if brand == "" {
		brand = credential.BrandFeishu
	}

	acct := &credential.Account{
		AppID:     appID,
		AppSecret: credential.NoAppSecret,
		Brand:     brand,
	}

	// Parse DefaultAs
	switch id := credential.Identity(os.Getenv(envvars.CliDefaultAs)); id {
	case "", credential.IdentityAuto:
		acct.DefaultAs = id
	case credential.IdentityUser, credential.IdentityBot:
		acct.DefaultAs = id
	default:
		return nil, &credential.BlockError{
			Provider: "sidecar",
			Reason:   fmt.Sprintf("invalid %s %q (want user, bot, or auto)", envvars.CliDefaultAs, id),
		}
	}

	// Parse SupportedIdentities from STRICT_MODE, default to SupportsAll.
	switch strictMode := os.Getenv(envvars.CliStrictMode); strictMode {
	case "bot":
		acct.SupportedIdentities = credential.SupportsBot
	case "user":
		acct.SupportedIdentities = credential.SupportsUser
	case "off", "":
		acct.SupportedIdentities = credential.SupportsAll
	default:
		return nil, &credential.BlockError{
			Provider: "sidecar",
			Reason:   fmt.Sprintf("invalid %s %q (want bot, user, or off)", envvars.CliStrictMode, strictMode),
		}
	}

	return acct, nil
}

// ResolveToken returns a sentinel token whose value encodes the token type.
// The transport interceptor reads this sentinel to determine the identity
// (user vs bot), strips it, and the sidecar injects the real token.
// Returns nil, nil when sidecar mode is not active.
func (p *Provider) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.Token, error) {
	if os.Getenv(envvars.CliAuthProxy) == "" {
		return nil, nil
	}

	var sentinel string
	switch req.Type {
	case credential.TokenTypeUAT:
		sentinel = sidecar.SentinelUAT
	case credential.TokenTypeTAT:
		sentinel = sidecar.SentinelTAT
	default:
		return nil, nil
	}

	return &credential.Token{
		Value:  sentinel,
		Scopes: "", // empty → scope pre-check is skipped
		Source: "sidecar",
	}, nil
}

func init() {
	credential.Register(&Provider{})
}
