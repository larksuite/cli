// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
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
	tests := []struct {
		name      string
		args      []string
		want      string
		wantParam string
	}{
		{name: "alias last", args: []string{"--order", "asc", "--sort-order", "desc"}, want: "desc", wantParam: "--sort-order"},
		{name: "canonical last", args: []string{"--sort-order", "desc", "--order", "asc"}, want: "asc", wantParam: "--order"},
		{name: "alias equals form", args: []string{"--sort-order=desc"}, want: "desc", wantParam: "--sort-order"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var observed string
			shortcut := &Shortcut{
				Service: "im", Command: "+alias-order", Description: "x", AuthTypes: []string{"bot"},
				Flags: []Flag{{
					Name: "order", Aliases: []string{"sort-order"}, Default: "desc",
				}},
				Validate: func(_ context.Context, runtime *RuntimeContext) error {
					observed = runtime.Str("order")
					return ValidationErrorf("invalid order for test").WithParam("--order")
				},
				Execute: func(context.Context, *RuntimeContext) error { return nil },
			}
			err := runAliasShortcut(t, shortcut, test.args...)
			assertValidationParam(t, err, test.wantParam)
			if observed != test.want {
				t.Fatalf("order = %q, want %q", observed, test.want)
			}
		})
	}
}

func TestShortcutFlagAliasEnumErrorUsesCallerSpelling(t *testing.T) {
	shortcut := Shortcut{
		Service: "im", Command: "+alias-enum", Description: "x", AuthTypes: []string{"bot"},
		Flags: []Flag{{
			Name: "order", Aliases: []string{"sort-order"}, Enum: []string{"asc", "desc"},
		}},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	}

	err := runAliasShortcut(t, &shortcut, "--sort-order=sideways")
	validationErr := assertValidationParam(t, err, "--sort-order")
	if !strings.Contains(validationErr.Message, "--order") {
		t.Fatalf("message = %q, want canonical --order guidance", validationErr.Message)
	}
}

func TestShortcutFlagAliasRangeErrorUsesCallerSpelling(t *testing.T) {
	shortcut := Shortcut{
		Service: "base", Command: "+alias-range", Description: "x", AuthTypes: []string{"bot"},
		Flags: []Flag{{
			Name: "limit", Aliases: []string{"page-size"}, Type: "int", Default: "10",
		}},
		Validate: func(_ context.Context, runtime *RuntimeContext) error {
			_, err := ValidatePageSizeTyped(runtime, "limit", 10, 1, 100)
			return err
		},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	}

	err := runAliasShortcut(t, &shortcut, "--page-size=101")
	assertValidationParam(t, err, "--page-size")
}

func TestRunnerAttributesBusinessValidationErrorAtAliasBoundary(t *testing.T) {
	shortcut := Shortcut{
		Service: "slides", Command: "+alias-business-validation", Description: "x", AuthTypes: []string{"bot"},
		Flags: []Flag{{
			Name: "presentation", Aliases: []string{"url"},
		}},
		Validate: func(context.Context, *RuntimeContext) error {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported --presentation input").
				WithParam("--presentation")
		},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	}
	err := runAliasShortcut(t, &shortcut, "--url=not/a/presentation")
	validationErr := assertValidationParam(t, err, "--url")
	if !strings.Contains(validationErr.Hint, "--url") || !strings.Contains(validationErr.Hint, "--presentation") {
		t.Fatalf("hint = %q, want alias-to-canonical guidance", validationErr.Hint)
	}
}

func runAliasShortcut(t *testing.T, shortcut *Shortcut, args ...string) error {
	t.Helper()
	factory := newTestFactory()
	cmd := newTestShortcutCmd(shortcut, factory)
	installFlagAliases(cmd, shortcut.Flags)
	parseArgs := append(append([]string(nil), args...), "--as=bot")
	if err := cmd.ParseFlags(parseArgs); err != nil {
		t.Fatalf("ParseFlags(%v) error = %v", parseArgs, err)
	}
	return runShortcut(cmd, factory, shortcut, true)
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
