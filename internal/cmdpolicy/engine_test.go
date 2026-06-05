// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
)

// buildTree assembles a tiny realistic tree for engine tests:
//
//	lark-cli (root)
//	├── docs
//	│   ├── +fetch       risk=read    identities=[user,bot]
//	│   ├── +update      risk=write   identities=[user]
//	│   └── +delete-doc  risk=high-risk-write
//	└── im
//	    └── +send        risk=write   identities=[bot]
func buildTree() *cobra.Command {
	root := &cobra.Command{Use: "lark-cli"}

	docs := &cobra.Command{Use: "docs"}
	cmdmeta.SetDomain(docs, "docs")
	root.AddCommand(docs)

	fetch := &cobra.Command{Use: "+fetch", RunE: noop}
	cmdutil.SetRisk(fetch, "read")
	cmdutil.SetSupportedIdentities(fetch, []string{"user", "bot"})
	docs.AddCommand(fetch)

	update := &cobra.Command{Use: "+update", RunE: noop}
	cmdutil.SetRisk(update, "write")
	cmdutil.SetSupportedIdentities(update, []string{"user"})
	docs.AddCommand(update)

	deleteDoc := &cobra.Command{Use: "+delete-doc", RunE: noop}
	cmdutil.SetRisk(deleteDoc, "high-risk-write")
	docs.AddCommand(deleteDoc)

	im := &cobra.Command{Use: "im"}
	cmdmeta.SetDomain(im, "im")
	root.AddCommand(im)

	send := &cobra.Command{Use: "+send", RunE: noop}
	cmdutil.SetRisk(send, "write")
	cmdutil.SetSupportedIdentities(send, []string{"bot"})
	im.AddCommand(send)

	return root
}

func noop(*cobra.Command, []string) error { return nil }

func TestEvaluate_nilRuleAllowsAll(t *testing.T) {
	root := buildTree()
	got := cmdpolicy.New(nil).EvaluateAll(root)
	for path, d := range got {
		if !d.Allowed {
			t.Fatalf("nil rule should allow all, got Allowed=false for %s", path)
		}
	}
}

func TestEvaluate_allowGlob(t *testing.T) {
	root := buildTree()
	e := cmdpolicy.New(&platform.Rule{
		Allow: []string{"docs/**"},
	})
	got := e.EvaluateAll(root)

	if !got["docs/+fetch"].Allowed {
		t.Errorf("docs/+fetch should be allowed by docs/** glob")
	}
	if got["im/+send"].Allowed {
		t.Errorf("im/+send should NOT be allowed when Allow=docs/**")
	}
	if got["im/+send"].ReasonCode != "domain_not_allowed" {
		t.Errorf("im/+send ReasonCode = %q, want domain_not_allowed",
			got["im/+send"].ReasonCode)
	}
}

func TestEvaluate_denyTakesPriorityOverAllow(t *testing.T) {
	root := buildTree()
	e := cmdpolicy.New(&platform.Rule{
		Allow: []string{"docs/**"},
		Deny:  []string{"docs/+delete-doc"},
	})
	got := e.EvaluateAll(root)

	if got["docs/+delete-doc"].Allowed {
		t.Errorf("docs/+delete-doc should be denied by Deny rule")
	}
	if got["docs/+delete-doc"].ReasonCode != "command_denylisted" {
		t.Errorf("ReasonCode = %q, want command_denylisted",
			got["docs/+delete-doc"].ReasonCode)
	}
	if !got["docs/+fetch"].Allowed {
		t.Errorf("docs/+fetch should still be allowed (not in Deny)")
	}
}

