// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

// ResolveStartupBrand retains the legacy startup-brand resolution order for
// wrapper mains that still pass its result to WithStartupBrand.
// Deprecated: committed catalog selection is brand-independent.
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
