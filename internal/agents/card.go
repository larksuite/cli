// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"

	"github.com/larksuite/cli/internal/core"
)

// capability key constants (the JSON key names in capabilities, also the
// capability identifiers used by Supports / capabilityError). Only capabilities
// that "can change the AI's next command line and are currently deliverable" are
// exposed.
const (
	CapTaskGet          = "task_get"
	CapTaskList         = "task_list"
	CapTaskCancel       = "task_cancel"
	CapInputRequired    = "input_required"
	CapFileInput        = "file_input"
	CapArtifactDownload = "artifact_download"
	// The three multi-turn (context) verbs are independently wired, so each has
	// its own capability bit — a provider may support listing sessions without
	// supporting get or delete. (There is no umbrella "multi_turn" bit: a single
	// flag cannot honestly represent three separately-deliverable hooks.)
	CapContextList   = "context_list"
	CapContextGet    = "context_get"
	CapContextDelete = "context_delete"
)

// Capabilities is the closed set of capabilities: making it a struct means an
// omitted field is an explicit false and a typo is a compile error. Fields are
// ordered by json tag alphabetically so the emitted key order is stable.
type Capabilities struct {
	ArtifactDownload bool `json:"artifact_download"`
	ContextDelete    bool `json:"context_delete"`
	ContextGet       bool `json:"context_get"`
	ContextList      bool `json:"context_list"`
	FileInput        bool `json:"file_input"`
	InputRequired    bool `json:"input_required"`
	TaskCancel       bool `json:"task_cancel"`
	TaskGet          bool `json:"task_get"`
	TaskList         bool `json:"task_list"`
}

// AgentCard is a remote agent's capability card (schema v3, lean): provider
// metadata, the supported capability matrix, identity precondition
// declarations, and the has_parameters cue. Parameter DETAILS are deliberately
// not embedded — they are fetched per operation via `agents card <ref>
// --operation <verb>` (or all at once with --operation all); HasParameters
// tells the caller which operations need that lookup. Scopes are not in the
// card; they are internal registration data for preflight only.
type AgentCard struct {
	Provider      string `json:"provider"`
	ProviderLabel string `json:"provider_label"`
	AgentID       string `json:"agent_id"`
	// Brand the card was rendered for: the capability matrix is brand-scoped, so
	// the same agent can honestly show different capabilities under feishu vs
	// lark. Callers must read the card for the CURRENT brand, not assume it is
	// cross-brand stable.
	Brand        string         `json:"brand"`
	Name         string         `json:"name,omitempty"` // dynamic card only
	Description  string         `json:"description,omitempty"`
	Capabilities Capabilities   `json:"capabilities"`
	Identity     []IdentitySpec `json:"identity"`
	// HasParameters lists the verbs that declare business parameters (always
	// emitted; empty is []). A verb absent here takes no --param at all.
	HasParameters []string `json:"has_parameters"`
	// ParametersSource is "template" for an instance provider: the parameter
	// declarations are template-level approximations shared by every runtime
	// agent_id — the platform's actual per-agent contract may differ. Empty for
	// catalog providers (declarations are exact).
	ParametersSource string      `json:"parameters_source,omitempty"`
	AgentIDSource    string      `json:"agent_id_source"`
	Skills           []CardSkill `json:"skills,omitempty"`
}

// DeriveCapabilities computes the capability matrix from which AgentSpec
// operations are wired AND available for brand — the single source of truth
// ("implement it = support it"). An op-backed capability is true iff its hook is
// wired and the op is not brand-excluded, so the same agent may show different
// capabilities under feishu vs lark. file_input / input_required are behavioral
// flags with no backing operation and stay brand-independent.
func DeriveCapabilities(s *AgentSpec, brand core.LarkBrand) Capabilities {
	w := make(map[string]bool, 8)
	for _, o := range s.Ops() {
		w[o.Verb] = o.Wired && OpAvailableForBrand(o.Brands, brand)
	}
	return Capabilities{
		TaskGet:          w[VerbTaskGet],
		TaskList:         w[VerbTaskList],
		TaskCancel:       w[VerbTaskCancel],
		ArtifactDownload: w[VerbArtifactDownload],
		ContextList:      w[VerbContextList],
		ContextGet:       w[VerbContextGet],
		ContextDelete:    w[VerbContextDelete],
		FileInput:        s.FileInput,
		InputRequired:    s.InputRequired,
	}
}

// HasParameters lists the wired verbs that declare at least one business
// parameter (fixed verb order, never nil) — the card's "which operations need
// a parameter lookup" cue.
func HasParameters(s *AgentSpec) []string {
	out := []string{}
	for _, o := range s.Ops() {
		if o.Wired && len(o.Params) > 0 {
			out = append(out, o.Verb)
		}
	}
	return out
}

