// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
)

func TestConfigRemoveRun_SaveFailureDoesNotClearSecretsOrTokens(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	loadConfig := func() (*core.MultiAppConfig, error) {
		return &core.MultiAppConfig{
			Apps: []core.AppConfig{{
				AppId:     "cli_app",
				AppSecret: core.SecretInput{Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:cli_app"}},
				Users:     []core.AppUser{{UserOpenId: "ou_123"}},
			}},
		}, nil
	}

	saveErr := errors.New("disk full")
	saveConfig := func(*core.MultiAppConfig) error {
		return saveErr
	}

	secretRemoved := false
	tokenRemoved := false
	removeSecret := func(input core.SecretInput, kc keychain.KeychainAccess) {
		secretRemoved = true
	}
	removeStoredToken := func(appID, userOpenID string) error {
		tokenRemoved = true
		return nil
	}

	err := configRemoveRun(&ConfigRemoveOptions{
		Factory:           f,
		LoadConfig:        loadConfig,
		SaveConfig:        saveConfig,
		RemoveSecret:      removeSecret,
		RemoveStoredToken: removeStoredToken,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to save config: disk full") {
		t.Fatalf("unexpected error: %v", err)
	}
	if secretRemoved {
		t.Fatal("secret cleanup should not run when saving empty config fails")
	}
	if tokenRemoved {
		t.Fatal("token cleanup should not run when saving empty config fails")
	}
}

func TestConfigRemoveRun_ClearsSecretsAndTokensAfterSuccessfulSave(t *testing.T) {
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
	loadConfig := func() (*core.MultiAppConfig, error) { return cfg, nil }

	savedEmpty := false
	saveConfig := func(next *core.MultiAppConfig) error {
		if len(next.Apps) != 0 {
			t.Fatalf("expected empty config, got %+v", next.Apps)
		}
		savedEmpty = true
		return nil
	}

	var secretRemovals []string
	removeSecret := func(input core.SecretInput, kc keychain.KeychainAccess) {
		if input.Ref != nil {
			secretRemovals = append(secretRemovals, input.Ref.ID)
		}
	}

	var tokenRemovals []string
	removeStoredToken := func(appID, userOpenID string) error {
		tokenRemovals = append(tokenRemovals, appID+":"+userOpenID)
		return nil
	}

	if err := configRemoveRun(&ConfigRemoveOptions{
		Factory:           f,
		LoadConfig:        loadConfig,
		SaveConfig:        saveConfig,
		RemoveSecret:      removeSecret,
		RemoveStoredToken: removeStoredToken,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !savedEmpty {
		t.Fatal("expected empty config to be saved before cleanup")
	}
	if len(secretRemovals) != 2 {
		t.Fatalf("secret removals = %v, want 2 entries", secretRemovals)
	}
	if len(tokenRemovals) != 3 {
		t.Fatalf("token removals = %v, want 3 entries", tokenRemovals)
	}
	if got := stderr.String(); got == "" {
		t.Fatal("expected success message on stderr")
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

	saveConfig := func(*core.MultiAppConfig) error { return nil }
	removeSecret := func(core.SecretInput, keychain.KeychainAccess) {}
	removeStoredToken := func(appID, userOpenID string) error {
		return errors.New("keychain unavailable")
	}

	if err := configRemoveRun(&ConfigRemoveOptions{
		Factory:           f,
		LoadConfig:        loadConfig,
		SaveConfig:        saveConfig,
		RemoveSecret:      removeSecret,
		RemoveStoredToken: removeStoredToken,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := stderr.String()
	if !strings.Contains(got, `warning: failed to remove stored token for app "cli_app" user "ou_123": keychain unavailable`) {
		t.Fatalf("expected token cleanup warning, got %q", got)
	}
}
