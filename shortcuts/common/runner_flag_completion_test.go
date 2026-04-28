// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// TestShortcutMount_FlagCompletionsRegistered exercises the two
// cmdutil.RegisterFlagCompletion call sites in registerShortcutFlagsWithContext:
// the per-flag enum completion (runner.go:879) and the auto-injected --format
// completion (runner.go:895).
func TestShortcutMount_FlagCompletionsRegistered(t *testing.T) {
	t.Cleanup(func() { cmdutil.SetFlagCompletionsEnabled(false) })
	cmdutil.SetFlagCompletionsEnabled(true)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	parent := &cobra.Command{Use: "root"}
	shortcut := Shortcut{
		Service:     "docs",
		Command:     "+fetch",
		Description: "fetch doc",
		HasFormat:   true,
		Flags: []Flag{
			{Name: "sort-by", Desc: "sort", Enum: []string{"asc", "desc"}},
		},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	}
	shortcut.Mount(parent, f)

	cmd, _, err := parent.Find([]string{"+fetch"})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}

	// Enum flag completion.
	fn, ok := cmd.GetFlagCompletionFunc("sort-by")
	if !ok {
		t.Fatal("expected completion func for --sort-by")
	}
	got, _ := fn(cmd, nil, "")
	if len(got) != 2 || got[0] != "asc" || got[1] != "desc" {
		t.Fatalf("sort-by completion = %v, want [asc desc]", got)
	}

	// HasFormat-injected --format completion.
	fn, ok = cmd.GetFlagCompletionFunc("format")
	if !ok {
		t.Fatal("expected completion func for --format")
	}
	got, _ = fn(cmd, nil, "")
	want := []string{"json", "pretty", "table", "ndjson", "csv"}
	if len(got) != len(want) {
		t.Fatalf("format completion = %v, want %v", got, want)
	}
	for i, v := range want {
		if got[i] != v {
			t.Fatalf("format completion[%d] = %q, want %q", i, got[i], v)
		}
	}
}

// TestShortcutMount_FlagCompletionsDisabled verifies the switch actually
// prevents the two registrations from landing in cobra's global map.
func TestShortcutMount_FlagCompletionsDisabled(t *testing.T) {
	t.Cleanup(func() { cmdutil.SetFlagCompletionsEnabled(false) })
	cmdutil.SetFlagCompletionsEnabled(false)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	parent := &cobra.Command{Use: "root"}
	shortcut := Shortcut{
		Service:     "docs",
		Command:     "+fetch",
		Description: "fetch doc",
		HasFormat:   true,
		Flags: []Flag{
			{Name: "sort-by", Desc: "sort", Enum: []string{"asc", "desc"}},
		},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	}
	shortcut.Mount(parent, f)

	cmd, _, err := parent.Find([]string{"+fetch"})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if _, ok := cmd.GetFlagCompletionFunc("sort-by"); ok {
		t.Fatal("did not expect completion func for --sort-by when disabled")
	}
	if _, ok := cmd.GetFlagCompletionFunc("format"); ok {
		t.Fatal("did not expect completion func for --format when disabled")
	}
}
