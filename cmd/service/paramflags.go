// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/util"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type boundParamFlag struct {
	field meta.Field
	read  func() interface{}
}

// paramsOnlyField is a path/query parameter that got no typed flag because its
// kebab name is already taken by another flag (a standard flag like --format, or
// a root persistent flag). It stays reachable via --params; the binder keeps it,
// with the flag that claimed the name, so --help can show the exact --params form
// and steer the reader off the wrong flag.
type paramsOnlyField struct {
	field   meta.Field
	claimed *pflag.Flag
}

// paramFlagBinder owns one service method's generated typed param flags: it
// registers them (kind, help, enum completion, reserved-name skip) and applies
// the --params overlay, where a changed typed flag overrides its key in the
// --params JSON. Holding the field<->flag binding here keeps the request builder
// from re-deriving which flags map to which param keys.
type paramFlagBinder struct {
	bound      []boundParamFlag
	paramsOnly []paramsOnlyField
}

// newParamFlagBinder registers one typed kebab flag per path/query parameter on
// cmd and returns a binder for the --params overlay. A name already taken by
// another flag is skipped — pflag panics on a local duplicate and a generated
// flag would silently shadow a persistent one — and recorded as paramsOnly so
// the parameter stays reachable (and discoverable) via --params. The taken set
// is derived, not hand-listed: local flags (the standard set, registered before
// this runs) via cmd, the lazily-added --help materialized here, and the root's
// persistent flags via reserved (nil for direct callers that have no root).
func newParamFlagBinder(cmd *cobra.Command, params []meta.Field, reserved *pflag.FlagSet) *paramFlagBinder {
	cmd.InitDefaultHelpFlag() // materialize --help/-h so the local guard below sees it
	b := &paramFlagBinder{}
	for _, f := range params {
		name := f.FlagName()
		if claimed := flagClaiming(cmd, reserved, name); claimed != nil {
			b.paramsOnly = append(b.paramsOnly, paramsOnlyField{field: f, claimed: claimed})
			continue
		}
		read := registerTypedFlag(cmd.Flags(), name, f.CanonicalType(), paramFlagUsage(f))
		if values := enumStrings(f.EnumValues()); len(values) > 0 {
			cmdutil.RegisterFlagCompletion(cmd, name, func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
				return values, cobra.ShellCompDirectiveNoFileComp
			})
		}
		// Group as an API parameter and mark required/optional for the
		// Required/Optional subsections of the grouped --help renderer.
		if fl := cmd.Flags().Lookup(name); fl != nil {
			annotate(fl, flagGroupAnnotation, []string{groupParams})
			sub := subOptional
			if f.Required {
				sub = subRequired
			}
			annotate(fl, flagSubAnnotation, []string{sub})
		}
		b.bound = append(b.bound, boundParamFlag{field: f, read: read})
	}
	return b
}

// flagClaiming returns the flag already occupying name (so a typed param flag
// would collide), or nil when the name is free. It checks the command's own
// flags (the standard set + the materialized --help) and the root's persistent
// flags — so the reserved set is whatever is actually registered, never a
// hand-kept list that drifts when a global flag is added.
func flagClaiming(cmd *cobra.Command, reserved *pflag.FlagSet, name string) *pflag.Flag {
	if fl := cmd.Flags().Lookup(name); fl != nil {
		return fl
	}
	if reserved != nil {
		return reserved.Lookup(name)
	}
	return nil
}

// paramsOnlyHelp renders the --help addendum for parameters that have no typed
// flag, or "" when there are none. Each line is copy-pasteable and names what
// the colliding flag actually does, so neither a human nor an agent sets the
// wrong one (e.g. --format, which is the output format, not the API parameter).
func (b *paramFlagBinder) paramsOnlyHelp() string {
	if len(b.paramsOnly) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\nParameters set via --params (no typed flag; the name is taken by another flag):\n")
	for _, p := range b.paramsOnly {
		name := p.field.Name
		fmt.Fprintf(&sb, "  %s: --params '{%q: %s}'\n", name, name, paramExample(p.field))
		if d := sanitizeFieldDesc(p.field.Description); d != "" {
			fmt.Fprintf(&sb, "      %s\n", d)
		}
		if vals := enumStrings(p.field.EnumValues()); len(vals) > 0 {
			fmt.Fprintf(&sb, "      allowed: %s\n", strings.Join(vals, " | "))
		}
		if b := formatBoundsInline(p.field); b != "" {
			fmt.Fprintf(&sb, "      %s\n", b)
		}
		if p.claimed != nil {
			fmt.Fprintf(&sb, "      do not use --%s (%s)\n", p.claimed.Name, p.claimed.Usage)
		}
	}
	return sb.String()
}

// paramExample picks a concrete sample for a params-only field's --help snippet:
// its first allowed enum value, else its example, else a placeholder.
func paramExample(f meta.Field) string {
	if vals := enumStrings(f.EnumValues()); len(vals) > 0 {
		return fmt.Sprintf("%q", vals[0])
	}
	if s := literalStr(f.CoercedExample()); s != "" {
		return fmt.Sprintf("%q", s)
	}
	return `"<value>"`
}

// overlay lets an explicit typed flag override the same key in --params
// (--params is the base). Only changed flags apply, so the --params-only path is
// unchanged. A nil binder or cmd is a no-op.
func (b *paramFlagBinder) overlay(cmd *cobra.Command, params map[string]interface{}) {
	if b == nil || cmd == nil {
		return
	}
	for _, pf := range b.bound {
		if cmd.Flags().Changed(pf.field.FlagName()) {
			params[pf.field.Name] = pf.read()
		}
	}
}

