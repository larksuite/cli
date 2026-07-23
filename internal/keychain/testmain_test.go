// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keychain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "lark-cli-keychain-test-*")
	if err != nil {
		panic(err)
	}
	for key, value := range map[string]string{
		"LARKSUITE_CLI_DATA_DIR": filepath.Join(root, "data"),
		"LARKSUITE_CLI_LOG_DIR":  filepath.Join(root, "logs"),
	} {
		if err := os.Setenv(key, value); err != nil {
			panic(err)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}
