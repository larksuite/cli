// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
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
	t.Setenv(envvars.CliProfile, "")
	inv, err := BootstrapInvocationContext([]string{"--help"})
	if err != nil {
		t.Fatalf("--help should not error, got: %v", err)
	}
	if inv.Profile != "" {
		t.Fatalf("profile = %q, want empty", inv.Profile)
	}
}

func TestBootstrapInvocationContext_ShortHelp(t *testing.T) {
	t.Setenv(envvars.CliProfile, "")
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

func TestBootstrapInvocationContext_ProfilePrecedence(t *testing.T) {
	for _, tc := range []struct {
		name                string
		environment         string
		args                []string
		wantProfile         string
		wantFromEnvironment bool
	}{
		{"environment default", "session", []string{"whoami"}, "session", true},
		{"flag overrides environment", "session", []string{"whoami", "--profile", "command"}, "command", false},
		{"empty flag suppresses environment", "session", []string{"whoami", "--profile="}, "", false},
		{"empty environment is unset", "", []string{"whoami"}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envvars.CliProfile, tc.environment)
			inv, err := BootstrapInvocationContext(tc.args)
			if err != nil {
				t.Fatalf("BootstrapInvocationContext() error = %v", err)
			}
			if inv.Profile != tc.wantProfile || inv.ProfileFromEnvironment != tc.wantFromEnvironment {
				t.Fatalf("profile = %q, fromEnvironment = %v", inv.Profile, inv.ProfileFromEnvironment)
			}
		})
	}
}

func TestIsDeferredBootstrapProfileError(t *testing.T) {
	if !isDeferredBootstrapProfileError(errors.New("flag needs an argument: --profile")) {
		t.Fatal("missing --profile value must be deferred to the completed Cobra tree")
	}
	for _, err := range []error{
		nil,
		errors.New("flag needs an argument: --future"),
		errors.New("invalid argument for --profile"),
	} {
		if isDeferredBootstrapProfileError(err) {
			t.Fatalf("unexpected deferred bootstrap error: %v", err)
		}
	}
}
