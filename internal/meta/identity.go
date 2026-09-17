// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package meta

import (
	"slices"
	"sort"
)

// Token is the metadata accessTokens vocabulary: which token kind a method
// accepts. It is a distinct type so the two directions of the token<->identity
// mapping below cannot be swapped silently — a bare string compiles on either
// side of a string/string signature, a Token does not. The CLI identity
// vocabulary ("bot"/"user") already has a home in internal/core (core.Identity);
// meta is a leaf and must not import core, so the identity side stays a plain
// string here and is typed at the core boundary.
type Token string

const (
	TokenTenant Token = "tenant" // bot calls use tenant_access_token
	TokenUser   Token = "user"
	// TokenAgent marks a method as callable by an Agent Employee principal. It is
	// not a third token kind: the call still travels on the user wire. It is also
	// not a CLI identity on its own: which identity it folds into depends on who
	// is asking, which is why IdentityForToken does not know it and IdentitiesFor
	// does.
	TokenAgent Token = "agent"
)

// IdentityForToken maps a metadata access token to the CLI identity (--as
// value) that uses it: tenant -> "bot", user -> "user". ok is false for every
// other token, including agent: an agent token is the user identity only for
// an Agent Employee principal, and a mapping that ignored the principal would
// tell a human profile it may call an endpoint the catalog reserves for agents.
// This is the single source of truth for the principal-free vocabulary;
// schema, registry and command code go through it or IdentitiesFor instead of
// re-spelling the mapping.
func IdentityForToken(token Token) (string, bool) {
	switch token {
	case TokenTenant:
		return "bot", true
	case TokenUser:
		return "user", true
	}
	return "", false
}

// TokenForIdentity is the inverse of IdentityForToken: "bot" -> TokenTenant;
// everything else (notably "user") maps to itself.
func TokenForIdentity(identity string) Token {
	if identity == "bot" {
		return TokenTenant
	}
	return Token(identity)
}

// RestrictsIdentity reports whether the method limits which identities may call
// it: true exactly when it declares one or more accessTokens. nil OR an empty
// slice means unrestricted (any identity). This is the single rule that both
// the strict-mode predicate (SupportsToken) and command identity gates use, so
// nil and [] never diverge across schema/scope and execution.
func (m Method) RestrictsIdentity() bool {
	return len(m.AccessTokens) > 0
}

// SupportsToken reports whether this method is reachable with the given access
// token (see TokenForIdentity). An unrestricted method (RestrictsIdentity ==
// false, i.e. nil or empty accessTokens) is reachable by any token. It is the
// token-side predicate registry scope collection and the strict-mode filter
// build on; command identity gates use IdentitiesFor, which is where the
// principal enters.
func (m Method) SupportsToken(token Token) bool {
	if !m.RestrictsIdentity() {
		return true
	}
	for _, t := range m.AccessTokens {
		if t == token {
			return true
		}
	}
	return false
}

// Identities returns the CLI identities (--as values) a human principal may
// use to call this method, derived from its metadata accessTokens: tenant ->
// "bot", user -> "user"; agent and unrecognized tokens are dropped; the result
// is deduped and name-sorted. It is IdentitiesFor(false); see there.
func (m Method) Identities() []string {
	return m.IdentitiesFor(false)
}

// IdentitiesFor returns the CLI identities (--as values) that can call this
// method for the given kind of principal. The agent token is the one place the
// principal matters: an Agent Employee rides the user wire, so for it agent
// folds into "user"; for a human principal it is not an identity at all. The
// slice is always non-nil so callers rendering it (e.g. the envelope's
// access_tokens) emit [] rather than null.
//
// An empty result does NOT imply unrestricted — use RestrictsIdentity() for
// that. A restricted method whose tokens map to nothing for this principal
// returns empty yet RestrictsIdentity() is true: nobody of this kind may call it.
func (m Method) IdentitiesFor(agentPrincipal bool) []string {
	seen := make(map[string]bool, len(m.AccessTokens))
	for _, t := range m.AccessTokens {
		if id, ok := IdentityForToken(t); ok {
			seen[id] = true
			continue
		}
		if agentPrincipal && t == TokenAgent {
			seen["user"] = true
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// SupportsAgent reports whether an Agent Employee principal may call this method.
//
// It is a separate predicate from SupportsToken because empty accessTokens must
// fail closed here, not read as "unrestricted", and it bypasses IdentityForToken
// so Identities() never reports a value that --as rejects.
func (m Method) SupportsAgent() bool {
	return slices.Contains(m.AccessTokens, TokenAgent)
}
