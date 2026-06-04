// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package meta

import (
	"reflect"
	"testing"
)

func TestIdentityTokenBijection(t *testing.T) {
	if got := TokenForIdentity("bot"); got != "tenant" {
		t.Errorf("TokenForIdentity(bot) = %q, want tenant", got)
	}
	if got := TokenForIdentity("user"); got != "user" {
		t.Errorf("TokenForIdentity(user) = %q, want user", got)
	}
	if id, ok := IdentityForToken("tenant"); id != "bot" || !ok {
		t.Errorf("IdentityForToken(tenant) = %q,%v want bot,true", id, ok)
	}
	if id, ok := IdentityForToken("user"); id != "user" || !ok {
		t.Errorf("IdentityForToken(user) = %q,%v want user,true", id, ok)
	}
	if _, ok := IdentityForToken("weird"); ok {
		t.Error("IdentityForToken(weird) ok=true, want false")
	}
}

func TestMethod_RestrictsIdentity(t *testing.T) {
	// nil and empty both mean "unrestricted"; only a populated list restricts.
	if (Method{}).RestrictsIdentity() {
		t.Error("nil accessTokens must be unrestricted")
	}
	if (Method{AccessTokens: []Token{}}).RestrictsIdentity() {
		t.Error("empty accessTokens must be unrestricted (same as nil)")
	}
	if !(Method{AccessTokens: []Token{"tenant"}}).RestrictsIdentity() {
		t.Error("populated accessTokens must restrict identity")
	}
}

func TestMethod_SupportsToken(t *testing.T) {
	// unrestricted (nil OR empty) -> permissive for any token; the two must not
	// diverge, else strict/scope and the command gate disagree.
	for _, m := range []Method{{}, {AccessTokens: []Token{}}} {
		if !m.SupportsToken("tenant") || !m.SupportsToken("user") {
			t.Errorf("unrestricted method %#v should support any token", m.AccessTokens)
		}
	}
	// restricted: only the declared tokens are reachable
	m := Method{AccessTokens: []Token{"tenant"}}
	if !m.SupportsToken("tenant") {
		t.Error("tenant-declared method should support tenant")
	}
	if m.SupportsToken("user") {
		t.Error("tenant-only method must NOT support user")
	}
}

func TestMethod_Identities(t *testing.T) {
	// tenant->bot, user stays; deduped + name-sorted (so order-independent);
	// unrecognized dropped; absent tokens -> empty but NON-nil so the envelope
	// renders [] not null.
	tests := []struct {
		name   string
		tokens []Token
		want   []string
	}{
		{"tenant only", []Token{"tenant"}, []string{"bot"}},
		{"user only", []Token{"user"}, []string{"user"}},
		{"tenant then user", []Token{"tenant", "user"}, []string{"bot", "user"}},
		{"user then tenant", []Token{"user", "tenant"}, []string{"bot", "user"}},
		{"deduped", []Token{"tenant", "tenant", "user"}, []string{"bot", "user"}},
		{"empty", []Token{}, []string{}},
		{"nil", nil, []string{}},
		{"unknown skipped", []Token{"user", "admin"}, []string{"user"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Method{AccessTokens: tt.tokens}).Identities(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Identities(%v) = %#v, want %#v", tt.tokens, got, tt.want)
			}
		})
	}
}
