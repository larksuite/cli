// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func writeBootstrapProjectConfig(t *testing.T, dir, body string) string {
	t.Helper()
	path := core.ProjectConfigPath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("MkdirAll(project config dir): %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile(project config): %v", err)
	}
	return path
}

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
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatalf("Mkdir(.git): %v", err)
	}
	writeBootstrapProjectConfig(t, repo, `{"profile":"bytedance"}`)
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
	if inv.ProfileConfigPath != core.ProjectConfigPath(wantRepo) {
		t.Fatalf("ProfileConfigPath = %q", inv.ProfileConfigPath)
	}
}

func TestBootstrapInvocationContext_ProfileFlagOverridesProjectProfile(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatalf("Mkdir(.git): %v", err)
	}
	writeBootstrapProjectConfig(t, repo, `{"profile":"project"}`)
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
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatalf("Mkdir(.git): %v", err)
	}
	writeBootstrapProjectConfig(t, repo, `{`)
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
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatalf("Mkdir(.git): %v", err)
	}
	writeBootstrapProjectConfig(t, repo, `{`)
	cmdutil.TestChdir(t, repo)

	_, err := BootstrapInvocationContext([]string{"profile", "current"})
	if err == nil {
		t.Fatal("BootstrapInvocationContext() error = nil, want malformed project config error")
	}
	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error type = %T, want *core.ConfigError", err)
	}
	if cfgErr.Code != 3 || cfgErr.Type != "config" {
		t.Fatalf("ConfigError metadata = code:%d type:%q", cfgErr.Code, cfgErr.Type)
	}
	wantRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("EvalSymlinks(repo): %v", err)
	}
	wantMsg := "invalid project config " + core.ProjectConfigPath(wantRepo) + ": unexpected end of JSON input"
	if cfgErr.Message != wantMsg {
		t.Fatalf("ConfigError.Message = %q, want %q", cfgErr.Message, wantMsg)
	}
}

func TestBootstrapInvocationContext_CompletionValueDoesNotSkipProjectProfile(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatalf("Mkdir(.git): %v", err)
	}
	writeBootstrapProjectConfig(t, repo, `{"profile":"bytedance"}`)
	cmdutil.TestChdir(t, repo)

	inv, err := BootstrapInvocationContext([]string{"auth", "status", "completion"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext() error = %v", err)
	}
	if inv.ProfileSource != core.ProfileSourceProject || inv.Profile != "bytedance" {
		t.Fatalf("invocation = %#v, want project profile", inv)
	}
}

func TestBootstrapInvocationContext_CompletionCommandSkipsMalformedProjectConfig(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatalf("Mkdir(.git): %v", err)
	}
	writeBootstrapProjectConfig(t, repo, `{`)
	cmdutil.TestChdir(t, repo)

	inv, err := BootstrapInvocationContext([]string{"completion"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext() error = %v", err)
	}
	if inv.Profile != "" || inv.ProfileSource != core.ProfileSourceGlobal {
		t.Fatalf("invocation = %#v, want global without profile", inv)
	}
}

func TestBootstrapInvocationContext_CompletionAfterUnknownValueFlagSkipsMalformedProjectConfig(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatalf("Mkdir(.git): %v", err)
	}
	writeBootstrapProjectConfig(t, repo, `{`)
	cmdutil.TestChdir(t, repo)

	inv, err := BootstrapInvocationContext([]string{"--config-dir", t.TempDir(), "completion"})
	if err != nil {
		t.Fatalf("BootstrapInvocationContext() error = %v", err)
	}
	if inv.Profile != "" || inv.ProfileSource != core.ProfileSourceGlobal {
		t.Fatalf("invocation = %#v, want global without profile", inv)
	}
}
