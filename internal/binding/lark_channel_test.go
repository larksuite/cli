// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadLarkChannelConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	data := `{"accounts":{"app":{"id":"cli_abc123","secret":"plain_secret","tenant":"feishu"}}}`
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	root, err := ReadLarkChannelConfig(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := root.Accounts.App.ID; got != "cli_abc123" {
		t.Errorf("ID = %q, want %q", got, "cli_abc123")
	}
	if got := root.Accounts.App.Secret; got != "plain_secret" {
		t.Errorf("Secret = %q, want %q", got, "plain_secret")
	}
	if got := root.Accounts.App.Tenant; got != "feishu" {
		t.Errorf("Tenant = %q, want %q", got, "feishu")
	}
}

func TestReadLarkChannelConfig_LarkTenant(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	data := `{"accounts":{"app":{"id":"cli_xyz","secret":"s","tenant":"lark"}}}`
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	root, err := ReadLarkChannelConfig(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := root.Accounts.App.Tenant; got != "lark" {
		t.Errorf("Tenant = %q, want %q", got, "lark")
	}
}

func TestReadLarkChannelConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "does-not-exist.json")

	_, err := ReadLarkChannelConfig(p)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist, got %v", err)
	}
}

func TestReadLarkChannelConfig_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	_, err := ReadLarkChannelConfig(p)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestReadLarkChannelConfig_PartialFields(t *testing.T) {
	// schema isComplete check belongs at the binder layer; the reader should
	// happily parse a partial config — emptiness is detected downstream.
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	data := `{"accounts":{"app":{}}}`
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	root, err := ReadLarkChannelConfig(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if root.Accounts.App.ID != "" {
		t.Errorf("expected empty ID, got %q", root.Accounts.App.ID)
	}
	if root.Accounts.App.Secret != "" {
		t.Errorf("expected empty Secret, got %q", root.Accounts.App.Secret)
	}
}

func TestReadLarkChannelConfig_UnknownFieldsIgnored(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	data := `{
		"accounts": {
			"app": {"id": "cli_a", "secret": "s", "tenant": "feishu"},
			"oauth": {"clientId": "ignored"}
		},
		"preferences": {"theme": "dark"}
	}`
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	root, err := ReadLarkChannelConfig(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := root.Accounts.App.ID; got != "cli_a" {
		t.Errorf("ID = %q, want %q", got, "cli_a")
	}
}
