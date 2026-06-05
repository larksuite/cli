// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
)

// EvaluateAll must skip non-runnable parent groups (their decision is
// derived in the aggregation pass). The previous regression: an
// Allow:["docs/**"] rule incorrectly denied the parent "docs" group too,
// because the parent's own path "docs" did not match "docs/**".
func TestEvaluateAll_skipsPureGroups(t *testing.T) {
	root := buildTree() // docs and im are pure groups, +fetch / +update / +send are leaves
	e := cmdpolicy.New(&platform.Rule{Allow: []string{"docs/**"}})
	got := e.EvaluateAll(root)

	if _, present := got["docs"]; present {
		t.Errorf("parent group 'docs' should not appear in Decisions (Allow=docs/**)")
	}
	if _, present := got["im"]; present {
		t.Errorf("parent group 'im' should not appear in Decisions")
	}

	// Children still evaluated normally.
	if !got["docs/+fetch"].Allowed {
		t.Errorf("docs/+fetch should still be allowed by docs/**")
	}
}

// BuildDeniedByPath must aggregate: a parent group whose every runnable
// child is denied must itself get an aggregated Denial in the map.
func TestBuildDeniedByPath_parentAggregationAllChildrenDenied(t *testing.T) {
	// Custom tree where ALL children of "im" will be denied.
	root := &cobra.Command{Use: "lark-cli"}
	im := &cobra.Command{Use: "im"}
	root.AddCommand(im)
	send := &cobra.Command{Use: "+send", RunE: noop}
	cmdutil.SetRisk(send, "write")
	im.AddCommand(send)
	search := &cobra.Command{Use: "+search", RunE: noop}
	cmdutil.SetRisk(search, "read")
	im.AddCommand(search)

	// Risk is set on both leaves so the rejection comes from the Allow
	// axis (the contract this test pins), not from the risk gate.
	e := cmdpolicy.New(&platform.Rule{Allow: []string{"docs/**"}}) // none of im/* matches
	decisions := e.EvaluateAll(root)

	// Pin the rejection axis: both leaves are rejected by Allow miss,
	// NOT by the risk_not_annotated gate. If a future edit drops the
	// SetRisk lines above, this assertion fails and the test stops
	// silently testing the wrong axis.
	if rc := decisions["im/+send"].ReasonCode; rc != "domain_not_allowed" {
		t.Errorf("im/+send ReasonCode = %q, want domain_not_allowed", rc)
	}
	if rc := decisions["im/+search"].ReasonCode; rc != "domain_not_allowed" {
		t.Errorf("im/+search ReasonCode = %q, want domain_not_allowed", rc)
	}

	denied := cmdpolicy.BuildDeniedByPath(root, decisions,
		cmdpolicy.ResolveSource{Kind: cmdpolicy.SourceYAML, Name: "/policy.yml"}, "agent")

	// Both leaves denied.
	if _, ok := denied["im/+send"]; !ok {
		t.Errorf("im/+send should be in denied map")
	}
	if _, ok := denied["im/+search"]; !ok {
		t.Errorf("im/+search should be in denied map")
	}
	// Parent must be aggregated.
	parent, ok := denied["im"]
	if !ok {
		t.Fatalf("parent 'im' should be aggregated into denied map")
	}
	if parent.Layer != "policy" {
		t.Errorf("parent.Layer = %q, want pruning", parent.Layer)
	}
}

// Partial children-denied means parent stays UN-denied. This is the
// counter-case to the previous regression: docs/** allowed children stays
// alive even if some siblings are denied.
func TestBuildDeniedByPath_partialDenialKeepsParent(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	docs := &cobra.Command{Use: "docs"}
	root.AddCommand(docs)

	fetch := &cobra.Command{Use: "+fetch", RunE: noop}
	cmdutil.SetRisk(fetch, "read")
	docs.AddCommand(fetch) // allowed

	delete := &cobra.Command{Use: "+delete", RunE: noop}
	cmdutil.SetRisk(delete, "high-risk-write")
	docs.AddCommand(delete) // denied by Deny

	e := cmdpolicy.New(&platform.Rule{
		Allow: []string{"docs/**"},
		Deny:  []string{"docs/+delete"},
	})
	denied := cmdpolicy.BuildDeniedByPath(root, e.EvaluateAll(root),
		cmdpolicy.ResolveSource{Kind: cmdpolicy.SourcePlugin, Name: "secaudit"}, "secaudit-policy")

	if _, ok := denied["docs"]; ok {
		t.Errorf("parent 'docs' must NOT be denied when some children are allowed")
	}
	if _, ok := denied["docs/+fetch"]; ok {
		t.Errorf("docs/+fetch should not be in denied map (it's allowed)")
	}
	if _, ok := denied["docs/+delete"]; !ok {
		t.Errorf("docs/+delete should be denied (in Deny)")
	}
}

