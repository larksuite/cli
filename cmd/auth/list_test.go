// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/surface"
	"github.com/zalando/go-keyring"
)

func TestAuthListRun_PreservesMalformedConfigError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	err := authListRun(&ListOptions{Factory: f, JSON: true})
	var configErr *errs.ConfigError
	if !errors.As(err, &configErr) || configErr.Subtype != errs.SubtypeInvalidConfig {
		t.Fatalf("error = %T (%v), want config/invalid_config", err, err)
	}
	if !errors.Is(err, core.ErrMalformedConfig) {
		t.Fatalf("error = %v, want malformed-config cause", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("direct runner wrote output before root error rendering: stdout=%q stderr=%q", stdout, stderr)
	}
}

// TestAuthListRun_NotConfigured_ReturnsExitZero pins the contract that
// `lark-cli auth list` is a read-only probe and must not fail-hard when no
// config exists yet — scripts and AI agents use it as an idempotent "do I
// have any users?" check, so the exit code carries semantic weight. Pair
// that with the existing "configured but no logged-in users" branch (also
// exit 0) and both empty states are consistent.
func TestAuthListRun_NotConfigured_ReturnsExitZero(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	if err := authListRun(&ListOptions{Factory: f}); err != nil {
		t.Fatalf("auth list should succeed when not configured (exit 0); got: %v", err)
	}
	// Local workspace → hint must mention init, not bind.
	out := stderr.String()
	if !strings.Contains(out, "config init") {
		t.Errorf("local hint missing config init: %s", out)
	}
	if strings.Contains(out, "config bind") {
		t.Errorf("local hint must not mention config bind: %s", out)
	}
}

func TestAuthListRun_JSONMode_NotConfigured_WritesStdoutOnly(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	if err := authListRun(&ListOptions{Factory: f, JSON: true}); err != nil {
		t.Fatalf("auth list should succeed when not configured (exit 0); got: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout must be valid JSON: %v\nstdout=%s", err, stdout.String())
	}
	if payload["ok"] != true {
		t.Errorf("stdout.ok = %v, want true", payload["ok"])
	}
	users, ok := payload["users"].([]any)
	if !ok || len(users) != 0 {
		t.Errorf("stdout.users = %v, want empty array", payload["users"])
	}
	if payload["reason"] != "not_configured" {
		t.Errorf("stdout.reason = %v, want not_configured", payload["reason"])
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr must stay empty in JSON mode, got:\n%s", stderr.String())
	}
}

// TestAuthListRun_NotConfigured_AgentWorkspace_RoutesToBindHelp covers the
// reason this hint exists workspace-aware in the first place: an AI agent
// in OpenClaw / Hermes that probes auth list before binding gets routed to
// `config bind --help` instead of the local-only `config init`.
func TestAuthListRun_NotConfigured_AgentWorkspace_RoutesToBindHelp(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	prev := core.CurrentWorkspace()
	t.Cleanup(func() { core.SetCurrentWorkspace(prev) })
	core.SetCurrentWorkspace(core.WorkspaceOpenClaw)

	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	if err := authListRun(&ListOptions{Factory: f}); err != nil {
		t.Fatalf("auth list should still succeed under agent workspace; got: %v", err)
	}
	out := stderr.String()
	if !strings.Contains(out, "config bind --help") {
		t.Errorf("agent hint must point at config bind --help: %s", out)
	}
	if strings.Contains(out, "config init") {
		t.Errorf("agent hint must not mention config init: %s", out)
	}
}

