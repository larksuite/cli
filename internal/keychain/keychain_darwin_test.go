// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin

package keychain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetFileMasterKeyCorruptExistingReturnsError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := StorageDir(LarkCliService)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "master.key")
	if err := os.WriteFile(path, []byte("not-base64"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := getFileMasterKey(LarkCliService, true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "is corrupt") {
		t.Fatalf("expected corrupt error, got %v", err)
	}
}
