// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "lark-cli-registry-test-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config")); err != nil {
		panic(err)
	}
	code := m.Run()
	// A test that ran Init without a trailing resetInit can leave a background
	// refresh goroutine alive; removing the temp root while it writes would
	// let it recreate the directory after cleanup. Wait it out first.
	waitBackgroundRefresh()
	_ = os.RemoveAll(root)
	os.Exit(code)
}
