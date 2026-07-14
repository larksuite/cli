// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Command-tree construction initializes the registry. Default unit tests
	// must use embedded metadata rather than start a background refresh that
	// touches the user's cache or network.
	if err := os.Setenv("LARKSUITE_CLI_REMOTE_META", "off"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
