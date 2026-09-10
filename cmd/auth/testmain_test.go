// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/core"
)

// TestMain isolates auth command tests from the host machine. The API Catalog
// Snapshot is embedded and requires no cache seeding.
//
// Note: os.Exit skips deferred functions, so cleanup runs explicitly after
// m.Run before exiting.
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "lark-cli-cmd-auth-test-*")
	if err != nil {
		println("cmd/auth test setup: MkdirTemp failed:", err.Error())
		os.Exit(2)
	}
	if err := os.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config")); err != nil {
		println("cmd/auth test setup: Setenv failed:", err.Error())
		os.RemoveAll(root)
		os.Exit(2)
	}
	if err := os.Setenv("LARKSUITE_CLI_LOG_DIR", filepath.Join(root, "logs")); err != nil {
		println("cmd/auth test setup: Setenv failed:", err.Error())
		os.RemoveAll(root)
		os.Exit(2)
	}
	// Never reach the live scopes.json endpoint from tests: default the remote
	// fetch to "unavailable" so authLoginRun exercises the local fallback
	// deterministically. A test that needs a specific remote sets fetchRemoteScopes
	// itself and restores it.
	fetchRemoteScopes = func(core.LarkBrand) (map[string][]string, bool) { return nil, false }
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}
