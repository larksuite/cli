// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestDisableAuthCheck(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	if IsAuthCheckDisabled(cmd) {
		t.Error("expected auth check to be enabled by default")
	}

	DisableAuthCheck(cmd)
	if !IsAuthCheckDisabled(cmd) {
		t.Error("expected auth check to be disabled after DisableAuthCheck")
	}
}

func TestIsAuthCheckDisabled_Inheritance(t *testing.T) {
	parent := &cobra.Command{Use: "parent"}
	child := &cobra.Command{Use: "child"}
	parent.AddCommand(child)

	if IsAuthCheckDisabled(child) {
		t.Error("expected child auth check enabled before parent annotation")
	}

	DisableAuthCheck(parent)
	if !IsAuthCheckDisabled(child) {
		t.Error("expected child to inherit disabled auth check from parent")
	}
}

func TestIsAuthCheckDisabled_NoInheritanceUpward(t *testing.T) {
	parent := &cobra.Command{Use: "parent"}
	child := &cobra.Command{Use: "child"}
	parent.AddCommand(child)

	DisableAuthCheck(child)
	if IsAuthCheckDisabled(parent) {
		t.Error("parent should not inherit disabled auth check from child")
	}
	if !IsAuthCheckDisabled(child) {
		t.Error("child should have disabled auth check")
	}
}

func TestSetGetSupportedIdentities(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	if got := GetSupportedIdentities(cmd); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	SetSupportedIdentities(cmd, []string{"user", "bot"})
	got := GetSupportedIdentities(cmd)
	if len(got) != 2 || got[0] != "user" || got[1] != "bot" {
		t.Errorf("expected [user bot], got %v", got)
	}
}

func TestSetSupportedIdentities_OverwriteExisting(t *testing.T) {
	cmd := &cobra.Command{Use: "test", Annotations: map[string]string{"other": "val"}}
	SetSupportedIdentities(cmd, []string{"bot"})
	if cmd.Annotations["other"] != "val" {
		t.Error("existing annotation should be preserved")
	}
	got := GetSupportedIdentities(cmd)
	if len(got) != 1 || got[0] != "bot" {
		t.Errorf("expected [bot], got %v", got)
	}
}
