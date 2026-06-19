// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/meta"
)

// methodsForProjects walks the runtime catalog once and returns the methods in
// the given projects that are reachable by the identity. Catalog navigation is
// owned by apicatalog; the collectors below only apply scope policy.
func methodsForProjects(projects []string, identity string) []apicatalog.MethodRef {
	want := make(map[string]bool, len(projects))
	for _, p := range projects {
		want[p] = true
	}
	wantToken := meta.TokenForIdentity(identity)
	supported := func(m meta.Method) bool { return m.SupportsToken(wantToken) }
	// Walk only the requested services (in catalog name order) instead of every
	// service's methods then discarding the rest.
	var out []apicatalog.MethodRef
	for _, svc := range RuntimeCatalog().Services() {
		if want[svc.Name] {
			out = append(out, apicatalog.ServiceMethods(svc, supported)...)
		}
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

// FilterScopes filters scopes by domain and permission level.
func FilterScopes(allScopes []string, domains []string, permissions []string) []string {
	var result []string
	for _, scope := range allScopes {
		parts := strings.Split(scope, ":")

		if len(domains) > 0 {
			if len(parts) == 0 {
				continue
			}
			found := false
			for _, d := range domains {
				if parts[0] == d {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		if len(permissions) > 0 {
			if len(parts) < 3 {
				continue
			}
			perm := parts[2]
			matched := false
			for _, p := range permissions {
				switch p {
				case "read":
					if strings.Contains(perm, "read") {
						matched = true
					}
				case "write":
					if strings.Contains(perm, "write") {
						matched = true
					}
				case "readonly":
					if perm == "readonly" {
						matched = true
					}
				case "writeonly":
					if perm == "writeonly" || perm == "write_only" {
						matched = true
					}
				}
			}
			if !matched {
				continue
			}
		}

		result = append(result, scope)
	}
	return result
}

var cachedAllScopes map[string][]string

// CollectAllScopesFromMeta collects all unique scopes from from_meta/*.json
// for the given identity ("user" or "tenant"). Results are deduplicated and sorted.
func CollectAllScopesFromMeta(identity string) []string {
	if cachedAllScopes == nil {
		cachedAllScopes = make(map[string][]string)
	}
	if cached, ok := cachedAllScopes[identity]; ok {
		return cached
	}

	wantToken := meta.TokenForIdentity(identity)
	supported := func(m meta.Method) bool { return m.SupportsToken(wantToken) }
	scopeSet := make(map[string]bool)
	for _, ref := range RuntimeCatalog().WalkMethods(supported) {
		for _, s := range ref.Method.Scopes {
			scopeSet[s] = true
		}
	}

	result := make([]string, 0, len(scopeSet))
	for s := range scopeSet {
		result = append(result, s)
	}
	sort.Strings(result)
	cachedAllScopes[identity] = result
	return result
}

// CollectScopesForProjects collects the recommended scope for each API method
// in the specified from_meta projects. For each method, only the scope with
// the highest priority score is selected.
func CollectScopesForProjects(projects []string, identity string) []string {
	priorities := LoadScopePriorities()
	scopeSet := make(map[string]bool)
	for _, ref := range methodsForProjects(projects, identity) {
		if best := bestScope(ref.Method.Scopes, priorities); best != "" {
			scopeSet[best] = true
		}
	}

	result := make([]string, 0, len(scopeSet))
	for s := range scopeSet {
		result = append(result, s)
	}
	sort.Strings(result)
	return result
}

// ScopeSource tracks which APIs and shortcuts contributed a scope.
type ScopeSource struct {
	APIs      []string // e.g. "POST calendar.event.create"
	Shortcuts []string // e.g. "+send", "+reply"
}

// CollectScopesWithSources is like CollectScopesForProjects but also records
// which API method contributed each scope. Used by scope-audit.
func CollectScopesWithSources(projects []string, identity string) ([]string, map[string]*ScopeSource) {
	priorities := LoadScopePriorities()
	scopeSet := make(map[string]bool)
	sources := make(map[string]*ScopeSource)

	for _, ref := range methodsForProjects(projects, identity) {
		m := ref.Method
		best := bestScope(m.Scopes, priorities)
		if best == "" {
			continue
		}
		scopeSet[best] = true
		if sources[best] == nil {
			sources[best] = &ScopeSource{}
		}
		methodID := m.ID
		if methodID == "" {
			methodID = ref.ServiceName() + "." + ref.ResourceName() + "." + ref.MethodName()
		}
		httpMethod := m.HTTPMethod
		if httpMethod == "" {
			httpMethod = "?"
		}
		sources[best].APIs = append(sources[best].APIs, httpMethod+" "+methodID)
	}

	// Sort API lists for stable output
	for _, src := range sources {
		sort.Strings(src.APIs)
	}

	result := make([]string, 0, len(scopeSet))
	for s := range scopeSet {
		result = append(result, s)
	}
	sort.Strings(result)
	return result, sources
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
func CollectCommandScopes(projects []string, identity string) []CommandEntry {
	var entries []CommandEntry

	for _, ref := range methodsForProjects(projects, identity) {
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

// GetScopesForDomains returns scopes for specific projects (by project name).
func GetScopesForDomains(projects []string, identity string) []string {
	return CollectScopesForProjects(projects, identity)
}

// GetReadOnlyScopes returns read-only scopes from the recommended (best-per-method) scope set.
func GetReadOnlyScopes(identity string) []string {
	allProjects := ListFromMetaProjects()
	return FilterScopes(CollectScopesForProjects(allProjects, identity), nil, []string{"read", "readonly"})
}

// ResolveScopesFromFilters resolves scopes from project and permission filters.
func ResolveScopesFromFilters(projects []string, permissions []string, identity string) []string {
	return FilterScopes(CollectScopesForProjects(projects, identity), nil, permissions)
}

// ComputeMinimumScopeSet computes the minimum set of scopes that covers all
// from_meta API methods. Equivalent to CollectScopesForProjects with all projects.
func ComputeMinimumScopeSet(identity string) []string {
	return CollectScopesForProjects(ListFromMetaProjects(), identity)
}
