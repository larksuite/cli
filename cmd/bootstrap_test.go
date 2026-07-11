// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"testing"

	"github.com/larksuite/cli/internal/envvars"
)

func TestBootstrapInvocationContext_ProfileFlag(t *testing.T) {
	inv, err := BootstrapInvocationContext([]string{"--profile", "target", "auth", "status"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext() error = %v", err)
	}
	if inv.Profile != "target" {
		t.Fatalf("BootstrapInvocationContext() profile = %q, want %q", inv.Profile, "target")
	}
}

func TestBootstrapInvocationContext_ProfileEquals(t *testing.T) {
	inv, err := BootstrapInvocationContext([]string{"auth", "status", "--profile=target"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext() error = %v", err)
	}
	if inv.Profile != "target" {
		t.Fatalf("BootstrapInvocationContext() profile = %q, want %q", inv.Profile, "target")
	}
}

func TestBootstrapInvocationContext_IgnoresUnknownFlags(t *testing.T) {
	inv, err := BootstrapInvocationContext([]string{"auth", "status", "--verify", "--profile", "target"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext() error = %v", err)
	}
	if inv.Profile != "target" {
		t.Fatalf("BootstrapInvocationContext() profile = %q, want %q", inv.Profile, "target")
	}
}

func TestBootstrapInvocationContext_MissingProfileValue(t *testing.T) {
	if _, err := BootstrapInvocationContext([]string{"auth", "status", "--profile"}); err == nil {
		t.Fatal("BootstrapInvocationContext() error = nil, want non-nil")
	}
}

func TestBootstrapInvocationContext_HelpFlag(t *testing.T) {
	inv, err := BootstrapInvocationContext([]string{"--help"})
	if err != nil {
		t.Fatalf("--help should not error, got: %v", err)
	}
	if inv.Profile != "" {
		t.Fatalf("profile = %q, want empty", inv.Profile)
	}
}

func TestBootstrapInvocationContext_ShortHelp(t *testing.T) {
	inv, err := BootstrapInvocationContext([]string{"-h"})
	if err != nil {
		t.Fatalf("-h should not error, got: %v", err)
	}
	if inv.Profile != "" {
		t.Fatalf("profile = %q, want empty", inv.Profile)
	}
}

func TestBootstrapInvocationContext_HelpWithProfile(t *testing.T) {
	inv, err := BootstrapInvocationContext([]string{"--profile", "target", "--help"})
	if err != nil {
		t.Fatalf("--profile + --help should not error, got: %v", err)
	}
	if inv.Profile != "target" {
		t.Fatalf("profile = %q, want %q", inv.Profile, "target")
	}
}

func TestBootstrapInvocationContext_RuntimeAppIDEnv(t *testing.T) {
	t.Setenv(envvars.CliRuntimeAppID, " env-app ")
	inv, err := BootstrapInvocationContext([]string{"auth", "status"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext() error = %v", err)
	}
	if inv.Profile != "env-app" {
		t.Fatalf("profile = %q, want %q (env value, trimmed)", inv.Profile, "env-app")
	}
}

func TestBootstrapInvocationContext_ProfileFlagBeatsRuntimeAppIDEnv(t *testing.T) {
	t.Setenv(envvars.CliRuntimeAppID, "env-app")
	inv, err := BootstrapInvocationContext([]string{"--profile", "flag-app", "auth", "status"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext() error = %v", err)
	}
	if inv.Profile != "flag-app" {
		t.Fatalf("profile = %q, want %q (--profile wins over env)", inv.Profile, "flag-app")
	}
}
