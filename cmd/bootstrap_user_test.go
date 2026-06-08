// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/envvars"
)

// User-selection precedence: --user flag > LARKSUITE_CLI_OPEN_ID env > unset.
// The bootstrap layer is the only reader of the env var; downstream resolvers
// stay env-agnostic (locked separately by canary tests).

func TestBootstrap_UserFlag_Pre(t *testing.T) {
	clearUserEnv(t)
	inv, err := BootstrapInvocationContext([]string{"--user", "ou_alice", "auth", "status"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext: %v", err)
	}
	if inv.UserOpenId != "ou_alice" {
		t.Errorf("UserOpenId = %q, want ou_alice", inv.UserOpenId)
	}
	if inv.UserSource != "flag" {
		t.Errorf("UserSource = %q, want flag", inv.UserSource)
	}
}

// --user=value after a subcommand requires Interspersed=true.
func TestBootstrap_UserFlag_Post(t *testing.T) {
	clearUserEnv(t)
	inv, err := BootstrapInvocationContext([]string{"auth", "status", "--user=ou_alice"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext: %v", err)
	}
	if inv.UserOpenId != "ou_alice" {
		t.Errorf("UserOpenId = %q, want ou_alice", inv.UserOpenId)
	}
	if inv.UserSource != "flag" {
		t.Errorf("UserSource = %q, want flag", inv.UserSource)
	}
}

// Explicit --user= must error rather than fall through to env (typo trap).
func TestBootstrap_UserFlag_EmptyValue_Errors(t *testing.T) {
	clearUserEnv(t)
	t.Setenv(envvars.CliOpenID, "ou_from_env")

	inv, err := BootstrapInvocationContext([]string{"--user=", "auth", "status"})
	if err == nil {
		t.Fatalf("expected error, got inv=%+v", inv)
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *errs.ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", cfgErr.Subtype, errs.SubtypeInvalidArgument)
	}
	if !strings.Contains(cfgErr.Message, "--user requires a non-empty value") {
		t.Errorf("Message missing key text: %q", cfgErr.Message)
	}
	if inv.UserOpenId != "" || inv.UserSource != "" {
		t.Errorf("InvocationContext should be zero-valued on error, got %+v", inv)
	}
}

// Whitespace-only --user is the same explicit-blank typo case.
func TestBootstrap_UserFlag_WhitespaceFlag_Errors(t *testing.T) {
	clearUserEnv(t)
	t.Setenv(envvars.CliOpenID, "ou_from_env")

	_, err := BootstrapInvocationContext([]string{"--user=   ", "auth"})
	if err == nil {
		t.Fatal("expected error for whitespace-only --user, got nil")
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) || cfgErr.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("expected ConfigError(SubtypeInvalidArgument), got %T %v", err, err)
	}
}

func TestBootstrap_UserEnv_Used(t *testing.T) {
	t.Setenv(envvars.CliOpenID, "ou_bob")
	inv, err := BootstrapInvocationContext([]string{"auth", "status"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext: %v", err)
	}
	if inv.UserOpenId != "ou_bob" {
		t.Errorf("UserOpenId = %q, want ou_bob", inv.UserOpenId)
	}
	if inv.UserSource != "env" {
		t.Errorf("UserSource = %q, want env", inv.UserSource)
	}
}

// Whitespace env is treated as unset (asymmetric with empty flag, which errors).
func TestBootstrap_UserEnv_WhitespaceTreatedAsUnset(t *testing.T) {
	t.Setenv(envvars.CliOpenID, "   ")
	inv, err := BootstrapInvocationContext([]string{"auth"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext: %v", err)
	}
	if inv.UserOpenId != "" {
		t.Errorf("UserOpenId = %q, want empty (whitespace env is unset)", inv.UserOpenId)
	}
	if inv.UserSource != "" {
		t.Errorf("UserSource = %q, want empty", inv.UserSource)
	}
}

// Flag wins over env, and Source must reflect flag so error hints attribute correctly.
func TestBootstrap_FlagBeatsEnv(t *testing.T) {
	t.Setenv(envvars.CliOpenID, "ou_bob")
	inv, err := BootstrapInvocationContext([]string{"--user", "ou_alice", "auth"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext: %v", err)
	}
	if inv.UserOpenId != "ou_alice" {
		t.Errorf("UserOpenId = %q, want ou_alice", inv.UserOpenId)
	}
	if inv.UserSource != "flag" {
		t.Errorf("UserSource = %q, want flag (env should not have been read)", inv.UserSource)
	}
}

// Legacy single-user path: empty UserOpenId so resolver walks CurrentUser then Users[0].
func TestBootstrap_BothUnset(t *testing.T) {
	clearUserEnv(t)
	inv, err := BootstrapInvocationContext([]string{"auth"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext: %v", err)
	}
	if inv.UserOpenId != "" || inv.UserSource != "" {
		t.Errorf("expected empty user fields, got %+v", inv)
	}
}

// Orthogonal selectors compose.
func TestBootstrap_UserAndProfileTogether(t *testing.T) {
	clearUserEnv(t)
	inv, err := BootstrapInvocationContext([]string{"--profile", "p1", "--user", "u1", "auth"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext: %v", err)
	}
	if inv.Profile != "p1" {
		t.Errorf("Profile = %q, want p1", inv.Profile)
	}
	if inv.UserOpenId != "u1" || inv.UserSource != "flag" {
		t.Errorf("user fields = (%q,%q), want (u1,flag)", inv.UserOpenId, inv.UserSource)
	}
}

// --help with --user must not error (matches --profile + --help behavior).
func TestBootstrap_UserFlag_HelpStillWorks(t *testing.T) {
	clearUserEnv(t)
	inv, err := BootstrapInvocationContext([]string{"--user", "ou_alice", "--help"})
	if err != nil {
		t.Fatalf("--help with --user should not error, got: %v", err)
	}
	if inv.UserOpenId != "ou_alice" {
		t.Errorf("UserOpenId = %q, want ou_alice", inv.UserOpenId)
	}
}

// Whitespace around a real flag value is trimmed.
func TestBootstrap_UserFlag_Trimmed(t *testing.T) {
	clearUserEnv(t)
	inv, err := BootstrapInvocationContext([]string{"--user", "  ou_alice  ", "auth"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext: %v", err)
	}
	if inv.UserOpenId != "ou_alice" {
		t.Errorf("UserOpenId = %q, want ou_alice (trimmed)", inv.UserOpenId)
	}
}

// Env values are trimmed too — flag and env paths agree on what "empty" means.
func TestBootstrap_UserEnv_Trimmed(t *testing.T) {
	t.Setenv(envvars.CliOpenID, "  ou_bob  ")
	inv, err := BootstrapInvocationContext([]string{"auth"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext: %v", err)
	}
	if inv.UserOpenId != "ou_bob" {
		t.Errorf("UserOpenId = %q, want ou_bob (trimmed)", inv.UserOpenId)
	}
}

// clearUserEnv unsets LARKSUITE_CLI_OPEN_ID so a developer's shell env can't
// leak into flag-only tests.
func clearUserEnv(t *testing.T) {
	t.Helper()
	t.Setenv(envvars.CliOpenID, "")
}
