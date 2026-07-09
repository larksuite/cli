// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/qualitygate/facts"
	"github.com/larksuite/cli/internal/qualitygate/semantic"
)

func TestRunLoadsPolicyAndWaivers(t *testing.T) {
	repo := t.TempDir()
	writeSemanticConfig(t, repo, `{
	  "schema_version": 1,
	  "default_enforcement": "observe",
	  "block_categories": ["skill_quality"],
	  "rollout_groups": [{
	    "id": "changed-only",
	    "enforcement": "blocking",
	    "scope": {"changed_only": true},
	    "categories": ["skill_quality"],
	    "owner": "cli-owner",
	    "reason": "test rollout"
	  }]
	}`, `{
	  "allowed": ["semantic-review-v1"],
	  "allowed_base_urls": ["https://ark.ap-southeast.bytepluses.com/api/v3"]
	}`, "wiki-move\tskill_quality\tskill\tskills/lark-wiki/SKILL.md\t30\t\twiki-owner\tmigration\t2026-06-08\t2026-07-15\n")

	factsPath := filepath.Join(t.TempDir(), "facts.json")
	f := facts.Facts{
		SchemaVersion: 1,
		Skills: []facts.SkillFact{{
			SourceFile:               "skills/lark-wiki/SKILL.md",
			Line:                     30,
			Changed:                  true,
			ReferencesInvalidCommand: true,
		}},
	}
	if err := f.WriteFile(factsPath); err != nil {
		t.Fatalf("write facts: %v", err)
	}
	reviewPath := filepath.Join(t.TempDir(), "review.json")
	if err := os.WriteFile(reviewPath, []byte(`{"verdict":"warn","findings":[{"category":"skill_quality","severity":"major","evidence":["facts.skills[0]"],"message":"bad","suggested_action":"fix"}]}`), 0o644); err != nil {
		t.Fatalf("write review: %v", err)
	}
	decisionPath := filepath.Join(t.TempDir(), "decision.json")
	code := run([]string{"--repo", repo, "--facts", factsPath, "--review-json", reviewPath, "--decision-out", decisionPath, "--block"})
	if code != 0 {
		t.Fatalf("run() = %d, want waived success", code)
	}
	decision := readDecision(t, decisionPath)
	if len(decision.Blockers) != 0 || len(decision.Warnings) != 1 || decision.Warnings[0].WaiverID != "wiki-move" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if decision.Warnings[0].ReviewAction != semantic.ReviewActionConfirm {
		t.Fatalf("review action = %q, want %q", decision.Warnings[0].ReviewAction, semantic.ReviewActionConfirm)
	}
}

