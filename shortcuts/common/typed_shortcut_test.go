// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"slices"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
)

type stubArgs struct{}

func newTypedFixture() TypedShortcut[*stubArgs] {
	return TypedShortcut[*stubArgs]{
		Service:     "im",
		Command:     "+demo",
		Description: "demo",
		AuthTypes:   []string{"user"},
		Risk:        "write",
		Scopes:      []string{"x"},
	}
}

func TestTypedShortcut_Descriptors(t *testing.T) {
	ts := newTypedFixture()
	if ts.GetService() != "im" {
		t.Errorf("GetService=%q", ts.GetService())
	}
	if ts.GetCommand() != "+demo" {
		t.Errorf("GetCommand=%q", ts.GetCommand())
	}
	if ts.GetDescription() != "demo" {
		t.Errorf("GetDescription=%q", ts.GetDescription())
	}
	if ts.GetRisk() != "write" {
		t.Errorf("GetRisk=%q", ts.GetRisk())
	}
}

func TestTypedShortcut_SatisfiesMountable(t *testing.T) {
	var _ Mountable = newTypedFixture()
}

type adapterArgs struct {
	Name string `flag:"name"`
}

// TestMountTyped_RegistersFlags verifies the mountTyped adapter wires the
// binder-registered flag into cobra. Full Validate/Execute integration is
// covered by tests_e2e/shortcuts/ (out of unit-test scope — runShortcut
// needs a fully-initialized Factory with auth/config).
func TestMountTyped_RegistersFlags(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	ts := TypedShortcut[*adapterArgs]{
		Service: "x", Command: "+demo", AuthTypes: []string{"user"},
		Risk: "read",
		Execute: func(ctx context.Context, args *adapterArgs, rt *RuntimeContext) error {
			return nil
		},
	}
	ts.MountWithContext(context.Background(), root, &cmdutil.Factory{})
	sub, _, err := root.Find([]string{"+demo"})
	if err != nil {
		t.Fatalf("find subcommand: %v", err)
	}
	if sub.Flag("name") == nil {
		t.Error("expected --name flag to be registered via binder")
	}
}

func TestMountTyped_HelpFuncInstalled(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	ts := TypedShortcut[*adapterArgs]{
		Service: "x", Command: "+demo", AuthTypes: []string{"user"},
		Risk:     "read",
		Examples: []HelpExample{{Title: "demo", Cmd: "--name alice"}},
		Execute:  func(ctx context.Context, args *adapterArgs, rt *RuntimeContext) error { return nil },
	}
	ts.MountWithContext(context.Background(), root, &cmdutil.Factory{})
	sub, _, _ := root.Find([]string{"+demo"})
	if sub == nil || sub.HelpFunc() == nil {
		t.Fatal("expected typed help func installed on subcommand")
	}
}

// TestTypedShortcut_Mount verifies the no-context convenience Mount API still
// works after migration, mirroring legacy Shortcut.Mount so existing tests of
// migrated shortcuts don't need to switch to MountWithContext.
func TestTypedShortcut_Mount(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	ts := TypedShortcut[*adapterArgs]{
		Service: "x", Command: "+demo", AuthTypes: []string{"user"},
		Risk:    "read",
		Execute: func(ctx context.Context, args *adapterArgs, rt *RuntimeContext) error { return nil },
	}
	ts.Mount(root, &cmdutil.Factory{})
	sub, _, err := root.Find([]string{"+demo"})
	if err != nil || sub == nil {
		t.Fatalf("find subcommand: %v (sub=%v)", err, sub)
	}
}

func TestTypedShortcut_GetAuthTypes(t *testing.T) {
	ts := TypedShortcut[*stubArgs]{AuthTypes: []string{"user", "bot"}}
	if got := ts.GetAuthTypes(); !slices.Equal(got, []string{"user", "bot"}) {
		t.Errorf("GetAuthTypes=%v", got)
	}
	if got := (TypedShortcut[*stubArgs]{}).GetAuthTypes(); got != nil {
		t.Errorf("empty GetAuthTypes=%v, want nil", got)
	}
}

func TestTypedShortcut_ScopesForIdentity(t *testing.T) {
	ts := TypedShortcut[*stubArgs]{
		Scopes:     []string{"base"},
		UserScopes: []string{"u"},
		BotScopes:  []string{"b"},
	}
	cases := []struct {
		identity string
		want     []string
	}{
		{"user", []string{"u"}},
		{"bot", []string{"b"}},
		{"other", []string{"base"}},
		{"", []string{"base"}},
	}
	for _, c := range cases {
		if got := ts.ScopesForIdentity(c.identity); !slices.Equal(got, c.want) {
			t.Errorf("ScopesForIdentity(%q)=%v, want %v", c.identity, got, c.want)
		}
	}
	// Falls back to Scopes when the identity-specific list is empty.
	fallback := TypedShortcut[*stubArgs]{Scopes: []string{"base"}}
	if got := fallback.ScopesForIdentity("user"); !slices.Equal(got, []string{"base"}) {
		t.Errorf("fallback user=%v", got)
	}
}

func TestTypedShortcut_ConditionalScopesForIdentity(t *testing.T) {
	ts := TypedShortcut[*stubArgs]{
		ConditionalScopes:     []string{"cbase"},
		ConditionalUserScopes: []string{"cu"},
		ConditionalBotScopes:  []string{"cb"},
	}
	cases := []struct {
		identity string
		want     []string
	}{
		{"user", []string{"cu"}},
		{"bot", []string{"cb"}},
		{"other", []string{"cbase"}},
	}
	for _, c := range cases {
		if got := ts.ConditionalScopesForIdentity(c.identity); !slices.Equal(got, c.want) {
			t.Errorf("ConditionalScopesForIdentity(%q)=%v, want %v", c.identity, got, c.want)
		}
	}
}

func TestTypedShortcut_DeclaredScopesForIdentity(t *testing.T) {
	// Merges base + conditional, dedupes overlap, drops empty strings.
	ts := TypedShortcut[*stubArgs]{
		UserScopes:            []string{"a", "b", ""},
		ConditionalUserScopes: []string{"b", "c"},
	}
	if got := ts.DeclaredScopesForIdentity("user"); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("merge+dedupe got %v", got)
	}
	// Returns nil when nothing is declared on either side.
	if got := (TypedShortcut[*stubArgs]{}).DeclaredScopesForIdentity("user"); got != nil {
		t.Errorf("empty got %v, want nil", got)
	}
}
