// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package localfileio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenValidated_RejectsHardLinkedFile(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "secret")
	link := filepath.Join(dir, "smuggled")
	if err := os.WriteFile(original, []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Link(original, link); err != nil {
		t.Skipf("Link: %v", err)
	}

	_, err := openValidated(link)
	if err == nil {
		t.Fatal("expected error for hard-linked file, got nil")
	}
	if !strings.Contains(err.Error(), "hard links") {
		t.Errorf("error should mention hard links, got: %v", err)
	}
}
