// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"regexp"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/meta"
)

// Convert renders a meta.Field as a JSON-Schema Property. meta owns the value
// normalization (canonical type, literal coercion, enum ordering); this adds
// only the JSON-Schema-specific shape: the "file" binary format, numeric
// bounds, nested object/array properties, and the array-items fallback.
func Convert(f meta.Field) Property {
	var p Property

	p.Type = f.CanonicalType()
	if f.Type == "file" {
		p.Format = "binary"
	}
	p.Description = normalizeDesc(f.Description)
	p.Default = f.CoercedDefault()
	p.Example = f.CoercedExample()
	p.Minimum = f.MinBound()
	p.Maximum = f.MaxBound()
	p.Enum, p.EnumDescriptions = enumSchema(f.EnumOptions())

	if children := f.Children(); len(children) > 0 {
		props, required := propsOf(children), requiredOf(children)
		if p.Type == "array" {
			// meta_data quirk: array element schema is wrapped in "properties".
			p.Items = &Property{Type: "object", Properties: props, Required: required}
		} else {
			if p.Type == "" {
				p.Type = "object" // infer
			}
			p.Properties = props
			p.Required = required
		}
	}

	// Every array needs an items schema to be valid for consumers that require
	// one, even when meta_data describes no element shape.
	if p.Type == "array" && p.Items == nil {
		p.Items = &Property{}
	}

	return p
}

var (
	sepRunRe   = regexp.MustCompile(`[;；]{2,}`)
	spaceRunRe = regexp.MustCompile(`[ \t]{2,}`)
)

// normalizeDesc de-crufts a meta_data description for the envelope — strips
// markdown emphasis and collapses doubled separators/spaces — but keeps content
// (links, newlines, sentences); the compact flag-help has its own stricter pass.
func normalizeDesc(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "**", "")
	s = sepRunRe.ReplaceAllString(s, "; ")
	s = spaceRunRe.ReplaceAllString(s, " ")
	return strings.TrimRight(s, " ;；。.，,、\n")
}

// enumSchema splits coerced enum options into the parallel enum / enumDescriptions
// arrays for the envelope. enumDescriptions is nil unless at least one value
// carries a description (so the bare-enum form stays values-only), keeping the
// two arrays index-aligned for AI consumers.
func enumSchema(opts []meta.EnumOption) (values []interface{}, descriptions []string) {
	if len(opts) == 0 {
		return nil, nil
	}
	values = make([]interface{}, len(opts))
	descs := make([]string, len(opts))
	hasDesc := false
	for i, o := range opts {
		values[i] = o.Value
		descs[i] = o.Description
		if o.Description != "" {
			hasDesc = true
		}
	}
	if hasDesc {
		descriptions = descs
	}
	return values, descriptions
}

// propsOf renders fields as an ordered JSON-Schema property map. meta's field
// accessors return fields sorted by name, so the property order is alphabetical.
func propsOf(fields []meta.Field) *OrderedProps {
	op := &OrderedProps{}
	for _, f := range fields {
		op.Set(f.Name, Convert(f))
	}
	return op
}

// paramPropsOf is propsOf for the params section: each property also carries
// its CLI flag (--kebab-name).
func paramPropsOf(fields []meta.Field) *OrderedProps {
	op := &OrderedProps{}
	for _, f := range fields {
		p := Convert(f)
		p.Flag = "--" + f.FlagName()
		op.Set(f.Name, p)
	}
	return op
}

// requiredOf returns the alphabetized names of the required fields.
func requiredOf(fields []meta.Field) []string {
	var required []string
	for _, f := range fields {
		if f.Required {
			required = append(required, f.Name)
		}
	}
	sort.Strings(required)
	return required
}

