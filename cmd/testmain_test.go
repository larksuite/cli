// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"testing"

	"github.com/larksuite/cli/internal/registry/registrytest"
)

// TestMain isolates command-tree tests from the host machine: config (and the
// registry cache under it) is redirected to a temp dir, then the registry is
// seeded from the tracked fixture and initialized eagerly. Tests pass on a
// clean checkout with no network, no `make fetch_meta`, and no user cache.
//
// Note: os.Exit skips deferred functions, so cleanup runs explicitly after
// m.Run before exiting.
func TestMain(m *testing.M) {
	if isStartupBrandHelper() {
		// Re-exec helper subprocess (startup_brand_test.go): the parent test
		// already provides an isolated config dir and disables remote metadata,
		// and the helper must own the first registry Init to prove the startup
		// order — do not seed or eagerly initialize here.
		os.Exit(m.Run())
	}
	root, err := os.MkdirTemp("", "lark-cli-cmd-test-*")
	if err != nil {
		println("cmd test setup: MkdirTemp failed:", err.Error())
		os.Exit(2)
	}
	if err := os.Setenv("LARKSUITE_CLI_CONFIG_DIR", root); err != nil {
		println("cmd test setup: Setenv failed:", err.Error())
		os.RemoveAll(root)
		os.Exit(2)
	}
	if err := registrytest.Seed(root); err != nil {
		println("cmd test setup: registrytest.Seed failed:", err.Error())
		os.RemoveAll(root)
		os.Exit(2)
	}
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}
