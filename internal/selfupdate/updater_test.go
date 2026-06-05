// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

// executableTestFS mocks vfs for tests that still need vfs.Executable.
type executableTestFS struct {
	vfs.OsFs
	exe string
}

func (f executableTestFS) Executable() (string, error) { return f.exe, nil }

// lookPathMock patches execLookPath within VerifyBinary for controlled testing.
// Do not use t.Parallel() in tests that install this mock — it mutates a package-level var.
type lookPathMock struct {
	oldLookPath func(string) (string, error)
	result      string
	resultErr   error
}

func (m *lookPathMock) install(bin string) {
	m.oldLookPath = execLookPath
	execLookPath = func(name string) (string, error) {
		if name == bin {
			return m.result, m.resultErr
		}
		return m.oldLookPath(name)
	}
}

func (m *lookPathMock) restore() {
	execLookPath = m.oldLookPath
}

func TestResolveExe(t *testing.T) {
	u := New()
	p, err := u.resolveExe()
	if err != nil {
		t.Fatalf("resolveExe() error: %v", err)
	}
	if !filepath.IsAbs(p) {
		t.Errorf("expected absolute path, got: %s", p)
	}
}

func TestPrepareSelfReplace_ReturnsNoError(t *testing.T) {
	u := New()
	restore, err := u.PrepareSelfReplace()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	restore()
}

func TestCleanupStaleFiles_NoPanic(t *testing.T) {
	u := New()
	u.CleanupStaleFiles()
}

func TestVerifyBinaryLookPath(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "lark-cli")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo \"lark-cli version 2.1.0\"; exit 0; fi\nexit 12\n"
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatalf("write test binary: %v", err)
	}

	mock := &lookPathMock{result: bin}
	mock.install("lark-cli")
	t.Cleanup(mock.restore)

	if err := New().VerifyBinary("2.1.0"); err != nil {
		t.Fatalf("VerifyBinary(2.1.0) error = %v, want nil", err)
	}

	if err := New().VerifyBinary("3.0.0"); err == nil {
		t.Fatal("VerifyBinary(mismatched) expected error, got nil")
	}

	// Regression: version must match exactly (not substring / prefix).
	if err := New().VerifyBinary("0.0"); err == nil {
		t.Fatal("VerifyBinary(substring-style mismatch) expected error, got nil")
	}
	if err := New().VerifyBinary("12.1.0"); err == nil {
		t.Fatal("VerifyBinary(prefix-style mismatch) expected error, got nil")
	}
}

func TestVerifyBinaryLookPathNotFound(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	mock := &lookPathMock{result: "", resultErr: fmt.Errorf("not found")}
	mock.install("lark-cli")
	t.Cleanup(mock.restore)

	oldFS := vfs.DefaultFS
	t.Cleanup(func() { vfs.DefaultFS = oldFS })
	// Without this, VerifyBinary would fall back to the real test binary, which
	// is not a lark-cli --version implementation.
	vfs.DefaultFS = executableTestFS{exe: filepath.Join(t.TempDir(), "missing-lark-cli")}

	if err := New().VerifyBinary("2.0.0"); err == nil {
		t.Fatal("VerifyBinary(not-found) expected error, got nil")
	}
}

func TestVerifyBinaryFallbackExecutableWhenNotOnPath(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "lark-cli-abs")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo \"lark-cli version 2.1.0\"; exit 0; fi\nexit 12\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write test binary: %v", err)
	}

	mock := &lookPathMock{result: "", resultErr: fmt.Errorf("not on PATH")}
	mock.install("lark-cli")
	t.Cleanup(mock.restore)

	oldFS := vfs.DefaultFS
	t.Cleanup(func() { vfs.DefaultFS = oldFS })
	vfs.DefaultFS = executableTestFS{exe: bin}

	if err := New().VerifyBinary("2.1.0"); err != nil {
		t.Fatalf("VerifyBinary(fallback executable) error = %v, want nil", err)
	}
}

func TestVerifyBinaryEmptyOutput(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "lark-cli")
	script := "#!/bin/sh\necho\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0755); err != nil {
		t.Fatalf("write test binary: %v", err)
	}

	mock := &lookPathMock{result: bin}
	mock.install("lark-cli")
	t.Cleanup(mock.restore)

	if err := New().VerifyBinary("2.0.0"); err == nil {
		t.Fatal("VerifyBinary(empty output) expected error, got nil")
	}
}

func TestSkillsCommandsUseExpectedArgs(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Updater) *NpmResult
		want string
	}{
		{
			name: "list official primary",
			run: func(u *Updater) *NpmResult {
				return u.runSkillsListOfficial("https://open.feishu.cn")
			},
			want: "-y skills add https://open.feishu.cn --list",
		},
		{
			name: "list global",
			run: func(u *Updater) *NpmResult {
				return u.runSkillsListGlobal()
			},
			want: "-y skills ls -g",
		},
		{
			name: "list global json",
			run: func(u *Updater) *NpmResult {
				return u.ListGlobalSkillsJSON()
			},
			want: "-y skills ls -g --json",
		},
		{
			name: "install skill primary",
			run: func(u *Updater) *NpmResult {
				return u.runSkillsInstall("https://open.feishu.cn", []string{"lark-mail"})
			},
			want: "-y skills add https://open.feishu.cn -s lark-mail -g -y",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("uses a POSIX shell script")
			}
			dir := t.TempDir()
			script := filepath.Join(dir, "npx")
			logPath := filepath.Join(dir, "npx.log")
			if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \""+logPath+"\"\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

			result := tt.run(New())
			if result.Err != nil {
				t.Fatalf("command err = %v, want nil", result.Err)
			}
			raw, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(raw)) != tt.want {
				t.Fatalf("args = %q, want %q", strings.TrimSpace(string(raw)), tt.want)
			}
		})
	}
}

func TestListOfficialSkillsFallsBack(t *testing.T) {
	called := []string{}
	updater := &Updater{
		SkillsCommandOverride: func(args ...string) *NpmResult {
			called = append(called, strings.Join(args, " "))
			r := &NpmResult{}
			if strings.Contains(strings.Join(args, " "), "https://open.feishu.cn") {
				r.Err = fmt.Errorf("primary failed")
				return r
			}
			r.Stdout.WriteString("lark-calendar\n")
			return r
		},
	}

	result := updater.ListOfficialSkills()
	if result.Err != nil {
		t.Fatalf("ListOfficialSkills() err = %v, want nil", result.Err)
	}
	if len(called) != 2 {
		t.Fatalf("called %d commands, want 2: %#v", len(called), called)
	}
	if !strings.Contains(called[1], "larksuite/cli --list") {
		t.Fatalf("fallback call = %q, want larksuite/cli --list", called[1])
	}
}
