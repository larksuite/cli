// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"runtime"
	"testing"
)

func TestPrepareSelfReplace_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this test verifies non-Windows no-op behavior")
	}
	cleanup, err := PrepareSelfReplace()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cleanup() // should be a no-op
}

func TestCleanupStaleFiles_NoError(t *testing.T) {
	// Should not panic on any platform.
	CleanupStaleFiles()
}
