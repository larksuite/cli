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

// TestDrive_PullDryRun locks in the request shape the +pull shortcut emits
// under --dry-run: the real CLI binary is invoked end-to-end, so flag
// parsing, Validate (still runs in dry-run mode), and the dry-run renderer
// all execute. The printed envelope is then inspected for GET method,
// list-files URL, the folder_token parameter, and key phrases from Desc.
//
// Fake credentials are sufficient because --dry-run short-circuits before
// any real network call.
func TestDrive_PullDryRun(t *testing.T) {
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
			"drive", "+pull",
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
}

// TestDrive_PullDryRunRejectsAbsoluteLocalDir confirms the path validator
// runs in the real binary's Validate stage and surfaces a structured error
// referencing --local-dir.
func TestDrive_PullDryRunRejectsAbsoluteLocalDir(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+pull",
			"--local-dir", "/etc",
			"--folder-token", "fldcnE2E001",
			"--dry-run",
		},
		WorkDir:   t.TempDir(),
		DefaultAs: "user",
	})
	require.NoError(t, err)
	if result.ExitCode == 0 {
		t.Fatalf("absolute --local-dir must be rejected, got exit=0\nstdout:\n%s", result.Stdout)
	}
	combined := result.Stdout + "\n" + result.Stderr
	if !strings.Contains(combined, "--local-dir") {
		t.Fatalf("expected --local-dir in error, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
}

// TestDrive_PullDryRunRejectsDeleteLocalWithoutYes locks in the safety
// guard: --delete-local without --yes must be refused upfront, even under
// --dry-run, so an unintended delete flag never silently slides through.
func TestDrive_PullDryRunRejectsDeleteLocalWithoutYes(t *testing.T) {
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
			"drive", "+pull",
			"--local-dir", "local",
			"--folder-token", "fldcnE2E001",
			"--delete-local",
			"--dry-run",
		},
		WorkDir:   workDir,
		DefaultAs: "user",
	})
	require.NoError(t, err)
	if result.ExitCode == 0 {
		t.Fatalf("--delete-local without --yes must be rejected, got exit=0\nstdout:\n%s", result.Stdout)
	}
	combined := result.Stdout + "\n" + result.Stderr
	if !strings.Contains(combined, "--yes") {
		t.Fatalf("expected --yes hint in error, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
}

// TestDrive_PullDryRunRejectsMissingFolderToken confirms cobra's
// required-flag enforcement runs before our custom Validate.
func TestDrive_PullDryRunRejectsMissingFolderToken(t *testing.T) {
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
			"drive", "+pull",
			"--local-dir", "local",
			"--dry-run",
		},
		WorkDir:   workDir,
		DefaultAs: "user",
	})
	require.NoError(t, err)
	if result.ExitCode == 0 {
		t.Fatalf("missing --folder-token must be rejected, got exit=0\nstdout:\n%s", result.Stdout)
	}
	combined := result.Stdout + "\n" + result.Stderr
	if !strings.Contains(combined, "folder-token") {
		t.Fatalf("expected folder-token in error, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
}
