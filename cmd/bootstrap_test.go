// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
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

func TestBootstrapInvocationContext_ProjectProfile(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatalf("Mkdir(.git): %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, core.ProjectConfigFileName), []byte(`{"profile":"bytedance"}`), 0600); err != nil {
		t.Fatalf("WriteFile(project config): %v", err)
	}
	sub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatalf("MkdirAll(sub): %v", err)
	}
	cmdutil.TestChdir(t, sub)

	inv, err := BootstrapInvocationContext([]string{"auth", "status"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext() error = %v", err)
	}
	if inv.Profile != "bytedance" {
		t.Fatalf("profile = %q, want bytedance", inv.Profile)
	}
	if inv.ProfileSource != core.ProfileSourceProject {
		t.Fatalf("ProfileSource = %q, want project", inv.ProfileSource)
	}
	wantRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("EvalSymlinks(repo): %v", err)
	}
	if inv.ProfileConfigPath != filepath.Join(wantRepo, core.ProjectConfigFileName) {
		t.Fatalf("ProfileConfigPath = %q", inv.ProfileConfigPath)
	}
}

func TestBootstrapInvocationContext_ProfileFlagOverridesProjectProfile(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatalf("Mkdir(.git): %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, core.ProjectConfigFileName), []byte(`{"profile":"project"}`), 0600); err != nil {
		t.Fatalf("WriteFile(project config): %v", err)
	}
	cmdutil.TestChdir(t, repo)

	inv, err := BootstrapInvocationContext([]string{"--profile", "cli", "auth", "status"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext() error = %v", err)
	}
	if inv.Profile != "cli" {
		t.Fatalf("profile = %q, want cli", inv.Profile)
	}
	if inv.ProfileSource != core.ProfileSourceCLI {
		t.Fatalf("ProfileSource = %q, want cli", inv.ProfileSource)
	}
	if inv.ProfileConfigPath != "" {
		t.Fatalf("ProfileConfigPath = %q, want empty", inv.ProfileConfigPath)
	}
}

func TestBootstrapInvocationContext_ProfileBindSkipsMalformedProjectConfig(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatalf("Mkdir(.git): %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, core.ProjectConfigFileName), []byte(`{`), 0600); err != nil {
		t.Fatalf("WriteFile(project config): %v", err)
	}
	cmdutil.TestChdir(t, repo)

	inv, err := BootstrapInvocationContext([]string{"profile", "bind", "bytedance"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext() error = %v", err)
	}
	if inv.Profile != "" || inv.ProfileSource != core.ProfileSourceGlobal {
		t.Fatalf("invocation = %#v, want global without profile", inv)
	}
}

func TestBootstrapInvocationContext_ProfileCurrentReadsMalformedProjectConfig(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatalf("Mkdir(.git): %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, core.ProjectConfigFileName), []byte(`{`), 0600); err != nil {
		t.Fatalf("WriteFile(project config): %v", err)
	}
	cmdutil.TestChdir(t, repo)

	if _, err := BootstrapInvocationContext([]string{"profile", "current"}); err == nil {
		t.Fatal("BootstrapInvocationContext() error = nil, want malformed project config error")
	}
}