// buildInputSchema produces the inputSchema sections — params (path+query →
// --params), data (non-file body → --data), file (file body → --file) — plus a
// `yes` confirmation gate for high-risk-write methods.
func buildInputSchema(m meta.Method) *InputSchema {
	is := &InputSchema{
		Type:       "object",
		Required:   []string{}, // never nil — stable envelope shape
		Properties: &OrderedProps{},
	}

	addInputObject(is, "params", "", m.Params(), true, "")
	addInputObject(is, "data", "", m.Data(), false, "--data")
	addInputObject(is, "file", "Binary file uploads. Each property is a file field with format:binary; CLI maps each to --file <key>=<path>.", m.Files(), false, "--file")

	if m.Risk == core.RiskHighRiskWrite {
		falseVal := false
		is.Properties.Set("yes", Property{
			Type:        "boolean",
			Flag:        "--yes",
			Default:     falseVal,
			Description: "CLI confirmation gate. Must be true to execute; lark-cli rejects with confirmation_required if absent or false. Pass --yes only after the user has explicitly confirmed; not sent to the backend.",
		})
	}

	sort.Strings(is.Required)
	return is
}

// addInputObject adds one section (params/data/file) when it has fields, marking
// the section required at top level when any field is. asFlags tags each property
// with its --flag (params only); carrier names the section's flag (--data/--file).
func addInputObject(is *InputSchema, name, description string, fields []meta.Field, asFlags bool, carrier string) {
	if len(fields) == 0 {
		return
	}
	props := propsOf(fields)
	if asFlags {
		props = paramPropsOf(fields)
	}
	req := requiredOf(fields)
	is.Properties.Set(name, Property{
		Type:        "object",
		Description: description,
		Carrier:     carrier,
		Required:    req,
		Properties:  props,
	})
	if len(req) > 0 {
		is.Required = append(is.Required, name)
	}
}

// buildOutputSchema produces the outputSchema from the response-body fields.
func buildOutputSchema(m meta.Method) *OutputSchema {
	return &OutputSchema{Type: "object", Properties: propsOf(m.Response())}
}

// buildMeta produces the _meta extension namespace.
func buildMeta(m meta.Method) *Meta {
	out := &Meta{
		EnvelopeVersion: "1.0",
		RequiredScopes:  []string{}, // never nil for stable JSON
		Scopes:          m.Scopes,
		AccessTokens:    m.Identities(),
		Danger:          m.Danger,
	}
	if a, ok := m.ParsedAffordance(); ok {
		out.Affordance = &a
	}
	if len(m.RequiredScopes) > 0 {
		out.RequiredScopes = m.RequiredScopes
	}
	if m.Risk != "" {
		out.Risk = m.Risk
	} else {
		out.Risk = core.RiskRead
	}
	if m.DocURL != "" {
		out.DocURL = m.DocURL
	}
	return out
}

// EnvelopeOf renders the MCP envelope for one method ref — the ref-based entry
// callers use, since apicatalog.MethodRef is the metadata navigation currency.
func EnvelopeOf(ref apicatalog.MethodRef) Envelope {
	m := ref.Method
	// The affordance overlay lives in the CLI, not the metadata; look it up
	// lazily here (it takes precedence over any affordance the metadata carries).
	if raw, ok := affordance.For(ref.Service.Name, m.ID); ok {
		m.Affordance = raw
	}
	return assemble(ref.Service.Name, ref.ResourcePath, m)
}

// Envelopes renders the given method refs into envelopes, sorted by name. The
// caller supplies the refs (from apicatalog navigation), so this package owns
// only rendering — never metadata source selection or traversal.
func Envelopes(refs []apicatalog.MethodRef) []Envelope {
	var out []Envelope
	for _, ref := range refs {
		out = append(out, EnvelopeOf(ref))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// assemble builds the envelope from a method's navigation context. The method
// name comes from m.Name, injected by the typed accessors.
func assemble(serviceName string, resourcePath []string, m meta.Method) Envelope {
	name := serviceName
	for _, r := range resourcePath {
		name += " " + r
	}
	name += " " + m.Name

	return Envelope{
		Name:         name,
		Description:  normalizeDesc(m.Description),
		InputSchema:  buildInputSchema(m),
		OutputSchema: buildOutputSchema(m),
		Meta:         buildMeta(m),
	}
}
