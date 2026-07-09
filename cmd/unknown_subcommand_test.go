// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
)

func newGroupTree() (root, drive, files *cobra.Command) {
	root = &cobra.Command{Use: "lark-cli"}
	drive = &cobra.Command{Use: "drive", Short: "drive ops"}
	root.AddCommand(drive)

	search := &cobra.Command{Use: "+search", RunE: func(*cobra.Command, []string) error { return nil }}
	upload := &cobra.Command{Use: "+upload", RunE: func(*cobra.Command, []string) error { return nil }}
	hidden := &cobra.Command{Use: "+secret", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
	drive.AddCommand(search, upload, hidden)

	files = &cobra.Command{Use: "files", Short: "files ops"}
	drive.AddCommand(files)
	files.AddCommand(&cobra.Command{Use: "list", RunE: func(*cobra.Command, []string) error { return nil }})

	return root, drive, files
}

func TestInstallUnknownSubcommandGuard_InstallsOnGroupsOnly(t *testing.T) {
	root, drive, files := newGroupTree()
	leaf := drive.Commands()[0] // +search

	installUnknownSubcommandGuard(root)

	if drive.RunE == nil {
		t.Error("drive should have RunE installed")
	}
	if files.RunE == nil {
		t.Error("files should have RunE installed")
	}
	if err := leaf.RunE(leaf, []string{"unexpected-arg"}); err != nil {
		t.Errorf("leaf +search RunE should be untouched, got error %v", err)
	}
}

func TestInstallUnknownSubcommandGuard_PreservesExistingRunE(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	called := false
	custom := &cobra.Command{
		Use: "custom",
		RunE: func(*cobra.Command, []string) error {
			called = true
			return nil
		},
	}
	// Child makes custom a "group" command, exercising the Run/RunE override guard.
	custom.AddCommand(&cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }})
	root.AddCommand(custom)

	installUnknownSubcommandGuard(root)

	if err := custom.RunE(custom, nil); err != nil {
		t.Fatalf("preserved RunE returned error: %v", err)
	}
	if !called {
		t.Error("guard must not overwrite a command that already defines Run/RunE")
	}
}

