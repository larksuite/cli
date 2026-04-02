// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestSuggestDomain_PrefixMatch(t *testing.T) {
	known := map[string]bool{
		"calendar": true,
		"task":     true,
		"drive":    true,
		"im":       true,
	}

	// Input is prefix of known domain
	if s := suggestDomain("cal", known); s != "calendar" {
		t.Errorf("expected 'calendar', got %q", s)
	}

	// Known domain is prefix of input
	if s := suggestDomain("calendar_extra", known); s != "calendar" {
		t.Errorf("expected 'calendar', got %q", s)
	}
}

func TestSuggestDomain_NoMatch(t *testing.T) {
	known := map[string]bool{
		"calendar": true,
		"task":     true,
	}

	if s := suggestDomain("zzz", known); s != "" {
		t.Errorf("expected empty suggestion, got %q", s)
	}
}

func TestSuggestDomain_ExactMatch(t *testing.T) {
	known := map[string]bool{
		"calendar": true,
	}

	// Exact match: input is prefix of known AND known is prefix of input
	if s := suggestDomain("calendar", known); s != "calendar" {
		t.Errorf("expected 'calendar', got %q", s)
	}
}

func TestShortcutSupportsIdentity_DefaultUser(t *testing.T) {
	// Empty AuthTypes defaults to ["user"]
	sc := common.Shortcut{AuthTypes: nil}
	if !shortcutSupportsIdentity(sc, "user") {
		t.Error("expected default to support 'user'")
	}
	if shortcutSupportsIdentity(sc, "bot") {
		t.Error("expected default to NOT support 'bot'")
	}
}

func TestShortcutSupportsIdentity_ExplicitTypes(t *testing.T) {
	sc := common.Shortcut{AuthTypes: []string{"user", "bot"}}
	if !shortcutSupportsIdentity(sc, "user") {
		t.Error("expected to support 'user'")
	}
	if !shortcutSupportsIdentity(sc, "bot") {
		t.Error("expected to support 'bot'")
	}
	if shortcutSupportsIdentity(sc, "tenant") {
		t.Error("expected to NOT support 'tenant'")
	}
}

func TestShortcutSupportsIdentity_BotOnly(t *testing.T) {
	sc := common.Shortcut{AuthTypes: []string{"bot"}}
	if shortcutSupportsIdentity(sc, "user") {
		t.Error("expected bot-only to NOT support 'user'")
	}
	if !shortcutSupportsIdentity(sc, "bot") {
		t.Error("expected bot-only to support 'bot'")
	}
}

func TestCompleteDomain(t *testing.T) {
	projects := registry.ListFromMetaProjects()
	if len(projects) == 0 {
		t.Skip("no from_meta data available")
	}

	// Complete from empty prefix
	completions := completeDomain("")
	if len(completions) == 0 {
		t.Fatal("expected completions for empty prefix")
	}
	// All completions should match from_meta projects
	if len(completions) != len(projects) {
		t.Errorf("expected %d completions, got %d", len(projects), len(completions))
	}

	// Complete with partial prefix
	completions = completeDomain("cal")
	for _, c := range completions {
		if c != "calendar" && c[:3] != "cal" {
			t.Errorf("unexpected completion %q for prefix 'cal'", c)
		}
	}
}

func TestCompleteDomain_CommaSeparated(t *testing.T) {
	projects := registry.ListFromMetaProjects()
	if len(projects) == 0 {
		t.Skip("no from_meta data available")
	}

	// After a comma, should complete the next segment
	completions := completeDomain("calendar,")
	for _, c := range completions {
		if c[:9] != "calendar," {
			t.Errorf("expected 'calendar,' prefix, got %q", c)
		}
	}
}

func TestAllKnownDomains(t *testing.T) {
	domains := allKnownDomains()
	if len(domains) == 0 {
		t.Fatal("expected non-empty known domains")
	}

	// Should include from_meta projects
	for _, p := range registry.ListFromMetaProjects() {
		if !domains[p] {
			t.Errorf("expected from_meta project %q in known domains", p)
		}
	}
}

func TestSortedKnownDomains(t *testing.T) {
	sorted := sortedKnownDomains()
	if len(sorted) == 0 {
		t.Fatal("expected non-empty sorted domains")
	}

	if !sort.StringsAreSorted(sorted) {
		t.Error("expected sorted result")
	}

	// Should match allKnownDomains
	known := allKnownDomains()
	if len(sorted) != len(known) {
		t.Errorf("sorted (%d) and known (%d) length mismatch", len(sorted), len(known))
	}
}

func TestGetShortcutOnlyDomainNames_HaveDescriptions(t *testing.T) {
	for _, name := range getShortcutOnlyDomainNames() {
		zhDesc := registry.GetServiceDescription(name, "zh")
		enDesc := registry.GetServiceDescription(name, "en")
		if zhDesc == "" {
			t.Errorf("missing zh description for shortcut-only domain %q", name)
		}
		if enDesc == "" {
			t.Errorf("missing en description for shortcut-only domain %q", name)
		}
	}
}

