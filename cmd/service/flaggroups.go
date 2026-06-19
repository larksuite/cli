// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Flag annotations the grouped service-method help renderer reads.
const (
	flagGroupAnnotation = "lark_flag_group" // display group key
	flagSubAnnotation   = "lark_flag_sub"   // "required" | "optional" within API Parameters
	flagNoteAnnotation  = "lark_flag_note"  // extra lines shown indented under a flag

	groupParams    = "params"    // typed path/query flags
	groupBody      = "body"      // --data, --file
	groupRaw       = "raw"       // --params
	groupExecution = "execution" // --as/--dry-run/--page-*/--yes
	groupOutput    = "output"    // --output/--format/--jq

	subRequired = "required"
	subOptional = "optional"
)

// serviceFlagGroupOrder is the display order + titles of the flag groups. API
// Parameters carries only typed path/query flags; raw --params, request body and
// execution/output controls each get their own group so an agent can tell the
// distinct input kinds apart.
var serviceFlagGroupOrder = []struct{ key, title string }{
	{groupParams, "API Parameters"},
	{groupBody, "Request Body"},
	{groupRaw, "Raw Parameter Input"},
	{groupExecution, "Execution"},
	{groupOutput, "Output"},
}

// applyGroupedUsage installs the grouped usage renderer on a service method
// cmd: local flags via the grouped renderer instead of cobra's flat Flags:
// list; global (inherited) flags and the Risk/Tips sections appended by the
// root help func are unaffected. Rendered by hand rather than via
// cmd.SetUsageTemplate: cobra lazy-links text/template on the first
// SetUsageTemplate call, whose executor reaches reflect.Value.MethodByName —
// that disables the linker's method-level dead-code elimination and costs
// ~19 MB of binary size.
func applyGroupedUsage(cmd *cobra.Command) {
	cmd.SetUsageFunc(func(c *cobra.Command) error {
		w := c.OutOrStderr()
		fmt.Fprintf(w, "Usage:\n  %s\n", c.UseLine())
		if c.HasAvailableLocalFlags() {
			fmt.Fprintf(w, "\n%s\n", renderServiceFlagGroups(c))
		}
		if c.HasAvailableInheritedFlags() {
			fmt.Fprintf(w, "\nGlobal Flags:\n%s\n", strings.TrimRight(c.InheritedFlags().FlagUsages(), " \t\n"))
		}
		return nil
	})
}

func annotate(f *pflag.Flag, key string, vals []string) {
	if f.Annotations == nil {
		f.Annotations = map[string][]string{}
	}
	f.Annotations[key] = vals
}

// tagFlagGroup records a flag's display group (no-op if the flag is absent).
func tagFlagGroup(fs *pflag.FlagSet, name, group string) {
	if f := fs.Lookup(name); f != nil {
		annotate(f, flagGroupAnnotation, []string{group})
	}
}

func annotationOf(f *pflag.Flag, key string) []string {
	if f.Annotations != nil {
		return f.Annotations[key]
	}
	return nil
}

func flagGroupOf(f *pflag.Flag) string {
	if v := annotationOf(f, flagGroupAnnotation); len(v) > 0 {
		return v[0]
	}
	return ""
}

func flagSubOf(f *pflag.Flag) string {
	if v := annotationOf(f, flagSubAnnotation); len(v) > 0 {
		return v[0]
	}
	return ""
}

// renderServiceFlagGroups renders the command's local flags into ordered,
// titled groups; the API Parameters group is further split into Required /
// Optional. It is the body of the usage func applyGroupedUsage installs.
func renderServiceFlagGroups(cmd *cobra.Command) string {
	var b strings.Builder
	seen := map[*pflag.Flag]bool{}
	for _, g := range serviceFlagGroupOrder {
		flags := groupFlags(cmd, g.key, seen)
		if len(flags) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s:\n", g.title)
		if g.key == groupParams {
			writeSection(&b, "  Required:", subFlags(flags, subRequired))
			writeSection(&b, "  Optional:", subFlags(flags, subOptional))
		} else {
			writeSection(&b, "", flags)
		}
		fmt.Fprintln(&b)
	}
	// Anything untagged (e.g. -h/--help) goes last under "Other".
	var other []*pflag.Flag
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || seen[f] {
			return
		}
		other = append(other, f)
	})
	if len(other) > 0 {
		fmt.Fprintln(&b, "Other:")
		writeSection(&b, "", other)
	}
	return strings.TrimRight(b.String(), "\n")
}

// groupFlags returns the visible local flags tagged with group key, marking them
// seen so the trailing "Other" bucket only catches genuinely untagged flags.
func groupFlags(cmd *cobra.Command, key string, seen map[*pflag.Flag]bool) []*pflag.Flag {
	var flags []*pflag.Flag
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || flagGroupOf(f) != key {
			return
		}
		flags = append(flags, f)
		seen[f] = true
	})
	return flags
}

func subFlags(flags []*pflag.Flag, sub string) []*pflag.Flag {
	var out []*pflag.Flag
	for _, f := range flags {
		s := flagSubOf(f)
		// Untagged subgroup defaults to Optional so nothing is dropped.
		if s == sub || (s == "" && sub == subOptional) {
			out = append(out, f)
		}
	}
	return out
}

// writeSection prints an optional (sub)header and the flags, aligned in a
// column, each flag row followed by its note lines indented under the usage.
func writeSection(b *strings.Builder, header string, flags []*pflag.Flag) {
	if len(flags) == 0 {
		return
	}
	if header != "" {
		fmt.Fprintf(b, "%s\n", header)
	}
	specs := make([]string, len(flags))
	maxSpec := 0
	for i, f := range flags {
		specs[i] = flagSpec(f)
		if len(specs[i]) > maxSpec {
			maxSpec = len(specs[i])
		}
	}
	for i, f := range flags {
		_, usage := pflag.UnquoteUsage(f)
		if showsDefault(f) {
			usage += fmt.Sprintf(" (default %s)", f.DefValue)
		}
		fmt.Fprintf(b, "%-*s   %s\n", maxSpec, specs[i], strings.TrimSpace(usage))
		for _, note := range annotationOf(f, flagNoteAnnotation) {
			fmt.Fprintf(b, "%*s%s\n", maxSpec+3+4, "", note)
		}
	}
}

// flagSpec is pflag's "      --name type" / "  -x, --name type" left column.
func flagSpec(f *pflag.Flag) string {
	typeName, _ := pflag.UnquoteUsage(f)
	spec := "      --" + f.Name
	if f.Shorthand != "" && f.ShorthandDeprecated == "" {
		spec = "  -" + f.Shorthand + ", --" + f.Name
	}
	if typeName != "" {
		spec += " " + typeName
	}
	return spec
}

// showsDefault mirrors pflag's "non-zero default" rule for the flag types these
// commands use, so the grouped rendering shows the same "(default x)" hints as
// cobra's flat list.
func showsDefault(f *pflag.Flag) bool {
	switch f.DefValue {
	case "", "0", "false", "[]":
		return false
	}
	return true
}
