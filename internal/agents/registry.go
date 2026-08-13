// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

// ProviderKind is the closed set of provider forms, derived from whether a
// Provider set Catalog or Instance (exposed via Provider.Kind()).
type ProviderKind string

const (
	// KindCatalog: the full agent set is known offline (Provider.Catalog).
	KindCatalog ProviderKind = "catalog"
	// KindInstance: agents are created on the platform at runtime, addressed by an
	// unbounded agent_id (Provider.Instance).
	KindInstance ProviderKind = "instance"
)

var providerRegistry = map[string]Provider{}

// Register records a provider (called from the agent/register.go aggregator,
// mirroring events/shortcuts). It is pure struct validation — no construction,
// no probe. Missing / invalid metadata is an integrator coding error and panics
// fail-fast (aligned with the sql.Register convention, including duplicate
// registration).
func Register(p Provider) {
	switch {
	case p.Scheme == "":
		panic("agent: provider registration with empty Scheme")
	case p.Label == "":
		panic("agent: provider missing Label: " + p.Scheme)
	case p.AgentIDSource == "":
		panic("agent: provider missing AgentIDSource: " + p.Scheme)
	case len(p.Identities) == 0:
		panic("agent: provider missing Identities: " + p.Scheme)
	}
	if _, dup := providerRegistry[p.Scheme]; dup {
		panic("agent: Register called twice for scheme: " + p.Scheme)
	}
	seenIdentity := make(map[IdentityType]bool, len(p.Identities))
	for _, id := range p.Identities {
		if id.Type != IdentityUser && id.Type != IdentityBot {
			panic("agent: provider invalid Identity Type (want user|bot): " + p.Scheme + ", got: " + string(id.Type))
		}
		// A duplicate identity entry would make the per-identity scope
		// declaration ambiguous (which entry's Scopes wins?), so it is rejected
		// at the source.
		if seenIdentity[id.Type] {
			panic("agent: provider duplicate Identity Type: " + p.Scheme + ", got: " + string(id.Type))
		}
		seenIdentity[id.Type] = true
		seenScope := make(map[string]bool, len(id.Scopes))
		for _, s := range id.Scopes {
			if s == "" {
				panic("agent: provider empty scope in Identity " + string(id.Type) + " Scopes: " + p.Scheme)
			}
			if seenScope[s] {
				panic("agent: provider duplicate scope in Identity " + string(id.Type) + " Scopes: " + p.Scheme + ", got: " + s)
			}
			seenScope[s] = true
		}
	}
	hasCatalog, hasInstance := len(p.Catalog) > 0, p.Instance != nil
	if hasCatalog == hasInstance {
		panic("agent: provider must set exactly one of Catalog / Instance: " + p.Scheme)
	}
	if hasCatalog {
		seen := make(map[string]bool, len(p.Catalog))
		for i := range p.Catalog {
			checkSpec(p.Scheme, &p.Catalog[i], true)
			if seen[p.Catalog[i].ID] {
				panic("agent: catalog duplicate entry ID for scheme " + p.Scheme + ": " + p.Catalog[i].ID)
			}
			seen[p.Catalog[i].ID] = true
		}
	} else {
		checkSpec(p.Scheme, p.Instance, false)
	}
	// ListParams declares the parameters of `agents list <scheme>` — meaningless
	// without an online enumeration hook to consume them.
	if len(p.ListParams) > 0 && p.ListAgents == nil {
		panic("agent: provider declares ListParams without a ListAgents hook: " + p.Scheme)
	}
	checkParams(p.Scheme+": agents list", p.ListParams)
	for i := range p.ListParams {
		if p.ListParams[i].Type == "" {
			p.ListParams[i].Type = "string"
		}
	}
	providerRegistry[p.Scheme] = p
}

