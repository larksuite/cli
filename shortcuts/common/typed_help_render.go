// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// typedCommandHelpFacts is the presentation-only input for grouped command usage.
type typedCommandHelpFacts struct {
	Parameters    []typedParameterHelpFact
	Constraints   []typedConstraintHelpFact
	Authorization []typedAuthorizationHelpFact
	Execution     []typedHelpFlagRef
	Output        []typedOutputHelpFact
	OutputFlags   []typedHelpFlagRef
	Other         []typedHelpFlagRef
}

// typedParameterHelpFact describes one public business parameter.
type typedParameterHelpFact struct {
	Name        string
	Type        string
	Description string
	Required    bool
	Explicit    bool
	DefaultSet  bool
	Default     any
	Enum        []string
	Format      string
	Minimum     *float64
	Maximum     *float64
	MinLength   *int
	MaxLength   *int
	MinItems    *int
	MaxItems    *int
	Sources     []string
	Encoding    string
	Aliases     []typedAliasHelpFact
}

// typedAliasHelpFact describes a public compatibility spelling for a parameter.
type typedAliasHelpFact struct {
	Name       string
	Hidden     bool
	Deprecated bool
}

// typedConstraintHelpFact describes a relation among top-level parameters.
type typedConstraintHelpFact struct {
	Kind     string
	Params   []string
	Presence string
	Text     string
}

type typedAuthorizationHelpFact struct {
	Identity          string
	RequiredScopes    []string
	ConditionalScopes []typedConditionalScopeHelpFact
}

type typedConditionalScopeHelpFact struct {
	Scopes      []string
	When        string
	Params      []string
	Requirement typedScopeRequirement
}

// typedHelpFlagRef selects a registered Cobra flag for a system section.
type typedHelpFlagRef struct{ Name string }

// typedOutputHelpFact is a concise output-contract sentence.
type typedOutputHelpFact struct{ Text string }

// installTypedGroupedUsage replaces Cobra's flat local flag list with a grouped,
// deterministic parameter view. Long descriptions, affordances, inherited
// flags and the root HelpFunc remain untouched.
func installTypedGroupedUsage(cmd *cobra.Command, facts typedCommandHelpFacts) {
	facts = cloneTypedHelpFacts(facts)
	cmd.SetUsageFunc(func(c *cobra.Command) error {
		w := c.OutOrStderr()
		fmt.Fprintf(w, "Usage:\n  %s\n", c.UseLine())
		body := renderTypedGroupedUsage(c, facts)
		if body != "" {
			fmt.Fprintf(w, "\n%s\n", body)
		}
		if c.HasAvailableInheritedFlags() {
			fmt.Fprintf(w, "\nGlobal Flags:\n%s\n", strings.TrimRight(c.InheritedFlags().FlagUsages(), " \t\n"))
		}
		return nil
	})
}

// renderTypedGroupedUsage renders local grouped sections without Usage or inherited
// flags for Typed Shortcut commands.
func renderTypedGroupedUsage(cmd *cobra.Command, facts typedCommandHelpFacts) string {
	var b strings.Builder
	seen := map[string]bool{}
	required, optional := splitTypedHelpParameters(facts.Parameters)
	if len(required)+len(optional) > 0 {
		b.WriteString("Parameters:\n")
		writeTypedHelpParameters(&b, "  Required:", required, seen)
		writeTypedHelpParameters(&b, "  Optional:", optional, seen)
	}
	if len(facts.Constraints) > 0 {
		writeTypedHelpGap(&b)
		b.WriteString("Constraints:\n")
		for _, fact := range facts.Constraints {
			fmt.Fprintf(&b, "  %s\n", typedHelpConstraintText(fact))
		}
	}
	writeTypedAuthorizationHelp(&b, facts.Authorization)
	writeTypedHelpFlagSection(&b, cmd, "Execution", facts.Execution, seen)
	if len(facts.OutputFlags) > 0 || len(facts.Output) > 0 {
		writeTypedHelpGap(&b)
		b.WriteString("Output:\n")
		writeTypedHelpFlags(&b, cmd, facts.OutputFlags, seen)
		for _, fact := range facts.Output {
			if strings.TrimSpace(fact.Text) != "" {
				fmt.Fprintf(&b, "  %s\n", strings.TrimSpace(fact.Text))
			}
		}
	}
	other := typedHelpReferencedFlags(cmd, facts.Other, seen)
	cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		if !flag.Hidden && !seen[flag.Name] {
			seen[flag.Name] = true
			other = append(other, flag)
		}
	})
	if len(other) > 0 {
		writeTypedHelpGap(&b)
		b.WriteString("Other:\n")
		writeTypedHelpPFlags(&b, other)
	}
	return strings.TrimRight(b.String(), "\n")
}

