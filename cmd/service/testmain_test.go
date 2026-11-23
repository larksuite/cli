// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"os"
	"testing"
)

// TestMain isolates service command tests from the host machine. The API
// Catalog Snapshot is embedded and requires no cache seeding.
//
// Note: os.Exit skips deferred functions, so cleanup runs explicitly after
// m.Run before exiting.
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "lark-cli-cmd-service-test-*")
	if err != nil {
		println("cmd/service test setup: MkdirTemp failed:", err.Error())
		os.Exit(2)
	}
	if err := os.Setenv("LARKSUITE_CLI_CONFIG_DIR", root); err != nil {
		println("cmd/service test setup: Setenv failed:", err.Error())
		os.RemoveAll(root)
		os.Exit(2)
	}
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}
