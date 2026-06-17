// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
)

// tmpHome creates a tempdir, points $HOME at it, and returns the path to
// the ~/.lark-cli/ subdirectory (created). The HOME env var is restored
// when the test ends.
//
// LARKSUITE_CLI_CONFIG_DIR is force-set to the same path. Without that
// override, a developer running the tests with a personal
// LARKSUITE_CLI_CONFIG_DIR exported in their shell (or a CI runner with
// a baked-in value) would resolve userPolicyPath() to their real
// machine and bleed unrelated yaml into the test fixtures. With the
// override pinned here, the test is hermetic regardless of the host
// environment.
func tmpHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir) // Windows fallback for os.UserHomeDir
	cfgDir := filepath.Join(dir, ".lark-cli")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", cfgDir)
	return cfgDir
}

// writePolicy writes a policy.yml into the user config dir.
func writePolicy(t *testing.T, cfgDir string, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(cfgDir, "policy.yml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}
}

// fakeTree builds a minimal command tree with the same shape the real
// CLI exposes for these tests: lark-cli has a docs group with +fetch and
// +update, and an im group with +send. Each leaf has its risk_level set
// so MaxRisk filtering exercises a real path.
func fakeTree(t *testing.T) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "lark-cli"}

	docs := &cobra.Command{Use: "docs"}
	root.AddCommand(docs)
	addLeaf(docs, "+fetch", "read")
	addLeaf(docs, "+update", "write")
	addLeaf(docs, "+delete-doc", "high-risk-write")

	im := &cobra.Command{Use: "im"}
	root.AddCommand(im)
	addLeaf(im, "+send", "write")

	return root
}

func addLeaf(parent *cobra.Command, use, risk string) {
	leaf := &cobra.Command{
		Use:  use,
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	cmdutil.SetRisk(leaf, risk)
	parent.AddCommand(leaf)
}

// findLeaf walks the tree by Use names.
func findLeaf(t *testing.T, parent *cobra.Command, names ...string) *cobra.Command {
	t.Helper()
	cur := parent
	for _, n := range names {
		var next *cobra.Command
		for _, c := range cur.Commands() {
			if c.Use == n {
				next = c
				break
			}
		}
		if next == nil {
			t.Fatalf("child %q not found under %q", n, cur.Use)
		}
		cur = next
	}
	return cur
}

// Happy path: a valid policy.yml denies one specific command. The denied
// command's RunE returns a typed error envelope; allowed commands are
// untouched.
func TestApplyUserPolicyPruning_appliesValidPolicy(t *testing.T) {
	cfgDir := tmpHome(t)
	writePolicy(t, cfgDir, `
name: test-policy
allow: ["docs/**", "contact/**"]
deny: ["docs/+delete-doc"]
max_risk: write
`)

	root := fakeTree(t)
	if err := applyUserPolicyPruning(root, nil); err != nil {
		t.Fatalf("apply policy: %v", err)
	}

	// docs/+delete-doc must be denied (Deny match).
	deleteCmd := findLeaf(t, root, "docs", "+delete-doc")
	if !deleteCmd.Hidden {
		t.Errorf("+delete-doc should be hidden after pruning")
	}
	err := deleteCmd.RunE(deleteCmd, nil)
	if err == nil {
		t.Fatalf("+delete-doc RunE should return an error")
	}
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *errs.ValidationError, got %T %+v", err, err)
	}
	if verr.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q, want failed_precondition", verr.Subtype)
	}
	if code := output.ExitCodeOf(err); code != output.ExitValidation {
		t.Errorf("exit code = %d, want %d (ExitValidation)", code, output.ExitValidation)
	}
	// The denial taxonomy (reason_code, layer, rule) is preserved on the
	// wrapped *platform.CommandDeniedError cause and folded into the hint.
	var cd *platform.CommandDeniedError
	if !errors.As(err, &cd) {
		t.Fatalf("error chain should expose *platform.CommandDeniedError")
	}
	if cd.ReasonCode != "command_denylisted" {
		t.Errorf("CommandDeniedError.ReasonCode = %q, want command_denylisted", cd.ReasonCode)
	}
	if !strings.Contains(verr.Hint, "command_denylisted") {
		t.Errorf("hint should surface reason_code command_denylisted, got %q", verr.Hint)
	}

	// im/+send must be denied (domain not in Allow).
	send := findLeaf(t, root, "im", "+send")
	if !send.Hidden {
		t.Errorf("im/+send should be hidden (not in Allow)")
	}

	// docs/+update must stay alive (domain matches, risk within max).
	update := findLeaf(t, root, "docs", "+update")
	if update.Hidden {
		t.Errorf("docs/+update should remain visible")
	}
	if err := update.RunE(update, nil); err != nil {
		t.Errorf("docs/+update RunE should succeed, got %v", err)
	}
}

// Missing file means no pruning -- the CLI runs unrestricted with the
// full command surface. This is the default case for users who haven't
// opted into pruning.
func TestApplyUserPolicyPruning_missingFileIsSilent(t *testing.T) {
	tmpHome(t) // home set but no policy.yml written

	root := fakeTree(t)
	if err := applyUserPolicyPruning(root, nil); err != nil {
		t.Fatalf("missing policy should not error, got %v", err)
	}

	// Every leaf must remain non-Hidden.
	for _, sub := range []string{"+fetch", "+update", "+delete-doc"} {
		cmd := findLeaf(t, root, "docs", sub)
		if cmd.Hidden {
			t.Errorf("%s should not be Hidden when no policy file exists", sub)
		}
	}
}

