// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDrive_PushDryRun locks in the request shape the +push shortcut emits
// under --dry-run: the real CLI binary is invoked end-to-end, so flag
// parsing, Validate (still runs in dry-run mode), and the dry-run renderer
// all execute. The printed envelope is then inspected for GET method,
// list-files URL, the folder_token parameter, and key phrases from Desc.
//
// Fake credentials are sufficient because --dry-run short-circuits before
// any real network call.
func TestDrive_PushDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "local"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+push",
			"--local-dir", "local",
			"--folder-token", "fldcnE2E001",
			"--dry-run",
		},
		WorkDir:   workDir,
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := gjson.Get(out, "api.0.method").String(); got != "GET" {
		t.Fatalf("method = %q, want GET\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.url").String(); got != "/open-apis/drive/v1/files" {
		t.Fatalf("url = %q, want /open-apis/drive/v1/files\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "folder_token").String(); got != "fldcnE2E001" {
		t.Fatalf("folder_token = %q, want fldcnE2E001\nstdout:\n%s", got, out)
	}
	desc := gjson.Get(out, "description").String()
	if !strings.Contains(desc, "list --folder-token") {
		t.Fatalf("description missing list phrase, got %q\nstdout:\n%s", desc, out)
	}
	if !strings.Contains(desc, "upload") {
		t.Fatalf("description missing upload phrase, got %q\nstdout:\n%s", desc, out)
	}
}

// TestDrive_PushDryRunRejectsAbsoluteLocalDir confirms the path validator
// runs in the real binary's Validate stage and surfaces a structured error
// referencing --local-dir.
func TestDrive_PushDryRunRejectsAbsoluteLocalDir(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+push",
			"--local-dir", "/etc",
			"--folder-token", "fldcnE2E001",
			"--dry-run",
		},
		WorkDir:   t.TempDir(),
		DefaultAs: "user",
	})
	require.NoError(t, err)
	// Validate-stage rejection emits ExitValidation (2). A regression
	// that reclassified this as a generic api_error (1) or success (0)
	// would slip through a loose `!= 0` check, so assert the exact code.
	if result.ExitCode != 2 {
		t.Fatalf("absolute --local-dir must be rejected with exit=2 (Validate), got exit=%d\nstdout:\n%s\nstderr:\n%s", result.ExitCode, result.Stdout, result.Stderr)
	}
	combined := result.Stdout + "\n" + result.Stderr
	if !strings.Contains(combined, "--local-dir") {
		t.Fatalf("expected --local-dir in error, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
}

// TestDrive_PushDryRunRejectsDeleteRemoteWithoutYes locks in the safety
// guard: --delete-remote without --yes must be refused upfront, even
// under --dry-run, so an unintended delete flag never silently slides
// through.
func TestDrive_PushDryRunRejectsDeleteRemoteWithoutYes(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "local"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+push",
			"--local-dir", "local",
			"--folder-token", "fldcnE2E001",
			"--delete-remote",
			"--dry-run",
		},
		WorkDir:   workDir,
		DefaultAs: "user",
	})
	require.NoError(t, err)
	// Same exact-code reasoning as the absolute-path test: this is a
	// Validate-stage rejection so it must surface as ExitValidation (2).
	if result.ExitCode != 2 {
		t.Fatalf("--delete-remote without --yes must be rejected with exit=2 (Validate), got exit=%d\nstdout:\n%s\nstderr:\n%s", result.ExitCode, result.Stdout, result.Stderr)
	}
	combined := result.Stdout + "\n" + result.Stderr
	if !strings.Contains(combined, "--yes") {
		t.Fatalf("expected --yes hint in error, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
}

// TestDrive_PushDryRunAcceptsDeleteRemoteWithYes is the symmetric guard
// to TestDrive_PushDryRunRejectsDeleteRemoteWithoutYes: when --yes is
// passed alongside --delete-remote, Validate must accept the run and
// hand off to the dry-run renderer.
//
// Specifically pins the conditional scope pre-check added to Validate:
// when the resolver has no token / no scope metadata (the e2e setup
// uses fake credentials with no real auth state), runtime.EnsureScopes
// is a silent no-op so dry-run still emits its envelope. A regression
// where the pre-check incorrectly fired against an empty scope list
// would surface here as a non-zero exit and a missing_scope error.
func TestDrive_PushDryRunAcceptsDeleteRemoteWithYes(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "local"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+push",
			"--local-dir", "local",
			"--folder-token", "fldcnE2E001",
			"--delete-remote",
			"--yes",
			"--dry-run",
		},
		WorkDir:   workDir,
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := gjson.Get(out, "api.0.method").String(); got != "GET" {
		t.Fatalf("method = %q, want GET\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "folder_token").String(); got != "fldcnE2E001" {
		t.Fatalf("folder_token = %q, want fldcnE2E001\nstdout:\n%s", got, out)
	}
	// No structured error envelope on stdout/stderr — the conditional
	// EnsureScopes call must not trip a missing_scope here.
	if strings.Contains(out, `"type": "missing_scope"`) || strings.Contains(result.Stderr, "missing_scope") {
		t.Fatalf("conditional scope pre-check fired in a no-credential env\nstdout:\n%s\nstderr:\n%s", out, result.Stderr)
	}
}

// TestDrive_PushDryRunRejectsMissingFolderToken confirms cobra's
// required-flag enforcement runs before our custom Validate.
func TestDrive_PushDryRunRejectsMissingFolderToken(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "local"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+push",
			"--local-dir", "local",
			"--dry-run",
		},
		WorkDir:   workDir,
		DefaultAs: "user",
	})
	require.NoError(t, err)
	// This is a cobra-level required-flag check that fires BEFORE our
	// Validate callback, so the exit code is cobra's generic flag-error
	// (1) — distinct from ExitValidation (2). Asserting the exact code
	// pins which layer rejected the run, which matters because a
	// regression that pushed required-flag validation into our own
	// Validate (changing the exit class to 2) would silently slip
	// through a loose `!= 0` check.
	if result.ExitCode != 1 {
		t.Fatalf("missing --folder-token must be rejected with exit=1 (cobra required-flag), got exit=%d\nstdout:\n%s\nstderr:\n%s", result.ExitCode, result.Stdout, result.Stderr)
	}
	combined := result.Stdout + "\n" + result.Stderr
	if !strings.Contains(combined, "folder-token") {
		t.Fatalf("expected folder-token in error, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
}

func TestDrive_PushDryRunAcceptsDuplicateRemoteStrategies(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	for _, strategy := range []string{"newest", "oldest"} {
		t.Run(strategy, func(t *testing.T) {
			workDir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(workDir, "local"), 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"drive", "+push",
					"--local-dir", "local",
					"--folder-token", "fldcnE2E001",
					"--on-duplicate-remote", strategy,
					"--dry-run",
				},
				WorkDir:   workDir,
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			if got := gjson.Get(out, "api.0.method").String(); got != "GET" {
				t.Fatalf("method = %q, want GET\nstdout:\n%s", got, out)
			}
			if got := gjson.Get(out, "folder_token").String(); got != "fldcnE2E001" {
				t.Fatalf("folder_token = %q, want fldcnE2E001\nstdout:\n%s", got, out)
			}
		})
	}
}
