// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/output"
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

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected denial ExitError, got %v", err)
	}
	detail := exitErr.Detail.Detail.(map[string]any)
	src, _ := detail["policy_source"].(string)
	if src != "yaml" {
		t.Errorf("policy_source = %q, want %q (no path leak)", src, "yaml")
	}
	// rule_name carries the disambiguating identifier.
	if detail["rule_name"] != "my-readonly-rule" {
		t.Errorf("rule_name = %v, want my-readonly-rule", detail["rule_name"])
	}
	// Direct probe: the absolute path must not appear anywhere in the
	// envelope detail (key OR value).
	for k, v := range detail {
		if strings.Contains(k, "/Users/alice") || strings.Contains(asString(v), "/Users/alice") {
			t.Errorf("envelope detail must not leak '/Users/alice', found in %s = %v", k, v)
		}
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
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError")
	}
	detail := exitErr.Detail.Detail.(map[string]any)
	if detail["policy_source"] != "plugin:secaudit" {
		t.Errorf("policy_source = %v, want plugin:secaudit", detail["policy_source"])
	}
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