func TestAuthListRun_JSONMode_NoLoggedInUsers_WritesStdoutOnly(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	writeLogoutConfig(t, nil)

	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	if err := authListRun(&ListOptions{Factory: f, JSON: true}); err != nil {
		t.Fatalf("auth list should succeed when no users exist (exit 0); got: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout must be valid JSON: %v\nstdout=%s", err, stdout.String())
	}
	if payload["ok"] != true {
		t.Errorf("stdout.ok = %v, want true", payload["ok"])
	}
	users, ok := payload["users"].([]any)
	if !ok || len(users) != 0 {
		t.Errorf("stdout.users = %v, want empty array", payload["users"])
	}
	if payload["reason"] != "not_logged_in" {
		t.Errorf("stdout.reason = %v, want not_logged_in", payload["reason"])
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr must stay empty in JSON mode, got:\n%s", stderr.String())
	}
}

func TestAuthListRun_DefaultMode_NoLoggedInUsers_KeepsTextOutput(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	writeLogoutConfig(t, nil)

	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	if err := authListRun(&ListOptions{Factory: f}); err != nil {
		t.Fatalf("auth list should succeed when no users exist (exit 0); got: %v", err)
	}

	if stdout.Len() != 0 {
		t.Errorf("stdout must stay empty in default mode, got:\n%s", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "No logged-in users") ||
		!strings.Contains(got, "auth login") {
		t.Errorf("stderr = %q, want established no-users login hint", got)
	}
}

func TestAuthListRun_DistinguishesMissingFromCorruptStoredToken(t *testing.T) {
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LARKSUITE_CLI_DATA_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	writeLogoutConfig(t, []core.AppUser{
		{UserOpenId: "ou_missing", UserName: "Missing Token"},
		{UserOpenId: "ou_corrupt", UserName: "Corrupt Token"},
	})

	if err := keychain.Set(keychain.LarkCliService, "test-app:ou_corrupt", `{"accessToken":`); err != nil {
		t.Fatalf("keychain.Set() error = %v", err)
	}

	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	if err := authListRun(&ListOptions{Factory: f}); err != nil {
		t.Fatalf("authListRun() error = %v; want successful diagnostic list", err)
	}

	var got []struct {
		UserOpenID  string        `json:"userOpenId"`
		TokenStatus string        `json:"tokenStatus"`
		Error       *errs.Problem `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\nstdout=%s", err, stdout.String())
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].UserOpenID != "ou_missing" || got[0].TokenStatus != "no_token" || got[0].Error != nil {
		t.Fatalf("missing item = %#v, want historical no_token without error", got[0])
	}
	if got[1].UserOpenID != "ou_corrupt" || got[1].TokenStatus != "error" {
		t.Fatalf("corrupt item = %#v, want tokenStatus=error", got[1])
	}
	if got[1].Error == nil || got[1].Error.Category != errs.CategoryInternal || got[1].Error.Subtype != errs.SubtypeStorage {
		t.Fatalf("corrupt item error = %#v, want internal/storage", got[1].Error)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want list diagnostics entirely on stdout", stderr.String())
	}
}

func TestAuthListRun_ConcealedLoginKeepsStateWithoutDeadRecovery(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	writeLogoutConfig(t, nil)

	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandAuthLogin: surface.CommandConcealed,
	})
	if err := authListRunWithRecovery(
		&ListOptions{Factory: f},
		recovery.NewProjector(func() *surface.Plan { return plan }),
	); err != nil {
		t.Fatalf("auth list should remain a successful probe: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout must stay empty, got:\n%s", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "No logged-in users") ||
		strings.Contains(got, "auth login") {
		t.Fatalf("concealed recovery = %q, want state without dead login action", got)
	}
}

func TestAuthListRun_ConcealedConfigInitProjectsManualErrorOutput(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandConfigInit: surface.CommandConcealed,
	})
	if err := authListRunWithRecovery(
		&ListOptions{Factory: f},
		recovery.NewProjector(func() *surface.Plan { return plan }),
	); err != nil {
		t.Fatalf("auth list should remain a successful probe: %v", err)
	}
	if got := stderr.String(); strings.Contains(got, "config init") {
		t.Fatalf("manual config error rendering retained concealed recovery: %q", got)
	}
}
