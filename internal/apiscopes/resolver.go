// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package apiscopes resolves auth domains to their API scopes from the embedded
// API catalog and the registered shortcut set. It is a pure function of
// (catalog, shortcuts, brand) with no login, credential, or cobra state, so both
// auth login and the offline scopes export tool depend on it.
package apiscopes

import (
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts"
	"github.com/larksuite/cli/shortcuts/common"
)

// batchExcludedScopes lists scopes deliberately withheld from the aggregate
// batch sets that --domain / --recommend / bare `auth login` compute. In some
// tenants im:message.send_as_user requires admin review even for a personal
// assistant, so requesting it in bulk blocks users on approval. It stays
// available through an explicit --scope and through the on-demand grant flow
// when a command actually needs it.
var batchExcludedScopes = map[string]bool{
	"im:message.send_as_user": true,
}

// FilterBatchExcludedScopes drops batchExcludedScopes entries from a
// domain-derived scope slice, preserving order.
func FilterBatchExcludedScopes(scopes []string) []string {
	out := scopes[:0:0]
	for _, s := range scopes {
		if !batchExcludedScopes[s] {
			out = append(out, s)
		}
	}
	return out
}

// Resolver answers auth domain and scope questions against one build's
// shortcut snapshot. The snapshot is a build-local input rather than a constant:
// a distribution assembled with cmd.WithCommandSets contributes business
// commands whose declared scopes must participate in --domain resolution, so
// every method here reads the snapshot it was constructed with instead of the
// built-in set.
type Resolver struct {
	catalog    apicatalog.Catalog
	registered []common.Shortcut
	// HasExternal is true when this build carries business commands injected via
	// WithCommandSets beyond the built-in set. Such a build's domain/scope
	// universe is not reflected in the remote scopes.json (generated from the
	// standard CLI), so auth login must resolve locally instead of remote-first.
	HasExternal bool
}

// NewResolver builds a Resolver from the embedded API catalog and the registered
// shortcut set for this build.
func NewResolver(catalog apicatalog.Catalog, registered []common.Shortcut) Resolver {
	return Resolver{
		catalog:     catalog,
		registered:  registered,
		HasExternal: hasExternalCommands(registered),
	}
}

// Preload eagerly parses the catalog shards for the given domain names against
// the resolver's own catalog, so a corrupt shard fails typed here rather than
// silently dropping that domain's API scopes from a request about to be built.
func (r Resolver) Preload(names ...string) error {
	return r.catalog.Preload(names...)
}

// hasExternalCommands reports whether registered carries any command beyond the
// built-in set — the mark of a build that injected business commands via
// WithCommandSets. Such a build's scope universe reaches past what the remote
// scopes.json (generated from the standard CLI) covers, so auth login must
// resolve locally rather than remote-first.
//
// It compares command paths rather than counts: a business command mounts onto
// an existing domain (WithCommandSets cannot create new domains) and only ever
// adds to the built-in set, so any registered path absent from the built-in
// snapshot came from an injected command. A build that instead forks the
// registry to change scopes without adding commands is not detected here — no
// supported build option does that, and every current custom build extends via
// WithCommandSets. Activating a build-tag feature that swaps only the credential
// provider or transport (e.g. the auth sidecar) registers no commands, so it is
// correctly treated as standard.
func hasExternalCommands(registered []common.Shortcut) bool {
	builtin := shortcuts.AllShortcuts()
	paths := make(map[string]struct{}, len(builtin))
	for _, sc := range builtin {
		paths[sc.Service+" "+sc.Command] = struct{}{}
	}
	for _, sc := range registered {
		if _, ok := paths[sc.Service+" "+sc.Command]; !ok {
			return true
		}
	}
	return false
}

// ScopesFor collects API scopes (from from_meta projects) and shortcut scopes
// for the given domain names.
// Domains with auth_domain children are automatically expanded to include
// their children's scopes.
func (r Resolver) ScopesFor(domains []string, identity string, brand core.LarkBrand) []string {
	scopeSet := make(map[string]bool)

	// 1. API scopes from from_meta projects
	for _, s := range registry.CollectScopesForProjects(r.catalog, domains, identity) {
		scopeSet[s] = true
	}

	// 2. Expand domains: include auth_domain children
	domainSet := make(map[string]bool, len(domains))
	for _, d := range domains {
		domainSet[d] = true
		for _, child := range registry.GetAuthChildren(d) {
			domainSet[child] = true
		}
	}

	// 3. Shortcut scopes matching by Service (only include shortcuts supporting the identity)
	for _, sc := range r.registered {
		if !shortcuts.IsShortcutServiceAvailable(sc.Service, brand) {
			continue
		}
		if domainSet[sc.Service] && shortcutSupportsIdentity(sc, identity) {
			for _, s := range sc.DeclaredScopesForIdentity(identity) {
				scopeSet[s] = true
			}
		}
	}

	// 4. Deduplicate and sort
	result := make([]string, 0, len(scopeSet))
	for s := range scopeSet {
		result = append(result, s)
	}
	sort.Strings(result)
	return result
}

// AllKnown returns all valid auth domain names (from_meta projects +
// shortcut services), excluding domains that have auth_domain set (they are
// folded into their parent domain).
func (r Resolver) AllKnown(brand core.LarkBrand) map[string]bool {
	domains := make(map[string]bool)
	// The manifest name list is the --domain vocabulary: it is cheap (no shard
	// is parsed) and a corrupt shard stays addressable so that selecting it
	// fails typed in Preload instead of being reported as an unknown domain.
	for _, p := range r.catalog.Names() {
		if !registry.HasAuthDomain(p) {
			domains[p] = true
		}
	}
	for _, sc := range r.registered {
		if !shortcuts.IsShortcutServiceAvailable(sc.Service, brand) {
			continue
		}
		// No scope filter here: matching main, a scope-less domain (e.g.
		// event) stays addressable via --domain and the --help list, and
		// fails later with "no matching scopes found".
		if !registry.HasAuthDomain(sc.Service) {
			domains[sc.Service] = true
		}
	}
	return domains
}

// Sorted returns all valid domain names sorted alphabetically.
func (r Resolver) Sorted(brand core.LarkBrand) []string {
	m := r.AllKnown(brand)
	domains := make([]string, 0, len(m))
	for d := range m {
		domains = append(domains, d)
	}
	sort.Strings(domains)
	return domains
}

// Complete returns completions for comma-separated domain values.
func (r Resolver) Complete(toComplete string, brand core.LarkBrand) []string {
	allDomains := r.Sorted(brand)
	parts := strings.Split(toComplete, ",")
	prefix := parts[len(parts)-1]
	base := strings.Join(parts[:len(parts)-1], ",")

	var completions []string
	for _, d := range allDomains {
		if strings.HasPrefix(d, prefix) {
			if base == "" {
				completions = append(completions, d)
			} else {
				completions = append(completions, base+","+d)
			}
		}
	}
	return completions
}

// shortcutSupportsIdentity checks if a shortcut supports the given identity ("user" or "bot").
// Empty AuthTypes defaults to ["user"].
func shortcutSupportsIdentity(sc common.Shortcut, identity string) bool {
	authTypes := sc.AuthTypes
	if len(authTypes) == 0 {
		authTypes = []string{"user"}
	}
	for _, t := range authTypes {
		if t == identity {
			return true
		}
	}
	return false
}
