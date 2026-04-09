// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package upgrade

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

func TestUpgradeAlreadyUpToDate_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)

	cmd := NewCmdUpgrade(f)
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

func TestUpgradeAlreadyUpToDate_Human(t *testing.T) {
	f, _, stderr := newTestFactory(t)

	cmd := NewCmdUpgrade(f)
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

func TestUpgradeManual_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpgrade(f)
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

func TestUpgradeManual_Human(t *testing.T) {
	f, _, stderr := newTestFactory(t)
	cmd := NewCmdUpgrade(f)
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

func TestUpgradeNpm_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpgrade(f)
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
	if !strings.Contains(out, `"action": "upgraded"`) {
		t.Errorf("expected upgraded in output, got: %s", out)
	}
}

func TestUpgradeNpm_Human(t *testing.T) {
	f, _, stderr := newTestFactory(t)
	cmd := NewCmdUpgrade(f)
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
	if !strings.Contains(out, "Successfully upgraded") {
		t.Errorf("expected success message in stderr, got: %s", out)
	}
}

func TestUpgradeForce_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpgrade(f)
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
	if !strings.Contains(out, `"action": "upgraded"`) {
		t.Errorf("expected upgraded in JSON output, got: %s", out)
	}
}

func TestUpgradeFetchError_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpgrade(f)
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

func TestUpgradeFetchError_Human(t *testing.T) {
	f, _, _ := newTestFactory(t)
	cmd := NewCmdUpgrade(f)
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

func TestUpgradeInvalidVersion_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpgrade(f)
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

func TestUpgradeDevVersion_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpgrade(f)
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
	if !strings.Contains(out, `"action": "upgraded"`) {
		t.Errorf("expected upgraded in JSON output, got: %s", out)
	}
}

func TestUpgradeNpmFail_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpgrade(f)
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

func TestUpgradeNpmFail_Human(t *testing.T) {
	f, _, stderr := newTestFactory(t)
	cmd := NewCmdUpgrade(f)
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
	if !strings.Contains(out, "Upgrade failed") {
		t.Errorf("expected 'Upgrade failed' in stderr, got: %s", out)
	}
	if !strings.Contains(out, "sudo") {
		t.Errorf("expected 'sudo' hint in stderr, got: %s", out)
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
