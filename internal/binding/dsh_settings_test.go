// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSettings writes a settings document and returns its path.
func writeSettings(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "settings.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return p
}

func TestReadDSHSettings_Valid(t *testing.T) {
	p := writeSettings(t, `lark-channel:
  appId: cli_abc123
  appSecret: plain_secret
`)

	root, err := ReadDSHSettings(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := root.LarkChannel.AppID; got != "cli_abc123" {
		t.Errorf("AppID = %q, want %q", got, "cli_abc123")
	}
	if got := root.LarkChannel.AppSecret; got != "plain_secret" {
		t.Errorf("AppSecret = %q, want %q", got, "plain_secret")
	}
	// An absent domain is the Feishu deployment; the caller maps it.
	if got := root.LarkChannel.Domain; got != "" {
		t.Errorf("Domain = %q, want empty", got)
	}
}

// The settings document is shared by every plugin that registers a section, so
// a foreign section must not interfere with the one we read.
func TestReadDSHSettings_IgnoresForeignSections(t *testing.T) {
	p := writeSettings(t, `ui-onboarding:
  welcomeNoticeVersion: 2026-08-13.1
lark-channel:
  appId: cli_shared
  appSecret: s
  domain: https://open.larksuite.com
  output: stream
  requireMention: true
`)

	root, err := ReadDSHSettings(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := root.LarkChannel.AppID; got != "cli_shared" {
		t.Errorf("AppID = %q, want %q", got, "cli_shared")
	}
	// The Lark deployment is identified by the raw open-platform URL, not a
	// brand name — the section stores whatever the plugin was configured with.
	if got := root.LarkChannel.Domain; got != "https://open.larksuite.com" {
		t.Errorf("Domain = %q, want the larksuite URL", got)
	}
}

// A harness that has never onboarded the channel still has a settings
// document; the section is simply absent.
func TestReadDSHSettings_MissingSection(t *testing.T) {
	p := writeSettings(t, "ui-onboarding:\n  welcomeNoticeVersion: 2026-08-13.1\n")

	root, err := ReadDSHSettings(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if root.LarkChannel.AppID != "" {
		t.Errorf("AppID = %q, want empty", root.LarkChannel.AppID)
	}
}

func TestReadDSHSettings_InvalidYAML(t *testing.T) {
	p := writeSettings(t, "lark-channel:\n  appId: [unclosed\n")

	if _, err := ReadDSHSettings(p); err == nil {
		t.Fatal("expected an error for malformed YAML, got nil")
	} else if !strings.Contains(err.Error(), p) {
		t.Errorf("error must name the file, got: %v", err)
	}
}

func TestReadDSHSettings_MissingFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.yaml")

	if _, err := ReadDSHSettings(p); err == nil {
		t.Fatal("expected an error for a missing file, got nil")
	}
}
