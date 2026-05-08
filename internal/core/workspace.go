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

	// WorkspaceOpenClaw activates when any OpenClaw-specific env signal is
	// present (see DetectWorkspaceFromEnv for the full list).
	WorkspaceOpenClaw Workspace = "openclaw"

	// WorkspaceHermes activates when any Hermes-specific env signal is
	// present (see DetectWorkspaceFromEnv for the full list).
	WorkspaceHermes Workspace = "hermes"

	// WorkspaceLarkChannel activates when LARK_CHANNEL == "1" is set by
	// lark-channel-bridge in subprocesses it spawns (e.g. claude). See
	// DetectWorkspaceFromEnv for the detection rule.
	WorkspaceLarkChannel Workspace = "lark-channel"
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
//
// Detection is signal-based, not credential-based: we look for environment
// variables that the host Agent itself sets when launching a subprocess.
// Generic FEISHU_APP_ID / FEISHU_APP_SECRET are intentionally NOT used —
// any third-party Feishu script can set those, so they would cause
// false-positive routing into a Hermes workspace.
//
// Priority:
//  1. Any OpenClaw signal → WorkspaceOpenClaw
//     - OPENCLAW_CLI == "1":  subprocess marker (added 2026-03-09 via
//     OpenClaw PR #41411). Most precise, but absent on older builds.
//     - OPENCLAW_HOME / OPENCLAW_STATE_DIR / OPENCLAW_CONFIG_PATH non-empty:
//     user-facing paths introduced with the 2026-01-30 rename. Detected
//     so that OpenClaw builds predating the subprocess marker — or
//     invocation paths that do not propagate the marker — still route
//     correctly.
//  2. Any Hermes signal → WorkspaceHermes. All of the checked variables are
//     set by Hermes itself (hermes_cli/main.py, gateway/run.py). No
//     unrelated tool uses the HERMES_* namespace.
//     - HERMES_HOME:          exported by the CLI at startup
//     - HERMES_QUIET == "1":  exported by the gateway
//     - HERMES_EXEC_ASK == "1": exported by the gateway (paired w/ QUIET)
//     - HERMES_GATEWAY_TOKEN: injected into every gateway subprocess
//     - HERMES_SESSION_KEY:   session identifier scoped to the current chat
//  3. LARK_CHANNEL == "1" → WorkspaceLarkChannel. Set by lark-channel-bridge
//     when spawning subprocesses (e.g. claude). Single boolean marker —
//     mirrors the OPENCLAW_CLI / HERMES_QUIET style.
//  4. Otherwise → WorkspaceLocal
func DetectWorkspaceFromEnv(getenv func(string) string) Workspace {
	if getenv("OPENCLAW_CLI") == "1" ||
		getenv("OPENCLAW_HOME") != "" ||
		getenv("OPENCLAW_STATE_DIR") != "" ||
		getenv("OPENCLAW_CONFIG_PATH") != "" ||
		getenv("OPENCLAW_SERVICE_MARKER") != "" ||
		getenv("OPENCLAW_SERVICE_VERSION") != "" ||
		getenv("OPENCLAW_GATEWAY_PORT") != "" ||
		getenv("OPENCLAW_SHELL") != "" {
		return WorkspaceOpenClaw
	}
	if getenv("HERMES_HOME") != "" ||
		getenv("HERMES_QUIET") == "1" ||
		getenv("HERMES_EXEC_ASK") == "1" ||
		getenv("HERMES_GATEWAY_TOKEN") != "" ||
		getenv("HERMES_SESSION_KEY") != "" {
		return WorkspaceHermes
	}
	if getenv("LARK_CHANNEL") == "1" {
		return WorkspaceLarkChannel
	}
	return WorkspaceLocal
}

// GetBaseConfigDir returns the root config directory, ignoring workspace.
// Priority: LARKSUITE_CLI_CONFIG_DIR env → ~/.lark-cli.
// If the home directory cannot be determined and no override is set, a
// warning is written to stderr and the path falls back to a relative
// ".lark-cli" — callers will then see an explicit I/O error at first use
// instead of a silent misconfiguration.
func GetBaseConfigDir() string {
	if dir := os.Getenv("LARKSUITE_CLI_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := vfs.UserHomeDir()
	if err != nil || home == "" {
		// Fall back to a relative ".lark-cli" so the first I/O operation
		// surfaces a clear "no such file or directory" error. We cannot
		// emit a stderr warning here — this package has no IOStreams in
		// scope, and direct writes to os.Stderr violate the IOStreams
		// injection boundary (enforced by lint). Users who hit this path
		// should set LARKSUITE_CLI_CONFIG_DIR explicitly.
		home = ""
	}
	return filepath.Join(home, ".lark-cli")
}

// GetRuntimeDir returns the workspace-aware config directory.
//   - WorkspaceLocal → GetBaseConfigDir() (unchanged, backward-compatible)
//   - WorkspaceOpenClaw → GetBaseConfigDir()/openclaw
//   - WorkspaceHermes → GetBaseConfigDir()/hermes
//   - WorkspaceLarkChannel → GetBaseConfigDir()/lark-channel
func GetRuntimeDir() string {
	base := GetBaseConfigDir()
	ws := CurrentWorkspace()
	if ws.IsLocal() {
		return base
	}
	return filepath.Join(base, string(ws))
}