func splitTypedHelpParameters(parameters []typedParameterHelpFact) (required, optional []typedParameterHelpFact) {
	for _, parameter := range parameters {
		if parameter.Required {
			required = append(required, parameter)
		} else {
			optional = append(optional, parameter)
		}
	}
	return required, optional
}

func writeTypedHelpParameters(b *strings.Builder, heading string, parameters []typedParameterHelpFact, seen map[string]bool) {
	if len(parameters) == 0 {
		return
	}
	fmt.Fprintln(b, heading)
	specs := make([]string, len(parameters))
	width := 0
	for i, parameter := range parameters {
		specs[i] = "    --" + parameter.Name
		if parameter.Type != "" && parameter.Type != "boolean" {
			specs[i] += " <" + parameter.Type + ">"
		}
		if len(specs[i]) > width {
			width = len(specs[i])
		}
	}
	for i, parameter := range parameters {
		seen[parameter.Name] = true
		fmt.Fprintf(b, "%-*s   %s\n", width, specs[i], strings.TrimSpace(parameter.Description))
		for _, note := range typedHelpParameterNotes(parameter) {
			fmt.Fprintf(b, "%*s%s\n", width+3+4, "", note)
		}
	}
}

func typedHelpParameterNotes(fact typedParameterHelpFact) []string {
	var notes []string
	if fact.Explicit {
		notes = append(notes, "must be explicitly provided")
	}
	if fact.DefaultSet {
		notes = append(notes, "default: "+formatTypedHelpValue(fact.Default))
	}
	if len(fact.Enum) > 0 {
		notes = append(notes, "allowed values: "+strings.Join(fact.Enum, ", "))
	}
	if fact.Format != "" {
		notes = append(notes, "format: "+fact.Format)
	}
	if fact.Minimum != nil || fact.Maximum != nil {
		switch {
		case fact.Minimum != nil && fact.Maximum != nil:
			notes = append(notes, fmt.Sprintf("range: %s to %s", typedHelpNumber(*fact.Minimum), typedHelpNumber(*fact.Maximum)))
		case fact.Minimum != nil:
			notes = append(notes, "minimum: "+typedHelpNumber(*fact.Minimum))
		default:
			notes = append(notes, "maximum: "+typedHelpNumber(*fact.Maximum))
		}
	}
	if fact.MinLength != nil {
		notes = append(notes, fmt.Sprintf("minimum length: %d", *fact.MinLength))
	}
	if fact.MaxLength != nil {
		notes = append(notes, fmt.Sprintf("maximum length: %d", *fact.MaxLength))
	}
	if fact.MinItems != nil {
		notes = append(notes, fmt.Sprintf("minimum items: %d", *fact.MinItems))
	}
	if fact.MaxItems != nil {
		notes = append(notes, fmt.Sprintf("maximum items: %d", *fact.MaxItems))
	}
	if source := typedHelpSourceText(fact.Sources, fact.Encoding); source != "" {
		notes = append(notes, source)
	}
	var aliases, deprecated []string
	for _, alias := range fact.Aliases {
		if alias.Hidden {
			continue
		}
		name := "--" + alias.Name
		if alias.Deprecated {
			deprecated = append(deprecated, name)
		} else {
			aliases = append(aliases, name)
		}
	}
	if len(aliases) > 0 {
		notes = append(notes, "aliases: "+strings.Join(aliases, ", "))
	}
	if len(deprecated) > 0 {
		notes = append(notes, "deprecated aliases: "+strings.Join(deprecated, ", "))
	}
	return notes
}

