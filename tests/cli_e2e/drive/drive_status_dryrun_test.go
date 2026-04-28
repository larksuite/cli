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

// TestDrive_StatusDryRun locks in the request shape the +status shortcut
// emits under --dry-run: the real CLI binary is invoked end-to-end, so the
// full flag-parsing, Validate (which still runs in dry-run mode), and the
// dry-run renderer all execute. The printed envelope is then inspected to
// confirm the GET method, list-files URL, and folder_token parameter, plus
// the descriptive text from Desc.
//
// Fake credentials are sufficient because --dry-run short-circuits before
// any network call.
func TestDrive_StatusDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	// Validate runs even under --dry-run, so we need a real --local-dir
	// inside the working directory; create one in a temp tree.
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "local"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+status",
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
	if !strings.Contains(desc, "Walk --local-dir") || !strings.Contains(desc, "SHA-256") {
		t.Fatalf("description missing key phrases, got %q\nstdout:\n%s", desc, out)
	}
}

// TestDrive_StatusDryRunRejectsAbsoluteLocalDir confirms that the
// --local-dir path validator runs in the real binary's Validate stage and
// surfaces a structured error referencing --local-dir (not the framework
// default --file).
func TestDrive_StatusDryRunRejectsAbsoluteLocalDir(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+status",
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
		t.Fatalf("expected --local-dir in error message, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
}

// TestDrive_StatusDryRunRejectsMissingFolderToken confirms cobra's
// required-flag enforcement runs before our custom Validate.
func TestDrive_StatusDryRunRejectsMissingFolderToken(t *testing.T) {
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
			"drive", "+status",
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
		t.Fatalf("expected folder-token in error message, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
}
