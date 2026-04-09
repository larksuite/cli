// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdupdate

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// newTestFactory creates a test factory with minimal config.
func newTestFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	f, stdout, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	return f, stdout, stderr
}

func TestUpdateAlreadyUpToDate_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)

	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "1.0.0", nil }
	defer func() { fetchLatest = origFetch }()

	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, `"action": "already_up_to_date"`) {
		t.Errorf("expected already_up_to_date in JSON output, got: %s", out)
	}
	if !strings.Contains(out, `"ok": true`) {
		t.Errorf("expected ok:true in JSON output, got: %s", out)
	}
}

func TestUpdateAlreadyUpToDate_Human(t *testing.T) {
	f, _, stderr := newTestFactory(t)

	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "1.0.0", nil }
	defer func() { fetchLatest = origFetch }()

	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stderr.String()
	if !strings.Contains(out, "already up to date") {
		t.Errorf("expected 'already up to date' in stderr, got: %s", out)
	}
}

func TestDetectInstallMethod_Npm(t *testing.T) {
	got := detectInstallMethod("/usr/local/lib/node_modules/@larksuite/cli/bin/lark-cli")
	if got != installNpm {
		t.Errorf("expected installNpm, got %v", got)
	}
}

func TestDetectInstallMethod_Manual(t *testing.T) {
	got := detectInstallMethod("/usr/local/bin/lark-cli")
	if got != installManual {
		t.Errorf("expected installManual, got %v", got)
	}
}

func TestDetectInstallMethod_Windows(t *testing.T) {
	got := detectInstallMethod(`C:\Users\user\AppData\Roaming\npm\node_modules\@larksuite\cli\bin\lark-cli.exe`)
	if got != installNpm {
		t.Errorf("expected installNpm for Windows path, got %v", got)
	}
}

func TestUpdateManual_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	origDetect := detectMethod
	detectMethod = func() (installMethod, string) { return installManual, "/usr/local/bin/lark-cli" }
	defer func() { detectMethod = origDetect }()

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"action": "manual_required"`) {
		t.Errorf("expected manual_required in output, got: %s", out)
	}
	if !strings.Contains(out, "github.com") {
		t.Errorf("expected github URL in output, got: %s", out)
	}
}

func TestUpdateManual_Human(t *testing.T) {
	f, _, stderr := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	origDetect := detectMethod
	detectMethod = func() (installMethod, string) { return installManual, "/usr/local/bin/lark-cli" }
	defer func() { detectMethod = origDetect }()

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stderr.String()
	if !strings.Contains(out, "not installed via npm") {
		t.Errorf("expected 'not installed via npm' in stderr, got: %s", out)
	}
	if !strings.Contains(out, "github.com") {
		t.Errorf("expected github URL in stderr, got: %s", out)
	}
}

func TestUpdateNpm_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	origDetect := detectMethod
	detectMethod = func() (installMethod, string) { return installNpm, "/node_modules/@larksuite/cli/bin/lark-cli" }
	defer func() { detectMethod = origDetect }()
	origRunNpm := runNpmInstall
	runNpmInstall = func(version string, stdout, stderr *bytes.Buffer) error { return nil }
	defer func() { runNpmInstall = origRunNpm }()

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"action": "updated"`) {
		t.Errorf("expected updated in output, got: %s", out)
	}
}

func TestUpdateNpm_Human(t *testing.T) {
	f, _, stderr := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	origDetect := detectMethod
	detectMethod = func() (installMethod, string) { return installNpm, "/node_modules/@larksuite/cli/bin/lark-cli" }
	defer func() { detectMethod = origDetect }()
	origRunNpm := runNpmInstall
	runNpmInstall = func(version string, stdout, stderr *bytes.Buffer) error { return nil }
	defer func() { runNpmInstall = origRunNpm }()

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stderr.String()
	if !strings.Contains(out, "Successfully updated") {
		t.Errorf("expected success message in stderr, got: %s", out)
	}
}

