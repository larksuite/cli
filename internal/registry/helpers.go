// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import "github.com/larksuite/cli/internal/meta"

// DeclaredScopesForMethod returns the scopes declared by a method for the given
// identity. The recommended entry from `scopes` is the method's base permission;
// `requiredScopes` contains additional all-must-match permissions. Both belong
// in the login-time conjunction. Returns nil when the method has no scope
// information.
func DeclaredScopesForMethod(m meta.Method, identity string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(m.RequiredScopes)+1)
	if recommended := SelectRecommendedScopeFromStrings(m.Scopes, identity); recommended != "" {
		seen[recommended] = struct{}{}
		out = append(out, recommended)
	}
	if len(m.RequiredScopes) > 0 {
		for _, s := range m.RequiredScopes {
			if s == "" {
				continue
			}
			if _, ok := seen[s]; !ok {
				seen[s] = struct{}{}
				out = append(out, s)
			}
		}
	}
	return out
}