func TestCollectScopesForDomains(t *testing.T) {
	projects := registry.ListFromMetaProjects()
	if len(projects) == 0 {
		t.Skip("no from_meta data available")
	}

	scopes := collectScopesForDomains([]string{"calendar"}, "user")
	if len(scopes) == 0 {
		t.Fatal("expected non-empty scopes for calendar domain")
	}

	// Should be sorted
	if !sort.StringsAreSorted(scopes) {
		t.Error("expected sorted result")
	}

	// Should include at least the API scopes
	apiScopes := registry.CollectScopesForProjects([]string{"calendar"}, "user")
	for _, s := range apiScopes {
		found := false
		for _, cs := range scopes {
			if cs == s {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("API scope %q missing from collectScopesForDomains result", s)
		}
	}
}

func TestCollectScopesForDomains_NonexistentDomain(t *testing.T) {
	scopes := collectScopesForDomains([]string{"nonexistent_domain_xyz"}, "user")
	if len(scopes) != 0 {
		t.Errorf("expected empty scopes for nonexistent domain, got %d", len(scopes))
	}
}

func TestGetDomainMetadata_IncludesFromMeta(t *testing.T) {
	domains := getDomainMetadata("zh")
	nameSet := make(map[string]bool)
	for _, dm := range domains {
		nameSet[dm.Name] = true
	}

	// from_meta projects must be present
	for _, p := range registry.ListFromMetaProjects() {
		if !nameSet[p] {
			t.Errorf("from_meta project %q missing from getDomainMetadata", p)
		}
	}
}

func TestGetDomainMetadata_IncludesShortcutOnlyDomains(t *testing.T) {
	domains := getDomainMetadata("zh")
	nameSet := make(map[string]bool)
	for _, dm := range domains {
		nameSet[dm.Name] = true
	}

	for _, name := range getShortcutOnlyDomainNames() {
		if !nameSet[name] {
			t.Errorf("shortcut-only domain %q missing from getDomainMetadata", name)
		}
	}
}

func TestGetDomainMetadata_Sorted(t *testing.T) {
	domains := getDomainMetadata("zh")
	for i := 1; i < len(domains); i++ {
		if domains[i].Name < domains[i-1].Name {
			t.Errorf("not sorted: %q before %q", domains[i-1].Name, domains[i].Name)
		}
	}
}

func TestGetDomainMetadata_HasTitleAndDescription(t *testing.T) {
	domains := getDomainMetadata("zh")
	for _, dm := range domains {
		if dm.Title == "" {
			t.Errorf("domain %q has empty Title", dm.Name)
		}
	}
}

func TestAuthLoginRun_NonTerminal_NoFlags_RejectsWithHint(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	// TestFactory has IsTerminal=false by default
	opts := &LoginOptions{Factory: f, Ctx: context.Background()}
	err := authLoginRun(opts)
	if err == nil {
		t.Fatal("expected error for non-terminal without flags")
	}
	// Should mention specifying scopes
	msg := err.Error()
	if !strings.Contains(msg, "scopes") {
		t.Errorf("expected error to mention scopes, got: %s", msg)
	}
	// Stderr should contain background hint
	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "background") {
		t.Errorf("expected stderr to mention background, got: %s", stderrStr)
	}
}

func TestGetDomainMetadata_ExcludesEvent(t *testing.T) {
	domains := getDomainMetadata("zh")
	for _, dm := range domains {
		if dm.Name == "event" {
			t.Error("event should not appear in interactive domain list")
		}
	}
}

func TestAllKnownDomains_ExcludesAuthDomainChildren(t *testing.T) {
	domains := allKnownDomains()
	if domains["whiteboard"] {
		t.Error("whiteboard should not appear in known auth domains (it has auth_domain=docs)")
	}
	if !domains["docs"] {
		t.Error("docs should still be a known auth domain")
	}
}

func TestCollectScopesForDomains_ExpandsAuthDomainChildren(t *testing.T) {
	scopes := collectScopesForDomains([]string{"docs"}, "user")
	// docs domain should include whiteboard shortcut scopes (board:whiteboard:*)
	found := false
	for _, s := range scopes {
		if strings.HasPrefix(s, "board:whiteboard:") {
			found = true
			break
		}
	}
	if !found {
		t.Error("collectScopesForDomains([docs]) should include whiteboard scopes (board:whiteboard:*)")
	}
}

func TestGetDomainMetadata_ExcludesAuthDomainChildren(t *testing.T) {
	domains := getDomainMetadata("zh")
	for _, dm := range domains {
		if dm.Name == "whiteboard" {
			t.Error("whiteboard should not appear in interactive domain list (has auth_domain=docs)")
		}
	}
}