func typedHelpSourceText(sources []string, encoding string) string {
	has := func(want string) bool {
		for _, source := range sources {
			if source == want {
				return true
			}
		}
		return false
	}
	var text string
	switch {
	case has("file") && has("stdin"):
		if encoding == "json" {
			text = "accepts inline JSON, @file, or stdin with -"
		} else {
			text = "accepts inline text, @file, or stdin with -"
		}
	case has("file"):
		if encoding == "json" {
			text = "accepts inline JSON or @file"
		} else {
			text = "accepts inline value or @file"
		}
	case has("stdin"):
		if encoding == "json" {
			text = "accepts inline JSON or stdin with -"
		} else {
			text = "accepts inline value or stdin with -"
		}
	}
	switch encoding {
	case "repeated":
		if text != "" {
			text += "; "
		}
		text += "flag may be repeated"
	case "comma_or_repeated":
		if text != "" {
			text += "; "
		}
		text += "accepts comma-separated values or repeated flags"
	case "json":
		if text == "" {
			text = "accepts inline JSON"
		}
	}
	return text
}

func writeTypedAuthorizationHelp(b *strings.Builder, identities []typedAuthorizationHelpFact) {
	if len(identities) == 0 {
		return
	}
	writeTypedHelpGap(b)
	b.WriteString("Authorization:\n")
	for _, identity := range identities {
		fmt.Fprintf(b, "  %s:\n", strings.ToUpper(identity.Identity[:1])+identity.Identity[1:])
		if len(identity.RequiredScopes) > 0 {
			b.WriteString("    Always required:\n")
			for _, scope := range identity.RequiredScopes {
				fmt.Fprintf(b, "      %s\n", scope)
			}
		}
		for _, requirement := range []typedScopeRequirement{typedScopeRequired, typedScopeBestEffort} {
			var conditional []typedConditionalScopeHelpFact
			for _, scope := range identity.ConditionalScopes {
				if scope.Requirement == requirement {
					conditional = append(conditional, scope)
				}
			}
			if len(conditional) == 0 {
				continue
			}
			heading := "Conditionally required:"
			if requirement == typedScopeBestEffort {
				heading = "Optional capability:"
			}
			fmt.Fprintf(b, "    %s\n", heading)
			for _, scope := range conditional {
				fmt.Fprintf(b, "      %s\n", strings.Join(scope.Scopes, ", "))
				if scope.When != "" {
					fmt.Fprintf(b, "        when: %s\n", scope.When)
				}
				if len(scope.Params) > 0 {
					params := make([]string, 0, len(scope.Params))
					for _, param := range scope.Params {
						params = append(params, "--"+param)
					}
					fmt.Fprintf(b, "        related parameters: %s\n", strings.Join(params, ", "))
				}
			}
		}
	}
}

func writeTypedHelpFlagSection(b *strings.Builder, cmd *cobra.Command, heading string, refs []typedHelpFlagRef, seen map[string]bool) {
	flags := typedHelpReferencedFlags(cmd, refs, seen)
	if len(flags) == 0 {
		return
	}
	writeTypedHelpGap(b)
	fmt.Fprintf(b, "%s:\n", heading)
	writeTypedHelpPFlags(b, flags)
}

func writeTypedHelpFlags(b *strings.Builder, cmd *cobra.Command, refs []typedHelpFlagRef, seen map[string]bool) {
	writeTypedHelpPFlags(b, typedHelpReferencedFlags(cmd, refs, seen))
}

func typedHelpReferencedFlags(cmd *cobra.Command, refs []typedHelpFlagRef, seen map[string]bool) []*pflag.Flag {
	var flags []*pflag.Flag
	for _, ref := range refs {
		flag := cmd.LocalFlags().Lookup(ref.Name)
		if flag == nil || flag.Hidden || seen[flag.Name] {
			continue
		}
		seen[flag.Name] = true
		flags = append(flags, flag)
	}
	return flags
}

func writeTypedHelpPFlags(b *strings.Builder, flags []*pflag.Flag) {
	if len(flags) == 0 {
		return
	}
	specs := make([]string, len(flags))
	width := 0
	for i, flag := range flags {
		specs[i] = typedHelpFlagSpec(flag)
		if len(specs[i]) > width {
			width = len(specs[i])
		}
	}
	for i, flag := range flags {
		_, usage := pflag.UnquoteUsage(flag)
		if showTypedHelpDefault(flag) && !strings.Contains(strings.ToLower(usage), "default") {
			usage += fmt.Sprintf(" (default %s)", flag.DefValue)
		}
		fmt.Fprintf(b, "%-*s   %s\n", width, specs[i], strings.TrimSpace(usage))
	}
}