// checkSpec asserts the mandatory core operations, the ID rule, and every
// operation's parameter declarations for one spec. The command layer
// dispatches Send/GetTask without a nil-check, so they must be wired.
func checkSpec(scheme string, s *AgentSpec, catalog bool) {
	if !s.Send.wired() {
		panic("agent: spec missing core Send handler: " + scheme + ":" + s.ID)
	}
	if !s.GetTask.wired() {
		panic("agent: spec missing core GetTask handler: " + scheme + ":" + s.ID)
	}
	// An agent that can pause on a question group MUST also let the user walk
	// away from it: with no TTL in the contract, question-asking without
	// task_cancel leaves an abandoned group holding awaiting_input forever
	// a registration-time coding error, not a runtime one.
	// The check includes brand coverage: a CancelTask scoped narrower than the
	// agent's own visibility recreates the dead end on the uncovered brand.
	if s.InputRequired {
		if !s.CancelTask.wired() {
			panic("agent: spec declares InputRequired but wires no CancelTask: " + scheme + ":" + s.ID)
		}
		if len(s.CancelTask.Brands) > 0 && !brandsCover(s.CancelTask.Brands, s.Brands) {
			panic("agent: spec declares InputRequired but CancelTask is brand-scoped narrower than the agent: " + scheme + ":" + s.ID)
		}
	}
	if catalog && s.ID == "" {
		panic("agent: catalog spec missing ID: " + scheme)
	}
	if !catalog && s.ID != "" {
		panic("agent: instance template must have empty ID: " + scheme + ", got: " + s.ID)
	}
	// Whole-agent brand scope: every declared value must be a known brand
	// (mirrors the identity Type fail-fast). Empty ⇒ all brands.
	for _, b := range s.Brands {
		if !validBrand(b) {
			panic("agent: spec invalid Brand (want feishu|lark): " + scheme + ":" + s.ID + ", got: " + string(b))
		}
	}
	for _, o := range s.Ops() {
		where := scheme + ":" + s.ID + " " + o.Verb
		// Params physically live on the Op, so the only declared-without-handler
		// mistake left is a non-empty Params next to a nil Handler.
		if len(o.Params) > 0 && !o.Wired {
			panic("agent: params declared on an unwired operation: " + where)
		}
		// A brand scope on an unimplemented op is a dead declaration — mirror the
		// params discipline above.
		if len(o.Brands) > 0 && !o.Wired {
			panic("agent: brands declared on an unwired operation: " + where)
		}
		// Per-capability brand scope: same known-brand rule as the whole-agent set.
		for _, b := range o.Brands {
			if !validBrand(b) {
				panic("agent: op invalid Brand (want feishu|lark): " + where + ", got: " + string(b))
			}
		}
		checkParams(where, o.Params)
	}
	normalizeSpecParams(s)
}

// validBrand reports whether b is one of the two known brands (feishu|lark) —
// the Register-time fail-fast guard for spec.Brands and each Op.Brands.
func validBrand(b core.LarkBrand) bool {
	return b == core.BrandFeishu || b == core.BrandLark
}

// brandsCover reports whether opBrands covers every brand the agent itself is
// visible under (specBrands empty = all known brands). Used by the
// InputRequired⇒CancelTask registration check.
func brandsCover(opBrands, specBrands []core.LarkBrand) bool {
	agentBrands := specBrands
	if len(agentBrands) == 0 {
		agentBrands = []core.LarkBrand{core.BrandFeishu, core.BrandLark}
	}
	for _, b := range agentBrands {
		if !OpAvailableForBrand(opBrands, b) {
			return false
		}
	}
	return true
}

// SpecAvailableForBrand reports whether the WHOLE agent is visible/usable under
// `brand`: an empty spec.Brands means every brand, otherwise `brand` must be
// listed. It backs catalog list filtering and the command layer's whole-agent
// brand gate. (Op-level scoping is OpAvailableForBrand.)
func SpecAvailableForBrand(s *AgentSpec, brand core.LarkBrand) bool {
	return OpAvailableForBrand(s.Brands, brand)
}

// paramNameRe is the parameter-name charset: a strict subset of the meta.next
// interpolation whitelist ([A-Za-z0-9_-]), so a declared name is safe to
// splice into a suggested command by construction, and snake→kebab mapping
// stays bijective for a future native-flag projection.
var paramNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// paramTypes is the closed Type vocabulary (empty normalizes to "string").
// "object" is declaration-only: it carries Fields and no value constraints of
// its own.
var paramTypes = map[string]bool{"string": true, "integer": true, "number": true, "boolean": true, "object": true}

// checkParams fail-fast validates one operation's parameter declarations
// (where names the operation for the panic message). Object params recurse one
// level into their Fields (scalar leaves only).
func checkParams(where string, params []CardParam) {
	checkParamsLevel(where, params, true)
}

