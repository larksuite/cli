// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

// Storage/fallback/rollback behavior tests for config init live here.
// New command/flag/wiring tests should go to init_command_test.go.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
)

// unavailableSetKeychain simulates a keychain that only fails on writes with an unavailable error.
type unavailableSetKeychain struct{}

// Get satisfies the keychain interface for read paths used by these tests.
func (f *unavailableSetKeychain) Get(service, account string) (string, error) { return "", nil }

// Set satisfies the keychain interface and forces encrypted fallback in storage tests.
func (f *unavailableSetKeychain) Set(service, account, value string) error {
	return keychain.WrapUnavailable(errors.New("sandbox denied"))
}

// Remove satisfies the keychain interface for cleanup paths used by these tests.
func (f *unavailableSetKeychain) Remove(service, account string) error { return nil }

// trackingKeychain records remove calls and optionally injects write behavior for config-init tests.
type trackingKeychain struct {
	setFunc     func(service, account, value string) error
	removeCalls []string
}

// Get satisfies the keychain interface for read paths used by these tests.
func (t *trackingKeychain) Get(service, account string) (string, error) { return "", nil }

// Set satisfies the keychain interface and delegates to the configured test hook.
func (t *trackingKeychain) Set(service, account, value string) error {
	if t.setFunc != nil {
		return t.setFunc(service, account, value)
	}
	return nil
}

// Remove satisfies the keychain interface while recording the removed account IDs.
func (t *trackingKeychain) Remove(service, account string) error {
	t.removeCalls = append(t.removeCalls, account)
	return nil
}

// TestConfigInitRun_FallsBackToEncryptedSecretWhenKeychainUnavailable verifies config init persists app secrets via encrypted fallback on keychain failure.
func TestConfigInitRun_FallsBackToEncryptedSecretWhenKeychainUnavailable(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	f.Keychain = &unavailableSetKeychain{}

	opts := &ConfigInitOptions{
		Factory:   f,
		Ctx:       context.Background(),
		AppID:     "cli_test",
		appSecret: "secret123",
		Brand:     "feishu",
		Lang:      "zh",
	}

	if err := configInitRun(opts); err != nil {
		t.Fatalf("configInitRun returned error: %v", err)
	}

	cfg, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig: %v", err)
	}
	if len(cfg.Apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(cfg.Apps))
	}
	ref := cfg.Apps[0].AppSecret.Ref
	if ref == nil {
		t.Fatal("expected app secret to be stored as an encrypted fallback reference")
	}
	if ref.Source != "encrypted_file" {
		t.Fatalf("expected encrypted_file secret, got %q", ref.Source)
	}
	resolved, err := core.ResolveSecretInput(cfg.Apps[0].AppSecret, f.Keychain)
	if err != nil {
		t.Fatalf("ResolveSecretInput: %v", err)
	}
	if resolved != "secret123" {
		t.Fatalf("resolved secret = %q, want %q", resolved, "secret123")
	}
	if got := stderr.String(); got == "" || !strings.Contains(got, "filesystem permissions") {
		t.Fatalf("expected fallback warning in stderr, got %q", got)
	}
}

// TestConfigRemoveRun_RemovesEncryptedFallbackSecret verifies config removal cleans up encrypted fallback secrets.
func TestConfigRemoveRun_RemovesEncryptedFallbackSecret(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	if err := keychain.SetFallback(keychain.LarkCliService, "appsecret:cli_test", "secret123"); err != nil {
		t.Fatalf("SetFallback: %v", err)
	}

	config := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: "cli_test",
			AppSecret: core.SecretInput{
				Ref: &core.SecretRef{Source: "encrypted_file", ID: "appsecret:cli_test"},
			},
			Brand: core.BrandFeishu,
			Users: []core.AppUser{},
		}},
	}
	if err := core.SaveMultiAppConfig(config); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	opts := &ConfigRemoveOptions{Factory: f}

	if err := configRemoveRun(opts); err != nil {
		t.Fatalf("configRemoveRun returned error: %v", err)
	}
	if got := keychain.GetFallback(keychain.LarkCliService, "appsecret:cli_test"); got != "" {
		t.Fatalf("expected encrypted fallback secret to be removed, got %q", got)
	}
}

// TestConfigInitRun_SaveFailureDoesNotCleanupExistingSecrets verifies save failures do not delete the currently active secret.
func TestConfigInitRun_SaveFailureDoesNotCleanupExistingSecrets(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	existing := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId:     "old-app",
			AppSecret: core.SecretInput{Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:old-app"}},
			Brand:     core.BrandFeishu,
			Users:     []core.AppUser{},
		}},
	}
	if err := core.SaveMultiAppConfig(existing); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	kc := &trackingKeychain{
		setFunc: func(service, account, value string) error {
			return os.Chmod(configDir, 0500)
		},
	}
	t.Cleanup(func() { _ = os.Chmod(configDir, 0700) })

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.Keychain = kc

	opts := &ConfigInitOptions{
		Factory:   f,
		Ctx:       context.Background(),
		AppID:     "new-app",
		appSecret: "secret123",
		Brand:     "feishu",
		Lang:      "zh",
	}

	err := configInitRun(opts)
	if err == nil {
		t.Fatal("expected configInitRun to fail when config save fails")
	}

	if len(kc.removeCalls) != 1 || kc.removeCalls[0] != "appsecret:new-app" {
		t.Fatalf("expected only newly stored secret to be rolled back, got remove calls %v", kc.removeCalls)
	}

	cfg, loadErr := core.LoadMultiAppConfig()
	if loadErr != nil {
		t.Fatalf("LoadMultiAppConfig: %v", loadErr)
	}
	if len(cfg.Apps) != 1 || cfg.Apps[0].AppId != "old-app" {
		t.Fatalf("expected existing config to stay unchanged, got %#v", cfg.Apps)
	}
}

