// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdupdate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/selfupdate"
	"github.com/larksuite/cli/internal/skillscheck"
)

// newTestFactory creates a test factory with minimal config.
func newTestFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	f, stdout, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	return f, stdout, stderr
}

// mockDetect sets up newUpdater to return an Updater with the given DetectResult.
func mockDetect(t *testing.T, result selfupdate.DetectResult) {
	t.Helper()
	origNew := newUpdater
	newUpdater = func() *selfupdate.Updater {
		u := selfupdate.New()
		u.DetectOverride = func() selfupdate.DetectResult { return result }
		return u
	}
	t.Cleanup(func() { newUpdater = origNew })
}

// mockDetectAndNpm sets up newUpdater with detect, npm install, and skills overrides all at once.
func mockDetectAndNpm(t *testing.T, result selfupdate.DetectResult, npmFn func(string) *selfupdate.NpmResult) {
	t.Helper()
	origNew := newUpdater
	newUpdater = func() *selfupdate.Updater {
		u := selfupdate.New()
		u.DetectOverride = func() selfupdate.DetectResult { return result }
		u.NpmInstallOverride = npmFn
		u.VerifyOverride = func(string) error { return nil }
		u.SkillsCommandOverride = successfulSkillsCommand()
		return u
	}
	t.Cleanup(func() { newUpdater = origNew })
}

