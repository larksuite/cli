// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/util"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// nonLocalReservedFlags are flags the collision guard can't see at registration
// time — cobra adds --help lazily and --profile is a root persistent flag,
// neither attached to the method command yet.
var nonLocalReservedFlags = map[string]bool{"help": true, "profile": true}

type boundParamFlag struct {
	field meta.Field
	read  func() interface{}
}

// paramFlagBinder owns one service method's generated typed param flags: it
// registers them (kind, help, enum completion, reserved-name skip) and applies
// the --params overlay, where a changed typed flag overrides its key in the
// --params JSON. Holding the field<->flag binding here keeps the request builder
// from re-deriving which flags map to which param keys.
type paramFlagBinder struct {
	bound []boundParamFlag
}

// newParamFlagBinder registers one typed kebab flag per path/query parameter on
// cmd and returns a binder for the --params overlay. A name colliding with an
// existing or reserved flag is skipped — pflag panics on duplicate registration
// — leaving that parameter reachable via --params.
func newParamFlagBinder(cmd *cobra.Command, params []meta.Field) *paramFlagBinder {
	b := &paramFlagBinder{}
	for _, f := range params {
		name := f.FlagName()
		if nonLocalReservedFlags[name] || cmd.Flags().Lookup(name) != nil {
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
//	<param_name>, required|optional[. enum: a|b|c][. API default: x]
//
// It leads with the canonical underscore param name (the key this flag overrides
// in --params), states required/optional, lists the allowed enum values, and the
// API default. Full prose descriptions and per-option meanings are intentionally
// left to `lark-cli schema` so --help stays scannable. Values come from the
// meta.Field accessors, so this carries no internal/schema dependency.
func paramFlagUsage(f meta.Field) string {
	req := "optional"
	if f.Required {
		req = "required"
	}
	parts := []string{fmt.Sprintf("%s, %s", f.Name, req)}
	if opts := f.EnumOptions(); len(opts) > 0 {
		parts = append(parts, "enum: "+formatEnumInline(opts))
	}
	if s := literalStr(f.CoercedDefault()); s != "" {
		parts = append(parts, "API default: "+s)
	}
	return strings.Join(parts, ". ") + "."
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
