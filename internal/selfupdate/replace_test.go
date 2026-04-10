// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentExePath(t *testing.T) {
	p, err := currentExePath()
	if err != nil {
		t.Fatalf("currentExePath() error: %v", err)
	}
	if !filepath.IsAbs(p) {
		t.Errorf("expected absolute path, got: %s", p)
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("resolved path should exist: %v", err)
	}
}

func TestPrepareSelfReplace_ReturnsNoError(t *testing.T) {
	// On any platform, calling PrepareSelfReplace should not panic.
	restore, err := PrepareSelfReplace()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	restore()
}

func TestCleanupStaleFiles_NoPanic(t *testing.T) {
	CleanupStaleFiles()
}
