// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"testing"

	"github.com/spf13/pflag"
)

// TestDebugFlagDefault verifies that Debug is false when --debug is not specified.
func TestDebugFlagDefault(t *testing.T) {
	opts := &GlobalOptions{}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	RegisterGlobalFlags(fs, opts)

	// Parse empty args (no flags)
	if err := fs.Parse([]string{}); err != nil {
		t.Fatalf("unexpected error during parse: %v", err)
	}

	if opts.Debug != false {
		t.Errorf("expected Debug=false by default, got %v", opts.Debug)
	}
}

// TestDebugFlagParsedTrue verifies that Debug is true when --debug is specified.
func TestDebugFlagParsedTrue(t *testing.T) {
	opts := &GlobalOptions{}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	RegisterGlobalFlags(fs, opts)

	// Parse with --debug flag
	if err := fs.Parse([]string{"--debug"}); err != nil {
		t.Fatalf("unexpected error during parse: %v", err)
	}

	if opts.Debug != true {
		t.Errorf("expected Debug=true when --debug is passed, got %v", opts.Debug)
	}
}

// TestDebugFlagWithProfile verifies that --debug and --profile work together.
func TestDebugFlagWithProfile(t *testing.T) {
	opts := &GlobalOptions{}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	RegisterGlobalFlags(fs, opts)

	// Parse with both --debug and --profile flags
	if err := fs.Parse([]string{"--debug", "--profile", "myprofile"}); err != nil {
		t.Fatalf("unexpected error during parse: %v", err)
	}

	if opts.Debug != true {
		t.Errorf("expected Debug=true, got %v", opts.Debug)
	}
	if opts.Profile != "myprofile" {
		t.Errorf("expected Profile=myprofile, got %s", opts.Profile)
	}
}

// TestDebugFlagReversedOrder verifies that flag order doesn't matter (--profile then --debug).
func TestDebugFlagReversedOrder(t *testing.T) {
	opts := &GlobalOptions{}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	RegisterGlobalFlags(fs, opts)

	// Parse with flags in reversed order: --profile then --debug
	if err := fs.Parse([]string{"--profile", "myprofile", "--debug"}); err != nil {
		t.Fatalf("unexpected error during parse: %v", err)
	}

	if opts.Debug != true {
		t.Errorf("expected Debug=true, got %v", opts.Debug)
	}
	if opts.Profile != "myprofile" {
		t.Errorf("expected Profile=myprofile, got %s", opts.Profile)
	}
}
