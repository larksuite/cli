// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

// newCmdWithAsFlag creates a cobra.Command with a --as string flag for testing.
func newCmdWithAsFlag(asValue string, changed bool) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("as", "auto", "identity")
	if changed {
		_ = cmd.Flags().Set("as", asValue)
	}
	return cmd
}

// --- ResolveAs tests ---

func TestResolveAs_ExplicitAs(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := newCmdWithAsFlag("bot", true)

	got := f.ResolveAs(context.Background(), cmd, core.AsBot)
	if got != core.AsBot {
		t.Errorf("want bot, got %s", got)
	}
	if f.IdentityAutoDetected {
		t.Error("IdentityAutoDetected should be false for explicit --as")
	}
	if f.ResolvedIdentity != core.AsBot {
		t.Errorf("ResolvedIdentity want bot, got %s", f.ResolvedIdentity)
	}
}

func TestResolveAs_ExplicitAsUser(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := newCmdWithAsFlag("user", true)

	got := f.ResolveAs(context.Background(), cmd, core.AsUser)
	if got != core.AsUser {
		t.Errorf("want user, got %s", got)
	}
	if f.ResolvedIdentity != core.AsUser {
		t.Errorf("ResolvedIdentity want user, got %s", f.ResolvedIdentity)
	}
}

func TestResolveAs_ExplicitAuto_FallsToAutoDetect(t *testing.T) {
	// --as auto explicitly: should fall through to auto-detect
	// Config has no UserOpenId → auto-detect returns bot
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := newCmdWithAsFlag("auto", true)

	got := f.ResolveAs(context.Background(), cmd, "auto")
	if got != core.AsBot {
		t.Errorf("want bot (auto-detect, no login), got %s", got)
	}
	if !f.IdentityAutoDetected {
		t.Error("IdentityAutoDetected should be true for auto-detect path")
	}
}

func TestResolveAs_DefaultAs_FromConfig(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{
		AppID: "a", AppSecret: "s",
		DefaultAs: "bot",
	})
	cmd := newCmdWithAsFlag("auto", false) // --as not changed

	got := f.ResolveAs(context.Background(), cmd, "auto")
	if got != core.AsBot {
		t.Errorf("want bot (from default-as config), got %s", got)
	}
	if f.IdentityAutoDetected {
		t.Error("IdentityAutoDetected should be false for default-as path")
	}
}

func TestResolveAs_DefaultAs_EnvDoesNotBypassConfigSource(t *testing.T) {
	t.Setenv(envvars.CliDefaultAs, "user")

	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := newCmdWithAsFlag("auto", false)

	got := f.ResolveAs(context.Background(), cmd, "auto")
	if got != core.AsBot {
		t.Errorf("want bot (env default-as should not bypass config source), got %s", got)
	}
	if !f.IdentityAutoDetected {
		t.Error("IdentityAutoDetected should be true when no account default-as is set")
	}
}

func TestResolveAs_DefaultAs_AutoValue_FallsToAutoDetect(t *testing.T) {
	// default-as = "auto" should fall through to auto-detect
	f, _, _, _ := TestFactory(t, &core.CliConfig{
		AppID: "a", AppSecret: "s",
		DefaultAs: "auto",
	})
	cmd := newCmdWithAsFlag("auto", false)

	got := f.ResolveAs(context.Background(), cmd, "auto")
	// No UserOpenId → auto-detect returns bot
	if got != core.AsBot {
		t.Errorf("want bot (auto-detect), got %s", got)
	}
	if !f.IdentityAutoDetected {
		t.Error("IdentityAutoDetected should be true")
	}
}

func TestResolveAs_NilCmd_AutoDetect(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})

	got := f.ResolveAs(context.Background(), nil, "auto")
	if got != core.AsBot {
		t.Errorf("want bot, got %s", got)
	}
}

// --- CheckIdentity tests ---

