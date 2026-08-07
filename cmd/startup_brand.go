// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"

	"github.com/larksuite/cli/brand"
	"github.com/larksuite/cli/envnames"
	configpkg "github.com/larksuite/cli/internal/config"
)

// ResolveStartupBrand resolves the brand before the command tree is built, so
// the registry's remote metadata overlay uses the configured brand from the
// first catalog access. It mirrors the credential chain's brand precedence —
// environment, then the active profile's raw config entry — without touching
// the keychain (no secrets are needed to know the brand).
func ResolveStartupBrand(profile string) brand.Brand {
	if raw := os.Getenv(envnames.CliBrand); raw != "" {
		return brand.ParseBrand(raw)
	}
	if cfg, err := configpkg.LoadMultiAppConfig(); err == nil {
		if app := cfg.CurrentAppConfig(profile); app != nil {
			return brand.ParseBrand(string(app.Brand))
		}
	}
	return brand.Feishu
}