func successfulSkillsCommand() func(args ...string) *selfupdate.NpmResult {
	return func(args ...string) *selfupdate.NpmResult {
		r := &selfupdate.NpmResult{}
		switch strings.Join(args, " ") {
		case "-y skills add https://open.feishu.cn --list":
			r.Stdout.WriteString("lark-calendar\nlark-mail\n")
		case "-y skills ls -g":
			r.Stdout.WriteString("lark-calendar\ncustom-skill\n")
		default:
		}
		return r
	}
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

func TestUpdateManual_JSON(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json"})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	mockDetect(t, selfupdate.DetectResult{Method: selfupdate.InstallManual, ResolvedPath: "/usr/local/bin/lark-cli"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"action": "manual_required"`) {
		t.Errorf("expected manual_required in output, got: %s", out)
	}
	if !strings.Contains(out, "not installed via npm") {
		t.Errorf("expected accurate reason in output, got: %s", out)
	}
	if !strings.Contains(out, "releases/tag/v2.0.0") {
		t.Errorf("expected version-pinned URL in output, got: %s", out)
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
	mockDetect(t, selfupdate.DetectResult{Method: selfupdate.InstallManual, ResolvedPath: "/usr/local/bin/lark-cli"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stderr.String()
	if !strings.Contains(out, "not installed via npm") {
		t.Errorf("expected 'not installed via npm' in stderr, got: %s", out)
	}
	if !strings.Contains(out, "releases/tag/v2.0.0") {
		t.Errorf("expected version-pinned URL in stderr, got: %s", out)
	}
}

func TestUpdateNpm_JSON(t *testing.T) {
	// Isolate config dir because skills sync writes skills-state.json.
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	mockDetectAndNpm(t,
		selfupdate.DetectResult{Method: selfupdate.InstallNpm, ResolvedPath: "/node_modules/@larksuite/cli/bin/lark-cli", NpmAvailable: true},
		func(version string) *selfupdate.NpmResult { return &selfupdate.NpmResult{} },
	)

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
	// Same isolation as TestUpdateNpm_JSON — see comment there.
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, stderr := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	mockDetectAndNpm(t,
		selfupdate.DetectResult{Method: selfupdate.InstallNpm, ResolvedPath: "/node_modules/@larksuite/cli/bin/lark-cli", NpmAvailable: true},
		func(version string) *selfupdate.NpmResult { return &selfupdate.NpmResult{} },
	)

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
	// Same state-isolation rationale as TestUpdateNpm_JSON.
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--force", "--json"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "1.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	mockDetectAndNpm(t,
		selfupdate.DetectResult{Method: selfupdate.InstallNpm, ResolvedPath: "/node_modules/@larksuite/cli/bin/lark-cli", NpmAvailable: true},
		func(version string) *selfupdate.NpmResult { return &selfupdate.NpmResult{} },
	)

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
	// Same state-isolation rationale as TestUpdateNpm_JSON.
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "1.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "DEV" }
	defer func() { currentVersion = origVersion }()
	mockDetectAndNpm(t,
		selfupdate.DetectResult{Method: selfupdate.InstallNpm, ResolvedPath: "/node_modules/@larksuite/cli/bin/lark-cli", NpmAvailable: true},
		func(version string) *selfupdate.NpmResult { return &selfupdate.NpmResult{} },
	)

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

	origNew := newUpdater
	newUpdater = func() *selfupdate.Updater {
		u := selfupdate.New()
		u.DetectOverride = func() selfupdate.DetectResult {
			return selfupdate.DetectResult{Method: selfupdate.InstallNpm, ResolvedPath: "/node_modules/@larksuite/cli/bin/lark-cli", NpmAvailable: true}
		}
		u.NpmInstallOverride = func(version string) *selfupdate.NpmResult {
			r := &selfupdate.NpmResult{}
			fmt.Fprint(&r.Stderr, "EACCES: permission denied")
			r.Err = errors.New("npm install failed")
			return r
		}
		return u
	}
	defer func() { newUpdater = origNew }()

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

	origNew := newUpdater
	newUpdater = func() *selfupdate.Updater {
		u := selfupdate.New()
		u.DetectOverride = func() selfupdate.DetectResult {
			return selfupdate.DetectResult{Method: selfupdate.InstallNpm, ResolvedPath: "/node_modules/@larksuite/cli/bin/lark-cli", NpmAvailable: true}
		}
		u.NpmInstallOverride = func(version string) *selfupdate.NpmResult {
			r := &selfupdate.NpmResult{}
			fmt.Fprint(&r.Stderr, "EACCES: permission denied")
			r.Err = errors.New("npm install failed")
			return r
		}
		return u
	}
	defer func() { newUpdater = origNew }()

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

func TestUpdateNpmVerifyFail_JSON_NoRestoreHintWhenBackupUnavailable(t *testing.T) {
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()

	origNew := newUpdater
	newUpdater = func() *selfupdate.Updater {
		u := selfupdate.New()
		u.DetectOverride = func() selfupdate.DetectResult {
			return selfupdate.DetectResult{Method: selfupdate.InstallNpm, ResolvedPath: "/node_modules/@larksuite/cli/bin/lark-cli", NpmAvailable: true}
		}
		u.NpmInstallOverride = func(version string) *selfupdate.NpmResult { return &selfupdate.NpmResult{} }
		u.VerifyOverride = func(string) error { return errors.New("bad binary") }
		u.RestoreAvailableOverride = func() bool { return false }
		u.SkillsCommandOverride = func(args ...string) *selfupdate.NpmResult {
			t.Fatal("skills sync should not run when binary verification fails")
			return nil
		}
		return u
	}
	defer func() { newUpdater = origNew }()

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected verification failure")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitAPI {
		t.Fatalf("expected ExitAPI (%d), got %d", output.ExitAPI, exitErr.Code)
	}

	out := stdout.String()
	if !strings.Contains(out, "automatic rollback is unavailable") {
		t.Errorf("expected unavailable rollback hint, got: %s", out)
	}
	if strings.Contains(out, "previous version has been restored") {
		t.Errorf("should not claim restore when no backup is available, got: %s", out)
	}
	if !strings.Contains(out, "npm install -g @larksuite/cli@2.0.0") {
		t.Errorf("expected manual reinstall command in hint, got: %s", out)
	}
	if !strings.Contains(out, "skills will not be synced") {
		t.Errorf("expected skills-not-synced warning in rollback hint, got: %s", out)
	}
	if !strings.Contains(out, "npx skills add larksuite/cli -y -g") {
		t.Errorf("expected npx skills add hint for skills sync, got: %s", out)
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
	mockDetect(t, selfupdate.DetectResult{Method: selfupdate.InstallNpm, ResolvedPath: "/node_modules/@larksuite/cli/bin/lark-cli", NpmAvailable: true})

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
	mockDetect(t, selfupdate.DetectResult{Method: selfupdate.InstallNpm, ResolvedPath: "/node_modules/@larksuite/cli/bin/lark-cli", NpmAvailable: true})

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
	mockDetect(t, selfupdate.DetectResult{Method: selfupdate.InstallManual, ResolvedPath: "/usr/local/bin/lark-cli"})

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
	// npm detected (node_modules in path) but npm binary not available
	mockDetect(t, selfupdate.DetectResult{
		Method:       selfupdate.InstallNpm,
		ResolvedPath: "/node_modules/@larksuite/cli/bin/lark-cli",
		NpmAvailable: false,
	})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"action": "manual_required"`) {
		t.Errorf("expected manual_required when npm not found, got: %s", out)
	}
	// Must say "npm is not available", not generic "not installed via npm"
	if !strings.Contains(out, "npm is not available") {
		t.Errorf("expected 'npm is not available' reason when npm detected but missing, got: %s", out)
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
	origOS := currentOS
	defer func() { currentOS = origOS }()

	// Linux: EACCES should produce a hint with npm prefix guidance.
	currentOS = "linux"
	hint := permissionHint("EACCES: permission denied, access '/usr/local/lib'")
	if !strings.Contains(hint, "npm global prefix") {
		t.Errorf("expected npm prefix hint on linux, got: %s", hint)
	}
	if strings.Contains(hint, "sudo npm install -g") {
		t.Errorf("should not suggest raw sudo npm install, got: %s", hint)
	}

	// Windows: EACCES hint is suppressed (no EACCES on Windows).
	currentOS = "windows"
	hint = permissionHint("EACCES: permission denied")
	if hint != "" {
		t.Errorf("expected empty hint on Windows, got: %s", hint)
	}

	// Non-EACCES error: always empty.
	currentOS = "linux"
	if got := permissionHint("some other error"); got != "" {
		t.Errorf("expected empty hint for non-EACCES, got: %s", got)
	}
}

func TestUpdateWindows_NpmSuccess_JSON(t *testing.T) {
	// With the rename trick, Windows npm installs can now auto-update.
	// Same state-isolation rationale as TestUpdateNpm_JSON.
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	origOS := currentOS
	currentOS = osWindows
	defer func() { currentOS = origOS }()
	mockDetectAndNpm(t,
		selfupdate.DetectResult{Method: selfupdate.InstallNpm, ResolvedPath: `C:\npm\node_modules\@larksuite\cli\bin\lark-cli.exe`, NpmAvailable: true},
		func(version string) *selfupdate.NpmResult { return &selfupdate.NpmResult{} },
	)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"action": "updated"`) {
		t.Errorf("expected updated on Windows with rename trick, got: %s", out)
	}
}

func TestUpdateWindows_Check_JSON(t *testing.T) {
	// --check on Windows npm should report auto_update: true (rename trick available).
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json", "--check"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	origOS := currentOS
	currentOS = osWindows
	defer func() { currentOS = origOS }()
	mockDetect(t, selfupdate.DetectResult{Method: selfupdate.InstallNpm, ResolvedPath: `C:\node_modules\@larksuite\cli\bin\lark-cli.exe`, NpmAvailable: true})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"auto_update": true`) {
		t.Errorf("expected auto_update:true on Windows (rename trick), got: %s", out)
	}
}

func TestUpdateWindows_Symbols(t *testing.T) {
	origOS := currentOS
	defer func() { currentOS = origOS }()

	currentOS = "windows"
	if symOK() != "[OK]" {
		t.Errorf("expected [OK] on Windows, got: %s", symOK())
	}
	if symFail() != "[FAIL]" {
		t.Errorf("expected [FAIL] on Windows, got: %s", symFail())
	}
	if symWarn() != "[WARN]" {
		t.Errorf("expected [WARN] on Windows, got: %s", symWarn())
	}
	if symArrow() != "->" {
		t.Errorf("expected -> on Windows, got: %s", symArrow())
	}

	currentOS = "darwin"
	if symOK() != "\u2713" {
		t.Errorf("expected \u2713 on darwin, got: %s", symOK())
	}
	if symArrow() != "\u2192" {
		t.Errorf("expected \u2192 on darwin, got: %s", symArrow())
	}
}

func TestUpdateNpm_SkillsSuccess_JSON(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()
	mockDetectAndNpm(t,
		selfupdate.DetectResult{Method: selfupdate.InstallNpm, ResolvedPath: "/node_modules/@larksuite/cli/bin/lark-cli", NpmAvailable: true},
		func(version string) *selfupdate.NpmResult { return &selfupdate.NpmResult{} },
	)

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	// Should NOT have skills_warning when skills succeed
	if strings.Contains(out, "skills_warning") {
		t.Errorf("expected no skills_warning on success, got: %s", out)
	}
}

func TestUpdateNpm_SkillsFail_JSON(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _ := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json"})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()

	origNew := newUpdater
	newUpdater = func() *selfupdate.Updater {
		u := selfupdate.New()
		u.DetectOverride = func() selfupdate.DetectResult {
			return selfupdate.DetectResult{Method: selfupdate.InstallNpm, ResolvedPath: "/node_modules/@larksuite/cli/bin/lark-cli", NpmAvailable: true}
		}
		u.NpmInstallOverride = func(version string) *selfupdate.NpmResult { return &selfupdate.NpmResult{} }
		u.VerifyOverride = func(string) error { return nil }
		u.SkillsCommandOverride = func(args ...string) *selfupdate.NpmResult {
			r := &selfupdate.NpmResult{}
			r.Stderr.WriteString("npx: command not found")
			r.Err = fmt.Errorf("exit status 127")
			return r
		}
		return u
	}
	defer func() { newUpdater = origNew }()

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	// CLI update should still succeed (ok:true)
	if !strings.Contains(out, `"ok": true`) {
		t.Errorf("expected ok:true despite skills failure, got: %s", out)
	}
	if !strings.Contains(out, `"action": "updated"`) {
		t.Errorf("expected action:updated despite skills failure, got: %s", out)
	}
	// Should have skills_warning with detail
	if !strings.Contains(out, "skills_warning") {
		t.Errorf("expected skills_warning in output, got: %s", out)
	}
	if !strings.Contains(out, "skills_summary") {
		t.Errorf("expected skills_summary in output, got: %s", out)
	}
}

func TestUpdateNpm_SkillsFail_Human(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, stderr := newTestFactory(t)
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{})

	origFetch := fetchLatest
	fetchLatest = func() (string, error) { return "2.0.0", nil }
	defer func() { fetchLatest = origFetch }()
	origVersion := currentVersion
	currentVersion = func() string { return "1.0.0" }
	defer func() { currentVersion = origVersion }()

	origNew := newUpdater
	newUpdater = func() *selfupdate.Updater {
		u := selfupdate.New()
		u.DetectOverride = func() selfupdate.DetectResult {
			return selfupdate.DetectResult{Method: selfupdate.InstallNpm, ResolvedPath: "/node_modules/@larksuite/cli/bin/lark-cli", NpmAvailable: true}
		}
		u.NpmInstallOverride = func(version string) *selfupdate.NpmResult { return &selfupdate.NpmResult{} }
		u.VerifyOverride = func(string) error { return nil }
		u.SkillsCommandOverride = func(args ...string) *selfupdate.NpmResult {
			r := &selfupdate.NpmResult{}
			r.Stderr.WriteString("npx: command not found")
			r.Err = fmt.Errorf("exit status 127")
			return r
		}
		return u
	}
	defer func() { newUpdater = origNew }()

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stderr.String()
	// CLI update should still show success
	if !strings.Contains(out, "Successfully updated") {
		t.Errorf("expected CLI success message, got: %s", out)
	}
	// Skills warning should be shown
	if !strings.Contains(out, "Skills update failed") {
		t.Errorf("expected skills failure warning, got: %s", out)
	}
	if !strings.Contains(out, "lark-cli update --force") {
		t.Errorf("expected force retry hint, got: %s", out)
	}
}

// newTestIO returns a cmdutil.IOStreams backed by bytes.Buffers.
func newTestIO() *cmdutil.IOStreams {
	return cmdutil.NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
}

func TestRunSkillsAndState_DedupHit(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := skillscheck.WriteState(skillscheck.SkillsState{Version: "1.0.21"}); err != nil {
		t.Fatal(err)
	}
	called := false
	updater := &selfupdate.Updater{
		SkillsCommandOverride: func(args ...string) *selfupdate.NpmResult {
			called = true
			return &selfupdate.NpmResult{}
		},
	}
	got := runSkillsAndState(updater, newTestIO(), "1.0.21", false)
	if got != nil {
		t.Errorf("runSkillsAndState() = %+v, want nil for dedup hit", got)
	}
	if called {
		t.Error("SkillsCommandOverride called, want skipped due to dedup")
	}
}

func TestRunSkillsAndState_DedupForceBypass(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := skillscheck.WriteState(skillscheck.SkillsState{Version: "1.0.21"}); err != nil {
		t.Fatal(err)
	}
	called := false
	updater := &selfupdate.Updater{
		SkillsCommandOverride: func(args ...string) *selfupdate.NpmResult {
			called = true
			return successfulSkillsCommand()(args...)
		},
	}
	got := runSkillsAndState(updater, newTestIO(), "1.0.21", true)
	if got == nil || got.Err != nil {
		t.Fatalf("runSkillsAndState(force=true) = %+v, want successful result", got)
	}
	if !called {
		t.Error("SkillsCommandOverride not called with force=true")
	}
}

func TestRunSkillsAndState_SuccessWritesState(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	updater := &selfupdate.Updater{SkillsCommandOverride: successfulSkillsCommand()}
	got := runSkillsAndState(updater, newTestIO(), "1.0.21", false)
	if got == nil || got.Err != nil {
		t.Fatalf("runSkillsAndState() = %+v, want non-nil with nil Err", got)
	}
	state, readable, err := skillscheck.ReadState()
	if err != nil || !readable {
		t.Fatalf("ReadState() = (_, %v, %v), want readable", readable, err)
	}
	if state.Version != "1.0.21" {
		t.Errorf("state.Version = %q, want \"1.0.21\"", state.Version)
	}
}

func TestRunSkillsAndState_FailureKeepsOldState(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := skillscheck.WriteState(skillscheck.SkillsState{Version: "1.0.20"}); err != nil {
		t.Fatal(err)
	}
	updater := &selfupdate.Updater{
		SkillsCommandOverride: func(args ...string) *selfupdate.NpmResult {
			r := &selfupdate.NpmResult{}
			r.Err = fmt.Errorf("npx failed")
			return r
		},
	}
	got := runSkillsAndState(updater, newTestIO(), "1.0.21", false)
	if got == nil || got.Err == nil {
		t.Fatalf("runSkillsAndState() = %+v, want non-nil with non-nil Err", got)
	}
	state, readable, err := skillscheck.ReadState()
	if err != nil || !readable {
		t.Fatalf("ReadState() = (_, %v, %v), want readable", readable, err)
	}
	if state.Version != "1.0.20" {
		t.Errorf("state.Version = %q, want \"1.0.20\" (failure must not overwrite)", state.Version)
	}
}

func TestTruncate(t *testing.T) {
	long := strings.Repeat("x", 3000)
	got := selfupdate.Truncate(long, 2000)
	if len(got) != 2000 {
		t.Errorf("expected truncated length 2000, got %d", len(got))
	}

	short := "hello"
	got2 := selfupdate.Truncate(short, 2000)
	if got2 != "hello" {
		t.Errorf("expected 'hello', got %q", got2)
	}
}

func TestUpdateRun_AlreadyLatest_RunsSkillsSync(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	origFetch := fetchLatest
	origCur := currentVersion
	t.Cleanup(func() { fetchLatest = origFetch; currentVersion = origCur })
	fetchLatest = func() (string, error) { return "1.0.21", nil }
	currentVersion = func() string { return "1.0.21" }

	skillsCalled := false
	origNew := newUpdater
	t.Cleanup(func() { newUpdater = origNew })
	newUpdater = func() *selfupdate.Updater {
		return &selfupdate.Updater{
			SkillsCommandOverride: func(args ...string) *selfupdate.NpmResult {
				skillsCalled = true
				return successfulSkillsCommand()(args...)
			},
		}
	}

	f, _, _ := newTestFactory(t)
	opts := &UpdateOptions{Factory: f, JSON: true}
	if err := updateRun(opts); err != nil {
		t.Fatalf("updateRun() err = %v, want nil", err)
	}
	if !skillsCalled {
		t.Error("skills sync not called in already-up-to-date branch")
	}
	state, readable, err := skillscheck.ReadState()
	if err != nil || !readable {
		t.Fatalf("ReadState() = (_, %v, %v), want readable", readable, err)
	}
	if state.Version != "1.0.21" {
		t.Errorf("state.Version = %q, want \"1.0.21\"", state.Version)
	}
}

func TestUpdateRun_Manual_RunsSkillsSync(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	origFetch := fetchLatest
	origCur := currentVersion
	t.Cleanup(func() { fetchLatest = origFetch; currentVersion = origCur })
	fetchLatest = func() (string, error) { return "1.0.22", nil }
	currentVersion = func() string { return "1.0.21" }

	skillsCalled := false
	origNew := newUpdater
	t.Cleanup(func() { newUpdater = origNew })
	newUpdater = func() *selfupdate.Updater {
		return &selfupdate.Updater{
			DetectOverride: func() selfupdate.DetectResult {
				return selfupdate.DetectResult{
					Method:       selfupdate.InstallManual,
					ResolvedPath: "/usr/local/bin/lark-cli",
				}
			},
			SkillsCommandOverride: func(args ...string) *selfupdate.NpmResult {
				skillsCalled = true
				return successfulSkillsCommand()(args...)
			},
		}
	}

	f, _, _ := newTestFactory(t)
	opts := &UpdateOptions{Factory: f, JSON: true}
	if err := updateRun(opts); err != nil {
		t.Fatalf("updateRun() err = %v, want nil", err)
	}
	if !skillsCalled {
		t.Error("skills sync not called in manual branch")
	}
	state, readable, err := skillscheck.ReadState()
	if err != nil || !readable {
		t.Fatalf("ReadState() = (_, %v, %v), want readable", readable, err)
	}
	if state.Version != "1.0.21" {
		t.Errorf("state.Version = %q, want \"1.0.21\" (manual path records current binary)", state.Version)
	}
}

func TestUpdateRun_Npm_RunsSkillsSync_WritesLatestState(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	origFetch := fetchLatest
	origCur := currentVersion
	t.Cleanup(func() { fetchLatest = origFetch; currentVersion = origCur })
	fetchLatest = func() (string, error) { return "1.0.22", nil }
	currentVersion = func() string { return "1.0.21" }

	skillsCalled := false
	origNew := newUpdater
	t.Cleanup(func() { newUpdater = origNew })
	newUpdater = func() *selfupdate.Updater {
		return &selfupdate.Updater{
			DetectOverride: func() selfupdate.DetectResult {
				return selfupdate.DetectResult{
					Method: selfupdate.InstallNpm, NpmAvailable: true,
					ResolvedPath: "/usr/local/bin/lark-cli",
				}
			},
			NpmInstallOverride: func(version string) *selfupdate.NpmResult {
				return &selfupdate.NpmResult{}
			},
			VerifyOverride: func(expectedVersion string) error { return nil },
			SkillsCommandOverride: func(args ...string) *selfupdate.NpmResult {
				skillsCalled = true
				return successfulSkillsCommand()(args...)
			},
		}
	}

	f, _, _ := newTestFactory(t)
	opts := &UpdateOptions{Factory: f, JSON: true}
	if err := updateRun(opts); err != nil {
		t.Fatalf("updateRun() err = %v, want nil", err)
	}
	if !skillsCalled {
		t.Error("skills sync not called in npm branch")
	}
	state, readable, err := skillscheck.ReadState()
	if err != nil || !readable {
		t.Fatalf("ReadState() = (_, %v, %v), want readable", readable, err)
	}
	if state.Version != "1.0.22" {
		t.Errorf("state.Version = %q, want \"1.0.22\" (npm path records latest binary)", state.Version)
	}
}

func TestUpdateRun_CheckIncludesSkillsStatus(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := skillscheck.WriteState(skillscheck.SkillsState{
		Version:              "1.0.20",
		OfficialSkills:       []string{"lark-calendar", "lark-mail"},
		UpdatedSkills:        []string{"lark-calendar"},
		SkippedDeletedSkills: []string{"lark-mail"},
	}); err != nil {
		t.Fatal(err)
	}

	origFetch := fetchLatest
	origCur := currentVersion
	t.Cleanup(func() { fetchLatest = origFetch; currentVersion = origCur })
	fetchLatest = func() (string, error) { return "1.0.22", nil }
	currentVersion = func() string { return "1.0.21" }

	origNew := newUpdater
	t.Cleanup(func() { newUpdater = origNew })
	skillsCalled := false
	newUpdater = func() *selfupdate.Updater {
		return &selfupdate.Updater{
			DetectOverride: func() selfupdate.DetectResult {
				return selfupdate.DetectResult{Method: selfupdate.InstallNpm, NpmAvailable: true}
			},
			SkillsCommandOverride: func(args ...string) *selfupdate.NpmResult {
				skillsCalled = true
				return successfulSkillsCommand()(args...)
			},
		}
	}

	f, stdout, _ := newTestFactory(t)
	opts := &UpdateOptions{Factory: f, JSON: true, Check: true}
	if err := updateRun(opts); err != nil {
		t.Fatalf("updateRun(--check) err = %v, want nil", err)
	}
	if skillsCalled {
		t.Error("skills sync called under --check, want skipped")
	}

	var env map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal stdout: %v\nstdout: %s", err, stdout.String())
	}
	status, ok := env["skills_status"].(map[string]interface{})
	if !ok {
		t.Fatalf("skills_status missing or wrong type in --check JSON: %s", stdout.String())
	}
	if status["current"] != "1.0.20" || status["target"] != "1.0.21" || status["in_sync"] != false {
		t.Errorf("skills_status = %+v, want {current:\"1.0.20\", target:\"1.0.21\", in_sync:false}", status)
	}
	if status["official"] != float64(2) || status["updated"] != float64(1) {
		t.Errorf("skills_status counts = %+v, want official:2 updated:1", status)
	}
}

func TestUpdateRun_CheckAlreadyLatest_NoSideEffect(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := skillscheck.WriteState(skillscheck.SkillsState{Version: "1.0.20"}); err != nil {
		t.Fatal(err)
	}

	origFetch := fetchLatest
	origCur := currentVersion
	t.Cleanup(func() { fetchLatest = origFetch; currentVersion = origCur })
	fetchLatest = func() (string, error) { return "1.0.21", nil }
	currentVersion = func() string { return "1.0.21" }

	skillsCalled := false
	origNew := newUpdater
	t.Cleanup(func() { newUpdater = origNew })
	newUpdater = func() *selfupdate.Updater {
		return &selfupdate.Updater{
			SkillsCommandOverride: func(args ...string) *selfupdate.NpmResult {
				skillsCalled = true
				return successfulSkillsCommand()(args...)
			},
		}
	}

	f, stdout, _ := newTestFactory(t)
	opts := &UpdateOptions{Factory: f, JSON: true, Check: true}
	if err := updateRun(opts); err != nil {
		t.Fatalf("updateRun(--check, already-latest) err = %v, want nil", err)
	}
	if skillsCalled {
		t.Error("skills sync called under --check (already-latest), want skipped")
	}

	state, readable, err := skillscheck.ReadState()
	if err != nil || !readable {
		t.Fatalf("ReadState() = (_, %v, %v), want readable", readable, err)
	}
	if state.Version != "1.0.20" {
		t.Errorf("state.Version mutated to %q under --check, want \"1.0.20\"", state.Version)
	}

	var env map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal stdout: %v\n%s", err, stdout.String())
	}
	if env["action"] != "already_up_to_date" {
		t.Errorf("action = %v, want \"already_up_to_date\"", env["action"])
	}
	if _, has := env["skills_action"]; has {
		t.Errorf("skills_action present under --check, want absent: %+v", env)
	}
	status, ok := env["skills_status"].(map[string]interface{})
	if !ok {
		t.Fatalf("skills_status missing under --check + already-latest: %s", stdout.String())
	}
	if status["current"] != "1.0.20" || status["target"] != "1.0.21" || status["in_sync"] != false {
		t.Errorf("skills_status = %+v, want {current:\"1.0.20\", target:\"1.0.21\", in_sync:false}", status)
	}
}

func TestRunSkillsAndState_StateWriteFailureWarns(t *testing.T) {
	origSync := syncSkills
	syncSkills = func(opts skillscheck.SyncOptions) *skillscheck.SyncResult {
		return &skillscheck.SyncResult{Err: fmt.Errorf("skills synced but state not written: denied")}
	}
	t.Cleanup(func() { syncSkills = origSync })

	f, _, stderr := newTestFactory(t)
	got := runSkillsAndState(&selfupdate.Updater{}, f.IOStreams, "1.0.21", false)
	if got == nil || got.Err == nil {
		t.Fatalf("runSkillsAndState() = %+v, want non-nil with write error", got)
	}
	if !strings.Contains(stderr.String(), "warning: skills synced but state not written") {
		t.Errorf("stderr does not contain warning: %q", stderr.String())
	}
}

func TestEmitSkillsTextHints_Success(t *testing.T) {
	f, _, stderr := newTestFactory(t)
	emitSkillsTextHints(f.IOStreams, &skillscheck.SyncResult{Official: []string{"lark-calendar"}, Updated: []string{"lark-calendar"}})
	if !strings.Contains(stderr.String(), "Skills updated") {
		t.Errorf("stderr does not contain 'Skills updated': %q", stderr.String())
	}
}
