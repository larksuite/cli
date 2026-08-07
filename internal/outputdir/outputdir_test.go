// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package outputdir

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
)

// TestEnsureRelativePathStaysInWorkingDir covers the path every
// --output-dir flag takes: a relative directory is validated, resolved inside
// the working directory, and created owner-only.
func TestEnsureRelativePathStaysInWorkingDir(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	if err := Ensure(filepath.Join("out", "nested")); err != nil {
		t.Fatalf("Ensure(relative) error = %v", err)
	}

	created := filepath.Join(dir, "out", "nested")
	info, err := os.Stat(created)
	if err != nil {
		t.Fatalf("stat %s: %v", created, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", created)
	}
	// Windows does not carry POSIX mode bits through MkdirAll.
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0700 {
			t.Fatalf("permissions on %s = %04o, want 0700", created, perm)
		}
	}
}

// TestEnsureRejectsEscapingRelativePath is the reason the relative
// branch goes through validate.SafeOutputPath at all: a path that climbs out of
// the working directory must fail before anything is created.
func TestEnsureRejectsEscapingRelativePath(t *testing.T) {
	parent := t.TempDir()
	work := filepath.Join(parent, "work")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatalf("create work dir: %v", err)
	}
	cmdutil.TestChdir(t, work)

	if err := Ensure(filepath.Join("..", "escaped")); err == nil {
		t.Fatal("Ensure(../escaped) = nil, want an error")
	}
	if _, err := os.Stat(filepath.Join(parent, "escaped")); !os.IsNotExist(err) {
		t.Fatalf("rejected path must not be created: stat error = %v", err)
	}
}

// TestEnsureAcceptsAbsolutePath pins the documented contract for
// callers that already resolved their path (RuntimeContext.ResolveSavePath,
// SafeOutputPath): an absolute directory is created, not re-validated against
// the working directory.
func TestEnsureAcceptsAbsolutePath(t *testing.T) {
	target := filepath.Join(t.TempDir(), "absolute", "out")

	if err := Ensure(target); err != nil {
		t.Fatalf("Ensure(absolute) error = %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat %s: %v", target, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", target)
	}
}

// TestEnsurePropagatesFilesystemFailure covers the branch the other cases never
// reach: a path whose parent is a regular file cannot be created, and Ensure has to
// return that error rather than reporting success for a directory that does not
// exist.
func TestEnsurePropagatesFilesystemFailure(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}

	if err := Ensure(filepath.Join("blocker", "child")); err == nil {
		t.Fatal("Ensure(under a regular file) = nil, want the filesystem error")
	}
	if err := Ensure(filepath.Join(blocker, "child")); err == nil {
		t.Fatal("Ensure(absolute, under a regular file) = nil, want the filesystem error")
	}
}