func TestUpdateForce_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--force", "--json"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "1.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	origDetect := detectMethod
	detectMethod = func() (installMethod, string) { return installNpm, "/node_modules/@larksuite/cli/bin/lark-cli" }
	defer func() { detectMethod = origDetect }()
	origRunNpm := runNpmInstall
	runNpmInstall = func(version string, stdout, stderr *bytes.Buffer) error { return nil }
	defer func() { runNpmInstall = origRunNpm }()

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"action": "updated"`) {
		t.Errorf("expected updated in JSON output, got: %s", out)
	}
}

func TestUpdateFetchError_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "", errors.New("network timeout") }
	defer func() { fetchLatest = origFetch }()

	err := cmd.Execute()
	// cobra silences errors when RunE returns; we just check stdout
	_ = err
	out := stdout.String()
	if !strings.Contains(out, `"ok": false`) {
		t.Errorf("expected ok:false in JSON output, got: %s", out)
	}
	if !strings.Contains(out, "network timeout") {
		t.Errorf("expected 'network timeout' in JSON output, got: %s", out)
	}
}

func TestUpdateFetchError_Human(t *testing.T) {
	f, _, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "", errors.New("network timeout") }
	defer func() { fetchLatest = origFetch }()

	// Suppress cobra's default error printing.
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected non-nil error, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitNetwork {
		t.Errorf("expected ExitNetwork (%d), got %d", output.ExitNetwork, exitErr.Code)
	}
}

func TestUpdateInvalidVersion_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "not-a-version", nil }
	defer func() { fetchLatest = origFetch }()

	_ = cmd.Execute()
	out := stdout.String()
	if !strings.Contains(out, "invalid version") {
		t.Errorf("expected 'invalid version' in JSON output, got: %s", out)
	}
}

func TestUpdateDevVersion_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "1.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "DEV" }
	defer func() { currentVersion = origVersion }()
	origDetect := detectMethod
	detectMethod = func() (installMethod, string) { return installNpm, "/node_modules/@larksuite/cli/bin/lark-cli" }
	defer func() { detectMethod = origDetect }()
	origRunNpm := runNpmInstall
	runNpmInstall = func(version string, stdout, stderr *bytes.Buffer) error { return nil }
	defer func() { runNpmInstall = origRunNpm }()

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"action": "updated"`) {
		t.Errorf("expected updated in JSON output, got: %s", out)
	}
}

func TestUpdateNpmFail_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	origDetect := detectMethod
	detectMethod = func() (installMethod, string) { return installNpm, "/node_modules/@larksuite/cli/bin/lark-cli" }
	defer func() { detectMethod = origDetect }()
	origRunNpm := runNpmInstall
	runNpmInstall = func(version string, outBuf, errBuf *bytes.Buffer) error {
		fmt.Fprint(errBuf, "EACCES: permission denied")
		return errors.New("npm install failed")
	}
	defer func() { runNpmInstall = origRunNpm }()

	_ = cmd.Execute()
	out := stdout.String()
	if !strings.Contains(out, "permission denied") {
		t.Errorf("expected 'permission denied' in JSON output, got: %s", out)
	}
	if !strings.Contains(out, `"hint"`) {
		t.Errorf("expected 'hint' field in JSON output, got: %s", out)
	}
}

func TestUpdateNpmFail_Human(t *testing.T) {
	f, _, stderr := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	origDetect := detectMethod
	detectMethod = func() (installMethod, string) { return installNpm, "/node_modules/@larksuite/cli/bin/lark-cli" }
	defer func() { detectMethod = origDetect }()
	origRunNpm := runNpmInstall
	runNpmInstall = func(version string, outBuf, errBuf *bytes.Buffer) error {
		fmt.Fprint(errBuf, "EACCES: permission denied")
		return errors.New("npm install failed")
	}
	defer func() { runNpmInstall = origRunNpm }()

	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	_ = cmd.Execute()
	out := stderr.String()
	if !strings.Contains(out, "Update failed") {
		t.Errorf("expected 'Update failed' in stderr, got: %s", out)
	}
	if !strings.Contains(out, "Permission denied") {
		t.Errorf("expected permission hint in stderr, got: %s", out)
	}
}

