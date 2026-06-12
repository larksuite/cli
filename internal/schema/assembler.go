// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"encoding/json"
	"sort"
	"strconv"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/registry"
)

// coerceLiteral converts a meta_data literal (default / enum / example) to
// the JSON Schema type declared by the field (integer/number/boolean/string).
// meta_data stores every literal as a string, so without coercion an
// `integer` field would emit string literals and fail any standard validator.
// Already-typed values pass through unchanged. Returns (value, true) on
// success, or (nil, false) when the literal cannot be coerced (caller should
// drop it).
func coerceLiteral(fieldType string, raw interface{}) (interface{}, bool) {
	s, isStr := raw.(string)
	if !isStr {
		// Already typed (e.g. meta_data emitted a JSON number/bool directly).
		return raw, true
	}
	switch fieldType {
	case "integer":
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			return v, true
		}
		return nil, false
	case "number":
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			return v, true
		}
		return nil, false
	case "boolean":
		switch s {
		case "true":
			return true, true
		case "false":
			return false, true
		}
		return nil, false
	default: // "string", "" (nested objects), or unknown
		return s, true
	}
}

// sortEnum sorts an enum slice in-place using a comparator appropriate for
// the declared JSON Schema type, so integer enums end up [1, 2, 10] rather
// than the lexicographic [1, 10, 2].
func sortEnum(fieldType string, vals []interface{}) {
	sort.SliceStable(vals, func(i, j int) bool {
		switch fieldType {
		case "integer":
			ai, _ := vals[i].(int64)
			bi, _ := vals[j].(int64)
			return ai < bi
		case "number":
			af, _ := vals[i].(float64)
			bf, _ := vals[j].(float64)
			return af < bf
		case "boolean":
			ab, _ := vals[i].(bool)
			bb, _ := vals[j].(bool)
			return !ab && bb // false < true
		default:
			as, _ := vals[i].(string)
			bs, _ := vals[j].(string)
			return as < bs
		}
	})
}

// convertProperty recursively converts one meta_data field map into a Property.
// nestedPath is the dotted lookup key into the current method's NestedKeys map
// (e.g. "responseBody.items.properties"). Empty path = top-level, no nested
// lookup needed.
func convertProperty(field map[string]interface{}, nestedPath string) Property {
	var p Property

	rawType, _ := field["type"].(string)
	switch rawType {
	case "file":
		p.Type = "string"
		p.Format = "binary"
	case "list":
		// meta_data uses non-standard "list" on a couple of fields;
		// translate to JSON Schema "array" so validators accept it.
		p.Type = "array"
	default:
		p.Type = rawType
	}

	if s, ok := field["description"].(string); ok {
		p.Description = s
	}
	if v, ok := field["default"]; ok {
		// Coerce default literal to match the declared JSON Schema type so
		// validators do not reject e.g. {type:"integer", default:"500"}.
		// When coercion fails (e.g. default:"" on an integer field, which
		// meta_data uses to mean "no default"), omit the field entirely
		// instead of emitting a type-mismatched default — the result is a
		// missing `default` key rather than a contract violation.
		if coerced, ok := coerceLiteral(p.Type, v); ok {
			p.Default = coerced
		}
	}
	if v, ok := field["example"]; ok {
		// meta_data stores examples as strings even when the field is integer/
		// boolean/number; coerce to the declared type so downstream validators
		// accept the envelope. Drop on coerce failure (same policy as default).
		if coerced, ok := coerceLiteral(p.Type, v); ok {
			p.Example = coerced
		}
	}

	// min / max are stored as strings in meta_data; parse on best-effort.
	if minStr, ok := field["min"].(string); ok && minStr != "" {
		if v, err := strconv.ParseFloat(minStr, 64); err == nil {
			p.Minimum = &v
		}
	}
	if maxStr, ok := field["max"].(string); ok && maxStr != "" {
		if v, err := strconv.ParseFloat(maxStr, 64); err == nil {
			p.Maximum = &v
		}
	}

	// enum: prefer existing "enum" array; else extract from options[].value.
	// Values are typed per p.Type so integer fields get integer enums, etc.
	// (JSON Schema 2020-12 requires enum value types to match the declared
	// type — meta_data stores everything as strings.)
	if enumRaw, ok := field["enum"].([]interface{}); ok && len(enumRaw) > 0 {
		for _, e := range enumRaw {
			if v, ok := coerceLiteral(p.Type, e); ok {
				p.Enum = append(p.Enum, v)
			}
		}
		// Numeric/boolean enums get sorted (no inherent meaning in meta_data
		// order); string enums keep meta_data order, which sometimes carries
		// semantic priority (e.g. image_type ["message","avatar"]).
		if p.Type != "string" && p.Type != "" {
			sortEnum(p.Type, p.Enum)
		}
	} else if optsRaw, ok := field["options"].([]interface{}); ok && len(optsRaw) > 0 {
		seen := make(map[string]bool)
		for _, o := range optsRaw {
			om, ok := o.(map[string]interface{})
			if !ok {
				continue
			}
			raw, ok := om["value"].(string)
			if !ok || seen[raw] {
				continue
			}
			seen[raw] = true
			if v, ok := coerceLiteral(p.Type, raw); ok {
				p.Enum = append(p.Enum, v)
			}
		}
		// Same policy as the `enum` branch: numeric/boolean enums get sorted
		// (no semantic meaning in source order); string enums keep meta_data
		// order, which may carry semantic priority.
		if p.Type != "string" && p.Type != "" {
			sortEnum(p.Type, p.Enum)
		}
	}

	// nested properties: recurse
	if propsRaw, ok := field["properties"].(map[string]interface{}); ok && len(propsRaw) > 0 {
		nested, nestedRequired := buildOrderedProps(propsRaw, nestedPath)
		if p.Type == "array" {
			// meta_data quirk: array element schema is wrapped in "properties".
			// Unfold into Items: { type: "object", properties: <nested> }
			p.Items = &Property{
				Type:       "object",
				Properties: nested,
				Required:   nestedRequired,
			}
			// Property.Properties stays nil for arrays
		} else {
			if p.Type == "" {
				p.Type = "object" // infer
			}
			p.Properties = nested
			p.Required = nestedRequired
		}
	}

	// array items fallback: emit `items: {}` (any schema) for every array that
	// meta_data does not describe an element shape for — whether it arrived as
	// "list" or natively as "array". Without this, typeless arrays (e.g. arrays
	// of bare ID strings) violate the L1 lint rule and are not JSON Schema valid
	// for consumers that require `items`.
	if p.Type == "array" && p.Items == nil {
		p.Items = &Property{}
	}

	return p
}

