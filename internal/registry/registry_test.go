// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/meta"
)

func snapshotServiceNames(t *testing.T) []string {
	t.Helper()
	services := scopeTestCatalog(t).Services()
	names := make([]string, 0, len(services))
	for _, service := range services {
		names = append(names, service.Name)
	}
	return names
}

func scopeTestCatalog(t *testing.T) apicatalog.Catalog {
	t.Helper()
	snapshot, err := OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.Catalog()
}

func TestLoadScopePriorities(t *testing.T) {
	priorities := LoadScopePriorities()
	if len(priorities) == 0 {
		t.Fatal("expected non-empty priorities map")
	}
	t.Logf("Loaded %d scope priorities", len(priorities))

	// Verify a known scope exists (im:message:recall is in the user's data)
	if _, ok := priorities["im:message:recall"]; !ok {
		t.Error("expected im:message:recall in priorities")
	}
}

func TestGetScopeScore(t *testing.T) {
	// Known scope should have a real score
	score := GetScopeScore("im:message:recall")
	if score == DefaultScopeScore {
		t.Errorf("expected real score for im:message:recall, got default %d", score)
	}
	t.Logf("im:message:recall score: %d", score)

	// Unknown scope should return default
	score = GetScopeScore("unknown:scope:here")
	if score != DefaultScopeScore {
		t.Errorf("expected %d, got %d", DefaultScopeScore, score)
	}

	// Override: im:chat:readonly should be overridden to 1
	score = GetScopeScore("im:chat:readonly")
	if score != 1 {
		t.Errorf("expected im:chat:readonly override score 1, got %d", score)
	}
}

func TestSelectRecommendedScope_PicksHighestScore(t *testing.T) {
	priorities := LoadScopePriorities()

	// Find two scopes with known different scores
	scopeA := "calendar:calendar:readonly"
	scopeB := "calendar:calendar"

	scoreA, okA := priorities[scopeA]
	scoreB, okB := priorities[scopeB]
	if !okA || !okB {
		t.Skipf("test scopes not in priorities (A=%v, B=%v)", okA, okB)
	}
	t.Logf("%s=%d, %s=%d", scopeA, scoreA, scopeB, scoreB)

	result := bestScope([]string{scopeB, scopeA}, priorities)

	// Should pick the higher-scored one (higher = more recommended)
	if scoreA > scoreB {
		if result != scopeA {
			t.Errorf("expected %s (score %d), got %s", scopeA, scoreA, result)
		}
	} else {
		if result != scopeB {
			t.Errorf("expected %s (score %d), got %s", scopeB, scoreB, result)
		}
	}
}

func TestSelectRecommendedScope_FallbackToFirst(t *testing.T) {
	scopes := []string{
		"zzz_unknown:scope:a",
		"zzz_unknown:scope:b",
	}
	result := bestScope(scopes, LoadScopePriorities())
	// All unknown scopes get DefaultScopeScore; first one with that score wins
	if result != "zzz_unknown:scope:a" {
		t.Errorf("expected zzz_unknown:scope:a, got %s", result)
	}
}

func TestSelectRecommendedScope_Empty(t *testing.T) {
	if result := bestScope(nil, LoadScopePriorities()); result != "" {
		t.Errorf("expected empty string, got %s", result)
	}
	if result := bestScope([]string{}, LoadScopePriorities()); result != "" {
		t.Errorf("expected empty string, got %s", result)
	}
}

// --- Auto-approve functions ---

func TestLoadAutoApproveSet(t *testing.T) {
	aaSet := LoadAutoApproveSet()
	if len(aaSet) == 0 {
		t.Fatal("expected non-empty auto-approve set")
	}

	// From scope_priorities.json recommend=="true"
	if !aaSet["sheets:spreadsheet:read"] {
		t.Error("expected sheets:spreadsheet:read in auto-approve set (recommend=true in priorities)")
	}

	t.Logf("Auto-approve set has %d scopes", len(aaSet))
}

func TestLoadPlatformAutoApproveSet(t *testing.T) {
	paaSet := LoadPlatformAutoApproveSet()
	// This should only include scopes from scope_priorities.json with AutoApprove rule.
	// It does NOT apply deny overrides.
	if len(paaSet) == 0 {
		t.Fatal("expected non-empty platform auto-approve set")
	}

	t.Logf("Platform auto-approve set has %d scopes", len(paaSet))
}

