// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestShortcutFlagAliasesResolveToCanonicalContract(t *testing.T) {
	shortcut := Shortcut{
		Service: "im", Command: "+alias-test", Description: "x",
		Flags: []Flag{
			{
				Name:     "order",
				Aliases:  []string{"sort", "sort-order"},
				Default:  "desc",
				Enum:     []string{"asc", "desc"},
				Required: true,
				Desc:     "message order",
			},
		},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	}

	cmd := mountTestShortcut(t, shortcut)
	if cmd.PreRunE != nil || cmd.PreRun != nil {
		t.Fatal("declarative aliases must not install or take over Cobra PreRun hooks")
	}
	if err := cmd.ParseFlags([]string{"--sort-order", "asc"}); err != nil {
		t.Fatalf("ParseFlags(alias) error = %v", err)
	}
	if got, _ := cmd.Flags().GetString("order"); got != "asc" {
		t.Fatalf("--sort-order resolved order = %q, want asc", got)
	}
	if !cmd.Flags().Changed("order") {
		t.Fatal("alias must mark the canonical flag changed")
	}
	if err := cmd.ValidateRequiredFlags(); err != nil {
		t.Fatalf("alias must satisfy canonical Required contract: %v", err)
	}
	if err := validateEnumFlags(&RuntimeContext{Cmd: cmd}, shortcut.Flags); err != nil {
		t.Fatalf("alias must share canonical Enum contract: %v", err)
	}

	aliasLookup := cmd.Flags().Lookup("sort-order")
	if aliasLookup == nil || aliasLookup.Name != "order" {
		t.Fatalf("Lookup(alias) = %#v, want canonical --order flag", aliasLookup)
	}
	if usage := cmd.Flags().FlagUsages(); strings.Contains(usage, "--sort") {
		t.Fatalf("aliases leaked into help:\n%s", usage)
	}
	var registeredAliases []string
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if flag.Name == "sort" || flag.Name == "sort-order" {
			registeredAliases = append(registeredAliases, flag.Name)
		}
	})
	if len(registeredAliases) != 0 {
		t.Fatalf("aliases were registered as independent flags: %v", registeredAliases)
	}
}

func TestShortcutFlagAliasesUseRepeatedFlagLastWinsSemantics(t *testing.T) {
	shortcut := Shortcut{
		Service: "im", Command: "+alias-order", Description: "x",
		Flags: []Flag{{
			Name: "order", Aliases: []string{"sort-order"}, Default: "desc",
		}},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "alias last", args: []string{"--order", "asc", "--sort-order", "desc"}, want: "desc"},
		{name: "canonical last", args: []string{"--sort-order", "desc", "--order", "asc"}, want: "asc"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := mountTestShortcut(t, shortcut)
			if err := cmd.ParseFlags(test.args); err != nil {
				t.Fatalf("ParseFlags(%v) error = %v", test.args, err)
			}
			if got, _ := cmd.Flags().GetString("order"); got != test.want {
				t.Fatalf("order = %q, want %q", got, test.want)
			}
		})
	}
}

func TestShortcutFlagAliasesComposeWithPostMountNormalizer(t *testing.T) {
	shortcut := Shortcut{
		Service: "im", Command: "+alias-compose", Description: "x",
		Flags: []Flag{{
			Name: "order", Aliases: []string{"sort-order"}, Default: "desc",
		}},
		PostMount: func(cmd *cobra.Command) {
			cmd.Flags().SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
				return pflag.NormalizedName(strings.ReplaceAll(name, "_", "-"))
			})
		},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	}

	cmd := mountTestShortcut(t, shortcut)
	if err := cmd.ParseFlags([]string{"--sort_order", "asc"}); err != nil {
		t.Fatalf("composed alias parse error = %v", err)
	}
	if got, _ := cmd.Flags().GetString("order"); got != "asc" {
		t.Fatalf("order = %q, want asc", got)
	}
}

func TestShortcutFlagAliasesRejectCollisionsAtMount(t *testing.T) {
	tests := []struct {
		name  string
		flags []Flag
		want  string
	}{
		{
			name: "canonical collision",
			flags: []Flag{
				{Name: "order", Aliases: []string{"query"}},
				{Name: "query"},
			},
			want: "conflicts with registered flag",
		},
		{
			name:  "framework flag collision",
			flags: []Flag{{Name: "order", Aliases: []string{"format"}}},
			want:  "conflicts with registered flag",
		},
		{
			name:  "cobra help collision",
			flags: []Flag{{Name: "order", Aliases: []string{"help"}}},
			want:  "conflicts with registered flag --help",
		},
		{
			name: "ambiguous alias",
			flags: []Flag{
				{Name: "order", Aliases: []string{"sort"}},
				{Name: "field", Aliases: []string{"sort"}},
			},
			want: "maps to both",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					t.Fatal("Mount() did not reject alias collision")
				}
				if !strings.Contains(fmt.Sprint(recovered), test.want) {
					t.Fatalf("panic = %q, want %q", recovered, test.want)
				}
			}()
			mountTestShortcut(t, Shortcut{
				Service: "im", Command: "+alias-collision", Description: "x",
				Flags:   test.flags,
				Execute: func(context.Context, *RuntimeContext) error { return nil },
			})
		})
	}
}

func TestShortcutFlagAliasesRejectCollisionAfterPostMountNormalization(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("Mount() did not reject normalized alias collision")
		}
		if got := fmt.Sprint(recovered); !strings.Contains(got, "conflicts with registered flag --sort-order after normalization") {
			t.Fatalf("panic = %q", got)
		}
	}()

	mountTestShortcut(t, Shortcut{
		Service: "im", Command: "+alias-normalized-collision", Description: "x",
		Flags: []Flag{
			{Name: "order", Aliases: []string{"sort_order"}},
			{Name: "sort-order"},
		},
		PostMount: func(cmd *cobra.Command) {
			cmd.Flags().SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
				return pflag.NormalizedName(strings.ReplaceAll(name, "_", "-"))
			})
		},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	})
}
