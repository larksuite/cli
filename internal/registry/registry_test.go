// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"sort"
	"strings"
	"testing"
)

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

	scopes := []interface{}{scopeB, scopeA}
	result := SelectRecommendedScope(scopes, "user")

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
	scopes := []interface{}{
		"zzz_unknown:scope:a",
		"zzz_unknown:scope:b",
	}
	result := SelectRecommendedScope(scopes, "user")
	// All unknown scopes get DefaultScopeScore; first one with that score wins
	if result != "zzz_unknown:scope:a" {
		t.Errorf("expected zzz_unknown:scope:a, got %s", result)
	}
}

func TestSelectRecommendedScope_Empty(t *testing.T) {
	result := SelectRecommendedScope(nil, "user")
	if result != "" {
		t.Errorf("expected empty string, got %s", result)
	}

	result = SelectRecommendedScope([]interface{}{}, "user")
	if result != "" {
		t.Errorf("expected empty string, got %s", result)
	}
}

func TestComputeMinimumScopeSet(t *testing.T) {
	minSet := ComputeMinimumScopeSet("user")
	if len(minSet) == 0 {
		if len(ListFromMetaProjects()) == 0 {
			t.Skip("no from_meta data available")
		}
		t.Fatal("expected non-empty minimum scope set")
	}

	// Verify sorted
	if !sort.StringsAreSorted(minSet) {
		t.Error("expected sorted result")
	}

	// Verify no duplicates
	seen := make(map[string]bool)
	for _, s := range minSet {
		if seen[s] {
			t.Errorf("duplicate scope: %s", s)
		}
		seen[s] = true
	}

	t.Logf("Minimum scope set (%d scopes): %v", len(minSet), minSet)
}

func TestComputeMinimumScopeSet_Tenant(t *testing.T) {
	minSet := ComputeMinimumScopeSet("tenant")
	if len(minSet) == 0 {
		if len(ListFromMetaProjects()) == 0 {
			t.Skip("no from_meta data available")
		}
		t.Fatal("expected non-empty minimum scope set for tenant")
	}
	t.Logf("Tenant minimum scope set (%d scopes): %v", len(minSet), minSet)
}

func TestFilterScopes(t *testing.T) {
	scopes := []string{
		"calendar:calendar.event:read",
		"calendar:calendar:readonly",
		"task:task:read",
		"drive:drive.metadata:readonly",
	}

	// Filter by domain
	result := FilterScopes(scopes, []string{"calendar"}, nil)
	if len(result) != 2 {
		t.Errorf("expected 2 calendar scopes, got %d: %v", len(result), result)
	}

	// Filter by permission
	result = FilterScopes(scopes, nil, []string{"read"})
	for _, s := range result {
		t.Logf("read-filtered: %s", s)
	}
}

func TestFilterScopes_WritePermission(t *testing.T) {
	scopes := []string{
		"calendar:calendar.event:read",
		"calendar:calendar:readonly",
		"task:task:write",
		"drive:drive:writeonly",
		"drive:drive:write_only",
	}

	result := FilterScopes(scopes, nil, []string{"write"})
	// "write" matches anything containing "write" (including writeonly, write_only)
	if len(result) != 3 {
		t.Errorf("expected 3 scopes matching 'write', got %d: %v", len(result), result)
	}

	result = FilterScopes(scopes, nil, []string{"writeonly"})
	if len(result) != 2 {
		t.Errorf("expected 2 writeonly scopes, got %d: %v", len(result), result)
	}
}

func TestFilterScopes_DomainAndPermission(t *testing.T) {
	scopes := []string{
		"calendar:calendar.event:read",
		"calendar:calendar:readonly",
		"task:task:read",
		"drive:drive.metadata:readonly",
	}

	// Filter by domain AND permission
	result := FilterScopes(scopes, []string{"calendar"}, []string{"readonly"})
	if len(result) != 1 || result[0] != "calendar:calendar:readonly" {
		t.Errorf("expected [calendar:calendar:readonly], got %v", result)
	}
}

func TestFilterScopes_NilFilters(t *testing.T) {
	scopes := []string{"a:b:c", "d:e:f"}
	result := FilterScopes(scopes, nil, nil)
	if len(result) != 2 {
		t.Errorf("expected all scopes returned when no filters, got %d", len(result))
	}
}