// buildOrderedProps converts a map[string]interface{} of field specs into an
// OrderedProps plus the alphabetized list of child keys marked `required:true`
// in meta_data. Callers attach that list to the enclosing object's `required`,
// so nested objects faithfully report their call contract (top-level required
// is handled separately by buildInputSchema).
func buildOrderedProps(raw map[string]interface{}, nestedPath string) (*OrderedProps, []string) {
	op := &OrderedProps{Map: make(map[string]Property, len(raw))}

	var required []string
	keys := orderedKeys(raw, nestedPath)
	for _, k := range keys {
		fieldRaw, _ := raw[k].(map[string]interface{})
		op.Order = append(op.Order, k)
		op.Map[k] = convertProperty(fieldRaw, nestedPath+"."+k+".properties")
		if req, _ := fieldRaw["required"].(bool); req {
			required = append(required, k)
		}
	}
	sort.Strings(required)
	return op, required
}

// parseAffordance lifts the affordance overlay from a method's raw meta_data.json
// entry into a typed *Affordance. Returns nil when the field is absent, malformed,
// or carries no populated subfields.
//
// Affordance is authored in larksuite-cli-registry's registry-config.yaml under
// overrides.<resource>.<method>.affordance and flows through gen-registry.py's
// deep_merge into the embedded meta_data.json.
func parseAffordance(raw interface{}) *Affordance {
	if raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var a Affordance
	if err := json.Unmarshal(b, &a); err != nil {
		return nil
	}
	if len(a.UseWhen) == 0 && len(a.DoNotUseWhen) == 0 && len(a.Prerequisites) == 0 && len(a.Examples) == 0 && len(a.Related) == 0 {
		return nil
	}
	return &a
}