// registerTypedFlag registers one flag of the given canonical JSON-Schema kind
// and returns a reader for its parsed value; the kind→pflag-type switch lives
// only here.
func registerTypedFlag(fs *pflag.FlagSet, name, kind, usage string) func() interface{} {
	switch kind {
	case "integer":
		return flagReader(fs.Int(name, 0, usage))
	case "boolean":
		return flagReader(fs.Bool(name, false, usage))
	case "array":
		return flagReader(fs.StringArray(name, nil, usage))
	default:
		return flagReader(fs.String(name, "", usage))
	}
}

func flagReader[T any](p *T) func() interface{} {
	return func() interface{} { return *p }
}

// paramFlagUsage renders the typed param flag's agent-readable help line:
//
//	<param_name>, required|optional[. <description>][. enum: a|b|c][. min: x, max: y][. API default: x]
//
// It leads with the canonical underscore param name (the key this flag overrides
// in --params), states required/optional, then the sanitized description — a
// bare name like user_mailbox_id carries no meaning on its own — followed by
// the allowed enum values, the min/max constraint, and the API default.
// Per-option meanings and the unabridged prose stay in `lark-cli schema` so
// --help stays scannable. Values come from the meta.Field accessors, so this
// carries no internal/schema dependency.
func paramFlagUsage(f meta.Field) string {
	req := "optional"
	if f.Required {
		req = "required"
	}
	parts := []string{fmt.Sprintf("%s, %s", f.Name, req)}
	if d := sanitizeFieldDesc(f.Description); d != "" {
		parts = append(parts, d)
	}
	if opts := f.EnumOptions(); len(opts) > 0 {
		parts = append(parts, "enum: "+formatEnumInline(opts))
	}
	if b := formatBoundsInline(f); b != "" {
		parts = append(parts, b)
	}
	if s := literalStr(f.CoercedDefault()); s != "" {
		parts = append(parts, "API default: "+s)
	}
	return strings.Join(parts, ". ") + "."
}

// sanitizeFieldDesc compresses a field description for the help line: strips
// markdown link URLs (keeping the text), cuts at the first note separator
// (;/；/newline — meta_data appends bullet notes after them), trims trailing
// sentence punctuation (the clause join adds its own "."), collapses
// whitespace and truncates. Unlike enum option descriptions it does NOT cut at
// the first sentence end (。): the later sentence often carries the key
// affordance — e.g. user_mailbox_id's `可以输入"me"`. Full prose stays in
// `lark-cli schema`.
func sanitizeFieldDesc(s string) string {
	if s == "" {
		return ""
	}
	s = markdownLinkRe.ReplaceAllString(s, "$1")
	if i := strings.IndexAny(s, "；;\n\r"); i >= 0 {
		s = s[:i]
	}
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimRight(s, "。.")
	return util.TruncateStrWithEllipsis(s, 60)
}

// formatBoundsInline renders the field's min/max constraint for the help line
// ("min: 1, max: 100", or the single declared side), or "" when the field
// declares neither. The vocabulary matches the envelope's minimum/maximum, so
// help and `lark-cli schema` state the same constraint.
func formatBoundsInline(f meta.Field) string {
	min, max := f.MinBound(), f.MaxBound()
	switch {
	case min != nil && max != nil:
		return fmt.Sprintf("min: %s, max: %s", formatBound(*min), formatBound(*max))
	case min != nil:
		return "min: " + formatBound(*min)
	case max != nil:
		return "max: " + formatBound(*max)
	}
	return ""
}

// formatBound renders a bound without a float artifact (100 not 100.000000).
func formatBound(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// formatEnumInline renders allowed values for the flag help line: "v=meaning"
// when the value carries a (sanitized, truncated) description — so opaque
// numeric enums like succeed_type read as "0=…|1=…|2=…" — else just "v". Full
// meanings live in the envelope's enumDescriptions / `lark-cli schema`.
func formatEnumInline(opts []meta.EnumOption) string {
	items := make([]string, len(opts))
	for i, o := range opts {
		if d := sanitizeOptionDesc(o.Description); d != "" {
			items[i] = fmt.Sprintf("%v=%s", o.Value, d)
		} else {
			items[i] = fmt.Sprintf("%v", o.Value)
		}
	}
	return strings.Join(items, "|")
}

var markdownLinkRe = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

// sanitizeOptionDesc compresses an enum option description for the inline help
// line: strips markdown link URLs (keeping the link text), keeps the first
// clause, collapses whitespace and truncates. The full text stays in the
// envelope / `lark-cli schema`.
func sanitizeOptionDesc(s string) string {
	if s == "" {
		return ""
	}
	s = markdownLinkRe.ReplaceAllString(s, "$1")
	if i := strings.IndexAny(s, "。；;\n\r"); i >= 0 {
		s = s[:i]
	}
	s = strings.Join(strings.Fields(s), " ")
	return util.TruncateStrWithEllipsis(s, 40)
}

// literalStr renders a coerced literal (default/example) for flag help,
// returning "" for a nil or empty value so the caller can omit the clause.
func literalStr(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func enumStrings(enum []interface{}) []string {
	out := make([]string, 0, len(enum))
	for _, e := range enum {
		out = append(out, fmt.Sprintf("%v", e))
	}
	return out
}
