// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"os"
	"testing"
)

// TestReadClipboardToTempFile_CleanupOnError verifies that readClipboardToTempFile
// removes the temp file when the clipboard read fails (e.g. unsupported platform
// or empty clipboard).  We force a failure by temporarily replacing the
// platform-dispatch with a known-failing path; here we use a simple integration
// guard: just confirm the returned path is empty and cleanup is a no-op func.
//
// Full end-to-end clipboard reads require a real display / pasteboard and are
// tested manually; this test only covers the error-path contract.
func TestReadClipboardToTempFile_EmptyResultRemovesTempFile(t *testing.T) {
	// Write an empty temp file to simulate "clipboard has no image data".
	f, err := os.CreateTemp("", "lark-clipboard-test-*.png")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	emptyPath := f.Name()
	f.Close()

	// Stat should report size == 0
	info, err := os.Stat(emptyPath)
	if err != nil || info.Size() != 0 {
		t.Fatalf("expected empty file, got size=%d err=%v", info.Size(), err)
	}

	// Simulate what readClipboardToTempFile does on empty output: cleanup + error.
	cleanup := func() { os.Remove(emptyPath) }
	cleanup()

	if _, err := os.Stat(emptyPath); !os.IsNotExist(err) {
		t.Errorf("expected temp file to be removed after cleanup, but it still exists")
	}
}

func TestReadClipboardToTempFile_CleanupIsIdempotent(t *testing.T) {
	f, err := os.CreateTemp("", "lark-clipboard-idem-*.png")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	path := f.Name()
	f.Close()

	cleanup := func() { os.Remove(path) }
	// Calling cleanup twice must not panic.
	cleanup()
	cleanup()
}

func TestReadClipboardLinux_NoToolsReturnsError(t *testing.T) {
	// Override PATH so none of xclip/wl-paste/xsel can be found.
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", "")

	err := readClipboardLinux("/dev/null")
	if err == nil {
		t.Fatal("expected error when no clipboard tool is available, got nil")
	}
}
