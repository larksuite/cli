// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

		// ── GIVEN: empty or blank paths → THEN: rejected ──
		{"empty path", "", true},
		{"blank path", "   ", true},

		// ── GIVEN: path traversal via .. → THEN: rejected ──
		{"dot-dot escape", "../../.ssh/authorized_keys", true},
		{"dot-dot mid path", "subdir/../../etc/passwd", true},
		{"triple dot-dot", "../../../etc/shadow", true},

		// ── GIVEN: absolute paths outside the built-in allowlist → THEN: rejected ──
		{"absolute path denylist", "/etc/passwd", true},
		{"allow prefix boundary", "/tmpfoo/x", true},
		{"absolute path windows drive", `C:\Users\agent\secret.txt`, true},
		{"absolute path windows drive slash", "C:/Users/agent/secret.txt", true},
		{"absolute path windows rooted", `\Users\agent\secret.txt`, true},
		{"absolute path windows unc", `\\server\share\secret.txt`, true},

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

		// ── GIVEN: ~ expands to the home directory, which is not an allow
		// root by itself (only ~/files is) → THEN: rejected ──
		{"tilde path outside allowlist", "~/file.txt", true},
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

func TestLocalInputPath_AllowsLocalNamespaceWithoutRewriting(t *testing.T) {
	for _, input := range []string{
		"/tmp/report.pdf",
		"../outside/report.pdf",
		"./report.pdf",
		"nested/../report.pdf",
		`C:\Users\agent\report.pdf`,
		"报告.pdf",
	} {
		t.Run(input, func(t *testing.T) {
			got, err := LocalInputPath(input)
			if err != nil {
				t.Fatalf("LocalInputPath(%q) error = %v", input, err)
			}
			if got != input {
				t.Fatalf("LocalInputPath(%q) = %q, want path preserved verbatim", input, got)
			}
		})
	}
}

func TestWindowsNonLocalNamespace(t *testing.T) {
	for _, input := range []string{
		`\\server\share\report.pdf`,
		`//server/share/report.pdf`,
		`\\.\pipe\upload`,
		`\\?\C:\Users\agent\report.pdf`,
		`\\?\UNC\server\share\report.pdf`,
		`\??\C:\Users\agent\report.pdf`,
	} {
		if !isWindowsNonLocalNamespace(input) {
			t.Errorf("isWindowsNonLocalNamespace(%q) = false, want true", input)
		}
	}

	for _, input := range []string{
		`C:\Users\agent\report.pdf`,
		`C:/Users/agent/report.pdf`,
		`..\outside\report.pdf`,
		`.\report.pdf`,
	} {
		if isWindowsNonLocalNamespace(input) {
			t.Errorf("isWindowsNonLocalNamespace(%q) = true, want false", input)
		}
	}
}

func TestLocalInputPath_RejectsEmptyControlAndDangerousUnicode(t *testing.T) {
	for _, input := range []string{
		"",
		"   ",
		"file\x00.txt",
		"file\tname.txt",
		"file\nname.txt",
		"file\rname.txt",
		"file\u202Ename.txt",
		"file\u200Bname.txt",
	} {
		t.Run(fmt.Sprintf("%q", input), func(t *testing.T) {
			if _, err := LocalInputPath(input); err == nil {
				t.Fatalf("LocalInputPath(%q) unexpectedly succeeded", input)
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

func TestSafeInputPath_AllowsBuiltinTmpAbsolutePath(t *testing.T) {
	// GIVEN: a real file under the built-in /tmp allow root (the system temp
	// dir on Windows)
	f, err := os.CreateTemp(tmpRoot(), "upload-test-*.bin")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	tmpPath := f.Name()
	f.Close()
	t.Cleanup(func() { os.Remove(tmpPath) })

	// WHEN: SafeInputPath validates the absolute temp path
	got, err := SafeInputPath(tmpPath)

	// THEN: accepted — /tmp is on the built-in allowlist
	if err != nil {
		t.Fatalf("SafeInputPath(%q) error = %v, want nil", tmpPath, err)
	}
	want, _ := filepath.EvalSymlinks(tmpPath)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSafeOutputPath_AllowsHomeFilesWithoutTouchingDisk(t *testing.T) {
	// GIVEN: a target under ~/files that does not exist (validation must not
	// require the directory to exist, and must not create it)
	// trustedHome, not $HOME: the policy derives ~/files from the account
	// record, and on a host where the two disagree (sudo, a container) the
	// $HOME-based target would not be the allow root under test.
	home, err := trustedHome()
	if err != nil {
		t.Skipf("no trusted home: %v", err)
	}
	target := filepath.Join(home, "files", "safeoutput-test-nonexistent", "out.xlsx")

	// WHEN: SafeOutputPath validates the path
	_, err = SafeOutputPath(target)

	// THEN: accepted — ~/files is on the built-in allowlist
	if err != nil {
		t.Fatalf("SafeOutputPath(%q) error = %v, want nil", target, err)
	}
	// The validation must not have created the directory it approved.
	if _, statErr := os.Stat(filepath.Dir(target)); !os.IsNotExist(statErr) {
		t.Errorf("validation created the target directory, stat err = %v", statErr)
	}
}

func TestSafePath_DenyBeatsCwd(t *testing.T) {
	// GIVEN: the process works inside a denylisted directory
	if runtime.GOOS == "windows" {
		t.Skip("/etc does not exist on Windows")
	}
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	if err := os.Chdir("/etc"); err != nil {
		t.Skipf("cannot chdir into /etc: %v", err)
	}

	// WHEN: a cwd-relative path is validated
	_, err := SafeInputPath("./passwd")

	// THEN: rejected — the denylist wins over the cwd allow root
	if err == nil {
		t.Fatal("expected denylist rejection inside /etc, got nil")
	}
	if !strings.Contains(err.Error(), "denylist") {
		t.Errorf("error should mention the denylist, got: %v", err)
	}
}

func TestLocalInputPath_DeniesProtectedDirs(t *testing.T) {
	// GIVEN/WHEN: the relaxed tier validates a denylisted absolute path
	_, err := LocalInputPath(denylistedAbsolutePath(t))

	// THEN: rejected — the denylist applies even without allowlist containment
	if err == nil {
		t.Fatal("expected denylist rejection, got nil")
	}
}

func TestSafeUploadPath_RejectsNonTempAbsolutePath(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
	}{
		{"absolute path unix", "/etc/passwd"},
		{"absolute path windows drive", `C:\Users\agent\secret.txt`},
		{"absolute path windows drive slash", "C:/Users/agent/secret.txt"},
		{"absolute path windows rooted", `\Users\agent\secret.txt`},
		{"absolute path windows unc", `\\server\share\secret.txt`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// WHEN / THEN: SafeInputPath rejects absolute paths on every platform.
			_, err := SafeInputPath(tt.input)
			if err == nil {
				t.Errorf("expected error for absolute path %q, got nil", tt.input)
			}
		})
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
