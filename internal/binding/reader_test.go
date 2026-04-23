// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadOpenClawConfig_ValidSingleAccount(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "openclaw.json")
	data := `{"channels":{"feishu":{"appId":"cli_abc","appSecret":"plain_secret","domain":"feishu"}}}`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	root, err := ReadOpenClawConfig(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if root.Channels.Feishu == nil {
		t.Fatal("expected Channels.Feishu to be non-nil")
	}
	if got := root.Channels.Feishu.AppID; got != "cli_abc" {
		t.Errorf("AppID = %q, want %q", got, "cli_abc")
	}
	if got := root.Channels.Feishu.AppSecret.Plain; got != "plain_secret" {
		t.Errorf("AppSecret.Plain = %q, want %q", got, "plain_secret")
	}
	if root.Channels.Feishu.AppSecret.Ref != nil {
		t.Error("AppSecret.Ref should be nil for a plain string")
	}
	if got := root.Channels.Feishu.Brand; got != "feishu" {
		t.Errorf("Brand = %q, want %q", got, "feishu")
	}
}

func TestReadOpenClawConfig_ValidMultiAccount(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "openclaw.json")
	data := `{
		"channels": {
			"feishu": {
				"domain": "feishu",
				"accounts": {
					"work": {"appId": "cli_work", "appSecret": "secret_work", "domain": "feishu"},
					"personal": {"appId": "cli_personal", "appSecret": "secret_personal", "domain": "lark"}
				}
			}
		}
	}`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	root, err := ReadOpenClawConfig(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if root.Channels.Feishu == nil {
		t.Fatal("expected Channels.Feishu to be non-nil")
	}

	apps := ListCandidateApps(root.Channels.Feishu)
	if len(apps) != 2 {
		t.Fatalf("ListCandidateApps returned %d apps, want 2", len(apps))
	}

	byLabel := make(map[string]CandidateApp, len(apps))
	for _, a := range apps {
		byLabel[a.Label] = a
	}

	work, ok := byLabel["work"]
	if !ok {
		t.Fatal("missing account label 'work'")
	}
	if work.AppID != "cli_work" {
		t.Errorf("work.AppID = %q, want %q", work.AppID, "cli_work")
	}

	personal, ok := byLabel["personal"]
	if !ok {
		t.Fatal("missing account label 'personal'")
	}
	if personal.AppID != "cli_personal" {
		t.Errorf("personal.AppID = %q, want %q", personal.AppID, "cli_personal")
	}
}

func TestReadOpenClawConfig_MissingFeishu(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "openclaw.json")
	data := `{"channels":{}}`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	root, err := ReadOpenClawConfig(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if root.Channels.Feishu != nil {
		t.Error("expected Channels.Feishu to be nil when not present in JSON")
	}
}

func TestReadOpenClawConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "openclaw.json")
	if err := os.WriteFile(p, []byte(`{not valid json`), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	_, err := ReadOpenClawConfig(p)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestReadOpenClawConfig_FileNotFound(t *testing.T) {
	_, err := ReadOpenClawConfig(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestReadOpenClawConfig_EnvTemplate(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "openclaw.json")
	data := `{"channels":{"feishu":{"appId":"cli_env","appSecret":"${FEISHU_APP_SECRET}","domain":"feishu"}}}`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	root, err := ReadOpenClawConfig(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	secret := root.Channels.Feishu.AppSecret
	if secret.Plain != "${FEISHU_APP_SECRET}" {
		t.Errorf("SecretInput.Plain = %q, want %q", secret.Plain, "${FEISHU_APP_SECRET}")
	}
	if secret.Ref != nil {
		t.Error("SecretInput.Ref should be nil for env template string")
	}
}

func TestReadOpenClawConfig_SecretRefObject(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "openclaw.json")
	data := `{"channels":{"feishu":{"appId":"cli_ref","appSecret":{"source":"file","provider":"fp","id":"/path"},"domain":"feishu"}}}`
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	root, err := ReadOpenClawConfig(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	secret := root.Channels.Feishu.AppSecret
	if secret.Plain != "" {
		t.Errorf("SecretInput.Plain = %q, want empty for object form", secret.Plain)
	}
	if secret.Ref == nil {
		t.Fatal("SecretInput.Ref should be non-nil for object form")
	}
	if secret.Ref.Source != "file" {
		t.Errorf("Ref.Source = %q, want %q", secret.Ref.Source, "file")
	}
	if secret.Ref.Provider != "fp" {
		t.Errorf("Ref.Provider = %q, want %q", secret.Ref.Provider, "fp")
	}
	if secret.Ref.ID != "/path" {
		t.Errorf("Ref.ID = %q, want %q", secret.Ref.ID, "/path")
	}
}
