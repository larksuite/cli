// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	// Default-factory tests initialize the registry and resolve config. Keep
	// them deterministic: never read the developer's real ~/.lark-cli and
	// prevent background remote-metadata refreshes from touching user state.
	root, err := os.MkdirTemp("", "lark-cli-cmdutil-test-*")
	if err != nil {
		println("internal/cmdutil test setup: MkdirTemp failed:", err.Error())
		os.Exit(2)
	}
	if err := os.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config")); err != nil {
		panic(err)
	}
	if err := os.Setenv("LARKSUITE_CLI_REMOTE_META", "off"); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}
