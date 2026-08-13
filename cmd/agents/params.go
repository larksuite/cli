// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// This file is the per-verb business-parameter engine: --param k=v parsing,
// collect-all validation against one operation's declared set (every violation
// reported in one pass, each self-contained enough to fix without a discovery
// round-trip), default backfill, and the meta.next carry rule.

package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
)

// flatParams expands declarations to value-bearing leaves: scalars keep their
// name, an object contributes one leaf per Field under "obj.field" dotted
// names. The object entry itself is NOT value-bearing and is excluded. Order is
// declaration order (meta.next determinism).
func flatParams(declared []iagents.CardParam) []iagents.CardParam {
	out := make([]iagents.CardParam, 0, len(declared))
	for _, cp := range declared {
		if cp.Type == "object" {
			for _, f := range cp.Fields {
				leaf := f
				leaf.Name = cp.Name + "." + f.Name
				out = append(out, leaf)
			}
			continue
		}
		out = append(out, cp)
	}
	return out
}

// objectDecls indexes the top-level object params by name.
func objectDecls(declared []iagents.CardParam) map[string]iagents.CardParam {
	out := map[string]iagents.CardParam{}
	for _, cp := range declared {
		if cp.Type == "object" {
			out[cp.Name] = cp
		}
	}
	return out
}

// validatedParams is the engine's product: Resolved is what the runtime hands
// to the provider hook (defaults backfilled); Given is only what the caller
// explicitly provided (no defaults) — the meta.next carry rule reads Given so
// backfilled defaults never turn into command-line noise.
type validatedParams struct {
	Resolved map[string]string
	Given    map[string]string
}

// addParamFlag registers the shared --param flag on a leaf.
func addParamFlag(cmd *cobra.Command, params *[]string) {
	cmd.Flags().StringArrayVar(params, "param", nil, "business parameter key=value, repeatable (run lark-cli agents card <agent_ref> --operation <verb> to see what a verb accepts)")
}

