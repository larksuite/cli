// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestNoFileCompletion(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	NoFileCompletion(cmd)

	if cmd.ValidArgsFunction == nil {
		t.Fatal("expected ValidArgsFunction to be set")
	}

	completions, directive := cmd.ValidArgsFunction(cmd, nil, "")
	if len(completions) != 0 {
		t.Errorf("expected no completions, got %v", completions)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp (%d), got %d", cobra.ShellCompDirectiveNoFileComp, directive)
	}
}

func TestRegisterEnumFlag_ValidValues(t *testing.T) {
	var val string
	cmd := &cobra.Command{Use: "test", Run: func(*cobra.Command, []string) {}}
	RegisterEnumFlag(cmd, &val, "color", "c", "red", []string{"red", "green", "blue"}, "pick a color")

	// default value
	if val != "red" {
		t.Errorf("expected default 'red', got %q", val)
	}

	// accept valid value
	cmd.SetArgs([]string{"--color", "green"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error for valid value: %v", err)
	}
	if val != "green" {
		t.Errorf("expected 'green', got %q", val)
	}
}

func TestRegisterEnumFlag_InvalidValue(t *testing.T) {
	var val string
	cmd := &cobra.Command{Use: "test", Run: func(*cobra.Command, []string) {}}
	RegisterEnumFlag(cmd, &val, "color", "c", "red", []string{"red", "green", "blue"}, "pick a color")

	cmd.SetArgs([]string{"--color", "yellow"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid value, got nil")
	}
}

func TestRegisterEnumFlag_Shorthand(t *testing.T) {
	var val string
	cmd := &cobra.Command{Use: "test", Run: func(*cobra.Command, []string) {}}
	RegisterEnumFlag(cmd, &val, "shell", "s", "", []string{"bash", "zsh"}, "shell type")

	cmd.SetArgs([]string{"-s", "zsh"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "zsh" {
		t.Errorf("expected 'zsh', got %q", val)
	}
}

func TestRegisterEnumFlag_Completion(t *testing.T) {
	var val string
	options := []string{"json", "csv", "table"}

	child := &cobra.Command{Use: "test", Run: func(*cobra.Command, []string) {}}
	RegisterEnumFlag(child, &val, "format", "", "json", options, "output format")

	root := &cobra.Command{Use: "root"}
	root.AddCommand(child)

	// Use Cobra's __complete mechanism to verify flag completion is registered.
	// Simulate: root test --format <TAB>
	out, _, err := executeCommand(root, cobra.ShellCompRequestCmd, "test", "--format", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, opt := range options {
		if !containsLine(out, opt) {
			t.Errorf("expected completion %q in output, got:\n%s", opt, out)
		}
	}
}

// executeCommand runs a cobra command and captures stdout.
func executeCommand(root *cobra.Command, args ...string) (string, string, error) {
	stdout := &strings.Builder{}
	root.SetOut(stdout)
	root.SetErr(stdout)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), "", err
}

// containsLine checks if any line in output matches the target.
func containsLine(output, target string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == target || strings.HasPrefix(line, target+"\t") {
			return true
		}
	}
	return false
}

func TestEnumValue_Set(t *testing.T) {
	var s string
	e := &enumValue{str: &s, options: []string{"a", "b", "c"}}

	if err := e.Set("b"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "b" {
		t.Errorf("expected 'b', got %q", s)
	}

	if err := e.Set("x"); err == nil {
		t.Fatal("expected error for invalid value, got nil")
	}
}

func TestEnumValue_String(t *testing.T) {
	s := "hello"
	e := &enumValue{str: &s, options: []string{"hello"}}
	if e.String() != "hello" {
		t.Errorf("expected 'hello', got %q", e.String())
	}
}

func TestEnumValue_Type(t *testing.T) {
	var s string
	e := &enumValue{str: &s, options: nil}
	if e.Type() != "string" {
		t.Errorf("expected 'string', got %q", e.Type())
	}
}
