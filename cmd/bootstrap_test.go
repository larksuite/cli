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

func TestBootstrapProfileEnvFallback(t *testing.T) {
	t.Run("flag wins over env", func(t *testing.T) {
		t.Setenv(envvars.CliProfile, "tenant_env")
		inv, err := BootstrapInvocationContext([]string{"--profile", "tenant_flag", "whoami"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if inv.Profile != "tenant_flag" {
			t.Errorf("got %q, want tenant_flag", inv.Profile)
		}
		if !inv.ProfileFromFlag {
			t.Errorf("ProfileFromFlag = false, want true")
		}
	})
	t.Run("explicit empty flag clears env selection", func(t *testing.T) {
		t.Setenv(envvars.CliProfile, "tenant_env")
		inv, err := BootstrapInvocationContext([]string{"--profile=", "whoami"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if inv.Profile != "" {
			t.Errorf("got %q, want empty", inv.Profile)
		}
		if !inv.ProfileFromFlag {
			t.Errorf("ProfileFromFlag = false, want true")
		}
	})
	t.Run("env used when flag absent", func(t *testing.T) {
		t.Setenv(envvars.CliProfile, "tenant_env")
		inv, err := BootstrapInvocationContext([]string{"whoami"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if inv.Profile != "tenant_env" {
			t.Errorf("got %q, want tenant_env", inv.Profile)
		}
		if inv.ProfileFromFlag {
			t.Errorf("ProfileFromFlag = true, want false")
		}
	})
	t.Run("empty when neither set", func(t *testing.T) {
		t.Setenv(envvars.CliProfile, "")
		inv, err := BootstrapInvocationContext([]string{"whoami"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if inv.Profile != "" {
			t.Errorf("got %q, want empty", inv.Profile)
		}
		if inv.ProfileFromFlag {
			t.Errorf("ProfileFromFlag = true, want false")
		}
	})
}
