// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	os.WriteFile(src, []byte("hello"), 0644)

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst failed: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", string(data))
	}
}

func TestReplaceUnix(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "lark-cli")
	newBin := filepath.Join(dir, "lark-cli-new")

	os.WriteFile(current, []byte("old"), 0755)
	os.WriteFile(newBin, []byte("new"), 0755)

	if err := replaceUnix(current, newBin); err != nil {
		t.Fatalf("replaceUnix failed: %v", err)
	}

	data, _ := os.ReadFile(current)
	if string(data) != "new" {
		t.Errorf("expected 'new', got %q", data)
	}
}

func TestReplaceWindows(t *testing.T) {
	// Test the Windows path logic on any platform (files aren't actually locked).
	dir := t.TempDir()
	current := filepath.Join(dir, "lark-cli.exe")
	newBin := filepath.Join(dir, "lark-cli-new.exe")

	os.WriteFile(current, []byte("old"), 0755)
	os.WriteFile(newBin, []byte("new"), 0755)

	if err := replaceWindows(current, newBin); err != nil {
		t.Fatalf("replaceWindows failed: %v", err)
	}

	data, _ := os.ReadFile(current)
	if string(data) != "new" {
		t.Errorf("expected 'new', got %q", data)
	}

	// .old should be cleaned up (not locked in test env).
	if _, err := os.Stat(current + ".old"); !os.IsNotExist(err) {
		t.Error("expected .old to be cleaned up")
	}
}
