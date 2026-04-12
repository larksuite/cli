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
	if got := stderr.String(); !strings.Contains(got, "Configuration removed") {
		t.Fatalf("expected success message on stderr, got %q", got)
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
	if !strings.Contains(got, `warning: failed to remove stored token for app "cli_app" user "ou_123": keychain unavailable`) {
		t.Fatalf("expected token cleanup warning, got %q", got)
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
