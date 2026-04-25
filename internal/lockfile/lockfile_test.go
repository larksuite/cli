// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lockfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestLock(t *testing.T) *LockFile {
	t.Helper()
	return New(filepath.Join(t.TempDir(), "test.lock"))
}

func TestTryLock_Success(t *testing.T) {
	l := newTestLock(t)

	if err := l.TryLock(); err != nil {
		t.Fatalf("TryLock failed: %v", err)
	}
	defer l.Unlock()

	if _, err := os.Stat(l.Path()); os.IsNotExist(err) {
		t.Error("lock file should exist after TryLock")
	}
}

func TestTryLock_Conflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")

	l1 := New(path)
	if err := l1.TryLock(); err != nil {
		t.Fatalf("first TryLock failed: %v", err)
	}
	defer l1.Unlock()

	l2 := New(path)
	err := l2.TryLock()
	if err == nil {
		l2.Unlock()
		t.Fatal("second TryLock should fail when lock is held by another instance")
	}
	if !errors.Is(err, ErrHeld) {
		t.Errorf("expected error to wrap ErrHeld, got: %v", err)
	}
}

func TestTryLock_AlreadyHeld(t *testing.T) {
	l := newTestLock(t)

	if err := l.TryLock(); err != nil {
		t.Fatalf("TryLock failed: %v", err)
	}
	defer l.Unlock()

	err := l.TryLock()
	if err == nil {
		t.Fatal("double TryLock on same instance should fail")
	}
	if !errors.Is(err, ErrHeld) {
		t.Errorf("expected error to wrap ErrHeld, got: %v", err)
	}
}

func TestTryLock_InvalidPath(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), "no-such-dir", "test.lock"))

	err := l.TryLock()
	if err == nil {
		l.Unlock()
		t.Fatal("TryLock should fail for non-existent parent directory")
	}
	if !strings.Contains(err.Error(), "open lock file") {
		t.Errorf("error should mention 'open lock file', got: %v", err)
	}
}

func TestUnlock_ReleasesLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")

	l1 := New(path)
	if err := l1.TryLock(); err != nil {
		t.Fatalf("TryLock failed: %v", err)
	}
	if err := l1.Unlock(); err != nil {
		t.Fatalf("Unlock failed: %v", err)
	}

	l2 := New(path)
	if err := l2.TryLock(); err != nil {
		t.Fatalf("TryLock after Unlock should succeed: %v", err)
	}
	defer l2.Unlock()
}

func TestUnlock_KeepsFileOnDisk(t *testing.T) {
	l := newTestLock(t)

	if err := l.TryLock(); err != nil {
		t.Fatalf("TryLock failed: %v", err)
	}
	path := l.Path()
	if err := l.Unlock(); err != nil {
		t.Fatalf("Unlock failed: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("lock file should remain on disk after Unlock")
	}
}

func TestUnlock_Idempotent(t *testing.T) {
	l := newTestLock(t)

	if err := l.Unlock(); err != nil {
		t.Fatalf("Unlock without lock should not error: %v", err)
	}

	if err := l.TryLock(); err != nil {
		t.Fatalf("TryLock failed: %v", err)
	}
	if err := l.Unlock(); err != nil {
		t.Fatalf("first Unlock failed: %v", err)
	}
	if err := l.Unlock(); err != nil {
		t.Fatalf("second Unlock should not error: %v", err)
	}
}

func TestPath(t *testing.T) {
	l := New("/tmp/test.lock")
	if l.Path() != "/tmp/test.lock" {
		t.Errorf("Path() = %q, want /tmp/test.lock", l.Path())
	}
}

func TestForSubscribe(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	l, err := ForSubscribe("cli_test123")
	if err != nil {
		t.Fatalf("ForSubscribe failed: %v", err)
	}

	expected := filepath.Join(dir, "locks", "subscribe_cli_test123.lock")
	if l.Path() != expected {
		t.Errorf("Path() = %q, want %q", l.Path(), expected)
	}

	lockDir := filepath.Join(dir, "locks")
	if _, err := os.Stat(lockDir); os.IsNotExist(err) {
		t.Error("locks directory should be created by ForSubscribe")
	}
}

func TestForSubscribe_SanitizesAppID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	for _, tt := range []struct {
		name     string
		appID    string
		wantBase string
	}{
		{"path traversal", "../../tmp/evil", "subscribe_.._.._tmp_evil.lock"},
		{"slashes", "cli/app/id", "subscribe_cli_app_id.lock"},
		{"normal id", "cli_a1b2c3", "subscribe_cli_a1b2c3.lock"},
		{"special chars", "app@id:123", "subscribe_app_id_123.lock"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l, err := ForSubscribe(tt.appID)
			if err != nil {
				t.Fatalf("ForSubscribe(%q) failed: %v", tt.appID, err)
			}
			gotBase := filepath.Base(l.Path())
			if gotBase != tt.wantBase {
				t.Errorf("Base(Path()) = %q, want %q", gotBase, tt.wantBase)
			}
			locksDir := filepath.Join(dir, "locks")
			if !strings.HasPrefix(l.Path(), locksDir) {
				t.Errorf("path %q escapes locks dir %q", l.Path(), locksDir)
			}
		})
	}
}

func TestForSubscribe_RejectsEmptyAppID(t *testing.T) {
	_, err := ForSubscribe("")
	if err == nil {
		t.Fatal("ForSubscribe should reject empty app ID")
	}
}

func TestErrHeld_RelockAfterUnlock(t *testing.T) {
	l := newTestLock(t)
	if err := l.TryLock(); err != nil {
		t.Fatalf("first TryLock failed: %v", err)
	}
	if err := l.Unlock(); err != nil {
		t.Fatalf("Unlock failed: %v", err)
	}
	if err := l.TryLock(); err != nil {
		t.Errorf("TryLock after Unlock should succeed; got: %v", err)
	}
	l.Unlock()
}
