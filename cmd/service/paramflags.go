// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/meta"
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
// flag, or "" when there are none. Per field: a copy-pasteable --params form,
// the same fieldFacts a typed flag would show on its usage line, and what the
// colliding flag actually does — so neither a human nor an agent sets the
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
		for _, fact := range fieldFacts(p.field) {
			fmt.Fprintf(&sb, "      %s\n", fact)
		}
		if p.claimed != nil {
			fmt.Fprintf(&sb, "      do not use --%s (%s)\n", p.claimed.Name, p.claimed.Usage)
		}
	}
	return sb.String()
}

// hasTypedFlag reports whether the binder registered a typed flag for the
// param named name. False for params-only fields — a flag with the same kebab
// name may exist (that's the collision), but it is not this param's input.
// Nil-safe for direct buildServiceRequest callers that have no binder.
func (b *paramFlagBinder) hasTypedFlag(name string) bool {
	if b == nil {
		return false
	}
	for _, pf := range b.bound {
		if pf.field.Name == name {
			return true
		}
	}
	return false
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
