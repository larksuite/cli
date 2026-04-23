// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAssertSecurePath_NonAbsolutePath(t *testing.T) {
	_, err := AssertSecurePath(AuditParams{
		TargetPath:        "relative/path.txt",
		Label:             "test",
		AllowInsecurePath: true,
	})
	if err == nil {
		t.Fatal("expected error for non-absolute path, got nil")
	}
	want := fmt.Sprintf("test: path must be absolute, got %q", "relative/path.txt")
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestAssertSecurePath_FileDoesNotExist(t *testing.T) {
	nonexistent := filepath.Join(t.TempDir(), "nonexistent.txt")
	_, err := AssertSecurePath(AuditParams{
		TargetPath:        nonexistent,
		Label:             "test",
		AllowInsecurePath: true,
	})
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
	wantPrefix := fmt.Sprintf("test: cannot stat %q: ", nonexistent)
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Errorf("error = %q, want prefix %q", err.Error(), wantPrefix)
	}
}

func TestAssertSecurePath_ValidAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "valid.txt")
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	got, err := AssertSecurePath(AuditParams{
		TargetPath:        p,
		Label:             "test",
		AllowInsecurePath: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != p {
		t.Errorf("got %q, want %q", got, p)
	}
}

func TestAssertSecurePath_WorldWritable_Rejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests not applicable on Windows")
	}

	dir := t.TempDir()
	p := filepath.Join(dir, "insecure.txt")
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := os.Chmod(p, 0o666); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	_, err := AssertSecurePath(AuditParams{
		TargetPath:            p,
		Label:                 "test",
		AllowInsecurePath:     false,
		AllowReadableByOthers: true, // only test writable check
	})
	if err == nil {
		t.Fatal("expected error for world-writable file, got nil")
	}
	want := fmt.Sprintf("test: path %q is world-writable (mode 0666)", p)
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestAssertSecurePath_AllowInsecurePath_Bypasses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests not applicable on Windows")
	}

	dir := t.TempDir()
	p := filepath.Join(dir, "insecure.txt")
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := os.Chmod(p, 0o666); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	got, err := AssertSecurePath(AuditParams{
		TargetPath:        p,
		Label:             "test",
		AllowInsecurePath: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != p {
		t.Errorf("got %q, want %q", got, p)
	}
}

func TestAssertSecurePath_DirectoryRejected(t *testing.T) {
	dir := t.TempDir()
	_, err := AssertSecurePath(AuditParams{
		TargetPath:        dir,
		Label:             "test",
		AllowInsecurePath: true,
	})
	if err == nil {
		t.Fatal("expected error for directory path, got nil")
	}
	want := fmt.Sprintf("test: path %q is a directory, not a file", dir)
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestAssertSecurePath_GroupWritable_Rejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests not applicable on Windows")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "groupw.txt")
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(p, 0o620); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	_, err := AssertSecurePath(AuditParams{
		TargetPath:            p,
		Label:                 "test",
		AllowInsecurePath:     false,
		AllowReadableByOthers: true,
	})
	if err == nil {
		t.Fatal("expected error for group-writable file, got nil")
	}
	want := fmt.Sprintf("test: path %q is group-writable (mode 0620)", p)
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestAssertSecurePath_WorldReadable_Rejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests not applicable on Windows")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "worldr.txt")
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(p, 0o604); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	_, err := AssertSecurePath(AuditParams{
		TargetPath:            p,
		Label:                 "test",
		AllowInsecurePath:     false,
		AllowReadableByOthers: false,
	})
	if err == nil {
		t.Fatal("expected error for world-readable file, got nil")
	}
	want := fmt.Sprintf("test: path %q is world-readable (mode 0604)", p)
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestAssertSecurePath_AllowReadableByOthers_Passes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests not applicable on Windows")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "readable.txt")
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	got, err := AssertSecurePath(AuditParams{
		TargetPath:            p,
		Label:                 "test",
		AllowInsecurePath:     false,
		AllowReadableByOthers: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != p {
		t.Errorf("got %q, want %q", got, p)
	}
}

func TestAssertSecurePath_OwnerUID_Valid(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("owner UID tests not applicable on Windows")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "owned.txt")
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := AssertSecurePath(AuditParams{
		TargetPath:            p,
		Label:                 "test",
		AllowInsecurePath:     false,
		AllowReadableByOthers: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != p {
		t.Errorf("got %q, want %q", got, p)
	}
}

func TestAssertSecurePath_Symlink_Rejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests not applicable on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(target, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, err := AssertSecurePath(AuditParams{
		TargetPath:        link,
		Label:             "test",
		AllowSymlinkPath:  false,
		AllowInsecurePath: true,
	})
	if err == nil {
		t.Fatal("expected error for symlink with AllowSymlinkPath=false, got nil")
	}
	want := fmt.Sprintf("test: path %q is a symlink (not allowed)", link)
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestAssertSecurePath_Symlink_Allowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests not applicable on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(target, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	got, err := AssertSecurePath(AuditParams{
		TargetPath:        link,
		Label:             "test",
		AllowSymlinkPath:  true,
		AllowInsecurePath: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// On macOS /var → /private/var, so compare resolved paths
	wantResolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("EvalSymlinks(target): %v", err)
	}
	if got != wantResolved {
		t.Errorf("got %q, want resolved %q", got, wantResolved)
	}
}

func TestAssertSecurePath_TrustedDirs_ExactMatch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := AssertSecurePath(AuditParams{
		TargetPath:        p,
		Label:             "test",
		TrustedDirs:       []string{p},
		AllowInsecurePath: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != p {
		t.Errorf("got %q, want %q", got, p)
	}
}

func TestAssertSecurePath_TrustedDirs(t *testing.T) {
	trustedDir := t.TempDir()
	untrustedDir := t.TempDir()

	trustedFile := filepath.Join(trustedDir, "secret.txt")
	if err := os.WriteFile(trustedFile, []byte("data"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	untrustedFile := filepath.Join(untrustedDir, "secret.txt")
	if err := os.WriteFile(untrustedFile, []byte("data"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	// File outside trusted dir should fail
	_, err := AssertSecurePath(AuditParams{
		TargetPath:        untrustedFile,
		Label:             "test",
		TrustedDirs:       []string{trustedDir},
		AllowInsecurePath: true,
	})
	if err == nil {
		t.Fatal("expected error for file outside trusted dir, got nil")
	}
	want := fmt.Sprintf("test: path %q is not inside any trusted directory", untrustedFile)
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}

	// File inside trusted dir should pass
	got, err := AssertSecurePath(AuditParams{
		TargetPath:        trustedFile,
		Label:             "test",
		TrustedDirs:       []string{trustedDir},
		AllowInsecurePath: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != trustedFile {
		t.Errorf("got %q, want %q", got, trustedFile)
	}
}
