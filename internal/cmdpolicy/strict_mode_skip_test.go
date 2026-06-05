// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy_test

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdpolicy"
)

// cmdpolicy.Apply MUST NOT overwrite the denial annotation on a command
// already marked as strict-mode denied. strict-mode is a hard boundary
// (credential-derived); a user-layer rule cannot relabel or replace
// the error path.
//
// Without this invariant: when a user yaml rule happened to match the
// path of a strict-mode stub, Apply would change layer=strict_mode to
// layer=pruning, and the user-visible error would say "denied by yaml"
// instead of "strict mode". The hard-boundary contract demands
// strict_mode wins.
func TestApply_PreservesStrictModeAnnotation(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	stub := &cobra.Command{
		Use:    "victim",
		Hidden: true,
		Annotations: map[string]string{
			cmdpolicy.AnnotationDenialLayer:  cmdpolicy.LayerStrictMode,
			cmdpolicy.AnnotationDenialSource: "strict-mode",
		},
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	root.AddCommand(stub)

	// User-layer pruning denies the same path.
	denied := map[string]cmdpolicy.Denial{
		"victim": {
			Layer:        cmdpolicy.LayerPolicy,
			PolicySource: "yaml",
			Reason:       "denied by user yaml",
			ReasonCode:   "command_denylisted",
		},
	}
	cmdpolicy.Apply(root, denied)

	if got := stub.Annotations[cmdpolicy.AnnotationDenialLayer]; got != cmdpolicy.LayerStrictMode {
		t.Errorf("strict-mode layer overwritten by pruning: got %q want %q",
			got, cmdpolicy.LayerStrictMode)
	}
	if got := stub.Annotations[cmdpolicy.AnnotationDenialSource]; got != "strict-mode" {
		t.Errorf("strict-mode source overwritten: got %q", got)
	}
}

// Regression for codex H13 / C6: a denied command that carries
// flag-like positional args (because DisableFlagParsing=true makes
// every `--doc xxx` look positional) MUST surface the pruning
// envelope, not a cobra usage error. Pre-fix, the original command's
// Args validator (e.g. cobra.NoArgs from shortcut registration) would
// fire BEFORE PersistentPreRunE / RunE and produce
// "Error: positional arguments are not supported".
//
// Fix: installDenyStub sets Args=ArbitraryArgs so cobra's validate
// step always passes, letting dispatch reach the wrapped RunE.
func TestApply_DenyStubBypassesArgsValidator(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	leaf := &cobra.Command{
		Use:  "+update",
		Args: cobra.NoArgs, // shortcut style: refuse all positional args
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	root.AddCommand(leaf)

	denied := map[string]cmdpolicy.Denial{
		"+update": {
			Layer:        cmdpolicy.LayerPolicy,
			PolicySource: "yaml",
			ReasonCode:   "command_denylisted",
			Reason:       "denied by user yaml",
		},
	}
	cmdpolicy.Apply(root, denied)

	if leaf.Args == nil {
		t.Fatal("denied command must have non-nil Args validator after Apply")
	}
	// ArbitraryArgs returns nil for every input -> Args validation no-ops.
	if err := leaf.Args(leaf, []string{"--doc", "xxx", "--mode", "append"}); err != nil {
		t.Errorf("denied command Args validator should accept any input, got %v", err)
	}
}

// Regression for codex C11 / C13: a denied command whose PARENT
// declares a PersistentPreRunE (e.g. cmd/auth/auth.go's
// external_provider check) MUST surface the pruning envelope, not
// the parent's error. Cobra's "first PersistentPreRunE walking up
// from leaf wins" semantics will pick the parent's PersistentPreRunE
// unless the denied leaf carries its own.
//
// Fix: installDenyStub installs a no-op PersistentPreRunE on the leaf
// so cobra stops there and proceeds to the wrapped RunE (which holds
// the real pruning envelope).
func TestApply_DenyStubBypassesParentPersistentPreRunE(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	parent := &cobra.Command{
		Use: "auth",
		PersistentPreRunE: func(*cobra.Command, []string) error {
			return errors.New("parent PersistentPreRunE fired (would mask pruning)")
		},
	}
	root.AddCommand(parent)
	leaf := &cobra.Command{
		Use:  "login",
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	parent.AddCommand(leaf)

	denied := map[string]cmdpolicy.Denial{
		"auth/login": {
			Layer:        cmdpolicy.LayerPolicy,
			PolicySource: "yaml",
			ReasonCode:   "identity_mismatch",
			Reason:       "denied",
		},
	}
	cmdpolicy.Apply(root, denied)

	if leaf.PersistentPreRunE == nil {
		t.Fatal("denied command must have leaf-level PersistentPreRunE")
	}
	// Our PersistentPreRunE must NOT propagate the parent's error.
	if err := leaf.PersistentPreRunE(leaf, nil); err != nil {
		t.Errorf("denied command leaf PersistentPreRunE should be no-op, got %v", err)
	}
}

// Sanity: a normal command (no prior annotation) still gets the
// pruning denial annotations after Apply.
func TestApply_NonStrictCommandStillGetsPruningAnnotation(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	leaf := &cobra.Command{
		Use:  "normal",
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	root.AddCommand(leaf)

	denied := map[string]cmdpolicy.Denial{
		"normal": {
			Layer:        cmdpolicy.LayerPolicy,
			PolicySource: "yaml",
			Reason:       "denied",
			ReasonCode:   "command_denylisted",
		},
	}
	cmdpolicy.Apply(root, denied)

	if got := leaf.Annotations[cmdpolicy.AnnotationDenialLayer]; got != cmdpolicy.LayerPolicy {
		t.Errorf("expected pruning layer annotation, got %q", got)
	}
}