func TestFilterScopes_Empty(t *testing.T) {
	result := FilterScopes(nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestFilterScopes_TooFewParts(t *testing.T) {
	scopes := []string{"onlyonepart", "two:parts"}
	// Permission filter requires at least 3 parts
	result := FilterScopes(scopes, nil, []string{"read"})
	if len(result) != 0 {
		t.Errorf("expected 0 results for short scopes, got %v", result)
	}
}

// --- Auto-approve functions ---

func TestLoadAutoApproveSet(t *testing.T) {
	aaSet := LoadAutoApproveSet()
	if len(aaSet) == 0 {
		t.Fatal("expected non-empty auto-approve set")
	}

	// From scope_overrides.json allow list
	if !aaSet["calendar:calendar.event:create"] {
		t.Error("expected calendar:calendar.event:create in auto-approve set (from allow list)")
	}

	// Verify allow list entries are present
	if !aaSet["sheets:spreadsheet:read"] {
		t.Error("expected sheets:spreadsheet:read in auto-approve set (from allow list)")
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
	if len(allowSet) == 0 {
		t.Fatal("expected non-empty override allow set")
	}

	// Known entries from scope_overrides.json
	if !allowSet["calendar:calendar.event:create"] {
		t.Error("expected calendar:calendar.event:create in allow set")
	}
	if !allowSet["mail:event"] {
		t.Error("expected mail:event in allow set")
	}
}

func TestLoadOverrideAutoApproveDeny(t *testing.T) {
	denySet := LoadOverrideAutoApproveDeny()
	// deny list may be empty if all entries are moved to _deny (commented out)
	t.Logf("Override deny set has %d scopes", len(denySet))
}

func TestIsAutoApproveScope(t *testing.T) {
	// Known auto-approve scope (in allow list)
	if !IsAutoApproveScope("calendar:calendar.event:create") {
		t.Error("expected calendar:calendar.event:create to be auto-approve")
	}

	// Completely unknown scope
	if IsAutoApproveScope("zzz:unknown:scope") {
		t.Error("expected unknown scope to NOT be auto-approve")
	}
}

func TestFilterAutoApproveScopes(t *testing.T) {
	scopes := []string{
		"calendar:calendar.event:create", // auto-approve (in allow list)
		"zzz:unknown:scope",              // not in auto-approve
		"sheets:spreadsheet:read",        // auto-approve (in allow list)
	}

	result := FilterAutoApproveScopes(scopes)
	if len(result) < 1 {
		t.Fatal("expected at least 1 auto-approve scope in result")
	}

	// Check that calendar:calendar.event:create is included
	found := false
	for _, s := range result {
		if s == "calendar:calendar.event:create" {
			found = true
		}
		// Ensure unknown scopes are not included
		if s == "zzz:unknown:scope" {
			t.Error("unknown scope should not be in auto-approve result")
		}
	}
	if !found {
		t.Error("expected calendar:calendar.event:create in result")
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

func TestGetStrFromMap(t *testing.T) {
	m := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
		"key3": nil,
	}

	if v := GetStrFromMap(m, "key1"); v != "value1" {
		t.Errorf("expected value1, got %s", v)
	}
	if v := GetStrFromMap(m, "key2"); v != "" {
		t.Errorf("expected empty for non-string value, got %s", v)
	}
	if v := GetStrFromMap(m, "missing"); v != "" {
		t.Errorf("expected empty for missing key, got %s", v)
	}
	if v := GetStrFromMap(nil, "key"); v != "" {
		t.Errorf("expected empty for nil map, got %s", v)
	}
}

func TestGetRegistryDir(t *testing.T) {
	dir := GetRegistryDir()
	if dir == "" {
		t.Error("expected non-empty registry dir")
	}
	t.Logf("Registry dir: %s", dir)
}

// --- Scope collection functions ---

func TestCollectAllScopesFromMeta(t *testing.T) {
	projects := ListFromMetaProjects()
	if len(projects) == 0 {
		t.Skip("no from_meta data available")
	}

	allScopes := CollectAllScopesFromMeta("user")
	if len(allScopes) == 0 {
		t.Fatal("expected non-empty scopes from from_meta")
	}

	// Should be sorted
	if !sort.StringsAreSorted(allScopes) {
		t.Error("expected sorted result")
	}

	// Should include more scopes than the minimum set (since minimum picks best per method)
	minSet := ComputeMinimumScopeSet("user")
	if len(allScopes) < len(minSet) {
		t.Errorf("all scopes (%d) should be >= minimum set (%d)", len(allScopes), len(minSet))
	}

	t.Logf("All scopes from meta: %d (min set: %d)", len(allScopes), len(minSet))
}

func TestCollectAllScopesFromMeta_Caching(t *testing.T) {
	projects := ListFromMetaProjects()
	if len(projects) == 0 {
		t.Skip("no from_meta data available")
	}

	result1 := CollectAllScopesFromMeta("user")
	result2 := CollectAllScopesFromMeta("user")

	if len(result1) != len(result2) {
		t.Errorf("cached result length mismatch: %d vs %d", len(result1), len(result2))
	}
}

func TestCollectScopesWithSources(t *testing.T) {
	projects := ListFromMetaProjects()
	if len(projects) == 0 {
		t.Skip("no from_meta data available")
	}

	// Use calendar project which is well-known
	scopes, sources := CollectScopesWithSources([]string{"calendar"}, "user")
	if len(scopes) == 0 {
		t.Fatal("expected non-empty scopes for calendar")
	}

	// Should be sorted
	if !sort.StringsAreSorted(scopes) {
		t.Error("expected sorted scopes")
	}

	// Each scope should have a source
	for _, s := range scopes {
		src, ok := sources[s]
		if !ok {
			t.Errorf("scope %s has no source entry", s)
			continue
		}
		if len(src.APIs) == 0 {
			t.Errorf("scope %s has no API sources", s)
		}
	}

	t.Logf("Calendar scopes with sources: %d scopes", len(scopes))
}

func TestCollectScopesWithSources_EmptyProject(t *testing.T) {
	scopes, sources := CollectScopesWithSources([]string{"nonexistent_project"}, "user")
	if len(scopes) != 0 {
		t.Errorf("expected empty scopes for nonexistent project, got %d", len(scopes))
	}
	if len(sources) != 0 {
		t.Errorf("expected empty sources for nonexistent project, got %d", len(sources))
	}
}

func TestCollectCommandScopes(t *testing.T) {
	projects := ListFromMetaProjects()
	if len(projects) == 0 {
		t.Skip("no from_meta data available")
	}

	entries := CollectCommandScopes([]string{"calendar"}, "user")
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
	entries := CollectCommandScopes([]string{"nonexistent_project"}, "user")
	if len(entries) != 0 {
		t.Errorf("expected empty entries for nonexistent project, got %d", len(entries))
	}
}

func TestGetScopesForDomains(t *testing.T) {
	projects := ListFromMetaProjects()
	if len(projects) == 0 {
		t.Skip("no from_meta data available")
	}

	// GetScopesForDomains is a wrapper for CollectScopesForProjects
	scopes := GetScopesForDomains([]string{"calendar"}, "user")
	expected := CollectScopesForProjects([]string{"calendar"}, "user")

	if len(scopes) != len(expected) {
		t.Errorf("GetScopesForDomains and CollectScopesForProjects differ: %d vs %d", len(scopes), len(expected))
	}
}

func TestGetReadOnlyScopes(t *testing.T) {
	projects := ListFromMetaProjects()
	if len(projects) == 0 {
		t.Skip("no from_meta data available")
	}

	readOnly := GetReadOnlyScopes("user")
	// May be empty if no read-only scopes exist, but should not panic
	for _, s := range readOnly {
		parts := strings.Split(s, ":")
		if len(parts) < 3 {
			t.Errorf("unexpected scope format (too few parts): %s", s)
			continue
		}
		perm := parts[2]
		if !strings.Contains(perm, "read") && perm != "readonly" {
			t.Errorf("non-read scope in read-only result: %s", s)
		}
	}

	t.Logf("Read-only scopes: %d", len(readOnly))
}

func TestResolveScopesFromFilters(t *testing.T) {
	projects := ListFromMetaProjects()
	if len(projects) == 0 {
		t.Skip("no from_meta data available")
	}

	// Should behave like CollectScopesForProjects + FilterScopes
	scopes := ResolveScopesFromFilters([]string{"calendar"}, []string{"read", "readonly"}, "user")
	for _, s := range scopes {
		parts := strings.Split(s, ":")
		if len(parts) < 3 {
			continue
		}
		perm := parts[2]
		if !strings.Contains(perm, "read") && perm != "readonly" {
			t.Errorf("non-read scope in filtered result: %s", s)
		}
	}

	t.Logf("Resolved filtered scopes: %d", len(scopes))
}

func TestCollectScopesForProjects_MultipleProjects(t *testing.T) {
	projects := ListFromMetaProjects()
	if len(projects) < 2 {
		t.Skip("need at least 2 from_meta projects")
	}

	// Multiple projects should yield more scopes than a single one
	single := CollectScopesForProjects(projects[:1], "user")
	multi := CollectScopesForProjects(projects[:2], "user")

	if len(multi) < len(single) {
		t.Errorf("multi-project scopes (%d) should be >= single-project (%d)", len(multi), len(single))
	}
}

func TestCollectScopesForProjects_NonexistentProject(t *testing.T) {
	scopes := CollectScopesForProjects([]string{"nonexistent_project_xyz"}, "user")
	if len(scopes) != 0 {
		t.Errorf("expected empty scopes for nonexistent project, got %d", len(scopes))
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
