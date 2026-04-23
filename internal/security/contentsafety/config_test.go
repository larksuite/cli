// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	content := `{
		"allowlist": ["im", "drive.upload"],
		"rules": [{"id": "r1", "pattern": "(?i)test_pattern"}]
	}`
	if err := os.WriteFile(filepath.Join(dir, "content-safety.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(cfg.Allowlist) != 2 || cfg.Allowlist[0] != "im" {
		t.Errorf("Allowlist = %v, want [im, drive.upload]", cfg.Allowlist)
	}
	if len(cfg.Rules) != 1 || cfg.Rules[0].ID != "r1" {
		t.Fatalf("Rules = %v, want [{r1, ...}]", cfg.Rules)
	}
	if !cfg.Rules[0].Pattern.MatchString("TEST_PATTERN here") {
		t.Error("compiled pattern should match")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "content-safety.json"), []byte(`{bad`), 0644)
	_, err := LoadConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadConfig_InvalidRegex(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "content-safety.json"), []byte(`{"allowlist":[],"rules":[{"id":"bad","pattern":"(?P<broken"}]}`), 0644)
	_, err := LoadConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestLoadConfig_EmptyRules(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "content-safety.json"), []byte(`{"allowlist":["all"],"rules":[]}`), 0644)
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(cfg.Rules) != 0 {
		t.Errorf("Rules length = %d, want 0", len(cfg.Rules))
	}
}

func TestEnsureDefaultConfig_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	var buf strings.Builder
	if err := EnsureDefaultConfig(dir, &buf); err != nil {
		t.Fatalf("EnsureDefaultConfig() error = %v", err)
	}
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("default config not loadable: %v", err)
	}
	if len(cfg.Rules) != 4 {
		t.Errorf("default rules = %d, want 4", len(cfg.Rules))
	}
	if len(cfg.Allowlist) != 1 || cfg.Allowlist[0] != "all" {
		t.Errorf("default allowlist = %v, want [all]", cfg.Allowlist)
	}
	if !strings.Contains(buf.String(), "notice: created default content-safety config") {
		t.Errorf("expected stderr notice, got %q", buf.String())
	}
}

func TestEnsureDefaultConfig_NoOverwrite(t *testing.T) {
	dir := t.TempDir()
	custom := `{"allowlist":[],"rules":[]}`
	os.WriteFile(filepath.Join(dir, "content-safety.json"), []byte(custom), 0644)
	EnsureDefaultConfig(dir, io.Discard)
	data, _ := os.ReadFile(filepath.Join(dir, "content-safety.json"))
	if string(data) != custom {
		t.Error("should not overwrite existing file")
	}
}

func TestIsAllowlisted(t *testing.T) {
	tests := []struct {
		name    string
		cmdPath string
		list    []string
		want    bool
	}{
		{"empty_list", "im.messages_search", nil, false},
		{"all", "anything", []string{"all"}, true},
		{"ALL_upper", "anything", []string{"ALL"}, true},
		{"exact", "im.messages_search", []string{"im.messages_search"}, true},
		{"prefix", "im.messages_search", []string{"im"}, true},
		{"no_match", "drive.upload", []string{"im"}, false},
		{"prefix_boundary", "im_extra", []string{"im"}, false},
		{"multi", "drive.upload", []string{"im", "drive"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAllowlisted(tt.cmdPath, tt.list)
			if got != tt.want {
				t.Errorf("IsAllowlisted(%q, %v) = %v, want %v", tt.cmdPath, tt.list, got, tt.want)
			}
		})
	}
}