func TestLoadOverrideAutoApproveAllow(t *testing.T) {
	allowSet := LoadOverrideAutoApproveAllow()
	// recommend.allow special-cases scopes absent from scope_priorities.json
	// (application v7 is not in the platform catalog yet) so interactive
	// login's "common scopes" tier still offers them. Only the read scope is
	// admitted: write stays out of the recommended tier by design.
	if !allowSet["application:app_slash_command:read"] {
		t.Error("expected application:app_slash_command:read in override allow set")
	}
	if allowSet["application:app_slash_command:write"] {
		t.Error("write scope must NOT be in the recommended tier")
	}
	if len(allowSet) != 1 {
		t.Errorf("expected exactly 1 override allow entry, got %d", len(allowSet))
	}
}

func TestLoadOverrideAutoApproveDeny(t *testing.T) {
	denySet := LoadOverrideAutoApproveDeny()
	// deny list may be empty if all entries are moved to _deny (commented out)
	t.Logf("Override deny set has %d scopes", len(denySet))
}

func TestIsAutoApproveScope(t *testing.T) {
	// Known auto-approve scope (recommend=true in scope_priorities.json)
	if !IsAutoApproveScope("sheets:spreadsheet:read") {
		t.Error("expected sheets:spreadsheet:read to be auto-approve")
	}

	// Completely unknown scope
	if IsAutoApproveScope("zzz:unknown:scope") {
		t.Error("expected unknown scope to NOT be auto-approve")
	}
}

func TestFilterAutoApproveScopes(t *testing.T) {
	scopes := []string{
		"sheets:spreadsheet:read", // auto-approve (recommend=true in priorities)
		"zzz:unknown:scope",       // not in auto-approve
	}

	result := FilterAutoApproveScopes(scopes)
	if len(result) < 1 {
		t.Fatal("expected at least 1 auto-approve scope in result")
	}

	// Check that sheets:spreadsheet:read is included
	found := false
	for _, s := range result {
		if s == "sheets:spreadsheet:read" {
			found = true
		}
		// Ensure unknown scopes are not included
		if s == "zzz:unknown:scope" {
			t.Error("unknown scope should not be in auto-approve result")
		}
	}
	if !found {
		t.Error("expected sheets:spreadsheet:read in result")
	}
}

