// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

// ResolveStartupBrand resolves the brand before the command tree is built, so
// the registry's remote metadata overlay uses the configured brand from the
// first catalog access. It mirrors the credential chain's brand precedence —
// environment, then the active profile's raw config entry — without touching
// the keychain (no secrets are needed to know the brand).
func ResolveStartupBrand(profile string) core.LarkBrand {
	if raw := os.Getenv(envvars.CliBrand); raw != "" {
		return core.ParseBrand(raw)
	}
	if cfg, err := core.LoadMultiAppConfig(); err == nil {
		if app := cfg.CurrentAppConfig(profile); app != nil {
			return core.ParseBrand(string(app.Brand))
		}
	}
	return core.BrandFeishu
}
