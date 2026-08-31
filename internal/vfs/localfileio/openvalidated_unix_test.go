// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package localfileio

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestOpenValidated_AcceptsRegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.txt")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	f, err := openValidated(path)
	if err != nil {
		t.Fatalf("openValidated(regular) error = %v", err)
	}
	defer f.Close()

	// Blocking mode must be restored: a full read succeeds normally.
	got, err := io.ReadAll(f)
	if err != nil || string(got) != "data" {
		t.Errorf("read got %q, err %v", got, err)
	}
}

func TestOpenValidated_RejectsFIFOWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Skipf("Mkfifo: %v", err)
	}

	// A FIFO with no writer would block a plain open; O_NONBLOCK must let the
	// open return so fstat can refuse it.
	_, err := openValidated(path)
	if err == nil {
		t.Fatal("expected error for FIFO, got nil")
	}
	if !strings.Contains(err.Error(), "regular file") {
		t.Errorf("error should mention regular file, got: %v", err)
	}
}

func TestOpenValidated_RejectsHardLinkedFile(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "secret")
	link := filepath.Join(dir, "smuggled")
	if err := os.WriteFile(orig, []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Link(orig, link); err != nil {
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

func TestOpenValidated_RejectsSymlinkFinalComponent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(target, []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("Symlink: %v", err)
	}

	// openValidated receives realpath-resolved paths in production; handing it
	// a symlink directly simulates a swap after validation — O_NOFOLLOW must
	// refuse to follow it.
	if _, err := openValidated(link); err == nil {
		t.Fatal("expected error for symlink final component, got nil")
	}
}
