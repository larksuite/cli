// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestInstallTypedGroupedUsage(t *testing.T) {
	cmd := &cobra.Command{Use: "+fixture"}
	cmd.Flags().String("token", "", "target token")
	cmd.Flags().StringSlice("labels", nil, "labels")
	cmd.Flags().Bool("dry-run", false, "print request without executing")
	cmd.Flags().String("format", "json", "output format")
	cmd.Flags().Bool("print-schema", false, "print schema")
	cmd.Flags().String("unused", "", "unclassified flag")
	cmd.Flags().String("hidden", "", "hidden flag")
	_ = cmd.Flags().MarkHidden("hidden")
	minimum, maximum, minItems := 1.0, 5.0, 1
	facts := typedCommandHelpFacts{
		Parameters: []typedParameterHelpFact{
			{Name: "token", Type: "string", Description: "target token", Required: true, Sources: []string{"flag", "file", "stdin"}, Aliases: []typedAliasHelpFact{{Name: "old-token", Deprecated: true}}},
			{Name: "labels", Type: "array", Description: "labels", DefaultSet: true, Default: []string{}, Enum: []string{"a", "b"}, Minimum: &minimum, Maximum: &maximum, MinItems: &minItems, Encoding: "comma_or_repeated"},
		},
		Constraints: []typedConstraintHelpFact{{Kind: "exactly_one", Params: []string{"token", "labels"}, Presence: "provided"}},
		Authorization: []typedAuthorizationHelpFact{{Identity: "user", RequiredScopes: []string{"fixture:write"}, ConditionalScopes: []typedConditionalScopeHelpFact{
			{Scopes: []string{"fixture:read"}, When: "--labels requests lookup", Params: []string{"labels"}, Requirement: typedScopeRequired},
			{Scopes: []string{"fixture:enrich"}, When: "detail enrichment is available", Requirement: typedScopeBestEffort},
		}}},
		Execution:   []typedHelpFlagRef{{Name: "dry-run"}},
		OutputFlags: []typedHelpFlagRef{{Name: "format"}},
		Output:      []typedOutputHelpFact{{Text: "successful output is a JSON object"}},
		Other:       []typedHelpFlagRef{{Name: "print-schema"}},
	}
	installTypedGroupedUsage(cmd, facts)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Usage(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"Parameters:\n  Required:\n    --token <string>",
		"accepts inline text, @file, or stdin with -",
		"deprecated aliases: --old-token",
		"  Optional:\n    --labels <array>",
		"default: []", "allowed values: a, b", "range: 1 to 5", "minimum items: 1",
		"accepts comma-separated values or repeated flags",
		"Constraints:\n  exactly one of: --token, --labels",
		"Authorization:\n  User:\n    Always required:\n      fixture:write",
		"Conditionally required:\n      fixture:read", "when: --labels requests lookup", "related parameters: --labels",
		"Optional capability:\n      fixture:enrich", "when: detail enrichment is available",
		"Execution:\n  --dry-run",
		"Output:\n  --format <string>", "successful output is a JSON object",
		"Other:\n  --print-schema", "--unused",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("usage missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "Other:\n") != 1 {
		t.Fatalf("Other section count != 1:\n%s", got)
	}
	if strings.Contains(got, "--hidden") {
		t.Fatalf("hidden flag leaked:\n%s", got)
	}
}

func TestConstraintTextKinds(t *testing.T) {
	tests := map[string]string{
		string(typedRelationExactlyOne): "exactly one of: --a, --b",
		string(typedRelationAtLeastOne): "at least one of: --a, --b",
		string(typedRelationRequires):   "--a requires --b",
		string(typedRelationConflicts):  "conflicting parameters: --a, --b",
		string(typedRelationCoOccur):    "all or none of: --a, --b",
	}
	for kind, want := range tests {
		if got := typedHelpConstraintText(typedConstraintHelpFact{Kind: kind, Params: []string{"a", "b"}}); got != want {
			t.Errorf("%s = %q, want %q", kind, got, want)
		}
	}
	if got := typedHelpConstraintText(typedConstraintHelpFact{Kind: string(typedRelationExactlyOne), Params: []string{"a", "b"}, Presence: string(typedPresenceNonZero)}); !strings.Contains(got, "using non-zero values") {
		t.Errorf("nonzero = %q", got)
	}
}
