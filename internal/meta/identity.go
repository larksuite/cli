// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package meta

import "sort"

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
)

// IdentityForToken maps a metadata access token to the CLI identity (--as
// value) that uses it: tenant -> "bot", user -> "user". ok is false for
// unrecognized tokens. This is the single source of truth for the
// token<->identity vocabulary; schema, registry and command code all go
// through it instead of re-spelling the mapping.
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
// false, i.e. nil or empty accessTokens) is reachable by any token. This is
// the single source of truth for the predicate; registry scope policy and
// command identity checks build on it.
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

// Identities returns the CLI identities (--as values) that can call this
// method, derived from its metadata accessTokens: tenant -> "bot", user
// stays "user"; unrecognized tokens are dropped; the result is deduped and
// name-sorted. The slice is always non-nil so callers rendering it (e.g. the
// envelope's access_tokens) emit [] rather than null.
//
// An empty result does NOT imply unrestricted — use RestrictsIdentity() for
// that. Identities() lists only CLI-known identities, so a method restricted
// solely to unrecognized tokens returns empty yet RestrictsIdentity() is true.
func (m Method) Identities() []string {
	seen := make(map[string]bool, len(m.AccessTokens))
	for _, t := range m.AccessTokens {
		if id, ok := IdentityForToken(t); ok {
			seen[id] = true
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
