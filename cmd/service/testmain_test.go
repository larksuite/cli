// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Service command tests exercise the embedded catalog. They must not start
	// a background remote-metadata refresh that touches user state or network.
	if err := os.Setenv("LARKSUITE_CLI_REMOTE_META", "off"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
