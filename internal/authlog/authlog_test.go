// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package authlog

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/vfs"
)

// TestAuthLogDir_UsesValidatedLogDirEnv verifies that a valid absolute
// LARKSUITE_CLI_LOG_DIR is normalized and used as the auth log directory.
func TestAuthLogDir_UsesValidatedLogDirEnv(t *testing.T) {
	base := t.TempDir()
	base, _ = filepath.EvalSymlinks(base)
	t.Setenv("LARKSUITE_CLI_LOG_DIR", filepath.Join(base, "logs", "..", "auth"))
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", "")

	logger := New(Options{RuntimeDir: func() string { return t.TempDir() }})
	got := logger.logDir()
	want := filepath.Join(base, "auth")
	if got != want {
		t.Fatalf("authLogDir() = %q, want %q", got, want)
	}
}

// TestAuthLogDir_InvalidLogDirFallsBackToConfigDir verifies that an invalid
// LARKSUITE_CLI_LOG_DIR falls back to LARKSUITE_CLI_CONFIG_DIR/logs.
func TestAuthLogDir_InvalidLogDirFallsBackToConfigDir(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_LOG_DIR", "relative-logs")
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	logger := New(Options{RuntimeDir: func() string { return configDir }})
	got := logger.logDir()
	want := filepath.Join(configDir, "logs")
	if got != want {
		t.Fatalf("authLogDir() = %q, want %q", got, want)
	}
}

// TestShared_ReturnsOneLoggerPerProcess pins the property the callers depend on:
// every call site must observe the same logger, otherwise each one opens its own
// file handle and re-runs the log prune.
func TestShared_ReturnsOneLoggerPerProcess(t *testing.T) {
	restoreShared(t)

	first := Shared()
	if first == nil {
		t.Fatal("Shared() returned nil")
	}
	if second := Shared(); second != first {
		t.Fatalf("Shared() returned a different logger on the second call: %p then %p", first, second)
	}
}

// TestSetShared_InstalledLoggerWins covers the startup wiring: the command
// factory installs a workspace-aware logger and later callers must get it
// instead of the pre-workspace fallback.
func TestSetShared_InstalledLoggerWins(t *testing.T) {
	restoreShared(t)

	installed := New(Options{RuntimeDir: func() string { return "workspace-dir" }})
	SetShared(installed)
	if got := Shared(); got != installed {
		t.Fatalf("Shared() = %p, want the installed logger %p", got, installed)
	}

	// A nil install must not wipe the configured logger.
	SetShared(nil)
	if got := Shared(); got != installed {
		t.Fatalf("SetShared(nil) replaced the logger: got %p, want %p", got, installed)
	}
}

// TestLogger_WritesUnderTheConfiguredRuntimeDir guards the directory the logger
// picks. A caller that supplies a workspace-aware RuntimeDir must get its lines
// there; falling back to the process default silently splits one investigation
// across two files.
func TestLogger_WritesUnderTheConfiguredRuntimeDir(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_LOG_DIR", "")
	runtimeDir := t.TempDir()
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)

	logger := New(Options{
		RuntimeDir: func() string { return runtimeDir },
		Now:        func() time.Time { return now },
		Args:       func() []string { return []string{"lark-cli", "auth", "login"} },
	})
	logger.LogResponse("/open-apis/authen/v1/user_info", 200, "logid-1")
	logger.LogError("keychain", "Get", errors.New("keychain locked"))

	path := filepath.Join(runtimeDir, "logs", "auth-2026-07-25.log")
	body, err := vfs.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	for _, want := range []string{
		"auth-response:", "status=200", "x-tt-logid=logid-1",
		"auth-error:", "component=keychain", "op=Get", "keychain locked",
		"cmdline=lark-cli auth login",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("log is missing %q:\n%s", want, body)
		}
	}
}

// TestLogger_NilReceiverIsInert covers the guard both entry points carry, so a
// caller holding no logger cannot turn a diagnostic into a crash.
func TestLogger_NilReceiverIsInert(t *testing.T) {
	var logger *Logger
	logger.LogResponse("/path", 500, "logid")
	logger.LogError("keychain", "Get", errors.New("boom"))
}