func TestEvaluate_maxRiskCutoff(t *testing.T) {
	root := buildTree()
	e := cmdpolicy.New(&platform.Rule{
		MaxRisk: "write", // allow read+write, deny high-risk-write
	})
	got := e.EvaluateAll(root)

	if !got["docs/+update"].Allowed {
		t.Errorf("+update (risk=write) should pass MaxRisk=write")
	}
	if !got["docs/+fetch"].Allowed {
		t.Errorf("+fetch (risk=read) should pass MaxRisk=write")
	}
	if got["docs/+delete-doc"].Allowed {
		t.Errorf("+delete-doc (risk=high-risk-write) should fail MaxRisk=write")
	}
	if rc := got["docs/+delete-doc"].ReasonCode; rc != "write_not_allowed" {
		t.Errorf("ReasonCode = %q, want write_not_allowed", rc)
	}
}

// Unannotated commands are implicit-deny when any Rule is registered.
// The closed risk taxonomy (read / write / high-risk-write) is the only
// vocabulary a Rule can reason about; an unannotated command falls
// outside that vocabulary and is denied with reason_code
// "risk_not_annotated", regardless of whether the rule sets MaxRisk.
func TestEvaluate_unannotatedRiskIsDeny(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	docs := &cobra.Command{Use: "docs"}
	root.AddCommand(docs)
	// Note: no SetRisk on this command -> unannotated
	orphan := &cobra.Command{Use: "+orphan", RunE: noop}
	docs.AddCommand(orphan)

	// Rule without MaxRisk still triggers the implicit deny.
	e := cmdpolicy.New(&platform.Rule{Allow: []string{"docs/**"}})
	got := e.EvaluateAll(root)
	if got["docs/+orphan"].Allowed {
		t.Fatalf("unannotated risk must be denied when a Rule is registered")
	}
	if got["docs/+orphan"].ReasonCode != "risk_not_annotated" {
		t.Errorf("ReasonCode = %q, want risk_not_annotated", got["docs/+orphan"].ReasonCode)
	}

	// And with MaxRisk it still uses risk_not_annotated (the missing-
	// annotation gate runs before the MaxRisk axis).
	e = cmdpolicy.New(&platform.Rule{MaxRisk: "read"})
	got = e.EvaluateAll(root)
	if got["docs/+orphan"].ReasonCode != "risk_not_annotated" {
		t.Errorf("ReasonCode under MaxRisk = %q, want risk_not_annotated", got["docs/+orphan"].ReasonCode)
	}

	// An empty Rule{} (no Allow / Deny / MaxRisk / Identities) still
	// triggers the implicit deny. "any registered Rule = enter the safety
	// boundary" is the design contract; pin it so future edits cannot
	// silently weaken it.
	e = cmdpolicy.New(&platform.Rule{})
	got = e.EvaluateAll(root)
	if got["docs/+orphan"].Allowed {
		t.Fatalf("empty Rule{} must still deny unannotated commands")
	}
	if got["docs/+orphan"].ReasonCode != "risk_not_annotated" {
		t.Errorf("empty Rule{} ReasonCode = %q, want risk_not_annotated", got["docs/+orphan"].ReasonCode)
	}

	// Without any Rule, unannotated commands are still allowed (no
	// policy engine is invoked when no plugin registers a Rule).
	e = cmdpolicy.New(nil)
	got = e.EvaluateAll(root)
	if !got["docs/+orphan"].Allowed {
		t.Fatalf("nil Rule must allow unannotated commands (no main-flow impact)")
	}
}

