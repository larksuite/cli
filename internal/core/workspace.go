// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/larksuite/cli/internal/vfs"
)

// Workspace identifies a config isolation context.
// Each non-local workspace maps to a subdirectory under the base config dir.
type Workspace string

const (
	// WorkspaceLocal is the default workspace. GetConfigDir returns the base
	// config dir without any subdirectory — identical to pre-workspace behavior.
	WorkspaceLocal Workspace = ""

	// WorkspaceOpenClaw activates when OPENCLAW_CLI=="1".
	WorkspaceOpenClaw Workspace = "openclaw"

	// WorkspaceHermes activates when FEISHU_APP_ID + FEISHU_APP_SECRET are set.
	WorkspaceHermes Workspace = "hermes"
)

// currentWorkspace holds the workspace for the current process invocation.
// Set once during Factory initialization; config bind's RunE may re-set it
// to the workspace being bound. Uses atomic.Value for goroutine safety
// (background registry refresh reads GetRuntimeDir concurrently with the
// Factory init that writes workspace).
var currentWorkspace atomic.Value // stores Workspace; zero value → Load returns nil → treated as Local

// SetCurrentWorkspace sets the active workspace for this process.
func SetCurrentWorkspace(ws Workspace) {
	currentWorkspace.Store(ws)
}

// CurrentWorkspace returns the active workspace.
// Returns WorkspaceLocal if not yet set (safe default, backward-compatible).
func CurrentWorkspace() Workspace {
	v := currentWorkspace.Load()
	if v == nil {
		return WorkspaceLocal
	}
	return v.(Workspace)
}

// Display returns the user-visible workspace label.
// Used in config show, doctor, and error messages.
func (w Workspace) Display() string {
	if w == WorkspaceLocal || w == "" {
		return "local"
	}
	return string(w)
}

// IsLocal returns true if this is the default local workspace.
func (w Workspace) IsLocal() bool {
	return w == WorkspaceLocal || w == ""
}

// DetectWorkspaceFromEnv determines the workspace from process environment.
// Priority:
//  1. OPENCLAW_CLI == "1" (strict equal) → WorkspaceOpenClaw
//  2. FEISHU_APP_ID + FEISHU_APP_SECRET both non-empty → WorkspaceHermes
//  3. Otherwise → WorkspaceLocal
func DetectWorkspaceFromEnv(getenv func(string) string) Workspace {
	if getenv("OPENCLAW_CLI") == "1" {
		return WorkspaceOpenClaw
	}
	if getenv("FEISHU_APP_ID") != "" && getenv("FEISHU_APP_SECRET") != "" {
		return WorkspaceHermes
	}
	return WorkspaceLocal
}

// GetBaseConfigDir returns the root config directory, ignoring workspace.
// Priority: LARKSUITE_CLI_CONFIG_DIR env → ~/.lark-cli
func GetBaseConfigDir() string {
	if dir := os.Getenv("LARKSUITE_CLI_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := vfs.UserHomeDir()
	if err != nil || home == "" {
		// filepath.Join yields relative ".lark-cli" — surfaces as a clear
		// "no such file" error at I/O time. Matches keychain_darwin.go.
		home = ""
	}
	return filepath.Join(home, ".lark-cli")
}

// GetRuntimeDir returns the workspace-aware config directory.
//   - WorkspaceLocal → GetBaseConfigDir() (unchanged, backward-compatible)
//   - WorkspaceOpenClaw → GetBaseConfigDir()/openclaw
//   - WorkspaceHermes → GetBaseConfigDir()/hermes
func GetRuntimeDir() string {
	base := GetBaseConfigDir()
	ws := CurrentWorkspace()
	if ws.IsLocal() {
		return base
	}
	return filepath.Join(base, string(ws))
}
