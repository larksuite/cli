// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Default-factory tests initialize the registry. Keep them deterministic and
	// prevent background remote-metadata refreshes from touching user state.
	if err := os.Setenv("LARKSUITE_CLI_REMOTE_META", "off"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
