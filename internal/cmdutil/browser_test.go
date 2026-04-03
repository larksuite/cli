// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import "testing"

func TestOpenBrowser_NoPanic(t *testing.T) {
	// Verify OpenBrowser doesn't panic on any URL.
	// On CI/headless this may return true (process spawned) or false — both are fine.
	_ = OpenBrowser("https://example.com")
}