// validateParams parses --param pairs and validates them against ONE
// operation's declared parameter set, collecting ALL violations into a single
// typed invalid_argument error (exit 2). spec is used for the cross-operation
// reverse lookup on unknown keys ("declared on: send") and may be nil (agents
// list path). Passing validation backfills declaration defaults into Resolved.
func validateParams(kvs []string, declared []iagents.CardParam, verb string, spec *iagents.AgentSpec, ref string) (validatedParams, error) {
	// decl indexes the value-bearing leaves: scalars by name, object fields by
	// dotted "obj.field" names — the canonical flat form every downstream
	// consumer (Resolved, meta.next, rt.Params()) speaks.
	leaves := flatParams(declared)
	decl := make(map[string]iagents.CardParam, len(leaves))
	for _, p := range leaves {
		decl[p.Name] = p
	}
	objects := objectDecls(declared)

	// seen records "this key appeared in argv" (both the duplicate check and the
	// missing-required suppression read it); given only collects values that
	// passed validation. The two must stay separate: a key whose VALUE failed is
	// still "provided", so reporting it as missing would contradict the more
	// precise violation already recorded. objChannel records which channel each
	// object used (dotted|json) so mixing the two errors instead of merging.
	seen := map[string]bool{}
	given := map[string]string{}
	objChannel := map[string]string{}
	var viols []errs.InvalidParam

	addViol := func(name, reason string, spec *iagents.CardParam, suggestions ...string) {
		v := errs.InvalidParam{Name: name, Reason: reason, Suggestions: suggestions}
		if spec != nil {
			v.Spec = *spec
		}
		viols = append(viols, v)
	}

	// ── parse + per-key checks (collect every violation) ──
	for _, kv := range kvs {
		k, val, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			addViol(kv, fmt.Sprintf("--param must be key=value, got %q", kv), nil)
			continue
		}
		if seen[k] {
			addViol(k, fmt.Sprintf("parameter %s given more than once (it is not repeatable)", k), nil)
			continue
		}
		seen[k] = true

		// Object via whole-value JSON: the key is exactly the object name.
		if obj, isObj := objects[k]; isObj {
			if objChannel[k] == "dotted" {
				addViol(k, fmt.Sprintf("parameter %s mixes the JSON and dotted-path forms (pick one per object)", k), nil)
				continue
			}
			objChannel[k] = "json"
			validateObjectJSON(k, val, obj, verb, seen, given, addViol)
			continue
		}

		// Dotted path: the key carries a ".", addressing one leaf of an object.
		if top, leaf, dotted := strings.Cut(k, "."); dotted {
			obj, isObj := objects[top]
			if !isObj {
				reason, sugg := unknownParamReason(k, verb, leaves, spec)
				addViol(k, reason, nil, sugg...)
				continue
			}
			if objChannel[top] == "json" {
				addViol(k, fmt.Sprintf("parameter %s mixes the JSON and dotted-path forms (pick one per object)", top), nil)
				continue
			}
			objChannel[top] = "dotted"
			cp, known := decl[k]
			if !known {
				addViol(k, fmt.Sprintf("unknown parameter %s (%s accepts: %s)", k, top, fieldNames(obj)), nil, dottedFieldNames(obj)...)
				continue
			}
			_ = leaf
			if val == "" {
				if cp.Required {
					addViol(k, fmt.Sprintf("required parameter %s must not be empty (%s requires it)", k, verb), &cp)
				}
				continue
			}
			if err := iagents.ValidateValue(cp, val); err != nil {
				addViol(k, fmt.Sprintf("parameter %s %s", k, err.Error()), &cp, cp.Enum...)
				continue
			}
			given[k] = canonicalValue(cp, val)
			continue
		}

		cp, known := decl[k]
		if !known {
			reason, sugg := unknownParamReason(k, verb, leaves, spec)
			addViol(k, reason, nil, sugg...)
			continue
		}
		if val == "" {
			// `k=` is treated as "not provided": it stays out of given so it
			// neither masks the Default backfill nor hands an unvalidated "" to
			// the hook. Required keys get their own violation; optional ones just
			// fall through to the default.
			if cp.Required {
				addViol(k, fmt.Sprintf("required parameter %s must not be empty (%s requires it)", k, verb), &cp)
			}
			continue
		}
		if err := iagents.ValidateValue(cp, val); err != nil {
			addViol(k, fmt.Sprintf("parameter %s %s", k, err.Error()), &cp, cp.Enum...)
			continue
		}
		given[k] = canonicalValue(cp, val)
	}

	// missing required, checked against the flat declaration. A key that showed
	// up in argv is skipped: it either passed or already carries a sharper
	// violation.
	for _, cp := range leaves {
		if !cp.Required || seen[cp.Name] {
			continue
		}
		c := cp
		addViol(cp.Name, fmt.Sprintf("missing required parameter %s (%s requires it)", cp.Name, verb), &c)
	}

	if len(viols) > 0 {
		return validatedParams{}, paramsError(viols, verb, ref)
	}

	// Default backfill, for wholly absent keys only.
	resolved := make(map[string]string, len(given))
	for k, v := range given {
		resolved[k] = v
	}
	for _, cp := range leaves {
		if cp.Default == "" {
			continue
		}
		if _, ok := resolved[cp.Name]; !ok {
			resolved[cp.Name] = cp.Default
		}
	}
	return validatedParams{Resolved: resolved, Given: given}, nil
}

