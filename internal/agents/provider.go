// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"

	"github.com/larksuite/cli/internal/core"
)

// SendInput is the input to send. Business parameters are NOT here — they ride
// Runtime.Params() like every other operation, so send and the other seven
// verbs share one parameter model.
type SendInput struct {
	Text      string
	Files     []string
	ContextID string
	TaskID    string

	// Answers is the structured reply to the task's pending input_required
	// question group. Each key is a question_id (bare form — every value must hit
	// one of that question's OptionIDs) or "<question_id>.text" (free-text form,
	// exactly one value, CLI-guarded). Values keep argv order; repeated bare
	// values on a multi-select accumulate. Nil/empty means this send is not
	// answering. Text alongside Answers is a message-level remark, never an answer.
	//
	// Semantic validation is the provider's own policy (tolerant backends may take
	// partial answers, strict ones validate), but any violation it does report must
	// use the collect-all ValidationError shape with per-question params entries,
	// acceptance must be atomic, and nothing may be silently dropped.
	Answers map[string][]string
}

// CardInfo is the per-agent descriptive metadata a provider supplies for its
// Card (the display Name/Description and Skills). It is returned by
// AgentSpec.Describe. It deliberately does NOT carry parameters: offline
// validation only trusts the static per-operation declarations, and dynamic
// per-agent parameter contracts belong to the future overlay phase.
type CardInfo struct {
	Name        string
	Description string
	Skills      []CardSkill
}

// Provider is one business domain (one scheme): registration metadata plus its
// agent set. It is a declarative value — registered from agent/register.go, not
// constructed via a factory. Exactly one of Catalog / Instance is set (Register
// enforces), which encodes the kind, so there is no separate Kind field to keep
// in sync.
type Provider struct {
	Scheme        string         // ref prefix, e.g. "base"
	Label         string         // `agents list` LABEL column
	AgentIDSource string         // where to get an agent_id (AI onboarding cue)
	Identities    []IdentitySpec // non-empty; Type ∈ {user,bot}; scopes are declared per identity (IdentitySpec.Scopes)

	// Exactly one of these is set:
	Catalog  []AgentSpec // finite, offline-enumerable set (kind = catalog)
	Instance *AgentSpec  // single template for any runtime agent_id (kind = instance)

	// ListAgents is the optional ONLINE enumeration hook, only meaningful for an
	// instance provider whose platform has a "list my agents" endpoint. Catalog
	// providers leave it nil (enumeration is derived offline from Catalog), as do
	// instance platforms with only get-by-id. It is distinct from
	// AgentSpec.Describe: ListAgents answers "which agents exist", Describe "what
	// one agent looks like". Paginated via PageParams/PageInfo.
	ListAgents func(ctx context.Context, rt Runtime, page PageParams) ([]AgentSummary, PageInfo, error)

	// ListParams declares the business parameters of `agents list <scheme>` itself:
	// list is provider-level, so its parameters live here rather than on an agent's
	// spec, and surface as providers[].list_parameters. Register panics when
	// declared without a ListAgents hook.
	ListParams []CardParam
}

// ScopesForIdentity returns the declared identity's Scopes verbatim, nil when the
// identity is not declared. Rejecting an unsupported identity belongs to the
// identity gate, not here; an empty result means "no scope preflight".
func (p Provider) ScopesForIdentity(t IdentityType) []string {
	for _, id := range p.Identities {
		if id.Type == t {
			return id.Scopes
		}
	}
	return nil
}

// AgentSpec is the declarative unit for one agent: card metadata plus the
// operations it implements. Each operation is an Op unit binding the business
// parameters it accepts to the handler that serves it — parameters physically
// cannot be declared on an unimplemented operation. Capability is derived from
// which handlers are wired ("implement it = support it", see
// DeriveCapabilities), so the card and the behavior are single-sourced and
// cannot drift.
//
//   - Catalog: each predefined agent is its own AgentSpec with its own wired
//     operations — two agents honestly differ in capability with zero bool
//     matrix and zero per-id branching.
//   - Instance: ONE template applied to every runtime agent_id; handlers read
//     rt.AgentID() to know which agent they serve.
type AgentSpec struct {
	ID string // catalog: required + unique; instance: MUST be empty

	// Brands scopes the WHOLE agent to a subset of brands (feishu/lark): empty
	// means visible/usable under every brand. It is declared at registration
	// (brand-agnostic) and filtered/gated at command time against the resolved
	// brand — catalog list visibility and every verb's brand gate consult
	// SpecAvailableForBrand. Register validates every value is feishu|lark.
	Brands []core.LarkBrand

	// Per-agent card metadata (static, read offline).
	Name        string
	Description string
	Skills      []CardSkill

	// Behavioral flags with no backing operation (the only capability bits not
	// derived from a handler).
	FileInput     bool
	InputRequired bool

	// Core operations (Register asserts both handlers non-nil for every spec).
	Send    SendOp
	GetTask TaskGetOp

	// Optional operations (zero-value Op = unsupported; the command layer gates
	// on the unwired handler and returns a unified unsupported_capability before
	// any network call, and derives the card matrix from which are wired).
	ListTasks        TaskListOp
	CancelTask       TaskCancelOp
	ListContexts     ContextListOp
	GetContext       ContextGetOp
	DeleteContext    ContextDeleteOp
	DownloadArtifact ArtifactDownloadOp

	// Describe optionally supplies per-agent Card metadata (Name/Description/
	// Skills) and is the place to validate an unknown agent_id (return a typed
	// error). It is invoked ONLY when a runtime is available (configured), so
	// offline the card is always caps + registration metadata + the static
	// fields above. It is card enrichment, not an operation, so it stays a plain
	// func. A catalog spec typically leaves it nil and uses the static
	// Name/Description; an instance provider wires it to fetch its card remotely.
	Describe func(ctx context.Context, rt Runtime) (*CardInfo, error)
}
