// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

func TestUnknownFlagName(t *testing.T) {
	cases := []struct {
		in   string
		name string
		ok   bool
	}{
		{"unknown flag: --query", "query", true},
		{"unknown flag: --with-styles", "with-styles", true},
		{"unknown shorthand flag: 'z' in -z", "", false},
		{"flag needs an argument: --find", "", false},
		{`invalid argument "x" for "--count"`, "", false},
	}
	for _, c := range cases {
		name, ok := unknownFlagName(errors.New(c.in))
		if name != c.name || ok != c.ok {
			t.Errorf("unknownFlagName(%q) = (%q,%v), want (%q,%v)", c.in, name, ok, c.name, c.ok)
		}
	}
}

func TestFlagDidYouMean_UnknownFlagSuggestsAndListsValid(t *testing.T) {
	c := &cobra.Command{Use: "demo"}
	c.Flags().String("range", "", "")
	c.Flags().String("find", "", "")
	c.Flags().Bool("dry-run", false, "")

	err := flagDidYouMean(c, errors.New("unknown flag: --rang")) // typo of --range
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if exitErr.Detail.Type != "unknown_flag" {
		t.Errorf("type = %q, want unknown_flag", exitErr.Detail.Type)
	}
	if !strings.Contains(exitErr.Detail.Hint, "--range") {
		t.Errorf("hint should suggest --range, got %q", exitErr.Detail.Hint)
	}
	detail, _ := exitErr.Detail.Detail.(map[string]any)
	valid, _ := detail["valid_flags"].([]string)
	if !slices.Contains(valid, "find") || !slices.Contains(valid, "range") {
		t.Errorf("valid_flags should list find & range, got %v", valid)
	}
}

func TestFlagDidYouMean_OtherErrorStaysGeneric(t *testing.T) {
	c := &cobra.Command{Use: "demo"}
	err := flagDidYouMean(c, errors.New("flag needs an argument: --find"))
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if exitErr.Detail.Type != "flag_error" {
		t.Errorf("type = %q, want flag_error (non-unknown-flag errors stay generic)", exitErr.Detail.Type)
	}
}