// validateObjectJSON is the JSON fallback channel: parse the value as a JSON
// object, validate each member against the declared Fields with the SAME leaf
// rules as the dotted channel, and normalize accepted members into flat dotted
// keys — a provider never sees which channel the caller used. Numbers decode
// via json.Number so "100" stays "100" (no float re-rendering).
func validateObjectJSON(name, val string, obj iagents.CardParam, verb string, seen map[string]bool, given map[string]string, addViol func(string, string, *iagents.CardParam, ...string)) {
	if val == "" {
		return // `obj=` means not provided, same as a scalar
	}
	dec := json.NewDecoder(strings.NewReader(val))
	dec.UseNumber()
	var anyVal any
	if err := dec.Decode(&anyVal); err != nil {
		addViol(name, fmt.Sprintf("parameter %s is not valid JSON (%v); you can also pass fields one by one: --param %s.<field>=<value>", name, err, name), nil)
		return
	}
	raw, isObj := anyVal.(map[string]any)
	if !isObj {
		// Syntactically valid but not an object. Described in caller vocabulary
		// so no Go deserialization type names leak out.
		addViol(name, fmt.Sprintf(`parameter %s must be a JSON object (e.g. {"k":"v"}), got %s; you can also pass fields one by one: --param %s.<field>=<value>`, name, jsonKindName(anyVal), name), nil)
		return
	}
	fields := map[string]iagents.CardParam{}
	for _, f := range obj.Fields {
		fields[f.Name] = f
	}
	for fk, fv := range raw {
		full := name + "." + fk
		seen[full] = true
		cp, ok := fields[fk]
		if !ok {
			addViol(full, fmt.Sprintf("unknown parameter %s (%s accepts: %s)", full, name, fieldNames(obj)), nil, obj.FieldNamesList()...)
			continue
		}
		var sval string
		switch tv := fv.(type) {
		case string:
			sval = tv
		case json.Number:
			sval = tv.String()
		case bool:
			sval = strconv.FormatBool(tv)
		case nil:
			continue // null means not provided
		default:
			addViol(full, fmt.Sprintf("parameter %s does not accept a nested structure (object fields must be scalars)", full), &cp)
			continue
		}
		if sval == "" {
			if cp.Required {
				c := cp
				c.Name = fk
				addViol(full, fmt.Sprintf("required parameter %s must not be empty (%s requires it)", full, verb), &c)
			}
			continue
		}
		if err := iagents.ValidateValue(cp, sval); err != nil {
			c := cp
			addViol(full, fmt.Sprintf("parameter %s %s", full, err.Error()), &c, cp.Enum...)
			continue
		}
		given[full] = canonicalValue(cp, sval)
	}
}

// fieldNames renders an object's field list for teaching errors.
func fieldNames(obj iagents.CardParam) string {
	return strings.Join(obj.FieldNamesList(), ", ")
}

// unknownParamReason builds the teaching sentence for an undeclared key: if
// another operation of the same spec declares it, name those operations (fix by
// changing the verb); otherwise list this operation's own set (fix the spelling).
func unknownParamReason(key, verb string, declared []iagents.CardParam, spec *iagents.AgentSpec) (string, []string) {
	if spec != nil {
		var elsewhere []string
		for _, o := range spec.Ops() {
			if o.Verb == verb || !o.Wired {
				continue
			}
			for _, p := range flatParams(o.Params) {
				if p.Name == key {
					elsewhere = append(elsewhere, o.Verb)
					break
				}
			}
		}
		if len(elsewhere) > 0 {
			sort.Strings(elsewhere)
			// suggestions stays single-purpose (directly substitutable parameter
			// names), so verb names do not go in there — the "declared on: X"
			// teaching already lives in reason.
			return fmt.Sprintf("parameter %s does not apply to %s (declared on: %s)", key, verb, strings.Join(elsewhere, ", ")), nil
		}
	}
	known := make([]string, 0, len(declared))
	for _, p := range declared {
		known = append(known, p.Name)
	}
	if len(known) == 0 {
		return fmt.Sprintf("unknown parameter %s (%s accepts no business parameters)", key, verb), nil
	}
	// suggestions offers edit-distance near misses (a typo is one step from
	// fixed), falling back to the full declaration order when nothing is close.
	// The message always lists the full set.
	sugg := nearestNames(key, known, 2)
	if len(sugg) == 0 {
		sugg = known
	}
	return fmt.Sprintf("unknown parameter %s (%s accepts: %s)", key, verb, strings.Join(known, ", ")), sugg
}

// nearestNames returns the candidates within maxDist Levenshtein distance of
// key, nearest first (stable for ties by candidate order).
func nearestNames(key string, candidates []string, maxDist int) []string {
	type scored struct {
		name string
		d    int
	}
	var hits []scored
	for _, c := range candidates {
		if d := levenshtein(key, c); d <= maxDist {
			hits = append(hits, scored{c, d})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].d < hits[j].d })
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.name)
	}
	return out
}

// levenshtein is the classic two-row edit distance over runes.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(min(cur[j-1]+1, prev[j]+1), prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// jsonKindName names a decoded JSON value's kind in caller vocabulary.
func jsonKindName(v any) string {
	switch v.(type) {
	case []any:
		return "an array"
	case string:
		return "a string"
	case json.Number:
		return "a number"
	case bool:
		return "a boolean"
	case nil:
		return "null"
	default:
		return "a non-object value"
	}
}

