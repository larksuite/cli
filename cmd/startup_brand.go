// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

// selectInvocationWorkspace establishes the workspace before any startup
// consumer reads workspace-scoped configuration. Execute needs this before
// resolving the registry brand, while Build/buildInternal needs it before
// capturing the immutable Profile snapshot passed to the Factory.
func selectInvocationWorkspace() core.Workspace {
	workspace := core.DetectWorkspaceFromEnv(os.Getenv)
	core.SetCurrentWorkspace(workspace)
	return workspace
}

// ResolveStartupBrand resolves the brand before the command tree is built, so
// the registry's remote metadata overlay uses the configured brand from the
// first catalog access. It mirrors the credential chain's brand precedence —
// environment, then the active profile's raw config entry — without touching
// the keychain (no secrets are needed to know the brand).
func ResolveStartupBrand(profile string) core.LarkBrand {
	config, _ := core.LoadMultiAppConfig()
	return resolveStartupBrandFromConfig(profile, config)
}

// resolveStartupBrandFromConfig keeps registry routing on the same immutable
// Profile snapshot used by credentials and runtime policy.
func resolveStartupBrandFromConfig(profile string, config *core.MultiAppConfig) core.LarkBrand {
	if raw := os.Getenv(envvars.CliBrand); raw != "" {
		return core.ParseBrand(raw)
	}
	if config != nil {
		if app := config.CurrentAppConfig(profile); app != nil {
			return core.ParseBrand(string(app.Brand))
		}
	}
	return core.BrandFeishu
}