// Invalid yaml content (parse error) surfaces as an error from the
// wiring. The build path then decides whether to fail-open or
// fail-closed; the wiring itself stays neutral.
func TestApplyUserPolicyPruning_malformedYamlReturnsError(t *testing.T) {
	cfgDir := tmpHome(t)
	writePolicy(t, cfgDir, "::: not yaml :::")

	root := fakeTree(t)
	err := applyUserPolicyPruning(root, nil)
	if err == nil {
		t.Fatalf("malformed yaml should produce an error")
	}
}

// When a plugin contributed rules, a malformed user policy.yml must NOT
// abort: plugin rules shadow yaml entirely, so the broken file is never
// read. Regression -- previously LoadYAMLPolicy ran first and an
// unrelated broken yaml on the user's machine could fatal a
// plugin-governed binary (build.go fail-CLOSES on policy errors when a
// plugin is present).
func TestApplyUserPolicyPruning_pluginRulesSkipBrokenYaml(t *testing.T) {
	cfgDir := tmpHome(t)
	t.Cleanup(cmdpolicy.ResetActiveForTesting)
	writePolicy(t, cfgDir, "::: not yaml :::") // broken on purpose

	pluginRules := []cmdpolicy.PluginRule{
		{PluginName: "secaudit", Rule: &platform.Rule{
			Name:    "docs-only",
			Allow:   []string{"docs/**"},
			MaxRisk: "write",
		}},
	}
	root := fakeTree(t)
	if err := applyUserPolicyPruning(root, pluginRules); err != nil {
		t.Fatalf("plugin rules must shadow (and skip reading) yaml; broken yaml should not error, got %v", err)
	}

	// Plugin rule actually applied: im/+send is outside docs/** -> hidden.
	if send := findLeaf(t, root, "im", "+send"); !send.Hidden {
		t.Errorf("im/+send should be hidden by plugin rule (not in docs/** allow)")
	}
	// docs/+update is within allow and at/below max_risk -> stays visible.
	if update := findLeaf(t, root, "docs", "+update"); update.Hidden {
		t.Errorf("docs/+update should remain visible under plugin rule")
	}
}

// Semantically-invalid Rule (bad MaxRisk) reaches ValidateRule inside
// Resolve and produces an error. This is the safety contract: a typo in
// the rule must not silently lower the pruning bar.
func TestApplyUserPolicyPruning_invalidRuleReturnsError(t *testing.T) {
	cfgDir := tmpHome(t)
	writePolicy(t, cfgDir, "max_risk: nukem\n")

	root := fakeTree(t)
	err := applyUserPolicyPruning(root, nil)
	if err == nil {
		t.Fatalf("invalid MaxRisk should produce an error")
	}
}

// warnPolicyError emits to the supplied writer when err is non-nil and
// stays silent for nil. Verifies the build.go fail-open behaviour can be
// observed by users.
func TestWarnPolicyError(t *testing.T) {
	var buf bytes.Buffer
	warnPolicyError(&buf, nil)
	if buf.Len() != 0 {
		t.Fatalf("warnPolicyError with nil err should write nothing, got %q", buf.String())
	}

	buf.Reset()
	warnPolicyError(&buf, errors.New("boom"))
	if buf.String() != "warning: user policy not applied: boom\n" {
		t.Fatalf("warnPolicyError output = %q", buf.String())
	}
}

// End-to-end through buildInternal: when a valid policy.yml exists in
// HOME, building the real command tree applies pruning to it. This is
// the "actually integrated" test -- it exercises the wiring point in
// build.go itself, not just the helper.
func TestBuildInternal_appliesPolicyToRealTree(t *testing.T) {
	cfgDir := tmpHome(t)
	// Deny one specific shortcut path that we know exists in the real
	// service tree -- we cannot enumerate it from a unit test, so we
	// use an Allow-list that matches nothing to deny everything except
	// the root, and then verify ANY non-root command was hidden.
	writePolicy(t, cfgDir, `
name: deny-everything
deny: ["**"]
`)

	root := Build(context.Background(), buildInvocationForTest(t))

	// Find any leaf and verify it was hidden.
	var foundHidden bool
	walk(root, func(c *cobra.Command) {
		if c.HasParent() && c.Runnable() && c.Hidden {
			foundHidden = true
		}
	})
	if !foundHidden {
		t.Fatalf("expected at least one runnable command to be Hidden after deny=** policy")
	}

	// Root itself must stay alive.
	if root.Hidden {
		t.Errorf("root command must not be Hidden even under deny-everything policy")
	}
}

func walk(cmd *cobra.Command, fn func(*cobra.Command)) {
	if cmd == nil {
		return
	}
	fn(cmd)
	for _, c := range cmd.Commands() {
		walk(c, fn)
	}
}

// buildInvocationForTest returns a minimal cmdutil.InvocationContext so
// build.go's pure-assembly path can construct a tree without touching
// real config / credentials. Profile name is the empty default.
func buildInvocationForTest(t *testing.T) cmdutil.InvocationContext {
	t.Helper()
	return cmdutil.InvocationContext{}
}
