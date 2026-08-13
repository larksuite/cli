// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

// IdentityType is the closed set of values for IdentitySpec.Type (validated at
// Register time to guard against typos).
type IdentityType string

const (
	IdentityUser IdentityType = "user"
	IdentityBot  IdentityType = "bot"
)

// IdentitySpec declares a supported identity and its precondition, if any.
type IdentitySpec struct {
	Type         IdentityType `json:"type"` // IdentityUser | IdentityBot
	Precondition string       `json:"precondition,omitempty"`

	// Scopes is the full scope set this identity needs for ANY real API verb of
	// the provider (the preflight is all-or-nothing, per identity). It is
	// registration data for the scope preflight only and never serializes into
	// the card — a missing scope teaches through the missing_scope error at call
	// time. Empty means this identity runs no scope preflight (e.g. a mock
	// provider, or an identity whose scopes are enforced server-side only).
	Scopes []string `json:"-"`
}

// AgentSummary is one discoverable agent in `agents list <scheme>` output.
type AgentSummary struct {
	AgentRef    string `json:"agent_ref"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}