// TestStoreAndSaveOnlyApp_RejectsSecretRefReuseAcrossAppIDChange verifies managed secret refs cannot be reused across app IDs.
func TestStoreAndSaveOnlyApp_RejectsSecretRefReuseAcrossAppIDChange(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	existing := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId:     "old-app",
			AppSecret: core.SecretInput{Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:old-app"}},
			Brand:     core.BrandFeishu,
			Lang:      "zh",
			Users: []core.AppUser{{
				UserOpenId: "ou_old_user",
				UserName:   "old user",
			}},
		}},
	}
	if err := core.SaveMultiAppConfig(existing); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	kc := &trackingKeychain{}
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.Keychain = kc

	err := storeAndSaveOnlyApp(existing, f, "new-app", existing.Apps[0].AppSecret, core.BrandFeishu, "zh")
	if err == nil {
		t.Fatal("expected reusing a secret ref with a different app id to fail")
	}

	if len(kc.removeCalls) != 0 {
		t.Fatalf("expected no secret cleanup on rejected app id change, got %v", kc.removeCalls)
	}

	cfg, loadErr := core.LoadMultiAppConfig()
	if loadErr != nil {
		t.Fatalf("LoadMultiAppConfig: %v", loadErr)
	}
	if len(cfg.Apps) != 1 || cfg.Apps[0].AppId != "old-app" {
		t.Fatalf("expected config to stay unchanged, got %#v", cfg.Apps)
	}
}

// TestValidateSecretReuse_RequiresNewSecretWhenAppIDChanges verifies app ID changes force a fresh managed secret.
func TestValidateSecretReuse_RequiresNewSecretWhenAppIDChanges(t *testing.T) {
	err := validateSecretReuse("new-app", core.SecretInput{
		Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:old-app"},
	})
	if err == nil {
		t.Fatal("expected app id change with existing secret ref to be rejected")
	}

	if err := validateSecretReuse("old-app", core.SecretInput{
		Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:old-app"},
	}); err != nil {
		t.Fatalf("expected same-app secret ref reuse to remain allowed, got %v", err)
	}
}

// TestValidateSecretReuse_AllowsFileSecretRefAcrossAppIDChange verifies external file refs remain reusable across app IDs.
func TestValidateSecretReuse_AllowsFileSecretRefAcrossAppIDChange(t *testing.T) {
	err := validateSecretReuse("new-app", core.SecretInput{
		Ref: &core.SecretRef{Source: "file", ID: "/tmp/app-secret.txt"},
	})
	if err != nil {
		t.Fatalf("expected file-based secret ref reuse to remain allowed, got %v", err)
	}
}

// TestStoreAndSaveOnlyApp_SameAppUpgradeToKeychainRemovesOldFallbackSecret verifies same-app backend upgrades clean up replaced fallback secrets.
func TestStoreAndSaveOnlyApp_SameAppUpgradeToKeychainRemovesOldFallbackSecret(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	if err := keychain.SetFallback(keychain.LarkCliService, "appsecret:cli_test", "old-secret"); err != nil {
		t.Fatalf("SetFallback: %v", err)
	}

	existing := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: "cli_test",
			AppSecret: core.SecretInput{
				Ref: &core.SecretRef{Source: "encrypted_file", ID: "appsecret:cli_test"},
			},
			Brand: core.BrandFeishu,
			Lang:  "zh",
			Users: []core.AppUser{},
		}},
	}
	if err := core.SaveMultiAppConfig(existing); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	kc := &trackingKeychain{}
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.Keychain = kc

	if err := storeAndSaveOnlyApp(existing, f, "cli_test", core.PlainSecret("new-secret"), core.BrandFeishu, "zh"); err != nil {
		t.Fatalf("storeAndSaveOnlyApp: %v", err)
	}

	cfg, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig: %v", err)
	}
	if got := cfg.Apps[0].AppSecret.Ref; got == nil || got.Source != "keychain" {
		t.Fatalf("expected config to switch to keychain ref, got %#v", got)
	}
	if fallback, err := keychain.GetFallbackWithError(keychain.LarkCliService, "appsecret:cli_test"); err == nil && fallback != "" {
		t.Fatalf("expected old fallback secret to be removed, got %q", fallback)
	}
}
