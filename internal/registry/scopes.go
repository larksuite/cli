// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"slices"
	"sort"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/meta"
)

// methodsForProjects loads the given projects from the catalog and returns the
// methods that are reachable by the identity. Catalog navigation is
// owned by apicatalog; the collectors below only apply scope policy.
func methodsForProjects(catalog apicatalog.Catalog, projects []string, identity string) []apicatalog.MethodRef {
	names := append([]string(nil), projects...)
	sort.Strings(names)
	names = slices.Compact(names)
	wantToken := meta.TokenForIdentity(identity)
	supported := func(m meta.Method) bool { return m.SupportsToken(wantToken) }
	// Load only the requested services (in name order) so a caller asking about
	// one domain does not parse every shard. A service the catalog cannot
	// provide contributes nothing here; callers that must not tolerate a corrupt
	// shard call Catalog.Preload first.
	var out []apicatalog.MethodRef
	for _, name := range names {
		svc, ok := catalog.Service(name)
		if !ok {
			continue
		}
		out = append(out, apicatalog.ServiceMethods(svc, supported)...)
	}
	return out
}

// bestScope returns the highest-priority scope from scopes (minimum privilege),
// or "" when scopes is empty.
func bestScope(scopes []string, priorities map[string]int) string {
	best := ""
	bestScore := -1
	for _, s := range scopes {
		score := DefaultScopeScore
		if v, ok := priorities[s]; ok {
			score = v
		}
		if score > bestScore {
			bestScore = score
			best = s
		}
	}
	return best
}

// FilterForStrictMode returns a method filter enforcing the strict-mode forced
// identity, or nil when strict mode is inactive (no filtering). The
// token/identity vocabulary (meta.TokenForIdentity) and the "no accessTokens =
// permissive" predicate (meta.Method.SupportsToken) both live in meta, so this
// only composes them — schema completion/render and service commands never
// re-derive identity semantics.
func FilterForStrictMode(mode core.StrictMode) apicatalog.MethodFilter {
	if !mode.IsActive() {
		return nil
	}
	token := meta.TokenForIdentity(string(mode.ForcedIdentity()))
	return func(m meta.Method) bool { return m.SupportsToken(token) }
}

// CollectScopesForProjects collects the effective scopes for each API method in
// the specified from_meta projects. It uses DeclaredScopesForMethod so a
// method's full requiredScopes conjunction is honored (e.g. reading a mail
// message needs the subject/address/body scopes together, not just the umbrella
// readonly scope), falling back to the single recommended scope when a method
// declares no requiredScopes.
func CollectScopesForProjects(catalog apicatalog.Catalog, projects []string, identity string) []string {
	scopeSet := make(map[string]bool)
	for _, ref := range methodsForProjects(catalog, projects, identity) {
		for _, s := range DeclaredScopesForMethod(ref.Method, identity) {
			scopeSet[s] = true
		}
	}

	result := make([]string, 0, len(scopeSet))
	for s := range scopeSet {
		result = append(result, s)
	}
	sort.Strings(result)
	return result
}

// CommandEntry represents a CLI command (API method or shortcut) and its scopes.
type CommandEntry struct {
	Command    string   // CLI label, e.g. "calendars create" or "+agenda"
	Type       string   // "api" or "shortcut"
	Scopes     []string // effective scopes (requiredScopes if present, else [bestScope])
	HTTPMethod string   // e.g. "POST" (API only)
}

// CollectCommandScopes walks from_meta methods for the given projects and
// returns one CommandEntry per API method, sorted by command label.
//
// Scope selection per method:
//   - If the method has a "requiredScopes" field, all of those scopes are needed (conjunction).
//   - Otherwise, only the highest-priority scope from "scopes" is shown (minimum privilege).
func CollectCommandScopes(catalog apicatalog.Catalog, projects []string, identity string) []CommandEntry {
	var entries []CommandEntry

	for _, ref := range methodsForProjects(catalog, projects, identity) {
		m := ref.Method
		if len(m.Scopes) == 0 {
			continue
		}

		// Effective-scope policy (requiredScopes conjunction, else recommended)
		// lives once in DeclaredScopesForMethod.
		effectiveScopes := DeclaredScopesForMethod(m, identity)
		if len(effectiveScopes) == 0 {
			continue
		}

		entries = append(entries, CommandEntry{
			Command:    ref.ResourceName() + " " + ref.MethodName(),
			Type:       "api",
			Scopes:     effectiveScopes,
			HTTPMethod: m.HTTPMethod,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Command < entries[j].Command
	})
	return entries
}
