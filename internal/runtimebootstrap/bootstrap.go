// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package runtimebootstrap resolves all workspace-scoped startup state once
// before command construction.
package runtimebootstrap

import (
	"os"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/runtimeplan"
)

// Result is the immutable startup snapshot shared by registry routing,
// credentials, transports, and workspace policy.
type Result struct {
	ProfileConfig *core.MultiAppConfig
	Plan          *runtimeplan.Plan
}

// Resolve establishes the workspace first, then captures the selected Profile
// and runtime policy exactly once.
func Resolve(profileOverride string) *Result {
	core.SetCurrentWorkspace(core.DetectWorkspaceFromEnv(os.Getenv))
	return resolveEdition(profileOverride)
}