// The binary root is never installed with a denyStub even when all its
// descendants are denied -- the entry point must remain dispatchable.
func TestBuildDeniedByPath_rootNeverDenied(t *testing.T) {
	root := buildTree()
	e := cmdpolicy.New(&platform.Rule{Allow: []string{"nonexistent/**"}})
	denied := cmdpolicy.BuildDeniedByPath(root, e.EvaluateAll(root),
		cmdpolicy.ResolveSource{Kind: cmdpolicy.SourceYAML, Name: "/p.yml"}, "")

	// Every leaf should be denied. We do not assert on the root entry
	// because Apply skips the root regardless; the contract is "root
	// stays dispatchable".
	if _, ok := denied["lark-cli"]; ok {
		t.Errorf("root should not be in denied map")
	}
}

// Hybrid command: a parent with its own RunE plus children. Aggregation
// requires both own RunE denied AND all children denied for the parent
// itself to be marked denied.
func TestBuildDeniedByPath_hybridParentOwnAllowedKeepsAlive(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	docs := &cobra.Command{Use: "docs", RunE: noop} // hybrid: own RunE + subs
	cmdutil.SetRisk(docs, "read")
	root.AddCommand(docs)
	delete := &cobra.Command{Use: "+delete", RunE: noop}
	cmdutil.SetRisk(delete, "high-risk-write")
	docs.AddCommand(delete)

	// Allow "docs" (parent) but deny "+delete" child.
	e := cmdpolicy.New(&platform.Rule{
		Allow: []string{"docs"},
	})
	denied := cmdpolicy.BuildDeniedByPath(root, e.EvaluateAll(root),
		cmdpolicy.ResolveSource{Kind: cmdpolicy.SourceYAML, Name: ""}, "")

	// docs/+delete denied (path doesn't match Allow=["docs"]).
	if _, ok := denied["docs/+delete"]; !ok {
		t.Errorf("docs/+delete should be denied")
	}
	// docs itself allowed (path matches Allow=["docs"] exactly).
	if _, ok := denied["docs"]; ok {
		t.Errorf("docs (hybrid) should NOT be denied -- own RunE is allowed")
	}
}

// Apply with the wrapped *output.ExitError exposes BOTH paths consumers
// rely on:
//  1. cmd/root.go's envelope writer (errors.As on *output.ExitError)
//  2. in-process consumers extracting the platform.CommandDeniedError
func TestApply_runEReturnsExitErrorAndCommandDeniedError(t *testing.T) {
	root := buildTree()
	denied := map[string]cmdpolicy.Denial{
		"docs/+update": {
			Layer:        "policy",
			PolicySource: "plugin:secaudit",
			RuleName:     "secaudit-policy",
			ReasonCode:   "write_not_allowed",
			Reason:       "write disabled",
		},
	}
	cmdpolicy.Apply(root, denied)
	update := findChild(t, root, "docs", "+update")

	err := update.RunE(update, []string{})
	if err == nil {
		t.Fatalf("denied command should return error")
	}

	// Path 1: envelope-writer view.
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error chain must contain *output.ExitError, got %T", err)
	}
	if exitErr.Detail == nil {
		t.Fatalf("ExitError.Detail required for envelope to render")
	}
	if exitErr.Detail.Type != "command_denied" {
		t.Errorf("envelope error.type = %q, want command_denied", exitErr.Detail.Type)
	}
	// JSON envelope shape: detail.reason_code must be present and
	// match the closed enum.
	detailMap, ok := exitErr.Detail.Detail.(map[string]any)
	if !ok {
		t.Fatalf("envelope detail should be map[string]any, got %T", exitErr.Detail.Detail)
	}
	if detailMap["reason_code"] != "write_not_allowed" {
		t.Errorf("detail.reason_code = %v, want write_not_allowed", detailMap["reason_code"])
	}
	if detailMap["policy_source"] != "plugin:secaudit" {
		t.Errorf("detail.policy_source = %v, want plugin:secaudit", detailMap["policy_source"])
	}

	// Path 2: in-process typed-error view.
	var cd *platform.CommandDeniedError
	if !errors.As(err, &cd) {
		t.Fatalf("error chain must expose *platform.CommandDeniedError")
	}
	if cd.Path != "docs/+update" || cd.ReasonCode != "write_not_allowed" {
		t.Errorf("CommandDeniedError = %+v", cd)
	}

	// Envelope round-trip sanity (the actual JSON cmd/root.go would emit).
	var buf strings.Builder
	output.WriteErrorEnvelope(&buf, exitErr, "user")
	if !strings.Contains(buf.String(), `"type": "command_denied"`) {
		t.Errorf("envelope JSON missing type=command_denied, got:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `"reason_code": "write_not_allowed"`) {
		t.Errorf("envelope JSON missing reason_code, got:\n%s", buf.String())
	}
	// Round-trip parse to verify it's well-formed JSON.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(buf.String()), &parsed); err != nil {
		t.Fatalf("envelope JSON malformed: %v\n%s", err, buf.String())
	}
}

