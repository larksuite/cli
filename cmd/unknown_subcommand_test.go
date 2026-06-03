// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

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

	// The detail must stay schema-compatible with flagDidYouMean's unknown_flag
	// (same Type → same keys), so a consumer keyed on Type reads a stable shape.
	exitErr, ok := err.(*output.ExitError)
	if !ok || exitErr.Detail == nil {
		t.Fatalf("expected *output.ExitError with Detail, got %T", err)
	}
	if exitErr.Detail.Type != "unknown_flag" {
		t.Errorf("detail.Type = %q, want unknown_flag", exitErr.Detail.Type)
	}
	detail, ok := exitErr.Detail.Detail.(map[string]any)
	if !ok {
		t.Fatalf("expected detail to be map[string]any, got %T", exitErr.Detail.Detail)
	}
	if detail["unknown"] != "--badflag" {
		t.Errorf("detail.unknown = %v, want --badflag", detail["unknown"])
	}
	if got, _ := detail["unknown_flags"].([]string); len(got) != 1 || got[0] != "--badflag" {
		t.Errorf("detail.unknown_flags = %v, want [--badflag]", detail["unknown_flags"])
	}
	for _, key := range []string{"suggestions", "valid_flags"} {
		if _, present := detail[key]; !present {
			t.Errorf("detail.%s missing; must be present (empty) to match the unknown_flag schema", key)
		}
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
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if exitErr.Code != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", exitErr.Code, output.ExitValidation)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "missing_subcommand" {
		t.Fatalf("detail.Type = %v, want missing_subcommand", exitErr.Detail)
	}
	detail, ok := exitErr.Detail.Detail.(map[string]any)
	if !ok {
		t.Fatalf("detail is not a map: %#v", exitErr.Detail.Detail)
	}
	if flags, _ := detail["flags"].([]string); len(flags) != 1 || flags[0] != "--query" {
		t.Errorf("detail.flags = %v, want [--query]", detail["flags"])
	}
	if detail["command_path"] != "lark-cli drive" {
		t.Errorf("detail.command_path = %v, want lark-cli drive", detail["command_path"])
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

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if exitErr.Code != output.ExitValidation {
		t.Errorf("expected exit code %d, got %d", output.ExitValidation, exitErr.Code)
	}
	if exitErr.Detail == nil {
		t.Fatal("expected ExitError to carry Detail")
	}
	if exitErr.Detail.Type != "unknown_subcommand" {
		t.Errorf("expected Detail.Type=unknown_subcommand, got %q", exitErr.Detail.Type)
	}
	if !strings.Contains(exitErr.Detail.Message, `"+bogus"`) {
		t.Errorf("message should echo the unknown token, got %q", exitErr.Detail.Message)
	}
	// "+bogus" has no close neighbor among drive's subcommands, so the hint falls
	// back to pointing at --help; the full machine-readable list lives in
	// detail.available below (which also excludes hidden commands).
	if !strings.Contains(exitErr.Detail.Hint, "--help") {
		t.Errorf("hint should guide to --help when there is no suggestion, got %q", exitErr.Detail.Hint)
	}

	detail, ok := exitErr.Detail.Detail.(map[string]any)
	if !ok {
		t.Fatalf("expected Detail.Detail to be map[string]any, got %T", exitErr.Detail.Detail)
	}
	if detail["unknown"] != "+bogus" {
		t.Errorf("detail.unknown should be +bogus, got %v", detail["unknown"])
	}
	if detail["command_path"] != "lark-cli drive" {
		t.Errorf("detail.command_path should be %q, got %v", "lark-cli drive", detail["command_path"])
	}
	available, ok := detail["available"].([]string)
	if !ok {
		t.Fatalf("detail.available should be []string, got %T", detail["available"])
	}
	if len(available) != 3 {
		t.Errorf("expected 3 available entries (hidden excluded), got %d: %v", len(available), available)
	}
}

func TestUnknownSubcommandRunE_NestedResourceGroup(t *testing.T) {
	root, _, files := newGroupTree()
	installUnknownSubcommandGuard(root)

	err := files.RunE(files, []string{"bogus"})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError on nested group, got %T", err)
	}
	if exitErr.Detail.Detail.(map[string]any)["command_path"] != "lark-cli drive files" {
		t.Errorf("command_path should reflect the nested resource, got %v",
			exitErr.Detail.Detail.(map[string]any)["command_path"])
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

// unknownSubcommandRunE must split current vs deprecated subcommands into
// separate detail buckets, while suggestions still rank across both so a
// mistyped legacy alias resolves.
func TestUnknownSubcommandRunE_SplitsDeprecatedBucket(t *testing.T) {
	svc := &cobra.Command{Use: "sheets"}
	svc.AddGroup(&cobra.Group{ID: cmdutil.DeprecatedGroupID, Title: "Deprecated"})
	svc.AddCommand(
		&cobra.Command{Use: "+cells-get", RunE: func(*cobra.Command, []string) error { return nil }},
		&cobra.Command{Use: "+read", GroupID: cmdutil.DeprecatedGroupID, RunE: func(*cobra.Command, []string) error { return nil }},
	)

	err := unknownSubcommandRunE(svc, []string{"+reat"})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	detail, ok := exitErr.Detail.Detail.(map[string]any)
	if !ok {
		t.Fatalf("detail is not a map: %#v", exitErr.Detail.Detail)
	}

	if available, _ := detail["available"].([]string); len(available) != 1 || available[0] != "+cells-get" {
		t.Errorf("available = %v, want [+cells-get]", available)
	}
	deprecated, ok := detail["deprecated"].([]string)
	if !ok || len(deprecated) != 1 || deprecated[0] != "+read" {
		t.Errorf("deprecated = %v, want [+read]", deprecated)
	}
	// suggestions rank across both buckets: "+reat" is closest to +read.
	suggestions, _ := detail["suggestions"].([]string)
	found := false
	for _, s := range suggestions {
		if s == "+read" {
			found = true
		}
	}
	if !found {
		t.Errorf("suggestions %v should include +read (typo target)", suggestions)
	}
}