func typedHelpFlagSpec(flag *pflag.Flag) string {
	typeName, _ := pflag.UnquoteUsage(flag)
	spec := "  --" + flag.Name
	if flag.Shorthand != "" && flag.ShorthandDeprecated == "" {
		spec = "  -" + flag.Shorthand + ", --" + flag.Name
	}
	if typeName != "" {
		spec += " <" + typeName + ">"
	}
	return spec
}

func showTypedHelpDefault(flag *pflag.Flag) bool {
	switch flag.DefValue {
	case "", "0", "false", "[]":
		return false
	}
	return true
}

func typedHelpConstraintText(fact typedConstraintHelpFact) string {
	if strings.TrimSpace(fact.Text) != "" {
		return strings.TrimSpace(fact.Text)
	}
	params := make([]string, 0, len(fact.Params))
	for _, param := range fact.Params {
		params = append(params, "--"+param)
	}
	joined := strings.Join(params, ", ")
	var text string
	switch fact.Kind {
	case "at_most_one":
		text = "at most one of: " + joined
	case "exactly_one":
		text = "exactly one of: " + joined
	case "at_least_one":
		text = "at least one of: " + joined
	case "requires":
		if len(params) > 1 {
			text = params[0] + " requires " + strings.Join(params[1:], ", ")
		} else {
			text = joined + " has an incomplete requires constraint"
		}
	case "conflicts":
		text = "conflicting parameters: " + joined
	case string(typedRelationCoOccur):
		text = "all or none of: " + joined
	case "same_value":
		text = "must have the same value: " + joined
	default:
		text = strings.ReplaceAll(fact.Kind, "_", " ") + ": " + joined
	}
	if fact.Presence == string(typedPresenceNonZero) {
		text += " (using non-zero values)"
	}
	return text
}

func writeTypedHelpGap(b *strings.Builder) {
	if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n\n") {
		b.WriteByte('\n')
	}
}
func typedHelpNumber(value float64) string { return fmt.Sprintf("%g", value) }
func formatTypedHelpValue(value any) string {
	encoded, err := json.Marshal(value)
	if err == nil {
		return string(encoded)
	}
	return fmt.Sprint(value)
}

func cloneTypedHelpFacts(facts typedCommandHelpFacts) typedCommandHelpFacts {
	facts.Parameters = append([]typedParameterHelpFact{}, facts.Parameters...)
	facts.Constraints = append([]typedConstraintHelpFact{}, facts.Constraints...)
	facts.Authorization = append([]typedAuthorizationHelpFact{}, facts.Authorization...)
	facts.Execution = append([]typedHelpFlagRef{}, facts.Execution...)
	facts.Output = append([]typedOutputHelpFact{}, facts.Output...)
	facts.OutputFlags = append([]typedHelpFlagRef{}, facts.OutputFlags...)
	facts.Other = append([]typedHelpFlagRef{}, facts.Other...)
	// The compiler order is meaningful for business parameters. System refs are
	// sorted only by the caller's declared order; remaining Cobra flags are
	// already deterministic. Keep this explicit to avoid map-order coupling.
	for i := range facts.Parameters {
		facts.Parameters[i].Enum = append([]string{}, facts.Parameters[i].Enum...)
		facts.Parameters[i].Sources = append([]string{}, facts.Parameters[i].Sources...)
		facts.Parameters[i].Aliases = append([]typedAliasHelpFact{}, facts.Parameters[i].Aliases...)
	}
	for i := range facts.Authorization {
		facts.Authorization[i].RequiredScopes = append([]string{}, facts.Authorization[i].RequiredScopes...)
		facts.Authorization[i].ConditionalScopes = append([]typedConditionalScopeHelpFact{}, facts.Authorization[i].ConditionalScopes...)
		for j := range facts.Authorization[i].ConditionalScopes {
			conditional := &facts.Authorization[i].ConditionalScopes[j]
			conditional.Scopes = append([]string{}, conditional.Scopes...)
			conditional.Params = append([]string{}, conditional.Params...)
		}
	}
	return facts
}