// canonicalValue normalizes an ACCEPTED scalar to its canonical wire form so a
// provider receives one deterministic literal regardless of the input variant
// or channel: boolean TRUE/1/t → true|false, integer +5/04 → 5/4. The JSON
// channel already produces canonical literals for native types; this closes
// the dotted-path (and JSON string-member) variants to the same form. Values
// that reach here have passed ValidateValue, so parse errors are impossible;
// the input is returned unchanged as a defensive fallback.
func canonicalValue(cp iagents.CardParam, val string) string {
	switch cp.Type {
	case "boolean":
		if b, err := strconv.ParseBool(val); err == nil {
			return strconv.FormatBool(b)
		}
	case "integer":
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			return strconv.FormatInt(n, 10)
		}
	case "number":
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return strconv.FormatFloat(f, 'g', -1, 64)
		}
	}
	return val
}

// dottedFieldNames returns an object's field names in their full dotted form
// (directly substitutable --param keys).
func dottedFieldNames(obj iagents.CardParam) []string {
	out := make([]string, 0, len(obj.Fields))
	for _, f := range obj.Fields {
		out = append(out, obj.Name+"."+f.Name)
	}
	return out
}

// paramsError folds collected violations into one typed error: a single
// violation keeps its sentence as the message (continuity with the old
// one-error style); several get a count summary, with every violation carried
// structurally in params[].
func paramsError(viols []errs.InvalidParam, verb, ref string) error {
	msg := viols[0].Reason
	if len(viols) > 1 {
		msg = fmt.Sprintf("%s parameter validation failed: %d problems (see params)", verb, len(viols))
	}
	e := errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", msg).
		WithParam("param:" + viols[0].Name).
		WithParams(viols...)
	return e.WithHint("%s", opHint(ref, verb))
}

// validateListParams is the `agents list <scheme>` variant of validateParams:
// list is a provider-level operation with no agent_ref yet, so there is no
// spec for cross-operation reverse lookup, and the discovery hint points at
// the provider listing's list_parameters instead of an agent card.
func validateListParams(kvs []string, declared []iagents.CardParam, scheme string) (validatedParams, error) {
	vp, err := validateParams(kvs, declared, "list", nil, "")
	if err != nil {
		var verr *errs.ValidationError
		if errors.As(err, &verr) {
			verr.Hint = fmt.Sprintf("fix each entry in params and resend; the parameters agents list %s accepts are listed in providers[].list_parameters of lark-cli agents list", scheme)
		}
		return validatedParams{}, err
	}
	return vp, nil
}

// opHint is the operation-scoped discovery hint (the ref is interpolated only
// once it passes the whitelist).
func opHint(ref, verb string) string {
	if safeNextRef(ref) {
		return fmt.Sprintf("fix each entry in params and resend; or run lark-cli agents card %s --operation %s to see the parameter declarations", ref, verb)
	}
	return "fix each entry in params and resend; or use the --operation subquery of agents card to see the parameter declarations"
}

// paramArgsFor renders the meta.next carry for target verb V per the
// three-way rule, in declaration order:
//  1. given + value passes the whitelist → carry literally;
//  2. given + value fails the whitelist → required degrades to a placeholder
//     (template), optional is dropped (better absent than ambiguous);
//  3. absent but required on V → placeholder (template) — the cross-verb hole:
//     without this, "a required parameter never falls off the chain" would only
//     hold when the previous verb happened to share the parameter.
//
// Defaults are NOT carried (the next hop deterministically re-backfills).
func paramArgsFor(spec *iagents.AgentSpec, verb string, given map[string]string) (args string, templated bool) {
	if spec == nil {
		return "", false
	}
	op, ok := spec.Op(verb)
	if !ok {
		return "", false
	}
	var b strings.Builder
	for _, p := range flatParams(op.Params) {
		v, has := given[p.Name]
		switch {
		case p.NoCarry:
			// Parameters that want a fresh value per call are never carried
			// literally; a required one degrades to a placeholder so the caller
			// fills in something new instead of reusing the last value.
			if p.Required {
				fmt.Fprintf(&b, " --param %s=<%s>", p.Name, p.Name)
				templated = true
			}
		case has && v != "" && safeNextID(v):
			fmt.Fprintf(&b, " --param %s=%s", p.Name, v)
		case p.Required:
			fmt.Fprintf(&b, " --param %s=<%s>", p.Name, p.Name)
			templated = true
		}
	}
	return b.String(), templated
}
