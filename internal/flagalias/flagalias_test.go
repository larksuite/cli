// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package flagalias

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestBindResolvesAliasesToOneCanonicalFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "messages"}
	cmd.Flags().String("order", "desc", "message order")
	if err := cmd.MarkFlagRequired("order"); err != nil {
		t.Fatal(err)
	}
	if err := Bind(cmd, []Spec{{Canonical: "order", Aliases: []string{"sort", "sort-order"}}}); err != nil {
		t.Fatal(err)
	}

	if err := cmd.ParseFlags([]string{"--sort-order", "asc"}); err != nil {
		t.Fatalf("ParseFlags(alias) error = %v", err)
	}
	canonical := cmd.Flags().Lookup("order")
	if got := canonical.Value.String(); got != "asc" {
		t.Fatalf("canonical value = %q, want asc", got)
	}
	if !canonical.Changed {
		t.Fatal("alias must mark canonical flag Changed")
	}
	if err := cmd.ValidateRequiredFlags(); err != nil {
		t.Fatalf("alias must satisfy required canonical flag: %v", err)
	}
	if got := cmd.Flags().Lookup("sort-order"); got != canonical {
		t.Fatalf("Lookup(alias) = %p, want canonical %p", got, canonical)
	}
	if got := Aliases(canonical); strings.Join(got, ",") != "sort,sort-order" {
		t.Fatalf("Aliases(canonical) = %v", got)
	}
	if usage := cmd.Flags().FlagUsages(); strings.Contains(usage, "--sort") {
		t.Fatalf("aliases leaked into help:\n%s", usage)
	}
	var names []string
	cmd.Flags().VisitAll(func(flag *pflag.Flag) { names = append(names, flag.Name) })
	if strings.Contains(strings.Join(names, ","), "sort") {
		t.Fatalf("aliases were registered as independent flags: %v", names)
	}
}

func TestBindUsesNativeRepeatedFlagSemantics(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "alias last", args: []string{"--order", "asc", "--sort", "desc"}, want: "desc"},
		{name: "canonical last", args: []string{"--sort", "desc", "--order", "asc"}, want: "asc"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "messages"}
			cmd.Flags().String("order", "", "")
			if err := Bind(cmd, []Spec{{Canonical: "order", Aliases: []string{"sort"}}}); err != nil {
				t.Fatal(err)
			}
			if err := cmd.ParseFlags(test.args); err != nil {
				t.Fatal(err)
			}
			if got, _ := cmd.Flags().GetString("order"); got != test.want {
				t.Fatalf("order = %q, want %q", got, test.want)
			}
		})
	}

	cmd := &cobra.Command{Use: "messages"}
	cmd.Flags().StringSlice("fields", nil, "")
	if err := Bind(cmd, []Spec{{Canonical: "fields", Aliases: []string{"field"}}}); err != nil {
		t.Fatal(err)
	}
	if err := cmd.ParseFlags([]string{"--field", "name", "--fields", "status"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := cmd.Flags().GetStringSlice("fields"); strings.Join(got, ",") != "name,status" {
		t.Fatalf("collection aliases did not accumulate: %v", got)
	}
}

func TestBindComposesExistingNormalizer(t *testing.T) {
	cmd := &cobra.Command{Use: "messages"}
	cmd.Flags().SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		return pflag.NormalizedName(strings.ReplaceAll(name, "_", "-"))
	})
	cmd.Flags().String("order", "", "")
	if err := Bind(cmd, []Spec{{Canonical: "order", Aliases: []string{"sort-order"}}}); err != nil {
		t.Fatal(err)
	}
	if err := cmd.ParseFlags([]string{"--sort_order", "asc"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := cmd.Flags().GetString("order"); got != "asc" {
		t.Fatalf("order = %q, want asc", got)
	}
}

func TestBindRejectsDuplicateCanonicalAfterNormalization(t *testing.T) {
	cmd := &cobra.Command{Use: "messages"}
	cmd.Flags().SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		return pflag.NormalizedName(strings.ReplaceAll(name, "_", "-"))
	})
	cmd.Flags().String("sort-order", "", "")

	err := Bind(cmd, []Spec{
		{Canonical: "sort_order", Aliases: []string{"order"}},
		{Canonical: "sort-order", Aliases: []string{"ordering"}},
	})
	if err == nil || !strings.Contains(err.Error(), "more than once after normalization") {
		t.Fatalf("Bind() error = %v", err)
	}
	if got := Aliases(cmd.Flags().Lookup("sort-order")); len(got) != 0 {
		t.Fatalf("failed bind partially mutated annotations: %v", got)
	}
}

func TestBindRejectsAcceptedNameCollisionsWithoutMutation(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*cobra.Command)
		specs []Spec
		want  string
	}{
		{
			name: "registered canonical",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().String("order", "", "")
				cmd.Flags().String("query", "", "")
			},
			specs: []Spec{{Canonical: "order", Aliases: []string{"query"}}},
			want:  "conflicts with registered flag --query",
		},
		{
			name: "ambiguous alias",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().String("order", "", "")
				cmd.Flags().String("field", "", "")
			},
			specs: []Spec{
				{Canonical: "order", Aliases: []string{"sort"}},
				{Canonical: "field", Aliases: []string{"sort"}},
			},
			want: "maps to both",
		},
		{
			name: "normalized collision",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
					return pflag.NormalizedName(strings.ReplaceAll(name, "_", "-"))
				})
				cmd.Flags().String("order", "", "")
				cmd.Flags().String("sort-order", "", "")
			},
			specs: []Spec{{Canonical: "order", Aliases: []string{"sort_order"}}},
			want:  "after normalization",
		},
		{
			name: "invalid spelling",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().String("order", "", "")
			},
			specs: []Spec{{Canonical: "order", Aliases: []string{"--sort"}}},
			want:  "must not include leading dashes",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "messages"}
			test.setup(cmd)
			err := Bind(cmd, test.specs)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Bind() error = %v, want %q", err, test.want)
			}
			if got := Aliases(cmd.Flags().Lookup("order")); len(got) != 0 {
				t.Fatalf("failed bind partially mutated annotations: %v", got)
			}
		})
	}
}

func TestBindRejectsInheritedFlagCollision(t *testing.T) {
	parent := &cobra.Command{Use: "root"}
	parent.PersistentFlags().String("profile", "", "")
	child := &cobra.Command{Use: "messages"}
	child.Flags().String("order", "", "")
	parent.AddCommand(child)

	err := Bind(child, []Spec{{Canonical: "order", Aliases: []string{"profile"}}})
	if err == nil || !strings.Contains(err.Error(), "registered flag --profile") {
		t.Fatalf("Bind() error = %v", err)
	}
}

func TestBindCanComposeIndependentAdapters(t *testing.T) {
	cmd := &cobra.Command{Use: "messages"}
	cmd.Flags().String("order", "", "")
	cmd.Flags().String("query", "", "")
	if err := Bind(cmd, []Spec{{Canonical: "order", Aliases: []string{"sort"}}}); err != nil {
		t.Fatal(err)
	}
	if err := Bind(cmd, []Spec{{Canonical: "query", Aliases: []string{"keyword"}}}); err != nil {
		t.Fatal(err)
	}
	if err := cmd.ParseFlags([]string{"--sort", "asc", "--keyword", "launch"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := cmd.Flags().GetString("order"); got != "asc" {
		t.Fatalf("order = %q", got)
	}
	if got, _ := cmd.Flags().GetString("query"); got != "launch" {
		t.Fatalf("query = %q", got)
	}
}
