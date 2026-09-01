// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
)

func TestExtractRequiredScopes_HappyPath(t *testing.T) {
	detail := map[string]interface{}{
		"permission_violations": []interface{}{
			map[string]interface{}{"subject": "docs:permission.member:create"},
			map[string]interface{}{"subject": "docs:doc"},
			map[string]interface{}{"subject": ""}, // empty subject filtered
			"not-a-map",                           // ignored
		},
	}
	got := ExtractRequiredScopes(detail)
	want := []string{"docs:permission.member:create", "docs:doc"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("ExtractRequiredScopes mismatch: got %v, want %v", got, want)
	}
}

func TestExtractRequiredScopes_NilOrMalformed(t *testing.T) {
	cases := []interface{}{
		nil,
		"plain string",
		map[string]interface{}{},
		map[string]interface{}{"permission_violations": "not-a-list"},
		map[string]interface{}{"permission_violations": []interface{}{}},
		map[string]interface{}{"permission_violations": []interface{}{
			map[string]interface{}{"subject": ""},
		}},
	}
	for i, in := range cases {
		if got := ExtractRequiredScopes(in); got != nil {
			t.Errorf("case %d: expected nil, got %v", i, got)
		}
	}
}

func TestBuildConsoleScopeURL_BrandSpecificHost(t *testing.T) {
	got := BuildConsoleScopeURL(core.BrandFeishu, "cli_xxx", "docs:permission.member:create")
	if !strings.Contains(got, "open.feishu.cn") {
		t.Errorf("feishu brand should use open.feishu.cn host, got %s", got)
	}
	if !strings.Contains(got, "clientID=cli_xxx") {
		t.Errorf("missing app id in url: %s", got)
	}
	if !strings.Contains(got, "scopes=docs%3Apermission.member%3Acreate") {
		t.Errorf("scope not URL-escaped: %s", got)
	}

	got = BuildConsoleScopeURL(core.BrandLark, "cli_yyy", "drive:drive")
	if !strings.Contains(got, "open.larksuite.com") {
		t.Errorf("lark brand should use open.larksuite.com host, got %s", got)
	}
}

func TestBuildConsoleScopeURL_EmptyInput(t *testing.T) {
	if got := BuildConsoleScopeURL(core.BrandFeishu, "", "docs:doc"); got != "" {
		t.Errorf("empty appID should yield empty url, got %s", got)
	}
	if got := BuildConsoleScopeURL(core.BrandFeishu, "cli_xxx", ""); got != "" {
		t.Errorf("empty scope should yield empty url, got %s", got)
	}
}

func TestSelectRecommendedScopeFromStrings_FallsBackToFirst(t *testing.T) {
	ensureFreshRegistry(t)
	// Unknown scopes (not in priority table) → fallback to first
	got := SelectRecommendedScopeFromStrings([]string{"unknown:foo", "unknown:bar"}, "tenant")
	if got != "unknown:foo" {
		t.Errorf("expected fallback to first, got %s", got)
	}
}

// When at least one scope is recognized by the priority table, the
// recommended scope wins over the fallback (first input).
func TestSelectRecommendedScopeFromStrings_PicksKnownScopeOverFallback(t *testing.T) {
	ensureFreshRegistry(t)
	// docs:permission.member:create is recommended (recommend=true) in
	// scope_priorities.json. Putting an unknown scope first would otherwise
	// win via the fallback path; this ensures the priority table is consulted
	// before falling back.
	got := SelectRecommendedScopeFromStrings([]string{"unknown:foo", "docs:permission.member:create"}, "tenant")
	if got != "docs:permission.member:create" {
		t.Errorf("expected priority-table winner, got %s", got)
	}
}

func TestSelectRecommendedScopeFromStrings_Empty(t *testing.T) {
	if got := SelectRecommendedScopeFromStrings(nil, "tenant"); got != "" {
		t.Errorf("nil slice should return empty, got %s", got)
	}
	if got := SelectRecommendedScopeFromStrings([]string{}, "tenant"); got != "" {
		t.Errorf("empty slice should return empty, got %s", got)
	}
}