// AllowUnannotated=true opts out of the "unannotated = deny" rule for
// gradual adoption. The flag does NOT loosen any other axis: Deny still
// rejects, MaxRisk is skipped (no rank to compare), Allow/Identities still
// apply.
func TestEvaluate_allowUnannotatedOptsOutOfDeny(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	docs := &cobra.Command{Use: "docs"}
	root.AddCommand(docs)
	orphan := &cobra.Command{Use: "+orphan", RunE: noop}
	docs.AddCommand(orphan)

	// Without opt-in: still denied
	e := cmdpolicy.New(&platform.Rule{Allow: []string{"docs/**"}})
	if got := e.EvaluateAll(root); got["docs/+orphan"].Allowed {
		t.Fatalf("default behaviour must deny unannotated; AllowUnannotated should be opt-in")
	}

	// With opt-in: allowed
	e = cmdpolicy.New(&platform.Rule{
		Allow:            []string{"docs/**"},
		AllowUnannotated: true,
	})
	got := e.EvaluateAll(root)
	if !got["docs/+orphan"].Allowed {
		t.Fatalf("AllowUnannotated=true must allow unannotated commands; got %+v", got["docs/+orphan"])
	}

	// AllowUnannotated does NOT bypass Deny: an unannotated command
	// hitting a Deny glob is still rejected.
	e = cmdpolicy.New(&platform.Rule{
		Deny:             []string{"docs/+orphan"},
		AllowUnannotated: true,
	})
	got = e.EvaluateAll(root)
	if got["docs/+orphan"].Allowed {
		t.Fatalf("AllowUnannotated must not bypass Deny; got %+v", got["docs/+orphan"])
	}
	if got["docs/+orphan"].ReasonCode != "command_denylisted" {
		t.Errorf("ReasonCode under Deny+AllowUnannotated = %q, want command_denylisted",
			got["docs/+orphan"].ReasonCode)
	}
}

// risk_invalid (typo) is unaffected by AllowUnannotated and emits a
// "did you mean" suggestion in the reason text.
func TestEvaluate_invalidRiskAlwaysDeny_andSuggests(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	docs := &cobra.Command{Use: "docs"}
	root.AddCommand(docs)
	typo := &cobra.Command{Use: "+typo", RunE: noop}
	cmdutil.SetRisk(typo, "wrtie")
	docs.AddCommand(typo)

	// AllowUnannotated=true must NOT bypass risk_invalid — typo is a
	// code bug, not a missing annotation.
	e := cmdpolicy.New(&platform.Rule{
		MaxRisk:          "read",
		AllowUnannotated: true,
	})
	got := e.EvaluateAll(root)
	if got["docs/+typo"].Allowed {
		t.Fatalf("AllowUnannotated must not bypass risk_invalid; got %+v", got["docs/+typo"])
	}
	if got["docs/+typo"].ReasonCode != "risk_invalid" {
		t.Errorf("ReasonCode = %q, want risk_invalid", got["docs/+typo"].ReasonCode)
	}
	if !strings.Contains(got["docs/+typo"].Reason, "write") {
		t.Errorf("Reason should contain suggestion 'write', got %q", got["docs/+typo"].Reason)
	}
}

// Invalid risk annotations (typos like "wrtie" or anything outside the
// read|write|high-risk-write taxonomy) are denied with reason_code
// "risk_invalid". Without this gate they used to pass the MaxRisk axis
// because RiskRank returned ok=false and the comparison was skipped --
// a typo SetRisk would silently slip past an "agent read-only" rule.
func TestEvaluate_invalidRiskIsDeny(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	docs := &cobra.Command{Use: "docs"}
	root.AddCommand(docs)
	typo := &cobra.Command{Use: "+typo", RunE: noop}
	cmdutil.SetRisk(typo, "wrtie") // typo for "write"
	docs.AddCommand(typo)

	// Even under MaxRisk=read the typo command must not slip through.
	e := cmdpolicy.New(&platform.Rule{MaxRisk: "read"})
	got := e.EvaluateAll(root)
	if got["docs/+typo"].Allowed {
		t.Fatalf("invalid risk must be denied under MaxRisk=read, got allowed")
	}
	if got["docs/+typo"].ReasonCode != "risk_invalid" {
		t.Errorf("ReasonCode = %q, want risk_invalid", got["docs/+typo"].ReasonCode)
	}

	// Same when no MaxRisk is set -- the taxonomy check runs unconditionally
	// once a Rule is present.
	e = cmdpolicy.New(&platform.Rule{Allow: []string{"docs/**"}})
	got = e.EvaluateAll(root)
	if got["docs/+typo"].ReasonCode != "risk_invalid" {
		t.Errorf("ReasonCode without MaxRisk = %q, want risk_invalid", got["docs/+typo"].ReasonCode)
	}

	// The risk_invalid gate must fire BEFORE Deny matching, otherwise a
	// typo command landing in the deny list would surface as
	// command_denylisted and mask the underlying taxonomy violation.
	e = cmdpolicy.New(&platform.Rule{Deny: []string{"docs/+typo"}})
	got = e.EvaluateAll(root)
	if got["docs/+typo"].ReasonCode != "risk_invalid" {
		t.Errorf("ReasonCode under Deny match = %q, want risk_invalid (taxonomy gate must precede Deny)", got["docs/+typo"].ReasonCode)
	}

	// Without any Rule, invalid risk is not policed (same main-flow
	// no-impact rule as risk_not_annotated).
	e = cmdpolicy.New(nil)
	got = e.EvaluateAll(root)
	if !got["docs/+typo"].Allowed {
		t.Fatalf("nil Rule must allow invalid risk (no main-flow impact)")
	}
}