// Regression: a pure parent group carrying AnnotationPureGroup must be
// skipped by both EvaluateAll and aggregateParents. Without the skip,
// the cmd.installUnknownSubcommandGuard pass (which attaches a RunE to
// every group for cobra's silent-help fallback) would flip Runnable()
// to true for `docs`, `drive`, etc., and a yaml rule like
// `max_risk: read` would deny every `<group> --help` invocation with
// reason_code = risk_not_annotated.
func TestEvaluateAll_skipsAnnotatedPureGroup(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	drive := &cobra.Command{
		Use:  "drive",
		RunE: func(*cobra.Command, []string) error { return nil }, // emulate guard injection
		Annotations: map[string]string{
			cmdpolicy.AnnotationPureGroup: "true",
		},
	}
	root.AddCommand(drive)
	pull := &cobra.Command{Use: "+pull", RunE: noop}
	cmdutil.SetRisk(pull, "read")
	drive.AddCommand(pull)

	e := cmdpolicy.New(&platform.Rule{MaxRisk: "read"})
	got := e.EvaluateAll(root)

	if d, present := got["drive"]; present {
		t.Errorf("annotated pure group should not appear in Decisions; got %+v", d)
	}
	if !got["drive/+pull"].Allowed {
		t.Errorf("leaf under pure group must still be evaluated; got %+v", got["drive/+pull"])
	}
}

// Regression: hasRunnableDescendant must also treat
// AnnotationPureGroup-tagged commands as non-runnable. Without the
// skip, an entire branch consisting of a pure-group placeholder + a
// single pure-group leaf would advertise itself as a "live" subtree
// and the parent aggregation pass would refuse to install a deny stub
// (allLiveChildrenDenied flips to false because the pure group is
// neither runnable nor in `denied`).
func TestHasRunnableDescendant_ignoresAnnotatedPureGroup(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	docs := &cobra.Command{Use: "docs"}
	root.AddCommand(docs)

	// A pure-group sibling of a real leaf. The parent must still
	// aggregate based on the real leaf alone.
	placeholder := &cobra.Command{
		Use:  "placeholder",
		RunE: func(*cobra.Command, []string) error { return nil },
		Annotations: map[string]string{
			cmdpolicy.AnnotationPureGroup: "true",
		},
	}
	docs.AddCommand(placeholder)
	noChild := &cobra.Command{
		Use:  "+ghost",
		RunE: func(*cobra.Command, []string) error { return nil },
		Annotations: map[string]string{
			cmdpolicy.AnnotationPureGroup: "true",
		},
	}
	placeholder.AddCommand(noChild)

	fetch := &cobra.Command{Use: "+fetch", RunE: noop}
	cmdutil.SetRisk(fetch, "write")
	docs.AddCommand(fetch)

	e := cmdpolicy.New(&platform.Rule{MaxRisk: "read"})
	decisions := e.EvaluateAll(root)
	denied := cmdpolicy.BuildDeniedByPath(root, decisions, cmdpolicy.ResolveSource{Kind: cmdpolicy.SourceYAML}, "")

	if _, ok := denied["docs"]; !ok {
		t.Fatalf("docs should be aggregated as fully denied (pure-group children excluded from live count); map=%+v", denied)
	}
}

// Regression: aggregateParents must treat an AnnotationPureGroup-tagged
// command exactly like a parent-only group. With cmdRunnable accidentally
// true (RunE attached by the guard), the aggregator would otherwise look
// for an own-RunE denial entry and skip aggregation, leaving `<group>
// --help` reachable even when every live child is denied.
func TestBuildDeniedByPath_aggregatesAnnotatedPureGroup(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	drive := &cobra.Command{
		Use:  "drive",
		RunE: func(*cobra.Command, []string) error { return nil },
		Annotations: map[string]string{
			cmdpolicy.AnnotationPureGroup: "true",
		},
	}
	root.AddCommand(drive)
	push := &cobra.Command{Use: "+push", RunE: noop}
	cmdutil.SetRisk(push, "write")
	drive.AddCommand(push)
	pull := &cobra.Command{Use: "+pull", RunE: noop}
	cmdutil.SetRisk(pull, "write")
	drive.AddCommand(pull)

	e := cmdpolicy.New(&platform.Rule{MaxRisk: "read"})
	decisions := e.EvaluateAll(root)
	denied := cmdpolicy.BuildDeniedByPath(root, decisions, cmdpolicy.ResolveSource{Kind: cmdpolicy.SourceYAML}, "")

	if _, ok := denied["drive"]; !ok {
		t.Fatalf("aggregator must install drive denial when all children denied; map=%+v", denied)
	}
}

// The binary root must never receive a denyStub even if every descendant
// is denied. cobra still needs root to dispatch help / completion.
func TestApply_neverInstallsOnRoot(t *testing.T) {
	root := buildTree()
	denied := map[string]cmdpolicy.Denial{
		"lark-cli": {Layer: "policy", ReasonCode: "all_children_denied"},
	}
	cmdpolicy.Apply(root, denied)
	if root.RunE != nil {
		t.Errorf("root.RunE should remain nil; got a denyStub installed")
	}
	if root.Hidden {
		t.Errorf("root must stay visible")
	}
}
