// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import "github.com/larksuite/cli/internal/meta"

// DeclaredScopesForMethod returns the scopes declared by a method for the given
// identity. Prefers the explicit `requiredScopes` field when present; otherwise
// returns the single recommended scope from `scopes` (or the first scope as a
// final fallback). Returns nil when the method has no scope information.
func DeclaredScopesForMethod(m meta.Method, identity string) []string {
	if len(m.RequiredScopes) > 0 {
		out := make([]string, 0, len(m.RequiredScopes))
		for _, s := range m.RequiredScopes {
			if s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if len(m.Scopes) == 0 {
		return nil
	}
	if recommended := SelectRecommendedScopeFromStrings(m.Scopes, identity); recommended != "" {
		return []string{recommended}
	}
	return nil
}