func TestUnknownFlagTokens(t *testing.T) {
	_, drive, _ := newGroupTree()
	// Give a subcommand a flag so a misplaced-but-known flag (the user omitted
	// the subcommand) is distinguished from a genuinely unknown one.
	for _, c := range drive.Commands() {
		if c.Name() == "+search" {
			c.Flags().String("query", "", "")
		}
	}
	cases := []struct {
		name    string
		rawArgs []string
		want    []string
	}{
		{"genuinely unknown long flag", []string{"drive", "--badflag"}, []string{"--badflag"}},
		{"flag known on a subcommand (misplaced)", []string{"drive", "--query", "x"}, nil},
		{"no flags at all", []string{"drive"}, nil},
		{"tokens after -- are positional", []string{"drive", "--", "--badflag"}, nil},
		{"unknown shorthand", []string{"drive", "-Z"}, []string{"-Z"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := unknownFlagTokens(drive, tc.rawArgs)
			if len(got) != len(tc.want) {
				t.Fatalf("unknownFlagTokens(%v) = %v, want %v", tc.rawArgs, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("token[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestUnknownSubcommandRunE_FlagBeforeSubcommandIsStructured(t *testing.T) {
	_, drive, _ := newGroupTree()
	installUnknownSubcommandGuard(drive.Root())

	// Simulate `lark-cli drive --badflag`: the UnknownFlags whitelist swallows
	// --badflag, so RunE sees no args; the guard must recover it from
	// rawInvocationArgs and fail structured rather than print help + exit 0.
	rawInvocationArgs = []string{"drive", "--badflag"}
	t.Cleanup(func() { rawInvocationArgs = nil })

	err := drive.RunE(drive, nil)
	if err == nil {
		t.Fatal("expected a structured unknown_flag error, got nil (help fallthrough)")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("error = %q, want it to mention an unknown flag", err.Error())
	}

	// Typed surface: a validation error (exit 2) whose Params carries the
	// offending flag so an agent can recover the token without parsing prose.
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	if verr.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype = %q, want invalid_argument", verr.Subtype)
	}
	if output.ExitCodeOf(err) != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", output.ExitCodeOf(err), output.ExitValidation)
	}
	if len(verr.Params) != 1 || verr.Params[0].Name != "--badflag" {
		t.Errorf("params = %v, want one entry named --badflag", verr.Params)
	}
}

func TestUnknownSubcommandRunE_ValidFlagWithoutSubcommandIsStructured(t *testing.T) {
	_, drive, _ := newGroupTree()
	// --query is defined on the +search subcommand, so it is a *valid* flag that
	// was placed before the (omitted) subcommand. Unlike an unknown flag, this
	// must still fail structured (missing_subcommand) rather than fall through to
	// help + exit 0 — `drive --query x` is a malformed call, not a help request.
	for _, c := range drive.Commands() {
		if c.Name() == "+search" {
			c.Flags().String("query", "", "")
		}
	}
	installUnknownSubcommandGuard(drive.Root())

	rawInvocationArgs = []string{"drive", "--query", "x"}
	t.Cleanup(func() { rawInvocationArgs = nil })

	err := drive.RunE(drive, nil)
	if err == nil {
		t.Fatal("expected a structured missing_subcommand error, got nil (help fallthrough)")
	}
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	if output.ExitCodeOf(err) != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", output.ExitCodeOf(err), output.ExitValidation)
	}
	if !strings.Contains(verr.Message, "missing subcommand") {
		t.Errorf("message = %q, want it to mention a missing subcommand", verr.Message)
	}
	if len(verr.Params) != 1 || verr.Params[0].Name != "--query" {
		t.Errorf("params = %v, want one entry named --query", verr.Params)
	}
	if !strings.Contains(verr.Message, "lark-cli drive") {
		t.Errorf("message = %q, want it to name the group path", verr.Message)
	}
}

// A bare group carrying only a group-valid global flag (e.g. the inherited
// --profile) is not missing a subcommand — those flags do not belong to a
// subcommand — so it must print help, not fail with missing_subcommand.
func TestUnknownSubcommandRunE_GroupValidGlobalFlagShowsHelp(t *testing.T) {
	_, drive, _ := newGroupTree()
	drive.Root().PersistentFlags().String("profile", "", "") // global, inherited by drive
	installUnknownSubcommandGuard(drive.Root())

	rawInvocationArgs = []string{"--profile", "p", "drive"}
	t.Cleanup(func() { rawInvocationArgs = nil })

	var buf bytes.Buffer
	drive.SetOut(&buf)
	drive.SetErr(&buf)
	if err := drive.RunE(drive, nil); err != nil {
		t.Fatalf("bare group with only a global flag should print help, got error: %v", err)
	}
	if !strings.Contains(buf.String(), "drive ops") {
		t.Errorf("expected help output, got:\n%s", buf.String())
	}
}

func TestUnknownSubcommandRunE_NoArgsShowsHelp(t *testing.T) {
	_, drive, _ := newGroupTree()
	installUnknownSubcommandGuard(drive.Root())

	var buf bytes.Buffer
	drive.SetOut(&buf)
	drive.SetErr(&buf)

	if err := drive.RunE(drive, nil); err != nil {
		t.Fatalf("expected no-args invocation to succeed, got: %v", err)
	}
	if !strings.Contains(buf.String(), "drive ops") {
		t.Errorf("expected help output to include the command's Short, got:\n%s", buf.String())
	}
}

func TestUnknownSubcommandRunE_UnknownReturnsStructuredError(t *testing.T) {
	_, drive, _ := newGroupTree()
	installUnknownSubcommandGuard(drive.Root())

	err := drive.RunE(drive, []string{"+bogus"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}

	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	if output.ExitCodeOf(err) != output.ExitValidation {
		t.Errorf("expected exit code %d, got %d", output.ExitValidation, output.ExitCodeOf(err))
	}
	if !strings.Contains(verr.Message, `"+bogus"`) {
		t.Errorf("message should echo the unknown token, got %q", verr.Message)
	}
	if !strings.Contains(verr.Message, "lark-cli drive") {
		t.Errorf("message should name the group path, got %q", verr.Message)
	}
	// "+bogus" has no close neighbor among drive's subcommands, so the hint falls
	// back to pointing at --help (suggestions, when present, are folded into hint).
	if !strings.Contains(verr.Hint, "--help") {
		t.Errorf("hint should guide to --help when there is no suggestion, got %q", verr.Hint)
	}
}

func TestUnknownSubcommandRunE_NestedResourceGroup(t *testing.T) {
	root, _, files := newGroupTree()
	installUnknownSubcommandGuard(root)

	err := files.RunE(files, []string{"bogus"})
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *errs.ValidationError on nested group, got %T", err)
	}
	if !strings.Contains(verr.Message, "lark-cli drive files") {
		t.Errorf("message should reflect the nested resource path, got %q", verr.Message)
	}
}

func TestAvailableSubcommandNames_FiltersHelpAndCompletion(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	root.AddCommand(
		&cobra.Command{Use: "alpha", RunE: func(*cobra.Command, []string) error { return nil }},
		&cobra.Command{Use: "help", RunE: func(*cobra.Command, []string) error { return nil }},
		&cobra.Command{Use: "completion", RunE: func(*cobra.Command, []string) error { return nil }},
		&cobra.Command{Use: "beta", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }},
		&cobra.Command{Use: "gamma", RunE: func(*cobra.Command, []string) error { return nil }},
	)

	got, _ := availableSubcommandNames(root)
	want := []string{"alpha", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("availableSubcommandNames[%d] = %q, want %q", i, got[i], name)
		}
	}
}

func TestAvailableSubcommandNames_SplitsDeprecatedGroup(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	root.AddGroup(&cobra.Group{ID: cmdutil.DeprecatedGroupID, Title: "Deprecated"})
	root.AddCommand(
		&cobra.Command{Use: "+new-cmd", RunE: func(*cobra.Command, []string) error { return nil }},
		&cobra.Command{Use: "+old-cmd", GroupID: cmdutil.DeprecatedGroupID, RunE: func(*cobra.Command, []string) error { return nil }},
	)

	available, deprecated := availableSubcommandNames(root)
	if len(available) != 1 || available[0] != "+new-cmd" {
		t.Errorf("available = %v, want [+new-cmd]", available)
	}
	if len(deprecated) != 1 || deprecated[0] != "+old-cmd" {
		t.Errorf("deprecated = %v, want [+old-cmd]", deprecated)
	}
}

// unknownSubcommandRunE ranks suggestions across both current and deprecated
// subcommands so a mistyped legacy alias resolves; the closest match is folded
// into the hint.
func TestUnknownSubcommandRunE_SuggestsAcrossDeprecatedBucket(t *testing.T) {
	svc := &cobra.Command{Use: "sheets"}
	svc.AddGroup(&cobra.Group{ID: cmdutil.DeprecatedGroupID, Title: "Deprecated"})
	svc.AddCommand(
		&cobra.Command{Use: "+cells-get", RunE: func(*cobra.Command, []string) error { return nil }},
		&cobra.Command{Use: "+read", GroupID: cmdutil.DeprecatedGroupID, RunE: func(*cobra.Command, []string) error { return nil }},
	)

	err := unknownSubcommandRunE(svc, []string{"+reat"})
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	// "+reat" is closest to the deprecated +read: the candidate must surface
	// both as a machine-readable param suggestion (for agent retry) and in the
	// hint, proving ranking spans the deprecated bucket.
	if len(verr.Params) != 1 || verr.Params[0].Name != "+reat" {
		t.Fatalf("params = %v, want one entry named +reat (the offending subcommand)", verr.Params)
	}
	foundSuggestion := false
	for _, s := range verr.Params[0].Suggestions {
		if s == "+read" {
			foundSuggestion = true
		}
	}
	if !foundSuggestion {
		t.Errorf("Params[0].Suggestions should include +read, got %v", verr.Params[0].Suggestions)
	}
	if !strings.Contains(verr.Hint, "+read") {
		t.Errorf("hint %q should suggest +read (typo target across deprecated bucket)", verr.Hint)
	}
}
