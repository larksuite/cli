// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseBlockMode(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"true", true},
		{"", false},
		{"false", false},
		{"TRUE", false},
		{"1", false},
	} {
		if got := ParseBlockMode(tc.in); got != tc.want {
			t.Fatalf("ParseBlockMode(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestLoadPolicyValidation(t *testing.T) {
	repo := t.TempDir()
	if _, err := LoadPolicy(repo); err == nil {
		t.Fatal("LoadPolicy accepted missing policy.json")
	}

	writeSemanticFile(t, repo, "policy.json", `{
	  "schema_version": 1,
	  "default_enforcement": "observe",
	  "block_categories": ["error_hint", "skill_quality"],
	  "rollout_groups": [{
	    "id": "changed-only",
	    "enforcement": "blocking",
	    "scope": {"changed_only": true},
	    "categories": ["skill_quality"],
	    "owner": "cli-owner",
	    "reason": "first rollout"
	  }]
	}`)
	p, err := LoadPolicy(repo)
	if err != nil {
		t.Fatalf("LoadPolicy() error = %v", err)
	}
	if p.SchemaVersion != 1 || p.RolloutGroups[0].ID != "changed-only" {
		t.Fatalf("unexpected policy: %#v", p)
	}

	for name, body := range map[string]string{
		"bad schema":             `{"schema_version":2,"default_enforcement":"observe","block_categories":["error_hint"]}`,
		"bad enforcement":        `{"schema_version":1,"default_enforcement":"blocking","block_categories":["error_hint"]}`,
		"empty block categories": `{"schema_version":1,"default_enforcement":"observe","block_categories":[]}`,
		"duplicate rollout":      `{"schema_version":1,"default_enforcement":"observe","block_categories":["error_hint"],"rollout_groups":[{"id":"a1","enforcement":"blocking","categories":["error_hint"],"owner":"o","reason":"r"},{"id":"a1","enforcement":"blocking","categories":["error_hint"],"owner":"o","reason":"r"}]}`,
		"bad category":           `{"schema_version":1,"default_enforcement":"observe","block_categories":["unknown"]}`,
		"category outside block": `{"schema_version":1,"default_enforcement":"observe","block_categories":["error_hint"],"rollout_groups":[{"id":"a1","enforcement":"blocking","categories":["skill_quality"],"owner":"o","reason":"r"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			writeSemanticFile(t, repo, "policy.json", body)
			if _, err := LoadPolicy(repo); err == nil {
				t.Fatalf("LoadPolicy accepted %s", name)
			}
		})
	}
}

func TestLoadModelConfig(t *testing.T) {
	repo := t.TempDir()
	if _, err := LoadModelConfig(repo); err == nil {
		t.Fatal("LoadModelConfig accepted missing models.json")
	}
	writeSemanticFile(t, repo, "models.json", `{
	  "allowed": ["semantic-review-v1"],
	  "allowed_base_urls": ["https://ark.ap-southeast.bytepluses.com/api/v3"]
	}`)
	cfg, err := LoadModelConfig(repo)
	if err != nil {
		t.Fatalf("LoadModelConfig() error = %v", err)
	}
	if !cfg.AllowsModel("semantic-review-v1") {
		t.Fatalf("default model not allowed: %#v", cfg)
	}

	for name, body := range map[string]string{
		"default model": `{"default":"semantic-review-v1","allowed":["semantic-review-v1"],"allowed_base_urls":["https://ark.ap-southeast.bytepluses.com/api/v3"]}`,
		"bad model id":  `{"allowed":["bad model"],"allowed_base_urls":["https://ark.ap-southeast.bytepluses.com/api/v3"]}`,
		"bad base url":  `{"allowed":["semantic-review-v1"],"allowed_base_urls":["http://example.com/api/v3"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			writeSemanticFile(t, repo, "models.json", body)
			if _, err := LoadModelConfig(repo); err == nil {
				t.Fatalf("LoadModelConfig accepted %s", name)
			}
		})
	}
}

func TestLoadModelConfigAllowsUnconfiguredModelList(t *testing.T) {
	repo := t.TempDir()
	writeSemanticFile(t, repo, "models.json", `{
	  "allowed": [],
	  "allowed_base_urls": ["https://ark.ap-southeast.bytepluses.com/api/v3"]
	}`)
	cfg, err := LoadModelConfig(repo)
	if err != nil {
		t.Fatalf("LoadModelConfig() error = %v", err)
	}
	if cfg.AllowsModel("semantic-review-v1") {
		t.Fatalf("empty allowed list must not allow model calls: %#v", cfg)
	}
}

func TestBaseURLAllowlist(t *testing.T) {
	cfg := ModelConfig{AllowedBaseURLs: []string{"https://ark.ap-southeast.bytepluses.com/api/v3"}}
	if !IsTrustedBaseURL("https://ark.ap-southeast.bytepluses.com/api/v3", cfg) {
		t.Fatal("expected exact allowed endpoint")
	}
	for _, raw := range []string{
		"https://evil.example.com/api/v3",
		"http://ark.ap-southeast.bytepluses.com/api/v3",
		"https://user@ark.ap-southeast.bytepluses.com/api/v3",
		"https://ark.ap-southeast.bytepluses.com/api/v3?x=1",
		"https://ark.ap-southeast.bytepluses.com/api/v3#frag",
		"https://ark.ap-southeast.bytepluses.com:8443/api/v3",
		"https://ark.ap-southeast.bytepluses.com/api/v3/",
	} {
		t.Run(raw, func(t *testing.T) {
			if IsTrustedBaseURL(raw, cfg) {
				t.Fatalf("trusted unsafe base URL %q", raw)
			}
		})
	}
}

func writeSemanticFile(t *testing.T, repo, name, body string) {
	t.Helper()
	dir := filepath.Join(repo, "internal", "qualitygate", "config", "semantic")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