// TestCleanupOldLogs_PrunesOnlyExpiredAuthLogs pins the retention window and the
// filename patterns it is allowed to touch.
func TestCleanupOldLogs_PrunesOnlyExpiredAuthLogs(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	survives := map[string]bool{
		"auth-2026-07-25.log":  true,  // today
		"auth-2026-07-18.log":  true,  // exactly at the seven-day cutoff
		"auth-2026-07-17.log":  false, // past the cutoff
		"auth-not-a-date.log":  true,  // unparsable, left alone
		"other-2020-01-01.log": true,  // not an auth log
	}
	for name := range survives {
		if err := vfs.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	cleanupOldLogs(dir, now)

	for name, want := range survives {
		_, err := vfs.Stat(filepath.Join(dir, name))
		if got := err == nil; got != want {
			t.Errorf("%s present = %v, want %v", name, got, want)
		}
	}
}

// TestSetShared_FirstExplicitInstallWins covers a factory being constructed more
// than once in one process. A later install must not swap the logger: that would
// open a second file and move subsequent lines to another workspace directory.
func TestSetShared_FirstExplicitInstallWins(t *testing.T) {
	restoreShared(t)

	first := New(Options{RuntimeDir: func() string { return "first" }})
	second := New(Options{RuntimeDir: func() string { return "second" }})
	SetShared(first)
	SetShared(second)

	if got := Shared(); got != first {
		t.Fatalf("Shared() = %p, want the first installed logger %p", got, first)
	}
}

// TestSetShared_ReleasesTheSupersededFallback pins the one replacement that is
// allowed: a lazily created fallback gives way to the first explicit install,
// and its file handle is released rather than held until the process exits.
func TestSetShared_ReleasesTheSupersededFallback(t *testing.T) {
	restoreShared(t)
	t.Setenv("LARKSUITE_CLI_LOG_DIR", "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	fallback := Shared()
	fallback.LogError("keychain", "Get", errors.New("before the factory exists"))
	fallback.mu.Lock()
	opened := fallback.file != nil
	fallback.mu.Unlock()
	if !opened {
		t.Fatal("fallback did not open a log file, so the release path is untested")
	}

	installed := New(Options{RuntimeDir: func() string { return t.TempDir() }})
	SetShared(installed)

	if got := Shared(); got != installed {
		t.Fatalf("Shared() = %p, want the installed logger %p", got, installed)
	}
	fallback.mu.Lock()
	defer fallback.mu.Unlock()
	if fallback.file != nil {
		t.Error("superseded fallback still holds its file handle")
	}
	if fallback.logger != nil {
		t.Error("superseded fallback can still write")
	}
}

// TestFormatAuthCmdline_DropsEverythingFromTheFirstFlag is the regression guard
// for the rule this replaced. Keeping the first three arguments was safe only
// while no sensitive flag appeared early; a global flag in front of the
// subcommand put its value straight into the file.
func TestFormatAuthCmdline_DropsEverythingFromTheFirstFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "flag before the subcommand",
			args: []string{"lark-cli", "--token=super-secret", "auth", "login"},
			want: "lark-cli ...",
		},
		{
			name: "separated flag value",
			args: []string{"lark-cli", "auth", "login", "--device-code", "device-secret"},
			want: "lark-cli auth login ...",
		},
		{
			name: "absolute install path is reduced to the binary name",
			args: []string{"/opt/internal-tools/build-42/lark-cli", "auth", "status"},
			want: "lark-cli auth status",
		},
		{
			name: "no flags at all",
			args: []string{"lark-cli", "auth", "status"},
			want: "lark-cli auth status",
		},
		{
			// `api <method> <path>` puts a document token in the fourth word.
			// Stopping at the first flag is not enough on its own; the word cap
			// is what keeps a positional identifier out of the file.
			name: "positional resource identifier past the cap",
			args: []string{"lark-cli", "api", "GET", "/open-apis/docx/v1/documents/doccnDOCTOKEN"},
			want: "lark-cli api GET ...",
		},
		{
			name: "cap applies before any flag appears",
			args: []string{"lark-cli", "drive", "upload", "/home/me/private/quarterly.xlsx", "--to", "folder"},
			want: "lark-cli drive upload ...",
		},
		{
			// Generated service commands are one level deeper than the cap, so
			// their last word is lost. Recorded here so the trade-off is visible:
			// widening the cap to keep it would also admit the first positional
			// argument.
			name: "generated service command loses its verb to the cap",
			args: []string{"lark-cli", "drive", "file.comments", "create_v2"},
			want: "lark-cli drive file.comments ...",
		},
		{
			name: "empty",
			args: nil,
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatAuthCmdline(tc.args)
			if got != tc.want {
				t.Fatalf("FormatAuthCmdline(%q) = %q, want %q", tc.args, got, tc.want)
			}
			for _, arg := range tc.args {
				for _, marker := range []string{"secret", "TOKEN", "private"} {
					if strings.Contains(arg, marker) && strings.Contains(got, marker) {
						t.Fatalf("FormatAuthCmdline leaked %q into %q", arg, got)
					}
				}
			}
		})
	}
}

// TestAuthLogDir_RejectedOverrideWarns covers the one case where staying silent
// misleads: the caller set the directory explicitly and it was refused.
func TestAuthLogDir_RejectedOverrideWarns(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_LOG_DIR", "relative-logs")
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	logger := New(Options{RuntimeDir: func() string { return configDir }})
	warning := captureStderr(t, func() { _ = logger.logDir() })

	for _, want := range []string{"LARKSUITE_CLI_LOG_DIR", "default directory"} {
		if !strings.Contains(warning, want) {
			t.Errorf("warning %q does not mention %q", warning, want)
		}
	}
}

// TestAuthLogDir_AcceptedOverrideStaysQuiet keeps the warning scoped to the
// failure: a usable override must not print anything.
func TestAuthLogDir_AcceptedOverrideStaysQuiet(t *testing.T) {
	base := t.TempDir()
	base, _ = filepath.EvalSymlinks(base)
	t.Setenv("LARKSUITE_CLI_LOG_DIR", filepath.Join(base, "auth"))

	logger := New(Options{RuntimeDir: func() string { return base }})
	if warning := captureStderr(t, func() { _ = logger.logDir() }); warning != "" {
		t.Fatalf("a usable override printed %q", warning)
	}
}

// captureStderr redirects os.Stderr for the duration of fn and returns what was
// written to it.
//
// This one uses os rather than vfs on purpose. os.Pipe and os.Stderr are process
// contracts, not filesystem access, so vfs neither wraps them nor should: the
// code under test writes to the real stderr and there is nothing for a
// substituted vfs.DefaultFS to intercept. Assertions that touch files stay on
// vfs so they follow whatever the implementation uses.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	previous := os.Stderr
	os.Stderr = writer
	defer func() { os.Stderr = previous }()

	fn()
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	return string(out)
}

// restoreShared isolates each test from the process-wide logger.
func restoreShared(t *testing.T) {
	t.Helper()

	sharedMu.Lock()
	previousLog, previousInstalled := sharedLog, sharedInstalled
	sharedLog, sharedInstalled = nil, false
	sharedMu.Unlock()

	t.Cleanup(func() {
		sharedMu.Lock()
		sharedLog, sharedInstalled = previousLog, previousInstalled
		sharedMu.Unlock()
	})
}
