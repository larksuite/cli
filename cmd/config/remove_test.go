// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
)

func TestConfigRemoveRun_UsesInjectedCallbacksAndSavesBeforeCleanup(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, nil)

	cfg := &core.MultiAppConfig{
		Apps: []core.AppConfig{
			{
				AppId:     "cli_app_a",
				AppSecret: core.SecretInput{Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:cli_app_a"}},
				Users:     []core.AppUser{{UserOpenId: "ou_a"}},
			},
			{
				AppId:     "cli_app_b",
				AppSecret: core.SecretInput{Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:cli_app_b"}},
				Users:     []core.AppUser{{UserOpenId: "ou_b1"}, {UserOpenId: "ou_b2"}},
			},
		},
	}

	callOrder := []string{}
	saveConfig := func(next *core.MultiAppConfig) error {
		callOrder = append(callOrder, "save")
		if len(next.Apps) != 0 {
			t.Fatalf("expected empty config, got %+v", next.Apps)
		}
		return nil
	}

	var secretRemovals []string
	removeSecret := func(input core.SecretInput, kc keychain.KeychainAccess) {
		callOrder = append(callOrder, "secret")
		if input.Ref != nil {
			secretRemovals = append(secretRemovals, input.Ref.ID)
		}
	}

	var tokenRemovals []string
	removeStoredToken := func(appID, userOpenID string) error {
		callOrder = append(callOrder, "token")
		tokenRemovals = append(tokenRemovals, appID+":"+userOpenID)
		return nil
	}

	if err := configRemoveRun(&ConfigRemoveOptions{
		Factory:           f,
		LoadConfig:        func() (*core.MultiAppConfig, error) { return cfg, nil },
		SaveConfig:        saveConfig,
		RemoveSecret:      removeSecret,
		RemoveStoredToken: removeStoredToken,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(callOrder) == 0 || callOrder[0] != "save" {
		t.Fatalf("expected save to happen first, order=%v", callOrder)
	}
	if len(secretRemovals) != 2 {
		t.Fatalf("secret removals = %v, want 2 entries", secretRemovals)
	}
	if len(tokenRemovals) != 3 {
		t.Fatalf("token removals = %v, want 3 entries", tokenRemovals)
	}
	got := stderr.String()
	if !strings.Contains(got, "Configuration removed") {
		t.Fatalf("expected success message on stderr, got %q", got)
	}
	if !strings.Contains(got, "Cleared tokens for 3 users") {
		t.Fatalf("expected exact cleanup summary, got %q", got)
	}
}

func TestConfigRemoveRun_WarnsWhenTokenCleanupFails(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, nil)

	loadConfig := func() (*core.MultiAppConfig, error) {
		return &core.MultiAppConfig{
			Apps: []core.AppConfig{{
				AppId:     "cli_app",
				AppSecret: core.SecretInput{Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:cli_app"}},
				Users:     []core.AppUser{{UserOpenId: "ou_123"}},
			}},
		}, nil
	}

	if err := configRemoveRun(&ConfigRemoveOptions{
		Factory:      f,
		LoadConfig:   loadConfig,
		SaveConfig:   func(*core.MultiAppConfig) error { return nil },
		RemoveSecret: func(core.SecretInput, keychain.KeychainAccess) {},
		RemoveStoredToken: func(appID, userOpenID string) error {
			return errors.New("keychain unavailable")
		},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := stderr.String()
	if !strings.Contains(got, `warning: failed to remove a stored token for app "cli_app": keychain unavailable`) {
		t.Fatalf("expected token cleanup warning, got %q", got)
	}
	if strings.Contains(got, "ou_123") {
		t.Fatalf("warning should not leak the user identifier, got %q", got)
	}
	if strings.Contains(got, "Cleared tokens for 1 users") {
		t.Fatalf("cleanup summary should not claim full success, got %q", got)
	}
	if !strings.Contains(got, "Token cleanup attempted for 1 users: removed 0, failed 1") {
		t.Fatalf("expected partial cleanup summary, got %q", got)
	}
}

func TestConfigRemoveRun_DoesNotCleanupWhenSaveFails(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	cfg := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId:     "cli_app",
			AppSecret: core.SecretInput{Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:cli_app"}},
			Users:     []core.AppUser{{UserOpenId: "ou_123"}},
		}},
	}

	secretCalls := 0
	tokenCalls := 0
	err := configRemoveRun(&ConfigRemoveOptions{
		Factory:    f,
		LoadConfig: func() (*core.MultiAppConfig, error) { return cfg, nil },
		SaveConfig: func(*core.MultiAppConfig) error { return errors.New("disk full") },
		RemoveSecret: func(core.SecretInput, keychain.KeychainAccess) {
			secretCalls++
		},
		RemoveStoredToken: func(string, string) error {
			tokenCalls++
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected save failure")
	}
	if !strings.Contains(err.Error(), "failed to save config: disk full") {
		t.Fatalf("unexpected error: %v", err)
	}
	if secretCalls != 0 {
		t.Fatalf("expected no secret cleanup on save failure, got %d calls", secretCalls)
	}
	if tokenCalls != 0 {
		t.Fatalf("expected no token cleanup on save failure, got %d calls", tokenCalls)
	}
}

func TestConfigRemoveRun_DistinguishesLoadErrors(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	notConfiguredErr := configRemoveRun(&ConfigRemoveOptions{
		Factory:           f,
		LoadConfig:        func() (*core.MultiAppConfig, error) { return nil, os.ErrNotExist },
		SaveConfig:        func(*core.MultiAppConfig) error { return nil },
		RemoveSecret:      func(core.SecretInput, keychain.KeychainAccess) {},
		RemoveStoredToken: func(string, string) error { return nil },
	})
	if notConfiguredErr == nil || !strings.Contains(notConfiguredErr.Error(), "not configured yet") {
		t.Fatalf("expected not configured error, got %v", notConfiguredErr)
	}

	loadErr := configRemoveRun(&ConfigRemoveOptions{
		Factory:           f,
		LoadConfig:        func() (*core.MultiAppConfig, error) { return nil, errors.New("permission denied") },
		SaveConfig:        func(*core.MultiAppConfig) error { return nil },
		RemoveSecret:      func(core.SecretInput, keychain.KeychainAccess) {},
		RemoveStoredToken: func(string, string) error { return nil },
	})
	if loadErr == nil {
		t.Fatal("expected load failure")
	}
	if !strings.Contains(loadErr.Error(), "failed to load config: permission denied") {
		t.Fatalf("unexpected load error: %v", loadErr)
	}
}

func TestConfigRemoveRun_RejectsUninitializedOptions(t *testing.T) {
	err := configRemoveRun(&ConfigRemoveOptions{})
	if err == nil {
		t.Fatal("expected initialization error")
	}
	if !strings.Contains(err.Error(), "config remove options not initialized") {
		t.Fatalf("unexpected error: %v", err)
	}
}
