// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "lark-cli-update-test-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config")); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}
