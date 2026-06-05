// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdmeta_test

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
)

func TestApply_writesAllFields(t *testing.T) {
	cmd := &cobra.Command{Use: "fetch"}
	cmdmeta.Apply(cmd, cmdmeta.Meta{
		Domain:     "docs",
		Risk:       "write",
		Identities: []string{"user", "bot"},
	})

	if got := cmdmeta.Domain(cmd); got != "docs" {
		t.Fatalf("Domain = %q, want %q", got, "docs")
	}
	if got, ok := cmdmeta.Risk(cmd); !ok || got != "write" {
		t.Fatalf("Risk = (%q,%v), want (%q,true)", got, ok, "write")
	}
	if got := cmdmeta.Identities(cmd); !reflect.DeepEqual(got, []string{"user", "bot"}) {
		t.Fatalf("Identities = %v, want [user bot]", got)
	}
}

func TestApply_emptyFieldsSkipped(t *testing.T) {
	cmd := &cobra.Command{Use: "fetch"}
	cmdmeta.Apply(cmd, cmdmeta.Meta{}) // nothing
	if got := cmdmeta.Domain(cmd); got != "" {
		t.Fatalf("Domain expected unset, got %q", got)
	}
	if _, ok := cmdmeta.Risk(cmd); ok {
		t.Fatalf("Risk expected unset")
	}
	if got := cmdmeta.Identities(cmd); got != nil {
		t.Fatalf("Identities expected nil, got %v", got)
	}
}

// Domain inherits from the nearest ancestor; risk and identities behave the
// same way. We verify each axis with a 3-level tree:
//
//	root (domain=docs, risk=read, identities=[user])
//	  group
//	    leaf
func TestGet_inheritsFromAncestor(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	group := &cobra.Command{Use: "docs"}
	leaf := &cobra.Command{Use: "fetch"}
	root.AddCommand(group)
	group.AddCommand(leaf)

	cmdmeta.Apply(root, cmdmeta.Meta{
		Domain:     "docs",
		Risk:       "read",
		Identities: []string{"user"},
	})

	got := cmdmeta.Get(leaf)
	want := cmdmeta.Meta{
		Domain:     "docs",
		Risk:       "read",
		Identities: []string{"user"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Get(leaf) = %+v, want %+v", got, want)
	}
}

// Closest ancestor wins -- a mid-level override is preferred over root.
func TestGet_nearestAncestorWins(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	group := &cobra.Command{Use: "docs"}
	leaf := &cobra.Command{Use: "fetch"}
	root.AddCommand(group)
	group.AddCommand(leaf)

	cmdmeta.SetDomain(root, "docs")
	cmdmeta.SetDomain(group, "docs-override")
	cmdutil.SetRisk(root, "read")
	cmdutil.SetRisk(group, "high-risk-write")

	if got := cmdmeta.Domain(leaf); got != "docs-override" {
		t.Fatalf("Domain = %q, want docs-override (nearest)", got)
	}
	if got, _ := cmdmeta.Risk(leaf); got != "high-risk-write" {
		t.Fatalf("Risk = %q, want high-risk-write (nearest)", got)
	}
}

// Unknown axes return zero / nil so the policy engine can apply the
// "unknown => ALLOW" contract.
func TestGet_unknownReturnsZero(t *testing.T) {
	cmd := &cobra.Command{Use: "orphan"}
	if got := cmdmeta.Domain(cmd); got != "" {
		t.Fatalf("Domain = %q, want empty for unknown", got)
	}
	if level, ok := cmdmeta.Risk(cmd); ok || level != "" {
		t.Fatalf("Risk = (%q,%v), want empty / false for unknown", level, ok)
	}
	if ids := cmdmeta.Identities(cmd); ids != nil {
		t.Fatalf("Identities = %v, want nil for unknown", ids)
	}
}

// Child explicitly overriding identities stops the parent walk.
func TestIdentities_childOverridesParent(t *testing.T) {
	parent := &cobra.Command{Use: "docs"}
	child := &cobra.Command{Use: "preview"}
	parent.AddCommand(child)

	cmdutil.SetSupportedIdentities(parent, []string{"user", "bot"})
	cmdutil.SetSupportedIdentities(child, []string{"bot"})

	got := cmdmeta.Identities(child)
	if !reflect.DeepEqual(got, []string{"bot"}) {
		t.Fatalf("Identities(child) = %v, want [bot]", got)
	}
}

// SetDomain with empty value is a no-op (no annotation written, so a
// later inherited read still works).
func TestSetDomain_emptyIsNoop(t *testing.T) {
	parent := &cobra.Command{Use: "docs"}
	cmdmeta.SetDomain(parent, "docs")

	child := &cobra.Command{Use: "fetch"}
	parent.AddCommand(child)

	cmdmeta.SetDomain(child, "") // no-op
	if got := cmdmeta.Domain(child); got != "docs" {
		t.Fatalf("Domain(child) = %q, want inherited 'docs'", got)
	}
}