func TestFilterAutoApproveScopes_Empty(t *testing.T) {
	result := FilterAutoApproveScopes(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}

	result = FilterAutoApproveScopes([]string{})
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

// --- Helper functions ---

func TestGetRegistryDir(t *testing.T) {
	dir := GetRegistryDir()
	if dir == "" {
		t.Error("expected non-empty registry dir")
	}
	t.Logf("Registry dir: %s", dir)
}

// --- Scope collection functions ---

func TestCollectCommandScopes(t *testing.T) {
	projects := snapshotServiceNames(t)
	if len(projects) == 0 {
		t.Skip("no from_meta data available")
	}

	entries := CollectCommandScopes(scopeTestCatalog(t), []string{"calendar"}, "user")
	if len(entries) == 0 {
		t.Fatal("expected non-empty command entries for calendar")
	}

	// Verify sorted by Command
	for i := 1; i < len(entries); i++ {
		if entries[i].Command < entries[i-1].Command {
			t.Errorf("entries not sorted: %s < %s", entries[i].Command, entries[i-1].Command)
		}
	}

	// Verify each entry has scopes and type
	for _, e := range entries {
		if e.Command == "" {
			t.Error("entry has empty command")
		}
		if e.Type != "api" {
			t.Errorf("expected type 'api', got %q", e.Type)
		}
		if len(e.Scopes) == 0 {
			t.Errorf("entry %s has no scopes", e.Command)
		}
	}

	t.Logf("Calendar command entries: %d", len(entries))
}

func TestCollectCommandScopes_EmptyProject(t *testing.T) {
	entries := CollectCommandScopes(scopeTestCatalog(t), []string{"nonexistent_project"}, "user")
	if len(entries) != 0 {
		t.Errorf("expected empty entries for nonexistent project, got %d", len(entries))
	}
}

func TestCollectScopesForProjects_MultipleProjects(t *testing.T) {
	projects := snapshotServiceNames(t)
	if len(projects) < 2 {
		t.Skip("need at least 2 from_meta projects")
	}

	// Multiple projects should yield more scopes than a single one
	single := CollectScopesForProjects(scopeTestCatalog(t), projects[:1], "user")
	multi := CollectScopesForProjects(scopeTestCatalog(t), projects[:2], "user")

	if len(multi) < len(single) {
		t.Errorf("multi-project scopes (%d) should be >= single-project (%d)", len(multi), len(single))
	}
}

func TestCollectScopesForProjects_NonexistentProject(t *testing.T) {
	scopes := CollectScopesForProjects(scopeTestCatalog(t), []string{"nonexistent_project_xyz"}, "user")
	if len(scopes) != 0 {
		t.Errorf("expected empty scopes for nonexistent project, got %d", len(scopes))
	}
}

func TestCollectScopesForProjects_APICatalog(t *testing.T) {
	snapshot, err := OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	catalog := apicatalog.Filter(snapshot.Catalog(), func(s meta.Service) (meta.Service, bool) {
		return s, s.Name == "drive"
	})
	if scopes := CollectScopesForProjects(catalog, []string{"drive"}, "user"); len(scopes) == 0 {
		t.Fatal("drive-only catalog returned no drive scopes")
	}
	if scopes := CollectScopesForProjects(catalog, []string{"calendar"}, "user"); len(scopes) != 0 {
		t.Fatalf("drive-only catalog returned calendar scopes: %v", scopes)
	}
}

// TestCollectScopesForProjects_HonorsRequiredScopes verifies that a method's
// full requiredScopes conjunction is collected, not just the umbrella scope.
// The mail message get/batch_get APIs declare requiredScopes =
// [readonly, subject:read, address:read, body:read]; the conjunction must be
// collected together — the umbrella readonly scope alone does not cover
// subject/address/body.
func TestCollectScopesForProjects_HonorsRequiredScopes(t *testing.T) {
	hasMail := false
	for _, p := range snapshotServiceNames(t) {
		if p == "mail" {
			hasMail = true
			break
		}
	}
	if !hasMail {
		t.Skip("mail domain not present in meta catalog")
	}

	scopes := CollectScopesForProjects(scopeTestCatalog(t), []string{"mail"}, "user")
	for _, want := range []string{
		"mail:user_mailbox.message.subject:read",
		"mail:user_mailbox.message.address:read",
		"mail:user_mailbox.message.body:read",
	} {
		found := false
		for _, s := range scopes {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected requiredScope %q in collected scopes, got %v", want, scopes)
		}
	}
}

// --- auth_domain functions ---

func TestGetAuthDomain_Configured(t *testing.T) {
	// whiteboard has auth_domain: "docs" in service_descriptions.json
	if got := GetAuthDomain("whiteboard"); got != "docs" {
		t.Errorf("GetAuthDomain(whiteboard) = %q, want %q", got, "docs")
	}
}

func TestGetAuthDomain_NotConfigured(t *testing.T) {
	if got := GetAuthDomain("calendar"); got != "" {
		t.Errorf("GetAuthDomain(calendar) = %q, want empty", got)
	}
}

func TestGetAuthDomain_Unknown(t *testing.T) {
	if got := GetAuthDomain("nonexistent_xyz"); got != "" {
		t.Errorf("GetAuthDomain(nonexistent_xyz) = %q, want empty", got)
	}
}

func TestHasAuthDomain(t *testing.T) {
	if !HasAuthDomain("whiteboard") {
		t.Error("HasAuthDomain(whiteboard) = false, want true")
	}
	if HasAuthDomain("calendar") {
		t.Error("HasAuthDomain(calendar) = true, want false")
	}
}

func TestGetAuthChildren(t *testing.T) {
	children := GetAuthChildren("docs")
	found := false
	for _, c := range children {
		if c == "whiteboard" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("GetAuthChildren(docs) = %v, want to contain 'whiteboard'", children)
	}
}

func TestGetAuthChildren_NoChildren(t *testing.T) {
	children := GetAuthChildren("calendar")
	if len(children) != 0 {
		t.Errorf("GetAuthChildren(calendar) = %v, want empty", children)
	}
}
