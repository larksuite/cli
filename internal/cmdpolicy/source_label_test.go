// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdpolicy"
)

// The envelope's policy_source must never leak the absolute home path.
// "yaml:/Users/alice/.lark-cli/policy.yml" would expose Alice's username
// to any agent or log consumer; the contract is to emit just "yaml" and
// rely on rule_name (from the yaml's "name:" field) for disambiguation.
func TestEnvelope_yamlPolicySourceDoesNotLeakHomePath(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	docs := &cobra.Command{Use: "docs"}
	root.AddCommand(docs)
	leaf := &cobra.Command{Use: "+write", RunE: func(*cobra.Command, []string) error { return nil }}
	docs.AddCommand(leaf)

	e := cmdpolicy.New(&platform.Rule{
		Name:  "my-readonly-rule",
		Allow: []string{"contact/**"}, // docs/* falls outside, denied
	})
	denied := cmdpolicy.BuildDeniedByPath(root, e.EvaluateAll(root),
		cmdpolicy.ResolveSource{
			Kind: cmdpolicy.SourceYAML,
			Name: "/Users/alice/.lark-cli/policy.yml", // simulate an absolute path
		}, "my-readonly-rule")

	cmdpolicy.Apply(root, denied)
	err := leaf.RunE(leaf, nil)

	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected denial *errs.ValidationError, got %T %v", err, err)
	}
	// The policy source is folded into the Hint as "yaml" -- the bare
	// kind, never the absolute path.
	if !strings.Contains(ve.Hint, "source yaml") {
		t.Errorf("hint must carry policy_source %q (no path leak), got %q", "yaml", ve.Hint)
	}
	// rule_name carries the disambiguating identifier.
	if !strings.Contains(ve.Hint, "my-readonly-rule") {
		t.Errorf("hint must carry rule_name my-readonly-rule, got %q", ve.Hint)
	}
	// Direct privacy probe: the absolute home path must not appear
	// anywhere in the user-facing message OR hint text.
	if strings.Contains(ve.Message, "/Users/alice") {
		t.Errorf("error message must not leak '/Users/alice', got %q", ve.Message)
	}
	if strings.Contains(ve.Hint, "/Users/alice") {
		t.Errorf("error hint must not leak '/Users/alice', got %q", ve.Hint)
	}
}

// Plugin name IS allowed in policy_source because plugins are in-binary
// and their names are part of the contract (an integrator debugging a
// denial wants to know which plugin fired). This test pins that intent
// so a future change does not silently strip the plugin name too.
func TestEnvelope_pluginPolicySourceCarriesName(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	leaf := &cobra.Command{Use: "+block", RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(leaf)

	e := cmdpolicy.New(&platform.Rule{
		Name: "secaudit-policy",
		Deny: []string{"+block"},
	})
	denied := cmdpolicy.BuildDeniedByPath(root, e.EvaluateAll(root),
		cmdpolicy.ResolveSource{Kind: cmdpolicy.SourcePlugin, Name: "secaudit"},
		"secaudit-policy")
	cmdpolicy.Apply(root, denied)

	err := leaf.RunE(leaf, nil)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	// The plugin name IS surfaced (in-binary, part of the contract): it
	// must appear in the Hint so an integrator debugging a denial knows
	// which plugin fired.
	if !strings.Contains(ve.Hint, "plugin:secaudit") {
		t.Errorf("hint must carry policy_source plugin:secaudit, got %q", ve.Hint)
	}
}
