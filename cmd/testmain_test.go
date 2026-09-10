// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"testing"
)

// TestMain isolates command-tree tests from the host machine. API catalog data
// is embedded in the binary and needs no cache seeding or network access.
//
// Note: os.Exit skips deferred functions, so cleanup runs explicitly after
// m.Run before exiting.
func TestMain(m *testing.M) {
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
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}
