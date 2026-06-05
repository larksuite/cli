// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy_test

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdpolicy"
)

// configPolicyTree builds the minimal slice of the real command tree
// where diagnostic exemption applies: root -> config -> policy -> show.
func configPolicyTree() *cobra.Command {
	root := &cobra.Command{Use: "lark-cli"}
	config := &cobra.Command{Use: "config"}
	root.AddCommand(config)
	policy := &cobra.Command{Use: "policy"}
	config.AddCommand(policy)
	policy.AddCommand(&cobra.Command{Use: "show", RunE: noop})
	// Plus an unrelated command that the Rule will deny, to anchor the
	// "everything except diagnostics" check.
	im := &cobra.Command{Use: "im"}
	root.AddCommand(im)
	im.AddCommand(&cobra.Command{Use: "+send", RunE: noop})
	return root
}

func TestEvaluate_diagnosticAllowedDespiteStrictAllow(t *testing.T) {
	root := configPolicyTree()
	// Rule that allows ONLY docs/** -- normally locks out everything else.
	e := cmdpolicy.New(&platform.Rule{
		Allow: []string{"docs/**"},
	})
	got := e.EvaluateAll(root)

	if !got["config/policy/show"].Allowed {
		t.Errorf("config/policy/show must be unconditionally allowed; got Allowed=false reason=%q",
			got["config/policy/show"].ReasonCode)
	}
	// Sanity: a non-diagnostic command is still denied so we know the
	// rule itself is active.
	if got["im/+send"].Allowed {
		t.Errorf("im/+send should be denied by Allow=[docs/**]; got Allowed=true")
	}
}

func TestEvaluate_diagnosticAllowedDespiteExplicitDeny(t *testing.T) {
	// Even a Rule that explicitly Denies the path must not lock the
	// operator out -- diagnostic is a permanent hole. If a security-
	// sensitive deployment needs to block introspection, they should
	// strip the binary, not rely on Rule.
	root := configPolicyTree()
	e := cmdpolicy.New(&platform.Rule{
		Allow: []string{"**"},
		Deny:  []string{"config/policy/**"},
	})
	got := e.EvaluateAll(root)

	if !got["config/policy/show"].Allowed {
		t.Errorf("config/policy/show must override explicit Deny; got Allowed=false reason=%q",
			got["config/policy/show"].ReasonCode)
	}
}

func TestIsDiagnosticPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"config/policy/show", true},
		{"config/plugins/show", true},
		{"config/policy", false},  // parent group itself is not exempt
		{"config/plugins", false}, // parent group itself is not exempt
		{"docs/+fetch", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := cmdpolicy.IsDiagnosticPath(tc.path); got != tc.want {
			t.Errorf("IsDiagnosticPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
