// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package secplugin provides a placeholder credential provider for SEC_AUTH mode.
//
// When ~/.lark-cli/sec_config.json has:
//
//	LARKSUITE_CLI_SEC_ENABLE=true
//	LARKSUITE_CLI_SEC_AUTH=true
//
// this provider returns a minimal Account and placeholder tokens. The proxy
// is expected to replace the placeholder tokens with real ones.
package secplugin

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	internalsec "github.com/larksuite/cli/internal/secplugin"
)

// Provider supplies placeholder credentials when SEC_AUTH mode is enabled.
type Provider struct{}

// Name returns the registered credential provider name.
func (p *Provider) Name() string { return "secplugin" }

// Priority is higher than env (default 10) but lower than sidecar (0),
// so authsidecar builds keep sidecar semantics when both are present.
func (p *Provider) Priority() int { return 1 }

// loadSecConfig is replaceable in tests so provider behavior can be isolated
// from on-disk SEC configuration state.
var loadSecConfig = internalsec.Load

func validateDefaultAs(value string) error {
	switch id := credential.Identity(strings.TrimSpace(value)); id {
	case "", credential.IdentityAuto, credential.IdentityUser, credential.IdentityBot:
		return nil
	default:
		return fmt.Errorf("invalid %s %q (want user, bot, or auto)", envvars.CliDefaultAs, id)
	}
}

// ResolveAccount builds an account that advertises SEC_AUTH placeholder support.
func (p *Provider) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	cfg, err := loadSecConfig()
	if err != nil {
		return nil, &credential.BlockError{Provider: p.Name(), Reason: err.Error()}
	}
	if cfg == nil || !cfg.AuthEnabled() {
		return nil, nil
	}

	appID := strings.TrimSpace(os.Getenv(envvars.CliAppID))
	brand := credential.Brand(strings.TrimSpace(os.Getenv(envvars.CliBrand)))
	var defaultAs credential.Identity

	// Prefer explicit env; if missing, allow sec_config.json to provide defaults.
	if appID == "" && strings.TrimSpace(cfg.AppID) != "" {
		appID = strings.TrimSpace(cfg.AppID)
	}
	if brand == "" && strings.TrimSpace(cfg.Brand) != "" {
		brand = credential.Brand(strings.TrimSpace(cfg.Brand))
	}
	if defaultAs == "" && strings.TrimSpace(cfg.DefaultAs) != "" {
		defaultAs = credential.Identity(strings.TrimSpace(cfg.DefaultAs))
		if err := validateDefaultAs(string(defaultAs)); err != nil {
			return nil, &credential.BlockError{
				Provider: p.Name(),
				Reason:   err.Error(),
			}
		}
	}

	// Prefer explicit env for sandbox use; otherwise fall back to on-disk config
	// without resolving any secrets.
	if appID == "" || brand == "" {
		multi, err := core.LoadMultiAppConfig()
		if err != nil || multi == nil {
			return nil, &credential.BlockError{
				Provider: p.Name(),
				Reason:   "SEC_AUTH is enabled but no app config is available; run `lark-cli config init --new` (trusted env), or set " + envvars.CliAppID + " and " + envvars.CliBrand,
			}
		}
		app := multi.CurrentAppConfig("") // profile override not available in provider API
		if app == nil {
			return nil, &credential.BlockError{
				Provider: p.Name(),
				Reason:   "SEC_AUTH is enabled but no active profile is available in config.json",
			}
		}
		if appID == "" {
			appID = app.AppId
		}
		if brand == "" {
			brand = credential.Brand(app.Brand)
		}
		if defaultAs == "" {
			defaultAs = credential.Identity(app.DefaultAs)
		}

		// Map strict mode to supported identities (0 = allow all).
		mode := multi.StrictMode
		if app.StrictMode != nil {
			mode = *app.StrictMode
		}
		switch mode {
		case core.StrictModeBot:
			// Keep sandbox locked down to bot.
			return &credential.Account{
				AppID:               appID,
				AppSecret:           credential.NoAppSecret,
				Brand:               brand,
				DefaultAs:           defaultAs,
				SupportedIdentities: credential.SupportsBot,
			}, nil
		case core.StrictModeUser:
			return &credential.Account{
				AppID:               appID,
				AppSecret:           credential.NoAppSecret,
				Brand:               brand,
				DefaultAs:           defaultAs,
				SupportedIdentities: credential.SupportsUser,
			}, nil
		}
	}

	if appID == "" {
		return nil, &credential.BlockError{
			Provider: p.Name(),
			Reason:   "SEC_AUTH is enabled but " + envvars.CliAppID + " is missing",
		}
	}
	if brand == "" {
		brand = credential.BrandFeishu
	}
	if brand != credential.BrandFeishu && brand != credential.BrandLark {
		return nil, &credential.BlockError{
			Provider: p.Name(),
			Reason:   fmt.Sprintf("invalid %s %q (want feishu or lark)", envvars.CliBrand, brand),
		}
	}

	// DefaultAs comes from env if present (optional).
	envDefaultAs := strings.TrimSpace(os.Getenv(envvars.CliDefaultAs))
	if err := validateDefaultAs(envDefaultAs); err != nil {
		return nil, &credential.BlockError{
			Provider: p.Name(),
			Reason:   err.Error(),
		}
	}
	switch id := credential.Identity(envDefaultAs); id {
	case "", credential.IdentityAuto:
		// keep defaultAs from config/env; empty is allowed
	case credential.IdentityUser, credential.IdentityBot:
		defaultAs = id
	}

	// If STRICT_MODE env is not set, allow sec_config.json to provide a default.
	strictModeRaw := strings.TrimSpace(os.Getenv(envvars.CliStrictMode))
	if strictModeRaw == "" && strings.TrimSpace(cfg.StrictMode) != "" {
		strictModeRaw = strings.TrimSpace(cfg.StrictMode)
	}

	// SupportedIdentities from STRICT_MODE (optional). Default: allow both.
	support := credential.SupportsAll
	switch strictMode := strictModeRaw; strictMode {
	case "bot":
		support = credential.SupportsBot
	case "user":
		support = credential.SupportsUser
	case "off", "":
		// Keep the default: allow both identities.
	default:
		return nil, &credential.BlockError{
			Provider: p.Name(),
			Reason:   fmt.Sprintf("invalid %s %q (want bot, user, or off)", envvars.CliStrictMode, strictMode),
		}
	}

	return &credential.Account{
		AppID:               appID,
		AppSecret:           credential.NoAppSecret,
		Brand:               brand,
		DefaultAs:           defaultAs,
		SupportedIdentities: support,
	}, nil
}

// ResolveToken returns placeholder tokens that a trusted proxy must replace.
func (p *Provider) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.Token, error) {
	cfg, err := internalsec.Load()
	if err != nil {
		return nil, &credential.BlockError{Provider: p.Name(), Reason: err.Error()}
	}
	if cfg == nil || !cfg.AuthEnabled() {
		return nil, nil
	}

	switch req.Type {
	case credential.TokenTypeUAT:
		return &credential.Token{
			Value:  internalsec.SentinelUAT,
			Scopes: "", // empty => skip scope pre-check
			Source: "secplugin",
		}, nil
	case credential.TokenTypeTAT:
		return &credential.Token{
			Value:  internalsec.SentinelTAT,
			Scopes: "",
			Source: "secplugin",
		}, nil
	default:
		return nil, nil
	}
}

// init registers the SEC_AUTH placeholder credential provider.
func init() {
	credential.Register(&Provider{})
}
