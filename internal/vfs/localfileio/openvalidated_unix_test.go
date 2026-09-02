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

// TestOpenValidated_RefusesHardLinkEntirelyInsideAllowedRoot records an
// accepted cost of the link check rather than a desired behavior: both names
// live in the working directory, nothing is smuggled, and the read is still
// refused because the other names cannot be enumerated. The message must offer
// a workaround the caller can actually act on — "copy the file", not "move it
// into an allowed directory", since it is already in one.
func TestOpenValidated_RefusesHardLinkEntirelyInsideAllowedRoot(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	if err := os.WriteFile("a.txt", []byte("hi"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Link("a.txt", "b.txt"); err != nil {
		t.Skipf("Link: %v", err)
	}

	fio := &LocalFileIO{}
	_, err := fio.Open("a.txt")
	if err == nil {
		t.Fatal("expected the link check to refuse the read")
	}
	if strings.Contains(err.Error(), "allowed directory") {
		t.Errorf("message must not tell the caller to move an already-allowed file: %v", err)
	}
	for _, want := range []string{"hard links", "copy the file"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message should contain %q, got: %v", want, err)
		}
	}

	// A copy has a single name and reads normally, so the advice works.
	data, err := os.ReadFile("a.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := os.WriteFile("copy.txt", data, 0600); err != nil {
		t.Fatalf("WriteFile copy: %v", err)
	}
	f, err := fio.Open("copy.txt")
	if err != nil {
		t.Fatalf("Open(copy) error = %v, want nil", err)
	}
	f.Close()
}