func checkParamsLevel(where string, params []CardParam, allowObject bool) {
	seen := make(map[string]bool, len(params))
	for _, cp := range params {
		at := where + " param " + cp.Name
		if !paramNameRe.MatchString(cp.Name) {
			panic("agent: param name must match ^[a-z][a-z0-9_]{0,63}$: " + at)
		}
		if seen[cp.Name] {
			panic("agent: duplicate param name within one operation: " + at)
		}
		seen[cp.Name] = true

		typ := cp.Type
		if typ == "" {
			typ = "string"
		}
		if typ == "object" {
			if !allowObject {
				panic("agent: nested object fields are not supported (flatten or wait for the schema slot): " + at)
			}
			if len(cp.Fields) == 0 {
				panic("agent: object param must declare non-empty Fields: " + at)
			}
			// An object declares nothing but its Fields: requiredness/constraints
			// live on the leaves, so a stray setting here is a coding error.
			if cp.Required || len(cp.Enum) > 0 || cp.Default != "" || cp.Min != nil || cp.Max != nil {
				panic("agent: object param must not set Required/Enum/Default/Min/Max (declare them on leaves): " + at)
			}
			checkParamsLevel(at, cp.Fields, false)
			continue
		}
		if len(cp.Fields) > 0 {
			panic("agent: Fields is only valid on Type \"object\": " + at)
		}
		if !paramTypes[typ] {
			panic("agent: param Type must be one of string|integer|number|boolean: " + at + ", got: " + cp.Type)
		}
		if len(cp.Enum) > 0 {
			if typ != "string" && typ != "integer" {
				panic("agent: Enum is only valid on string|integer params: " + at)
			}
			if cp.Min != nil || cp.Max != nil {
				panic("agent: Enum and Min/Max are mutually exclusive: " + at)
			}
			ev := make(map[string]bool, len(cp.Enum))
			for _, e := range cp.Enum {
				if e == "" {
					panic("agent: Enum member must be non-empty: " + at)
				}
				if ev[e] {
					panic("agent: duplicate Enum member: " + at + ", member: " + e)
				}
				ev[e] = true
				if typ == "integer" {
					if _, err := strconv.ParseInt(e, 10, 64); err != nil {
						panic("agent: integer Enum member must parse as integer: " + at + ", member: " + e)
					}
				}
			}
		}
		if cp.Min != nil || cp.Max != nil {
			if typ != "integer" && typ != "number" {
				panic("agent: Min/Max are only valid on integer|number params: " + at)
			}
			if cp.Min != nil && cp.Max != nil && *cp.Min > *cp.Max {
				panic("agent: Min must be <= Max: " + at)
			}
		}
		if cp.Default != "" {
			if cp.Required {
				panic("agent: Default and Required are mutually exclusive: " + at)
			}
			if err := ValidateValue(cp, cp.Default); err != nil {
				panic("agent: Default violates the param's own declaration: " + at + ": " + err.Error())
			}
		}
	}
}

// ValidateValue validates one value against a declaration's Type/Enum/Min/Max
// (shared by Register's Default check and the command layer's per-call
// validation).
func ValidateValue(cp CardParam, val string) error {
	typ := cp.Type
	if typ == "" {
		typ = "string"
	}
	switch typ {
	case "integer":
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			// Overflowing int64 is a RANGE problem, not a TYPE problem: the
			// message must match the fact, or the caller edits the type
			// instead of the value.
			if errors.Is(err, strconv.ErrRange) {
				if cp.Min != nil || cp.Max != nil {
					return fmt.Errorf("must be within %s, got %s", rangeText(cp), val)
				}
				return fmt.Errorf("exceeds the representable integer range (int64), got %q", val)
			}
			return fmt.Errorf("must be an integer, got %q", val)
		}
		if cp.Min != nil && float64(n) < *cp.Min {
			return fmt.Errorf("must be within %s, got %s", rangeText(cp), val)
		}
		if cp.Max != nil && float64(n) > *cp.Max {
			return fmt.Errorf("must be within %s, got %s", rangeText(cp), val)
		}
	case "number":
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return fmt.Errorf("must be a number, got %q", val)
		}
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return fmt.Errorf("must be a finite number, got %q", val)
		}
		if cp.Min != nil && f < *cp.Min {
			return fmt.Errorf("must be within %s, got %s", rangeText(cp), val)
		}
		if cp.Max != nil && f > *cp.Max {
			return fmt.Errorf("must be within %s, got %s", rangeText(cp), val)
		}
	case "boolean":
		if _, err := strconv.ParseBool(val); err != nil {
			return fmt.Errorf("must be a boolean, got %q", val)
		}
	}
	if len(cp.Enum) > 0 {
		for _, e := range cp.Enum {
			if val == e {
				return nil
			}
		}
		return fmt.Errorf("must be one of %s, got %q", strings.Join(cp.Enum, "|"), val)
	}
	return nil
}

