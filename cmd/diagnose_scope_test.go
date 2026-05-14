// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts"
	shortcutTypes "github.com/larksuite/cli/shortcuts/common"
)

// ── Data types ────────────────────────────────────────────────────────

type diagMethodEntry struct {
	Domain   string   `json:"domain"`
	Type     string   `json:"type"`     // "api" or "shortcut"
	Method   string   `json:"method"`   // "calendar.calendars.search" or "+agenda"
	Scope    string   `json:"scope"`    // minimum-privilege scope
	Identity []string `json:"identity"` // ["user"], ["bot"], or ["user","bot"]
}

type diagScopeInfo struct {
	Scope      string `json:"scope"`
	Recommend  bool   `json:"recommend"`
	InPriority bool   `json:"in_priority"`
}

type diagOutput struct {
	Methods []diagMethodEntry `json:"methods"`
	Scopes  []diagScopeInfo   `json:"scopes"`
}

// ── Core logic ────────────────────────────────────────────────────────

// diagAllKnownDomains returns sorted, deduplicated domain names from both
// from_meta projects and shortcuts.
func diagAllKnownDomains() []string {
	seen := make(map[string]bool)
	for _, p := range registry.ListFromMetaProjects() {
		seen[p] = true
	}
	for _, s := range shortcuts.AllShortcuts() {
		if s.Service != "" {
			seen[s.Service] = true
		}
	}
	result := make([]string, 0, len(seen))
	for d := range seen {
		result = append(result, d)
	}
	sort.Strings(result)
	return result
}

// methodKey uniquely identifies a method+scope pair for merging identities.
type methodKey struct {
	domain string
	typ    string
	method string
	scope  string
}

// diagBuild builds the full output: flat methods list (merged identities) + scopes.
func diagBuild(domains []string) diagOutput {
	recommend := registry.LoadAutoApproveSet()
	identities := []string{"user", "bot"}

	merged := make(map[methodKey]*diagMethodEntry)
	allSC := shortcuts.AllShortcuts()

	for _, domain := range domains {
		for _, identity := range identities {
			for _, ce := range registry.CollectCommandScopes([]string{domain}, identity) {
				for _, scope := range ce.Scopes {
					method := domain + "." + strings.ReplaceAll(ce.Command, " ", ".")
					k := methodKey{domain, "api", method, scope}
					if e, ok := merged[k]; ok {
						e.Identity = appendUniq(e.Identity, identity)
					} else {
						merged[k] = &diagMethodEntry{
							Domain: domain, Type: "api",
							Method: method,
							Scope:  scope, Identity: []string{identity},
						}
					}
				}
			}

			for _, sc := range allSC {
				if sc.Service != domain || !diagShortcutSupportsIdentity(&sc, identity) {
					continue
				}
				for _, scope := range sc.DeclaredScopesForIdentity(identity) {
					k := methodKey{domain, "shortcut", sc.Command, scope}
					if e, ok := merged[k]; ok {
						e.Identity = appendUniq(e.Identity, identity)
					} else {
						merged[k] = &diagMethodEntry{
							Domain: domain, Type: "shortcut",
							Method: sc.Command,
							Scope:  scope, Identity: []string{identity},
						}
					}
				}
			}
		}
	}

	methods := make([]diagMethodEntry, 0, len(merged))
	scopeSet := make(map[string]bool)
	for _, e := range merged {
		methods = append(methods, *e)
		scopeSet[e.Scope] = true
	}
	sort.Slice(methods, func(i, j int) bool {
		if methods[i].Domain != methods[j].Domain {
			return methods[i].Domain < methods[j].Domain
		}
		if methods[i].Type != methods[j].Type {
			return methods[i].Type < methods[j].Type
		}
		if methods[i].Method != methods[j].Method {
			return methods[i].Method < methods[j].Method
		}
		return methods[i].Scope < methods[j].Scope
	})

	scopeList := make([]string, 0, len(scopeSet))
	for s := range scopeSet {
		scopeList = append(scopeList, s)
	}
	sort.Strings(scopeList)

	priorities := registry.LoadScopePriorities()
	scopes := make([]diagScopeInfo, len(scopeList))
	for i, s := range scopeList {
		_, inPri := priorities[s]
		scopes[i] = diagScopeInfo{Scope: s, Recommend: recommend[s], InPriority: inPri}
	}

	return diagOutput{Methods: methods, Scopes: scopes}
}

func diagShortcutSupportsIdentity(sc *shortcutTypes.Shortcut, identity string) bool {
	if len(sc.AuthTypes) == 0 {
		return identity == "user"
	}
	for _, a := range sc.AuthTypes {
		if a == identity {
			return true
		}
	}
	return false
}

func appendUniq(ss []string, s string) []string {
	for _, existing := range ss {
		if existing == s {
			return ss
		}
	}
	return append(ss, s)
}

func TestDiagBuild_ShortcutIncludesConditionalScopes(t *testing.T) {
	out := diagBuild([]string{"drive"})
	var sawMetadata, sawDownload bool
	for _, method := range out.Methods {
		if method.Domain != "drive" || method.Type != "shortcut" || method.Method != "+status" {
			continue
		}
		if method.Scope == "drive:drive.metadata:readonly" {
			sawMetadata = true
		}
		if method.Scope == "drive:file:download" {
			sawDownload = true
		}
	}
	if !sawMetadata || !sawDownload {
		t.Fatalf("drive +status should advertise both metadata and conditional download scopes, saw metadata=%v download=%v", sawMetadata, sawDownload)
	}
}

// ── Snapshot generation ───────────────────────────────────────────────
//
// Generates a JSON snapshot of all API methods and shortcuts with their
// minimum-privilege scopes. Consumed by scripts/scope_audit.py.
//
// Usage:
//
//	SCOPE_SNAPSHOT_DIR=/tmp/scope-audit go test ./cmd/ -run TestScopeSnapshot -v
func TestScopeSnapshot(t *testing.T) {
	dir := os.Getenv("SCOPE_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("set SCOPE_SNAPSHOT_DIR to enable snapshot generation")
	}

	registry.Init()
	result := diagBuild(diagAllKnownDomains())

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "snapshot.json")

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	t.Logf("Wrote %s (%d methods, %d scopes)", path, len(result.Methods), len(result.Scopes))
}
