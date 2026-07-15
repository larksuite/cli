// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/registry/registrytest"
)

// TestMain isolates auth command tests from the host machine: config, logs
// and the registry cache are redirected to a temp dir, then the registry is
// seeded from the tracked fixture and initialized eagerly. Domain-completion
// tests read the registry, so without seeding a clean checkout would either
// fail or trigger a remote metadata fetch.
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
	if err := registrytest.Seed(root); err != nil {
		println("cmd/auth test setup: registrytest.Seed failed:", err.Error())
		os.RemoveAll(root)
		os.Exit(2)
	}
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}