// convertAccessTokens translates from_meta accessTokens (uses "tenant") into
// CLI --as form (uses "bot"). The result is deduped and sorted alphabetically.
// Unknown tokens are dropped. Returns an empty slice for nil/empty input.
func convertAccessTokens(raw []interface{}) []string {
	seen := make(map[string]bool)
	for _, t := range raw {
		s, ok := t.(string)
		if !ok {
			continue
		}
		switch s {
		case "tenant":
			seen["bot"] = true
		case "user":
			seen["user"] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// buildMeta produces the _meta extension namespace.
func buildMeta(method map[string]interface{}) *Meta {
	m := &Meta{
		EnvelopeVersion: "1.0",
		RequiredScopes:  []string{}, // never nil for stable JSON
	}

	if scopesRaw, ok := method["scopes"].([]interface{}); ok {
		for _, s := range scopesRaw {
			if str, ok := s.(string); ok {
				m.Scopes = append(m.Scopes, str)
			}
		}
	}
	if rsRaw, ok := method["requiredScopes"].([]interface{}); ok {
		for _, s := range rsRaw {
			if str, ok := s.(string); ok {
				m.RequiredScopes = append(m.RequiredScopes, str)
			}
		}
	}

	atRaw, _ := method["accessTokens"].([]interface{})
	m.AccessTokens = convertAccessTokens(atRaw)

	m.Danger, _ = method["danger"].(bool)

	if risk, _ := method["risk"].(string); risk != "" {
		m.Risk = risk
	} else {
		m.Risk = cmdutil.RiskRead
	}

	if docURL, _ := method["docUrl"].(string); docURL != "" {
		m.DocURL = docURL
	}

	m.Affordance = parseAffordance(method["affordance"])
	return m
}

// buildInputSchema produces the inputSchema for one API method.
//
// Top-level shape:
//
//	{ type: object,
//	  required: [<"params" if any param required>, <"data" if any body required>],
//	  properties: {
//	    params: { type: object, required: [...], properties: { ...path/query fields } },  // only if method has parameters
//	    data:   { type: object, required: [...], properties: { ...body fields } },         // only if method has requestBody
//	    yes:    { type: boolean, default: false, ... }                                     // only when risk == "high-risk-write"
//	  } }
//
// The params / data wrapping mirrors the CLI's actual flag layout:
// path+query → --params JSON, body → --data JSON, file → --file. AI consumers
// can pluck inputSchema.properties.params and pass it verbatim to --params.
func buildInputSchema(method map[string]interface{}) *InputSchema {
	is := &InputSchema{
		Type:       "object",
		Required:   []string{}, // never nil — stable envelope shape
		Properties: &OrderedProps{Map: make(map[string]Property)},
	}

	// Build the "params" sub-object from method.parameters (path + query).
	paramsRaw, _ := method["parameters"].(map[string]interface{})
	paramsProps := &OrderedProps{Map: make(map[string]Property)}
	var paramsRequired []string
	for _, k := range orderedKeys(paramsRaw, "parameters") {
		field, _ := paramsRaw[k].(map[string]interface{})
		prop := convertProperty(field, "parameters."+k+".properties")
		paramsProps.Order = append(paramsProps.Order, k)
		paramsProps.Map[k] = prop
		if req, _ := field["required"].(bool); req {
			paramsRequired = append(paramsRequired, k)
		}
	}
	if len(paramsProps.Order) > 0 {
		sort.Strings(paramsRequired)
		is.Properties.Order = append(is.Properties.Order, "params")
		is.Properties.Map["params"] = Property{
			Type:       "object",
			Required:   paramsRequired,
			Properties: paramsProps,
		}
		if len(paramsRequired) > 0 {
			is.Required = append(is.Required, "params")
		}
	}

	// Split method.requestBody into two buckets:
	//   - data: non-file body fields → corresponds to CLI --data JSON
	//   - file: type:file body fields → corresponds to CLI --file <key>=<path>
	// File fields are kept *out* of `data` so the schema mirrors the actual
	// CLI flag dispatch: --file owns one wire format (multipart upload),
	// --data owns the rest (JSON body).
	bodyRaw, _ := method["requestBody"].(map[string]interface{})
	dataProps := &OrderedProps{Map: make(map[string]Property)}
	fileProps := &OrderedProps{Map: make(map[string]Property)}
	var dataRequired []string
	var fileRequired []string
	for _, k := range orderedKeys(bodyRaw, "requestBody") {
		field, _ := bodyRaw[k].(map[string]interface{})
		prop := convertProperty(field, "requestBody."+k+".properties")
		isFile := false
		if t, _ := field["type"].(string); t == "file" {
			isFile = true
		}
		if isFile {
			fileProps.Order = append(fileProps.Order, k)
			fileProps.Map[k] = prop
			if req, _ := field["required"].(bool); req {
				fileRequired = append(fileRequired, k)
			}
		} else {
			dataProps.Order = append(dataProps.Order, k)
			dataProps.Map[k] = prop
			if req, _ := field["required"].(bool); req {
				dataRequired = append(dataRequired, k)
			}
		}
	}
	if len(dataProps.Order) > 0 {
		sort.Strings(dataRequired)
		is.Properties.Order = append(is.Properties.Order, "data")
		is.Properties.Map["data"] = Property{
			Type:       "object",
			Required:   dataRequired,
			Properties: dataProps,
		}
		if len(dataRequired) > 0 {
			is.Required = append(is.Required, "data")
		}
	}
	if len(fileProps.Order) > 0 {
		sort.Strings(fileRequired)
		is.Properties.Order = append(is.Properties.Order, "file")
		is.Properties.Map["file"] = Property{
			Type:        "object",
			Description: "Binary file uploads. Each property is a file field with format:binary; CLI maps each to --file <key>=<path>.",
			Required:    fileRequired,
			Properties:  fileProps,
		}
		if len(fileRequired) > 0 {
			is.Required = append(is.Required, "file")
		}
	}

	// high-risk-write injects a top-level `yes` confirmation flag — sibling
	// of params/data. It is a CLI gate (consumed by lark-cli, not sent to
	// the backend), not an API field.
	if risk, _ := method["risk"].(string); risk == cmdutil.RiskHighRiskWrite {
		is.Properties.Order = append(is.Properties.Order, "yes")
		falseVal := false
		is.Properties.Map["yes"] = Property{
			Type:        "boolean",
			Default:     falseVal,
			Description: "CLI confirmation gate. Must be true to execute; lark-cli rejects with confirmation_required if absent or false. Not sent to the backend.",
		}
		// yes is intentionally NOT added to top-level Required; the gate is
		// enforced semantically (yes==true) by the CLI, not structurally.
	}

	sort.Strings(is.Required) // alphabetical
	return is
}

// buildOutputSchema produces the outputSchema for one API method.
func buildOutputSchema(method map[string]interface{}) *OutputSchema {
	os := &OutputSchema{
		Type:       "object",
		Properties: &OrderedProps{Map: make(map[string]Property)},
	}
	respRaw, _ := method["responseBody"].(map[string]interface{})
	for _, k := range orderedKeys(respRaw, "responseBody") {
		field, _ := respRaw[k].(map[string]interface{})
		os.Properties.Order = append(os.Properties.Order, k)
		os.Properties.Map[k] = convertProperty(field, "responseBody."+k+".properties")
	}
	return os
}

// AssembleEnvelope is the main entry point: takes a service / resource path /
// method name plus its meta_data spec, and produces a fully assembled MCP
// envelope. Output is fully determined by inputs (same arguments → same
// envelope).
func AssembleEnvelope(serviceName string, resourcePath []string, methodName string, method map[string]interface{}) Envelope {
	name := serviceName
	for _, r := range resourcePath {
		name += " " + r
	}
	name += " " + methodName

	desc, _ := method["description"].(string)

	return Envelope{
		Name:         name,
		Description:  desc,
		InputSchema:  buildInputSchema(method),
		OutputSchema: buildOutputSchema(method),
		Meta:         buildMeta(method),
	}
}

// MethodFilter is an optional predicate used by AssembleService and
// AssembleAll to filter methods (e.g. by access token for strict mode).
// Pass nil to include all methods.
type MethodFilter func(method map[string]interface{}) bool

// AssembleService assembles all methods under one service into a sorted
// envelope slice (sorted by Envelope.Name ascending).
func AssembleService(serviceName string, spec map[string]interface{}, filter MethodFilter) []Envelope {
	if spec == nil {
		return nil
	}
	resources, _ := spec["resources"].(map[string]interface{})
	var out []Envelope
	walkMethods(resources, nil, func(resourcePath []string, methodName string, method map[string]interface{}) {
		if filter != nil && !filter(method) {
			return
		}
		out = append(out, AssembleEnvelope(serviceName, resourcePath, methodName, method))
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// AssembleAll assembles every embedded service into one big sorted slice.
// Uses embedded data only (bypasses remote overlay) so envelope output is
// deterministic across machines (CI vs dev vs different user brands).
func AssembleAll(filter MethodFilter) []Envelope {
	var out []Envelope
	for _, svc := range registry.EmbeddedServiceNames() {
		spec := registry.EmbeddedSpec(svc)
		out = append(out, AssembleService(svc, spec, filter)...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// walkMethods recursively walks resources -> methods, calling visit for each
// terminal method. It supports nested resources via the optional "resources"
// key inside a resource value (matches meta_data.json structure).
func walkMethods(resources map[string]interface{}, parentPath []string,
	visit func(resourcePath []string, methodName string, method map[string]interface{})) {
	for resName, resRaw := range resources {
		resMap, ok := resRaw.(map[string]interface{})
		if !ok {
			continue
		}
		curPath := append(append([]string(nil), parentPath...), resName)
		if methods, ok := resMap["methods"].(map[string]interface{}); ok {
			for mName, mRaw := range methods {
				if m, ok := mRaw.(map[string]interface{}); ok {
					visit(curPath, mName, m)
				}
			}
		}
		if nested, ok := resMap["resources"].(map[string]interface{}); ok {
			walkMethods(nested, curPath, visit)
		}
	}
}

// orderedKeys returns the keys of raw in alphabetical order. Field display
// order is not preserved: the schema envelope is consumed as a JSON Schema (MCP
// tool spec), where object property order carries no meaning.
func orderedKeys(raw map[string]interface{}, _ string) []string {
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
