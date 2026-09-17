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
	// agent is not an identity on its own: it folds into user only for an
	// Agent Employee principal, which is IdentitiesFor's job, not this one's.
	if id, ok := IdentityForToken("agent"); id != "" || ok {
		t.Errorf("IdentityForToken(agent) = %q,%v want \"\",false", id, ok)
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

func TestMethodSupportsAgent(t *testing.T) {
	cases := []struct {
		name   string
		tokens []Token
		want   bool
	}{
		{"empty is not agent-callable", nil, false},
		{"user only", []Token{TokenUser}, false},
		{"tenant and user", []Token{TokenTenant, TokenUser}, false},
		{"user and agent", []Token{TokenUser, TokenAgent}, true},
		{"agent alone", []Token{TokenAgent}, true},
		{"tenant and agent", []Token{TokenTenant, TokenAgent}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := Method{AccessTokens: tc.tokens}
			if got := m.SupportsAgent(); got != tc.want {
				t.Errorf("SupportsAgent() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestIdentitiesForFoldsAgentOnlyUnderAgentPrincipal pins the one place the
// principal enters the token vocabulary. The agent token rides the user wire,
// so for an Agent Employee it is the user identity; for a human principal it is
// not an identity at all, or a human profile would be told it may call an
// endpoint the catalog reserves for agents.
func TestIdentitiesForFoldsAgentOnlyUnderAgentPrincipal(t *testing.T) {
	cases := []struct {
		name      string
		tokens    []Token
		wantHuman []string
		wantAgent []string
	}{
		{"agent only", []Token{TokenAgent}, []string{}, []string{"user"}},
		{"tenant and agent", []Token{TokenTenant, TokenAgent}, []string{"bot"}, []string{"bot", "user"}},
		{"user and agent", []Token{TokenUser, TokenAgent}, []string{"user"}, []string{"user"}},
		{"tenant and user", []Token{TokenTenant, TokenUser}, []string{"bot", "user"}, []string{"bot", "user"}},
		{"unknown only", []Token{"admin"}, []string{}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := Method{AccessTokens: tc.tokens}
			if got := m.IdentitiesFor(false); !reflect.DeepEqual(got, tc.wantHuman) {
				t.Errorf("IdentitiesFor(false) = %v, want %v", got, tc.wantHuman)
			}
			if got := m.IdentitiesFor(true); !reflect.DeepEqual(got, tc.wantAgent) {
				t.Errorf("IdentitiesFor(true) = %v, want %v", got, tc.wantAgent)
			}
			if got := m.Identities(); !reflect.DeepEqual(got, tc.wantHuman) {
				t.Errorf("Identities() = %v, want the human view %v", got, tc.wantHuman)
			}
		})
	}
	agentOnly := Method{AccessTokens: []Token{TokenAgent}}
	if !agentOnly.RestrictsIdentity() {
		t.Error("RestrictsIdentity() should stay true for a non-empty accessTokens")
	}
	// Scope collection for a human login is token-side and must not pick up
	// agent-only methods either.
	if agentOnly.SupportsToken(TokenUser) {
		t.Error("agent-only method must NOT support the user token")
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
		{"agent only is not a human identity", []Token{"agent"}, []string{}},
		{"user and agent", []Token{"user", "agent"}, []string{"user"}},
		{"tenant and agent", []Token{"tenant", "agent"}, []string{"bot"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Method{AccessTokens: tt.tokens}).Identities(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Identities(%v) = %#v, want %#v", tt.tokens, got, tt.want)
			}
		})
	}
}