func TestUpdateCheck_JSON_Npm(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json", "--check"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	origDetect := detectMethod
	detectMethod = func() (installMethod, string) { return installNpm, "/node_modules/@larksuite/cli/bin/lark-cli" }
	defer func() { detectMethod = origDetect }()

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"action": "update_available"`) {
		t.Errorf("expected update_available action, got: %s", out)
	}
	if !strings.Contains(out, `"auto_update": true`) {
		t.Errorf("expected auto_update:true for npm, got: %s", out)
	}
	if !strings.Contains(out, "releases/tag/v2.0.0") {
		t.Errorf("expected version-pinned release URL, got: %s", out)
	}
	if !strings.Contains(out, "CHANGELOG") {
		t.Errorf("expected changelog URL, got: %s", out)
	}
}

func TestUpdateCheck_Human_Npm(t *testing.T) {
	f, _, stderr := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--check"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	origDetect := detectMethod
	detectMethod = func() (installMethod, string) { return installNpm, "/node_modules/@larksuite/cli/bin/lark-cli" }
	defer func() { detectMethod = origDetect }()

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stderr.String()
	if !strings.Contains(out, "Update available") {
		t.Errorf("expected 'Update available' in stderr, got: %s", out)
	}
	if !strings.Contains(out, "lark-cli update") {
		t.Errorf("expected 'lark-cli update' instruction for npm, got: %s", out)
	}
}

func TestUpdateCheck_Human_Manual(t *testing.T) {
	f, _, stderr := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--check"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	origDetect := detectMethod
	detectMethod = func() (installMethod, string) { return installManual, "/usr/local/bin/lark-cli" }
	defer func() { detectMethod = origDetect }()

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stderr.String()
	if !strings.Contains(out, "Update available") {
		t.Errorf("expected 'Update available' in stderr, got: %s", out)
	}
	if !strings.Contains(out, "manually") {
		t.Errorf("expected manual download instruction for non-npm, got: %s", out)
	}
	if strings.Contains(out, "lark-cli update` to install") {
		t.Errorf("should NOT suggest 'lark-cli update' for manual install, got: %s", out)
	}
}

func TestUpdateNpmNotFound_FallsBackToManual(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	// Detected as npm install, but npm is not in PATH
	origDetect := detectMethod
	detectMethod = func() (installMethod, string) { return installNpm, "/node_modules/@larksuite/cli/bin/lark-cli" }
	defer func() { detectMethod = origDetect }()
	origLookPath := lookPath
	lookPath = func(file string) (string, error) { return "", fmt.Errorf("not found") }
	defer func() { lookPath = origLookPath }()

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	// Should fall back to manual_required instead of failing
	if !strings.Contains(out, `"action": "manual_required"`) {
		t.Errorf("expected manual_required when npm not found, got: %s", out)
	}
}

func TestReleaseURL(t *testing.T) {
	got := releaseURL("2.0.0")
	if got != "https://github.com/larksuite/cli/releases/tag/v2.0.0" {
		t.Errorf("expected version-pinned URL, got: %s", got)
	}
	got2 := releaseURL("v1.5.0")
	if got2 != "https://github.com/larksuite/cli/releases/tag/v1.5.0" {
		t.Errorf("expected no double v prefix, got: %s", got2)
	}
}

func TestPermissionHint(t *testing.T) {
	hint := permissionHint("EACCES: permission denied, access '/usr/local/lib'")
	if !strings.Contains(hint, "npm global prefix") {
		t.Errorf("expected neutral npm prefix hint, got: %s", hint)
	}
	if strings.Contains(hint, "sudo npm install") {
		t.Errorf("hint should not suggest raw sudo npm install, got: %s", hint)
	}

	empty := permissionHint("some other error")
	if empty != "" {
		t.Errorf("expected empty hint for non-EACCES error, got: %s", empty)
	}
}

func TestTruncate(t *testing.T) {
	long := strings.Repeat("x", 3000)
	got := truncate(long, 2000)
	if len(got) != 2000 {
		t.Errorf("expected truncated length 2000, got %d", len(got))
	}

	short := "hello"
	got2 := truncate(short, 2000)
	if got2 != "hello" {
		t.Errorf("expected 'hello', got %q", got2)
	}
}
