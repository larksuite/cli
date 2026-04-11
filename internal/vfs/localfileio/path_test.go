// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeOutputPath_RejectsPathTraversalAndDangerousInput(t *testing.T) {
	for _, tt := range []struct {
		name    string
		input   string
		wantErr bool
	}{
		// ── GIVEN: normal relative paths → THEN: allowed ──
		{"normal file", "report.xlsx", false},
		{"subdir file", "output/report.xlsx", false},
		{"current dir explicit", "./file.txt", false},
		{"nested subdir", "a/b/c/file.txt", false},
		{"dot in name", "my.report.v2.xlsx", false},
		{"space in name", "my file.txt", false},
		{"unicode normal", "报告.xlsx", false},
		{"dot-dot resolves to cwd", "subdir/..", false},

		// ── GIVEN: path traversal via .. → THEN: rejected ──
		{"dot-dot escape", "../../.ssh/authorized_keys", true},
		{"dot-dot mid path", "subdir/../../etc/passwd", true},
		{"triple dot-dot", "../../../etc/shadow", true},

		// ── GIVEN: absolute paths → THEN: rejected ──
		{"absolute path unix", "/etc/passwd", true},
		{"absolute path root", "/tmp/evil", true},

		// ── GIVEN: control characters in path → THEN: rejected ──
		{"null byte", "file\x00.txt", true},
		{"carriage return", "file\r.txt", true},
		{"bell char", "file\x07.txt", true},

		// ── GIVEN: dangerous Unicode in path → THEN: rejected ──
		{"bidi RLO", "file\u202Ename.txt", true},
		{"zero width space", "file\u200Bname.txt", true},
		{"BOM char", "file\uFEFFname.txt", true},
		{"line separator", "file\u2028name.txt", true},
		{"bidi LRI", "file\u2066name.txt", true},

		// ── GIVEN: looks dangerous but is actually safe → THEN: allowed ──
		{"literal percent 2e", "%2e%2e/etc/passwd", false},
		{"tilde path", "~/file.txt", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// WHEN: SafeOutputPath validates the path
			_, err := SafeOutputPath(tt.input)

			// THEN: error matches expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("SafeOutputPath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestSafeOutputPath_ReturnsCanonicalAbsolutePath(t *testing.T) {
	// GIVEN: a clean temp directory as CWD
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// WHEN: SafeOutputPath validates a relative path
	got, err := SafeOutputPath("output/file.txt")

	// THEN: returns the canonical absolute path for subsequent I/O
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(dir, "output", "file.txt")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSafeOutputPath_RejectsSymlinkEscapingCWD(t *testing.T) {
	// GIVEN: a symlink in CWD pointing to /etc (outside CWD)
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)
	os.Symlink("/etc", filepath.Join(dir, "link-to-etc"))

	// WHEN: SafeOutputPath validates a path through the symlink
	_, err := SafeOutputPath("link-to-etc/passwd")

	// THEN: rejected because the resolved path is outside CWD
	if err == nil {
		t.Error("expected error for symlink escaping CWD, got nil")
	}
}

func TestSafeOutputPath_AllowsSymlinkWithinCWD(t *testing.T) {
	// GIVEN: a symlink in CWD pointing to a subdirectory within CWD
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)
	os.MkdirAll(filepath.Join(dir, "real"), 0755)
	os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "link"))

	// WHEN: SafeOutputPath validates a path through the internal symlink
	got, err := SafeOutputPath("link/file.txt")

	// THEN: allowed, resolved to the real path within CWD
	if err != nil {
		t.Fatalf("symlink within CWD should be allowed: %v", err)
	}
	want := filepath.Join(dir, "real", "file.txt")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSafeOutputPath_ResolvesAncestorSymlinkWhenParentMissing(t *testing.T) {
	// GIVEN: CWD contains a symlink "escape" → /etc, and the target path
	// goes through "escape/sub/file.txt" where "sub" does not exist.
	// The old code failed to resolve the symlink because the immediate
	// parent ("escape/sub") didn't exist, leaving resolved un-anchored.
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)
	os.Symlink("/etc", filepath.Join(dir, "escape"))

	// WHEN: SafeOutputPath validates a path through the symlink with missing intermediate dirs
	_, err := SafeOutputPath("escape/nonexistent/file.txt")

	// THEN: rejected — the resolved path is under /etc, outside CWD
	if err == nil {
		t.Error("expected error for symlink escaping CWD via non-existent parent, got nil")
	}
}

func TestSafeOutputPath_DeepNonExistentPathStaysInCWD(t *testing.T) {
	// GIVEN: a deeply nested non-existent path with no symlinks
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// WHEN: SafeOutputPath validates "a/b/c/d/file.txt" (none of a/b/c/d exist)
	got, err := SafeOutputPath("a/b/c/d/file.txt")

	// THEN: allowed, resolved to canonical path under CWD
	if err != nil {
		t.Fatalf("deep non-existent path within CWD should be allowed: %v", err)
	}
	want := filepath.Join(dir, "a", "b", "c", "d", "file.txt")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSafeUploadPath_AllowsTempFileAbsolutePath(t *testing.T) {
	// GIVEN: a real temp file (absolute path under os.TempDir())
	f, err := os.CreateTemp("", "upload-test-*.bin")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	tmpPath := f.Name()
	f.Close()
	t.Cleanup(func() { os.Remove(tmpPath) })

	// WHEN: SafeUploadPath validates the absolute temp path
	_, err = SafeInputPath(tmpPath)

	// THEN: absolute paths are rejected even in temp dir
	if err == nil {
		t.Fatal("expected error for absolute temp path, got nil")
	}
}

func TestSafeUploadPath_RejectsNonTempAbsolutePath(t *testing.T) {
	// GIVEN: an absolute path outside the temp directory
	// WHEN / THEN: SafeUploadPath rejects it
	_, err := SafeInputPath("/etc/passwd")
	if err == nil {
		t.Error("expected error for absolute non-temp path, got nil")
	}
}

func TestSafeUploadPath_AcceptsRelativePath(t *testing.T) {
	// GIVEN: a clean temp CWD with a real file
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	os.WriteFile(filepath.Join(dir, "upload.bin"), []byte("data"), 0600)

	// WHEN: SafeUploadPath validates a relative path to an existing file
	got, err := SafeInputPath("upload.bin")

	// THEN: accepted and returned as absolute canonical path
	if err != nil {
		t.Fatalf("SafeUploadPath(relative) error = %v", err)
	}
	want := filepath.Join(dir, "upload.bin")
	if got != want {
		t.Errorf("SafeUploadPath(relative) = %q, want %q", got, want)
	}
}

func Test_SafeInputPath_ErrorMessageContainsCorrectFlagName(t *testing.T) {
	// GIVEN: an absolute path

	// WHEN: SafeInputPath rejects it
	_, err := SafeInputPath("/etc/passwd")

	// THEN: error message mentions --file (not --output)
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("error should mention --file, got: %s", err.Error())
	}

	// WHEN: SafeOutputPath rejects it
	_, err = SafeOutputPath("/etc/passwd")

	// THEN: error message mentions --output (not --file)
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
	if !strings.Contains(err.Error(), "--output") {
		t.Errorf("error should mention --output, got: %s", err.Error())
	}
}