func TestRunLoadsWaiversFromOverrideFile(t *testing.T) {
	repo := t.TempDir()
	writeSemanticConfig(t, repo, `{
	  "schema_version": 1,
	  "default_enforcement": "observe",
	  "block_categories": ["error_hint"],
	  "rollout_groups": [{
	    "id": "changed-only",
	    "enforcement": "blocking",
	    "scope": {"changed_only": true},
	    "categories": ["error_hint"],
	    "owner": "cli-owner",
	    "reason": "test rollout"
	  }]
	}`, `{
	  "allowed": ["semantic-review-v1"],
	  "allowed_base_urls": ["https://ark.ap-southeast.bytepluses.com/api/v3"]
	}`, "")

	factsPath := filepath.Join(t.TempDir(), "facts.json")
	f := facts.Facts{
		SchemaVersion: 1,
		Errors: []facts.ErrorFact{{
			File:            "shortcuts/contact/contact_search_user.go",
			Line:            199,
			CommandPath:     "contact +search-user",
			Changed:         true,
			Boundary:        true,
			RequiredHint:    true,
			HintActionCount: 0,
			Code:            "validation",
		}},
	}
	if err := f.WriteFile(factsPath); err != nil {
		t.Fatalf("write facts: %v", err)
	}
	reviewPath := filepath.Join(t.TempDir(), "review.json")
	if err := os.WriteFile(reviewPath, []byte(`{"verdict":"warn","findings":[{"category":"error_hint","severity":"major","evidence":["facts.errors[0]"],"message":"bad","suggested_action":"fix"}]}`), 0o644); err != nil {
		t.Fatalf("write review: %v", err)
	}
	waiversPath := filepath.Join(t.TempDir(), "waivers.txt")
	if err := os.WriteFile(waiversPath, []byte("semantic-error-hint-confirm\terror_hint\terror\tshortcuts/contact/contact_search_user.go\t199\t\tcli-owner\tsandbox confirm case\t2026-06-11\t2026-07-11\n"), 0o644); err != nil {
		t.Fatalf("write override waivers: %v", err)
	}
	decisionPath := filepath.Join(t.TempDir(), "decision.json")
	code := run([]string{"--repo", repo, "--facts", factsPath, "--review-json", reviewPath, "--waivers-file", waiversPath, "--decision-out", decisionPath, "--block"})
	if code != 0 {
		t.Fatalf("run() = %d, want waived success", code)
	}
	decision := readDecision(t, decisionPath)
	if len(decision.Blockers) != 0 || len(decision.Warnings) != 1 {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if decision.Warnings[0].ReviewAction != semantic.ReviewActionConfirm || decision.Warnings[0].WaiverID != "semantic-error-hint-confirm" {
		t.Fatalf("override waiver was not used: %#v", decision.Warnings[0])
	}
}

func TestRunCommentOnlyDowngradesPolicyBlockers(t *testing.T) {
	repo := t.TempDir()
	writeSemanticConfig(t, repo, `{
	  "schema_version": 1,
	  "default_enforcement": "observe",
	  "block_categories": ["skill_quality"],
	  "rollout_groups": [{
	    "id": "changed-only",
	    "enforcement": "blocking",
	    "scope": {"changed_only": true},
	    "categories": ["skill_quality"],
	    "owner": "cli-owner",
	    "reason": "test rollout"
	  }]
	}`, `{
	  "allowed": ["semantic-review-v1"],
	  "allowed_base_urls": ["https://ark.ap-southeast.bytepluses.com/api/v3"]
	}`, "")

	factsPath := filepath.Join(t.TempDir(), "facts.json")
	f := facts.Facts{
		SchemaVersion: 1,
		Skills: []facts.SkillFact{{
			SourceFile:               "skills/lark-wiki/SKILL.md",
			Line:                     30,
			Changed:                  true,
			ReferencesInvalidCommand: true,
		}},
	}
	if err := f.WriteFile(factsPath); err != nil {
		t.Fatalf("write facts: %v", err)
	}
	reviewPath := filepath.Join(t.TempDir(), "review.json")
	if err := os.WriteFile(reviewPath, []byte(`{"verdict":"warn","findings":[{"category":"skill_quality","severity":"major","evidence":["facts.skills[0]"],"message":"bad","suggested_action":"fix"}]}`), 0o644); err != nil {
		t.Fatalf("write review: %v", err)
	}
	decisionPath := filepath.Join(t.TempDir(), "decision.json")
	code := run([]string{"--repo", repo, "--facts", factsPath, "--review-json", reviewPath, "--decision-out", decisionPath})
	if code != 0 {
		t.Fatalf("run() = %d, want success", code)
	}
	decision := readDecision(t, decisionPath)
	if decision.BlockMode || len(decision.Blockers) != 0 || len(decision.Warnings) != 1 {
		t.Fatalf("comment-only should warn only: %#v", decision)
	}
	if decision.Warnings[0].ReviewAction != semantic.ReviewActionObserve {
		t.Fatalf("review action = %q, want %q", decision.Warnings[0].ReviewAction, semantic.ReviewActionObserve)
	}
}

func TestRunWritesInfrastructureFailureDecisionForMissingPolicy(t *testing.T) {
	repo := t.TempDir()
	writeSemanticConfig(t, repo, "", `{
	  "allowed": ["semantic-review-v1"],
	  "allowed_base_urls": ["https://ark.ap-southeast.bytepluses.com/api/v3"]
	}`, "")
	factsPath := filepath.Join(t.TempDir(), "facts.json")
	if err := (facts.Facts{SchemaVersion: 1}).WriteFile(factsPath); err != nil {
		t.Fatalf("write facts: %v", err)
	}
	reviewPath := filepath.Join(t.TempDir(), "review.json")
	if err := os.WriteFile(reviewPath, []byte(`{"verdict":"warn","findings":[]}`), 0o644); err != nil {
		t.Fatalf("write review: %v", err)
	}
	decisionPath := filepath.Join(t.TempDir(), "decision.json")
	code := run([]string{"--repo", repo, "--facts", factsPath, "--review-json", reviewPath, "--decision-out", decisionPath, "--block"})
	if code != 0 {
		t.Fatalf("run() = %d, want infrastructure handoff", code)
	}
	decision := readDecision(t, decisionPath)
	if !decision.InfrastructureFailure || !decision.Degraded || !decision.BlockMode {
		t.Fatalf("expected infrastructure blocking decision: %#v", decision)
	}
}

func TestRunWritesSkippedDecisionForUnavailableReviewer(t *testing.T) {
	t.Setenv("ARK_API_KEY", "")
	t.Setenv("ARK_BASE_URL", "")
	t.Setenv("ARK_MODEL", "")

	repo := t.TempDir()
	writeSemanticConfig(t, repo, `{
	  "schema_version": 1,
	  "default_enforcement": "observe",
	  "block_categories": ["skill_quality"]
	}`, `{
	  "allowed": ["semantic-review-v1"],
	  "allowed_base_urls": ["https://ark.ap-southeast.bytepluses.com/api/v3"]
	}`, "")
	factsPath := filepath.Join(t.TempDir(), "facts.json")
	f := facts.Facts{
		SchemaVersion: 1,
		Skills: []facts.SkillFact{{
			SourceFile:               "skills/lark-wiki/SKILL.md",
			Line:                     30,
			Changed:                  true,
			ReferencesInvalidCommand: true,
		}},
	}
	if !semantic.BuildInputView(f).HasReviewableFacts() {
		t.Fatal("test setup must contain reviewable facts")
	}
	if err := f.WriteFile(factsPath); err != nil {
		t.Fatalf("write facts: %v", err)
	}
	decisionPath := filepath.Join(t.TempDir(), "decision.json")
	code := run([]string{"--repo", repo, "--facts", factsPath, "--decision-out", decisionPath, "--block"})
	if code != 0 {
		t.Fatalf("run() = %d, want skipped handoff", code)
	}
	decision := readDecision(t, decisionPath)
	if !decision.Skipped || decision.Degraded || decision.InfrastructureFailure || !decision.BlockMode {
		t.Fatalf("expected skipped non-infrastructure decision: %#v", decision)
	}
	if len(decision.SystemWarnings) != 1 || len(decision.Warnings) != 0 || len(decision.Blockers) != 0 {
		t.Fatalf("skipped decision should only carry system warnings: %#v", decision)
	}
}

func TestRunShortCircuitsEmptySemanticInputWithoutReviewer(t *testing.T) {
	t.Setenv("ARK_API_KEY", "")
	t.Setenv("ARK_BASE_URL", "")
	t.Setenv("ARK_MODEL", "")

	repo := t.TempDir()
	writeSemanticConfig(t, repo, `{
	  "schema_version": 1,
	  "default_enforcement": "observe",
	  "block_categories": ["skill_quality"]
	}`, `{
	  "allowed": ["semantic-review-v1"],
	  "allowed_base_urls": ["https://ark.ap-southeast.bytepluses.com/api/v3"]
	}`, "")
	factsPath := filepath.Join(t.TempDir(), "facts.json")
	f := facts.Facts{
		SchemaVersion: 1,
		Commands: []facts.CommandFact{{
			Path:    "service command 1",
			Domain:  "service",
			Changed: true,
			Source:  "service",
		}},
		Outputs: []facts.OutputFact{{
			Command:          "service command 1",
			Domain:           "service",
			Changed:          true,
			Source:           "service",
			IsList:           true,
			HasDefaultLimit:  true,
			HasDecisionField: true,
		}},
	}
	if semantic.BuildInputView(f).HasReviewableFacts() {
		t.Fatal("test setup must not contain reviewable facts")
	}
	if err := f.WriteFile(factsPath); err != nil {
		t.Fatalf("write facts: %v", err)
	}
	decisionPath := filepath.Join(t.TempDir(), "decision.json")
	markdownPath := filepath.Join(t.TempDir(), "semantic.md")
	code := run([]string{"--repo", repo, "--facts", factsPath, "--decision-out", decisionPath, "--markdown-out", markdownPath, "--block"})
	if code != 0 {
		t.Fatalf("run() = %d, want clean pass", code)
	}
	decision := readDecision(t, decisionPath)
	if decision.Skipped || decision.Degraded || decision.InfrastructureFailure || !decision.BlockMode {
		t.Fatalf("expected non-degraded pass decision: %#v", decision)
	}
	if len(decision.SystemWarnings) != 0 || len(decision.Warnings) != 0 || len(decision.Blockers) != 0 {
		t.Fatalf("empty semantic view should not produce findings: %#v", decision)
	}
	data, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read markdown: %v", err)
	}
	markdown := string(data)
	if !strings.Contains(markdown, "No semantic blockers.") {
		t.Fatalf("markdown missing pass summary: %s", markdown)
	}
	if strings.Contains(strings.ToLower(markdown), "skipped") || strings.Contains(strings.ToLower(markdown), "degraded") {
		t.Fatalf("markdown should not report semantic review as skipped/degraded: %s", markdown)
	}
}