// BuildCard synthesizes an agent's lean Card: Provider registration metadata, the
// brand-scoped capability matrix, the has_parameters cue, and the spec's static
// metadata. When rt != nil and the spec wires Describe it best-effort enriches
// Name/Description/Skills from the remote, swallowing a Describe error so the card
// degrades to the offline version rather than hard-failing. Pass rt=nil for the
// guaranteed-offline path. A provider never assembles its own card.
func BuildCard(ctx context.Context, p Provider, s *AgentSpec, agentID string, brand core.LarkBrand, rt Runtime) *AgentCard {
	card := &AgentCard{
		Provider:      p.Scheme,
		ProviderLabel: p.Label,
		AgentID:       agentID,
		Brand:         string(brand),
		Name:          s.Name,
		Description:   s.Description,
		Capabilities:  DeriveCapabilities(s, brand),
		Identity:      p.Identities,
		HasParameters: HasParameters(s),
		AgentIDSource: p.AgentIDSource,
		Skills:        s.Skills,
	}
	if p.Kind() == KindInstance {
		// Honesty label: an instance template's parameter declarations are shared
		// by every runtime agent_id — approximate, not per-agent exact.
		card.ParametersSource = "template"
	}
	if rt != nil && s.Describe != nil {
		if info, err := s.Describe(ctx, rt); err == nil && info != nil {
			if info.Name != "" {
				card.Name = info.Name
			}
			if info.Description != "" {
				card.Description = info.Description
			}
			if info.Skills != nil {
				card.Skills = info.Skills
			}
		}
	}
	return card
}

// CardParam declares one business parameter of one operation (used for --param
// validation, `agents card --operation` discovery, and error teaching).
type CardParam struct {
	// Name must match ^[a-z][a-z0-9_]{0,63}$ (Register panics otherwise). The
	// charset is a subset of the meta.next interpolation whitelist, so the key
	// side of a carried `--param k=v` is safe by construction, and snake→kebab
	// mapping stays bijective should native flags ever be generated.
	Name string `json:"name"`
	// Type is one of string|integer|number|boolean; empty is normalized to
	// "string" at Register time. It participates in real validation.
	Type     string `json:"type"`
	Required bool   `json:"required"` // required on THIS operation; empty value (`k=`) does not count as provided
	Desc     string `json:"desc,omitempty"`
	// Enum restricts the value to a closed set (string and integer types only;
	// for integer every member must parse). Mutually exclusive with Min/Max.
	Enum []string `json:"enum,omitempty"`
	// Default is backfilled into rt.Params() when the parameter is absent.
	// Mutually exclusive with Required; must satisfy Type/Enum/Min/Max.
	Default string `json:"default,omitempty"`
	// Min/Max bound numeric types (closed interval, either side optional).
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`

	// Fields declares an object parameter's members (Type MUST be "object", and
	// an object declares nothing else: no Required/Enum/Default/Min/Max on the
	// object itself — requiredness, defaults and constraints all live on the
	// scalar leaves). Leaves are ordinary CardParams (scalars only — no nested
	// objects this round; a shape that needs deeper nesting should flatten or
	// wait for the schema evolution slot). On the wire an object travels either
	// as dotted-path leaves (--param filter.region=east, the primary channel)
	// or as one JSON value (--param filter='{"region":"east"}', the fallback);
	// both normalize to flat dotted keys in rt.Params(), so a provider never
	// sees which channel the caller used.
	Fields []CardParam `json:"fields,omitempty"`

	// NoCarry opts this parameter out of the meta.next carry: a suggested next
	// command never carries its given value literally (a required NoCarry param
	// degrades to a placeholder so the caller supplies a FRESH value). Declare
	// it on per-call parameters (trace tags, one-shot tokens) that are shared
	// across operations but must not ride the chain — the carry rule's
	// same-resource continuity assumption does not hold for them.
	NoCarry bool `json:"no_carry,omitempty"`
	// NOTE(reserved): Repeated bool — multi-value parameters (same key given
	// several times, aggregated in argv order). Not implemented this round; the
	// duplicate-key error wording is already scoped per-parameter so activating
	// it later cannot contradict published error semantics.
}

// CardSkill is one skill / scenario declared by a Card (with example usages).
type CardSkill struct {
	ID       string   `json:"id"`
	Name     string   `json:"name,omitempty"`
	Examples []string `json:"examples,omitempty"`
}

// FieldNamesList returns an object param's field names in declaration order
// (teaching errors and suggestions).
func (p CardParam) FieldNamesList() []string {
	out := make([]string, 0, len(p.Fields))
	for _, f := range p.Fields {
		out = append(out, f.Name)
	}
	return out
}

// Supports reports whether a capability is declared as supported (an unknown key
// or a nil card is treated as unsupported).
func (c *AgentCard) Supports(capKey string) bool {
	if c == nil {
		return false
	}
	switch capKey {
	case CapArtifactDownload:
		return c.Capabilities.ArtifactDownload
	case CapFileInput:
		return c.Capabilities.FileInput
	case CapInputRequired:
		return c.Capabilities.InputRequired
	case CapContextList:
		return c.Capabilities.ContextList
	case CapContextGet:
		return c.Capabilities.ContextGet
	case CapContextDelete:
		return c.Capabilities.ContextDelete
	case CapTaskCancel:
		return c.Capabilities.TaskCancel
	case CapTaskGet:
		return c.Capabilities.TaskGet
	case CapTaskList:
		return c.Capabilities.TaskList
	default:
		return false
	}
}