// rangeText renders a Min/Max declaration for error messages ("1..100",
// ">=1", "<=100").
func rangeText(cp CardParam) string {
	switch {
	case cp.Min != nil && cp.Max != nil:
		return trimFloat(*cp.Min) + ".." + trimFloat(*cp.Max)
	case cp.Min != nil:
		return ">=" + trimFloat(*cp.Min)
	default:
		return "<=" + trimFloat(*cp.Max)
	}
}

func trimFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// normalizeSpecParams normalizes declarations in place after validation
// (currently: empty Type ⇒ "string"), so every downstream consumer reads a
// canonical form.
func normalizeSpecParams(s *AgentSpec) {
	var normalize func(ps []CardParam)
	normalize = func(ps []CardParam) {
		for i := range ps {
			if ps[i].Type == "" {
				ps[i].Type = "string"
			}
			normalize(ps[i].Fields)
		}
	}
	normalize(s.Send.Params)
	normalize(s.GetTask.Params)
	normalize(s.ListTasks.Params)
	normalize(s.CancelTask.Params)
	normalize(s.ListContexts.Params)
	normalize(s.GetContext.Params)
	normalize(s.DeleteContext.Params)
	normalize(s.DownloadArtifact.Params)
}

// Info returns the registered provider for a scheme (ok=false if not registered).
func Info(scheme string) (Provider, bool) {
	p, ok := providerRegistry[scheme]
	return p, ok
}

// LookupSpec resolves the AgentSpec addressed by ref, fully offline: it parses
// the ref, finds the provider, and returns the matching spec (the instance
// template, or the catalog entry whose ID matches) plus the parsed agent_id (so
// callers need not re-parse for rt.AgentID() / the card). An unknown scheme or
// unknown catalog id returns a typed error (the command layer promotes
// ParseRef/scheme errors via wrapRefResolveError; the unknown-id error is
// already typed).
func LookupSpec(ref string) (Provider, *AgentSpec, string, error) {
	r, err := ParseRef(ref)
	if err != nil {
		return Provider{}, nil, "", err
	}
	p, ok := providerRegistry[r.Scheme]
	if !ok {
		return Provider{}, nil, "", fmt.Errorf("unknown agent provider '%s', currently registered: %s", r.Scheme, KnownSchemes())
	}
	if p.Instance != nil {
		return p, p.Instance, r.AgentID, nil
	}
	for i := range p.Catalog {
		if p.Catalog[i].ID == r.AgentID {
			return p, &p.Catalog[i], r.AgentID, nil
		}
	}
	return p, nil, "", errs.NewValidationError(errs.SubtypeInvalidArgument,
		"unknown %s agent '%s'", r.Scheme, r.AgentID).
		WithHint("run lark-cli agents list %s to see the available agents", r.Scheme)
}

// Kind reports the provider form derived from Catalog vs Instance.
func (p Provider) Kind() ProviderKind {
	if p.Instance != nil {
		return KindInstance
	}
	return KindCatalog
}

// AgentRefFormat is the written form of an agent_ref for this provider, always
// "<scheme>:<agent_id>" (derived, not stored).
func (p Provider) AgentRefFormat() string {
	return p.Scheme + ":<agent_id>"
}

// ListCatalog is the offline enumeration for a catalog provider (sorted by
// AgentRef, stable), filtered to the agents visible under `brand`
// (SpecAvailableForBrand). An instance provider has no static set and returns
// nil — the command layer then falls back to the optional ListAgents online hook.
func (p Provider) ListCatalog(brand core.LarkBrand) []AgentSummary {
	if p.Instance != nil {
		return nil
	}
	out := make([]AgentSummary, 0, len(p.Catalog))
	for _, s := range p.Catalog {
		if !SpecAvailableForBrand(&s, brand) {
			continue
		}
		out = append(out, AgentSummary{
			AgentRef:    p.Scheme + ":" + s.ID,
			Name:        s.Name,
			Description: s.Description,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AgentRef < out[j].AgentRef })
	return out
}

// KnownSchemes returns a comma-separated list of registered schemes (stably
// sorted), or "(none)" when empty (reused by cmd/agent's unknown-scheme message).
func KnownSchemes() string {
	s := RegisteredSchemes()
	if len(s) == 0 {
		return "(none)"
	}
	return strings.Join(s, ", ")
}

// RegisteredSchemes lets `agents list` enumerate registered providers (sorted).
func RegisteredSchemes() []string {
	s := make([]string, 0, len(providerRegistry))
	for k := range providerRegistry {
		s = append(s, k)
	}
	sort.Strings(s)
	return s
}