func TestRunWritesInfrastructureFailureDecisionForInvalidReviewerConfig(t *testing.T) {
	t.Setenv("ARK_API_KEY", "test-key")
	t.Setenv("ARK_BASE_URL", "")
	t.Setenv("ARK_MODEL", "not-allowed-model")

	repo := t.TempDir()
	writeSemanticConfig(t, repo, `{
	  "schema_version": 1,
	  "default_enforcement": "observe",
	  "block_categories": ["skill_quality"]
	}`, `{
	  "allowed": ["semantic-review-v1"],
	  "allowed_base_urls": ["https://ark.ap-southeast.bytepluses.com/api/v3"]
	}`, "")
	factsPath := filepath.Join(t.TempDir(), "facts.json")
	f := facts.Facts{
		SchemaVersion: 1,
		Skills: []facts.SkillFact{{
			SourceFile:               "skills/lark-wiki/SKILL.md",
			Line:                     30,
			Changed:                  true,
			ReferencesInvalidCommand: true,
		}},
	}
	if !semantic.BuildInputView(f).HasReviewableFacts() {
		t.Fatal("test setup must contain reviewable facts")
	}
	if err := f.WriteFile(factsPath); err != nil {
		t.Fatalf("write facts: %v", err)
	}
	decisionPath := filepath.Join(t.TempDir(), "decision.json")
	code := run([]string{"--repo", repo, "--facts", factsPath, "--decision-out", decisionPath, "--block"})
	if code != 0 {
		t.Fatalf("run() = %d, want infrastructure handoff", code)
	}
	decision := readDecision(t, decisionPath)
	if !decision.InfrastructureFailure || !decision.Degraded || decision.Skipped || !decision.BlockMode {
		t.Fatalf("expected infrastructure failure decision: %#v", decision)
	}
}

func writeSemanticConfig(t *testing.T, repo, policy, models, waivers string) {
	t.Helper()
	dir := filepath.Join(repo, "internal", "qualitygate", "config", "semantic")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if policy != "" {
		if err := os.WriteFile(filepath.Join(dir, "policy.json"), []byte(policy), 0o644); err != nil {
			t.Fatalf("write policy: %v", err)
		}
	}
	if models != "" {
		if err := os.WriteFile(filepath.Join(dir, "models.json"), []byte(models), 0o644); err != nil {
			t.Fatalf("write models: %v", err)
		}
	}
	if waivers != "" {
		if err := os.WriteFile(filepath.Join(dir, "waivers.txt"), []byte(waivers), 0o644); err != nil {
			t.Fatalf("write waivers: %v", err)
		}
	}
}

func readDecision(t *testing.T, path string) semantic.Decision {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read decision: %v", err)
	}
	var decision semantic.Decision
	if err := json.Unmarshal(data, &decision); err != nil {
		t.Fatalf("decode decision: %v", err)
	}
	return decision
}