func TestCheckIdentity_Supported(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})

	err := f.CheckIdentity(core.AsBot, []string{"bot", "user"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.ResolvedIdentity != core.AsBot {
		t.Errorf("ResolvedIdentity want bot, got %s", f.ResolvedIdentity)
	}
}

func TestCheckIdentity_Supported_UserOnly(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})

	err := f.CheckIdentity(core.AsUser, []string{"user"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.ResolvedIdentity != core.AsUser {
		t.Errorf("ResolvedIdentity want user, got %s", f.ResolvedIdentity)
	}
}

func TestCheckIdentity_Unsupported_Explicit(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	f.IdentityAutoDetected = false // explicit --as

	err := f.CheckIdentity(core.AsUser, []string{"bot"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--as user is not supported") {
		t.Errorf("unexpected error message: %v", err)
	}
	if !strings.Contains(err.Error(), "bot") {
		t.Errorf("error should mention supported identity: %v", err)
	}
}

func TestCheckIdentity_Unsupported_AutoDetected(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	f.IdentityAutoDetected = true

	err := f.CheckIdentity(core.AsBot, []string{"user"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "requires user identity") {
		t.Errorf("expected 'requires user identity' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "auth login") {
		t.Errorf("expected 'auth login' hint in error, got: %v", err)
	}
}

// --- NewAPIClient / NewAPIClientWithConfig tests ---

func TestNewAPIClient(t *testing.T) {
	cfg := &core.CliConfig{AppID: "a", AppSecret: "s", Brand: core.BrandLark}
	f, _, _, _ := TestFactory(t, cfg)

	ac, err := f.NewAPIClient()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ac.Config.AppID != "a" {
		t.Errorf("want AppID a, got %s", ac.Config.AppID)
	}
}

func TestNewAPIClientWithConfig(t *testing.T) {
	cfg := &core.CliConfig{AppID: "a", AppSecret: "s", Brand: core.BrandLark}
	f, _, _, _ := TestFactory(t, cfg)

	ac, err := f.NewAPIClientWithConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ac.Config.AppID != "a" {
		t.Errorf("want AppID a, got %s", ac.Config.AppID)
	}
	if ac.SDK == nil {
		t.Error("SDK should not be nil")
	}
	if ac.HTTP == nil {
		t.Error("HTTP should not be nil")
	}
}

func TestNewAPIClientWithConfig_NilIOStreams(t *testing.T) {
	cfg := &core.CliConfig{AppID: "a", AppSecret: "s", Brand: core.BrandLark}
	f, _, _, _ := TestFactory(t, cfg)
	f.IOStreams = nil

	ac, err := f.NewAPIClientWithConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ac == nil {
		t.Fatal("expected non-nil APIClient")
	}
}

// --- ResolveStrictMode tests ---

func TestResolveStrictMode_Off(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	if got := f.ResolveStrictMode(context.Background()); got != core.StrictModeOff {
		t.Errorf("expected off, got %q", got)
	}
}

func TestResolveStrictMode_BotFromAccount(t *testing.T) {
	cfg := &core.CliConfig{AppID: "a", AppSecret: "s", SupportedIdentities: 2} // SupportsBot = 2
	f, _, _, _ := TestFactory(t, cfg)
	if got := f.ResolveStrictMode(context.Background()); got != core.StrictModeBot {
		t.Errorf("expected bot, got %q", got)
	}
}

func TestResolveStrictMode_UserFromAccount(t *testing.T) {
	cfg := &core.CliConfig{AppID: "a", AppSecret: "s", SupportedIdentities: 1} // SupportsUser = 1
	f, _, _, _ := TestFactory(t, cfg)
	if got := f.ResolveStrictMode(context.Background()); got != core.StrictModeUser {
		t.Errorf("expected user, got %q", got)
	}
}

func TestResolveStrictMode_BothIdentities(t *testing.T) {
	cfg := &core.CliConfig{AppID: "a", AppSecret: "s", SupportedIdentities: 3} // SupportsAll = 3
	f, _, _, _ := TestFactory(t, cfg)
	if got := f.ResolveStrictMode(context.Background()); got != core.StrictModeOff {
		t.Errorf("expected off when both supported, got %q", got)
	}
}

func TestResolveStrictMode_NilCredential(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	f.Credential = nil
	if got := f.ResolveStrictMode(context.Background()); got != core.StrictModeOff {
		t.Errorf("expected off with nil credential, got %q", got)
	}
}

// --- CheckStrictMode tests ---

func TestCheckStrictMode_BotMode_BotAllowed(t *testing.T) {
	cfg := &core.CliConfig{AppID: "a", AppSecret: "s", SupportedIdentities: 2}
	f, _, _, _ := TestFactory(t, cfg)
	if err := f.CheckStrictMode(context.Background(), core.AsBot); err != nil {
		t.Errorf("bot should be allowed in bot mode, got: %v", err)
	}
}

func TestCheckStrictMode_BotMode_UserBlocked(t *testing.T) {
	cfg := &core.CliConfig{AppID: "a", AppSecret: "s", SupportedIdentities: 2}
	f, _, _, _ := TestFactory(t, cfg)
	err := f.CheckStrictMode(context.Background(), core.AsUser)
	if err == nil {
		t.Fatal("expected error for user in bot mode")
	}
	if !strings.Contains(err.Error(), "strict mode") {
		t.Errorf("error should mention strict mode, got: %v", err)
	}
}

func TestCheckStrictMode_UserMode_UserAllowed(t *testing.T) {
	cfg := &core.CliConfig{AppID: "a", AppSecret: "s", SupportedIdentities: 1}
	f, _, _, _ := TestFactory(t, cfg)
	if err := f.CheckStrictMode(context.Background(), core.AsUser); err != nil {
		t.Errorf("user should be allowed in user mode, got: %v", err)
	}
}

func TestCheckStrictMode_UserMode_BotBlocked(t *testing.T) {
	cfg := &core.CliConfig{AppID: "a", AppSecret: "s", SupportedIdentities: 1}
	f, _, _, _ := TestFactory(t, cfg)
	err := f.CheckStrictMode(context.Background(), core.AsBot)
	if err == nil {
		t.Fatal("expected error for bot in user mode")
	}
}

func TestCheckStrictMode_Off_BothAllowed(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	if err := f.CheckStrictMode(context.Background(), core.AsUser); err != nil {
		t.Errorf("user should be allowed when off: %v", err)
	}
	if err := f.CheckStrictMode(context.Background(), core.AsBot); err != nil {
		t.Errorf("bot should be allowed when off: %v", err)
	}
}

// --- ResolveAs strict mode tests ---

func TestResolveAs_StrictModeBot_ForceBot(t *testing.T) {
	cfg := &core.CliConfig{AppID: "a", AppSecret: "s", SupportedIdentities: 2}
	f, _, _, _ := TestFactory(t, cfg)
	cmd := newCmdWithAsFlag("auto", false)
	got := f.ResolveAs(context.Background(), cmd, "auto")
	if got != core.AsBot {
		t.Errorf("bot mode should force bot, got %s", got)
	}
}

func TestResolveAs_StrictModeUser_ForceUser(t *testing.T) {
	cfg := &core.CliConfig{AppID: "a", AppSecret: "s", SupportedIdentities: 1}
	f, _, _, _ := TestFactory(t, cfg)
	cmd := newCmdWithAsFlag("auto", false)
	got := f.ResolveAs(context.Background(), cmd, "auto")
	if got != core.AsUser {
		t.Errorf("user mode should force user, got %s", got)
	}
}

func TestResolveAs_StrictModeBot_IgnoresDefaultAsUser(t *testing.T) {
	cfg := &core.CliConfig{AppID: "a", AppSecret: "s", DefaultAs: "user", SupportedIdentities: 2}
	f, _, _, _ := TestFactory(t, cfg)
	cmd := newCmdWithAsFlag("auto", false)
	got := f.ResolveAs(context.Background(), cmd, "auto")
	if got != core.AsBot {
		t.Errorf("bot mode should override default-as user, got %s", got)
	}
}