func TestEvaluate_identitiesIntersection(t *testing.T) {
	root := buildTree()
	e := cmdpolicy.New(&platform.Rule{
		Identities: []platform.Identity{"bot"}, // bot-only rule
	})
	got := e.EvaluateAll(root)

	// docs/+fetch has [user, bot] -- intersection includes bot -> ALLOW
	if !got["docs/+fetch"].Allowed {
		t.Errorf("+fetch (identities=user,bot) should intersect bot rule")
	}
	// docs/+update has [user] -- no intersection with bot -> DENY
	if got["docs/+update"].Allowed {
		t.Errorf("+update (identities=user) should fail bot-only rule")
	}
	if got["docs/+update"].ReasonCode != "identity_mismatch" {
		t.Errorf("ReasonCode = %q, want identity_mismatch",
			got["docs/+update"].ReasonCode)
	}
}

// Reason strings must carry both the attempted value and the rule's
// constraint so the envelope is self-contained for AI consumers.
// Asserting on substrings (not exact match) leaves room for minor wording
// tweaks while pinning the value-carrying behaviour.
func TestEvaluate_reasonCarriesAttemptAndConstraint(t *testing.T) {
	root := buildTree()

	cases := []struct {
		name         string
		rule         *platform.Rule
		path         string
		wantInReason []string
	}{
		{
			name:         "identity_mismatch surfaces both identity sets",
			rule:         &platform.Rule{Identities: []platform.Identity{"bot"}},
			path:         "docs/+update", // identities=[user]
			wantInReason: []string{"[user]", "[bot]"},
		},
		{
			name:         "domain_not_allowed surfaces path and allow list",
			rule:         &platform.Rule{Allow: []string{"docs/**"}},
			path:         "im/+send",
			wantInReason: []string{`"im/+send"`, "docs/**"},
		},
		{
			name:         "command_denylisted surfaces matched deny pattern",
			rule:         &platform.Rule{Deny: []string{"docs/+delete-*"}},
			path:         "docs/+delete-doc",
			wantInReason: []string{`"docs/+delete-doc"`, `"docs/+delete-*"`},
		},
		{
			name:         "risk_too_high surfaces cmd risk and max_risk",
			rule:         &platform.Rule{MaxRisk: "write"},
			path:         "docs/+delete-doc", // risk=high-risk-write
			wantInReason: []string{`"high-risk-write"`, `"write"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cmdpolicy.New(tc.rule).EvaluateAll(root)
			d, ok := got[tc.path]
			if !ok {
				t.Fatalf("no decision for %q", tc.path)
			}
			if d.Allowed {
				t.Fatalf("%q should have been denied", tc.path)
			}
			for _, sub := range tc.wantInReason {
				if !strings.Contains(d.Reason, sub) {
					t.Errorf("reason %q missing %q", d.Reason, sub)
				}
			}
		})
	}
}

// Unknown identities defaults to ALLOW. A command with risk annotated
// but without supportedIdentities passes any identity filter.
func TestEvaluate_unknownIdentitiesIsAllow(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	cmd := &cobra.Command{Use: "+x", RunE: noop}
	cmdutil.SetRisk(cmd, "read")
	root.AddCommand(cmd)
	// no SetSupportedIdentities

	e := cmdpolicy.New(&platform.Rule{Identities: []platform.Identity{"bot"}})
	got := e.EvaluateAll(root)
	if !got["+x"].Allowed {
		t.Fatalf("unknown identities must pass any identity rule")
	}
}

// --- Multi-rule (OR) semantics ---

// Two scoped rules (docs read-only, im writable) are OR-combined: a
// command is allowed when it satisfies ANY rule. This is the headline
// multi-rule use case -- different command groups need different risk
// ceilings within one policy.
func TestEvaluate_multiRuleOR(t *testing.T) {
	root := buildTree()
	e := cmdpolicy.NewSet([]*platform.Rule{
		{Name: "docs-ro", Allow: []string{"docs/**"}, MaxRisk: "read"},
		{Name: "im-rw", Allow: []string{"im/**"}, MaxRisk: "write"},
	})
	got := e.EvaluateAll(root)

	// docs/+fetch (read) clears docs-ro.
	if !got["docs/+fetch"].Allowed {
		t.Errorf("docs/+fetch should be allowed by docs-ro")
	}
	// im/+send (write) clears im-rw even though docs-ro rejects it.
	if !got["im/+send"].Allowed {
		t.Errorf("im/+send (write) should be allowed by im-rw")
	}
	// docs/+update (write) exceeds docs-ro's read ceiling AND is outside
	// im-rw's allow list -> rejected by both -> no_matching_rule.
	if got["docs/+update"].Allowed {
		t.Fatalf("docs/+update should be denied: read-only in docs, not allowed in im")
	}
	if rc := got["docs/+update"].ReasonCode; rc != "no_matching_rule" {
		t.Errorf("docs/+update ReasonCode = %q, want no_matching_rule", rc)
	}
}

// Identity can differ per rule: docs limited to user, im open to bot.
// This is the second half of the requirement -- some commands restrict
// identity, others allow the bot identity.
func TestEvaluate_multiRulePerRuleIdentity(t *testing.T) {
	root := buildTree()
	e := cmdpolicy.NewSet([]*platform.Rule{
		{Name: "docs-user", Allow: []string{"docs/**"}, MaxRisk: "write", Identities: []platform.Identity{"user"}},
		{Name: "im-bot", Allow: []string{"im/**"}, MaxRisk: "write", Identities: []platform.Identity{"bot"}},
	})
	got := e.EvaluateAll(root)

	// docs/+update identities=[user] -> docs-user grants.
	if !got["docs/+update"].Allowed {
		t.Errorf("docs/+update (user) should be allowed by docs-user")
	}
	// im/+send identities=[bot] -> im-bot grants.
	if !got["im/+send"].Allowed {
		t.Errorf("im/+send (bot) should be allowed by im-bot")
	}
	// docs/+delete-doc is high-risk-write -> exceeds both ceilings -> denied.
	if got["docs/+delete-doc"].Allowed {
		t.Errorf("docs/+delete-doc (high-risk-write) should be denied by both rules")
	}
}

// NewSet with a single rule must behave exactly like New: the per-rule
// rejection (not the aggregate no_matching_rule) is preserved so the
// single-rule envelope is unchanged.
func TestEvaluate_newSetSingleRuleKeepsReason(t *testing.T) {
	root := buildTree()
	e := cmdpolicy.NewSet([]*platform.Rule{
		{Allow: []string{"docs/**"}},
	})
	got := e.EvaluateAll(root)
	if got["im/+send"].Allowed {
		t.Fatalf("im/+send should be denied by docs-only rule")
	}
	if rc := got["im/+send"].ReasonCode; rc != "domain_not_allowed" {
		t.Errorf("single-rule reason must be preserved verbatim, got %q want domain_not_allowed", rc)
	}
}

// NewSet drops nil entries; an all-nil/empty set means "no restriction".
func TestNewSet_emptyAndNilMeansNoRestriction(t *testing.T) {
	root := buildTree()
	for _, rules := range [][]*platform.Rule{nil, {}, {nil}} {
		got := cmdpolicy.NewSet(rules).EvaluateAll(root)
		for path, d := range got {
			if !d.Allowed {
				t.Fatalf("empty/nil rule set must allow all, got deny for %s", path)
			}
		}
	}
}

// Apply must install denyStubs only on Layer="policy" entries. A
// "strict_mode" denial in the same map must be left for
// applyStrictModeDenials in cmd/.
func TestApply_onlyTouchesPruningLayer(t *testing.T) {
	root := buildTree()
	denied := map[string]cmdpolicy.Denial{
		"docs/+update": {Layer: "policy", ReasonCode: "write_not_allowed"},
		"docs/+fetch":  {Layer: "strict_mode", ReasonCode: "identity_not_supported"},
	}

	count := cmdpolicy.Apply(root, denied)
	if count != 1 {
		t.Fatalf("Apply count = %d, want 1 (only pruning-layer entries)", count)
	}

	update := findChild(t, root, "docs", "+update")
	if !update.Hidden {
		t.Errorf("+update should be Hidden after Apply")
	}
	if !update.DisableFlagParsing {
		t.Errorf("+update should have DisableFlagParsing=true (constraint #4)")
	}

	// strict-mode entry must NOT have been touched here.
	fetch := findChild(t, root, "docs", "+fetch")
	if fetch.Hidden || fetch.DisableFlagParsing {
		t.Errorf("+fetch (strict_mode layer) should NOT be touched by cmdpolicy.Apply")
	}
}

// Calling the denied RunE must produce a typed CommandDeniedError with the
// right Layer/ReasonCode. This is the contract every external consumer
// (agent, integration) depends on.
func TestApply_runEReturnsTypedError(t *testing.T) {
	root := buildTree()
	cmdpolicy.Apply(root, map[string]cmdpolicy.Denial{
		"docs/+update": {
			Layer:        "policy",
			PolicySource: "plugin:secaudit",
			RuleName:     "secaudit-policy",
			ReasonCode:   "write_not_allowed",
			Reason:       "write disabled",
		},
	})

	update := findChild(t, root, "docs", "+update")
	err := update.RunE(update, []string{})
	if err == nil {
		t.Fatalf("denied command should return error")
	}
	var denied *platform.CommandDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("error should be *platform.CommandDeniedError, got %T", err)
	}
	if denied.Layer != "policy" || denied.ReasonCode != "write_not_allowed" {
		t.Errorf("denial = %+v, want layer=pruning code=write_not_allowed", denied)
	}
	if denied.Path != "docs/+update" {
		t.Errorf("Path = %q, want docs/+update", denied.Path)
	}
	if denied.PolicySource != "plugin:secaudit" || denied.RuleName != "secaudit-policy" {
		t.Errorf("policy source / rule name lost in stub: %+v", denied)
	}
}

func TestApply_emptyMapNoop(t *testing.T) {
	root := buildTree()
	if got := cmdpolicy.Apply(root, nil); got != 0 {
		t.Fatalf("nil deniedByPath should yield count=0, got %d", got)
	}
}

// CanonicalPath strips the root and joins with slashes -- the form
// doublestar globs need to work.
func TestCanonicalPath(t *testing.T) {
	root := buildTree()
	update := findChild(t, root, "docs", "+update")
	if got := cmdpolicy.CanonicalPath(update); got != "docs/+update" {
		t.Fatalf("CanonicalPath = %q, want docs/+update", got)
	}
	if got := cmdpolicy.CanonicalPath(root); got != "lark-cli" {
		t.Fatalf("CanonicalPath(root) = %q, want lark-cli (orphan fallback)", got)
	}
}

// findChild is a test helper: descend a path of cmd.Use names through the
// tree, failing the test if any step is missing.
func findChild(t *testing.T, parent *cobra.Command, names ...string) *cobra.Command {
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
