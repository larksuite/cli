// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "lark-cli-internal-auth-test-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("LARKSUITE_CLI_LOG_DIR", filepath.Join(root, "logs")); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}
