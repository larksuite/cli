// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package cmdpolicy is the user-layer command policy engine. It consumes a
// platform.Rule and the cobra command tree, evaluates each runnable command
// against the rule's four-axis filter (Allow / Deny / MaxRisk / Identities),
// and produces a path -> Decision map. A separate BuildDeniedByPath step
// converts those leaf decisions into a deniedByPath map (with parent-group
// aggregation), which the Apply step consumes to install denyStubs.
//
// This package only implements the user-layer half. Strict-mode is handled
// by cmd/prune.go, which produces typed validation errors of the same shape
// (failed_precondition, *platform.CommandDeniedError preserved as Cause) so
// external agents see a uniform envelope regardless of which layer rejected
// the call.
package cmdpolicy

import (
	"fmt"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdmeta"
)

// Decision is the user-layer single-rule evaluation result. Distinct from
// Denial: Decision carries Allowed=true/false and the
// rejection reason when Allowed=false; Denial only ever exists when the
// command is rejected. Keeping them separate avoids a perpetually-false
// Allowed field on Denial.
type Decision struct {
	Allowed    bool
	ReasonCode string // "" when Allowed=true
	Reason     string // human-readable
}

// Engine evaluates a set of Rules against the command tree with OR
// semantics: a command is allowed when it satisfies every axis of AT
// LEAST ONE rule. It is stateless except for the Rule snapshot it was
// constructed with.
type Engine struct {
	rules []*platform.Rule
}

// New returns an Engine bound to a single Rule. A nil Rule means "no
// user-layer restriction" -- EvaluateOne always returns Allowed=true.
// It is the ergonomic single-rule constructor, kept so existing callers
// (and the single-rule decision path) stay byte-for-byte unchanged.
func New(rule *platform.Rule) *Engine {
	if rule == nil {
		return &Engine{}
	}
	return &Engine{rules: []*platform.Rule{rule}}
}

// NewSet returns an Engine bound to a set of Rules evaluated with OR
// semantics. An empty/nil slice means "no user-layer restriction". nil
// entries are dropped so callers may pass a slice with gaps without a
// separate filter step.
//
// With exactly one rule the behaviour is identical to New(rule): the
// rejection Decision is returned verbatim. With multiple rules a command
// rejected by all of them gets the aggregate reason_code
// "no_matching_rule" (see mergeDenials).
func NewSet(rules []*platform.Rule) *Engine {
	cleaned := make([]*platform.Rule, 0, len(rules))
	for _, r := range rules {
		if r != nil {
			cleaned = append(cleaned, r)
		}
	}
	if len(cleaned) == 0 {
		return &Engine{}
	}
	return &Engine{rules: cleaned}
}

// EvaluateAll walks the command tree and evaluates every **runnable**
// command against the Rule. Pure parent groups (no RunE) are deliberately
// skipped here: their decision is derived from children by
// BuildDeniedByPath. Evaluating groups directly would incorrectly deny
// "docs" under an Allow:["docs/**"] rule (the group's own path "docs"
// does not match the "**"-requiring glob).
//
// Hybrid commands (own RunE plus children) are evaluated as ordinary
// leaves here; the aggregation pass treats them specially.
func (e *Engine) EvaluateAll(root *cobra.Command) map[string]Decision {
	out := map[string]Decision{}
	walkTree(root, func(c *cobra.Command) {
		if !c.Runnable() {
			return
		}
		// Pure parent groups carrying the AnnotationPureGroup marker
		// (installed by cmd.installUnknownSubcommandGuard) look
		// Runnable to cobra but are not a real leaf: skip them just
		// like cobra-native parent groups, so a user-level Rule does
		// not block `<group> --help` discovery.
		if IsPureGroup(c) {
			return
		}
		path := CanonicalPath(c)
		if path == "" {
			return
		}
		out[path] = e.EvaluateOne(c)
	})
	return out
}

// EvaluateOne returns the user-layer decision for a single command. Always
// Allowed=true when the engine has no Rule. With multiple rules the
// decision is the OR over per-rule evaluations: the command is allowed as
// soon as one rule grants it; if every rule rejects it, the rejections are
// merged (see mergeDenials).
func (e *Engine) EvaluateOne(cmd *cobra.Command) Decision {
	if len(e.rules) == 0 {
		return Decision{Allowed: true}
	}
	path := CanonicalPath(cmd)

	if IsDiagnosticPath(path) {
		return Decision{Allowed: true}
	}

	// risk_invalid is a property of the COMMAND's own annotation (the
	// annotation exists but is a typo / not in the closed taxonomy
	// read / write / high-risk-write). It is independent of any Rule and
	// is always fail-closed regardless of AllowUnannotated -- a typo is a
	// code bug, not a migration phase. So it is checked once up front,
	// before the per-rule OR loop, and short-circuits to deny.
	//
	// The "absent" case (no risk_level annotation at all) is per-rule:
	// each rule's AllowUnannotated decides, so it lives inside evalRule.
	cmdRiskStr, hasRisk := cmdmeta.Risk(cmd)
	cmdRisk := platform.Risk(cmdRiskStr)
	var (
		cmdRank   int
		cmdRankOk bool
	)
	if hasRisk {
		cmdRank, cmdRankOk = cmdRisk.Rank()
		if !cmdRankOk {
			return Decision{
				Allowed:    false,
				ReasonCode: "risk_invalid",
				Reason:     fmt.Sprintf("invalid risk %q; did you mean %q?", cmdRiskStr, suggestRisk(cmdRiskStr)),
			}
		}
	}

	// OR across rules: the first rule that fully grants the command wins.
	denials := make([]Decision, 0, len(e.rules))
	for _, r := range e.rules {
		d := evalRule(r, path, cmd, hasRisk, cmdRisk, cmdRank, cmdRankOk)
		if d.Allowed {
			return Decision{Allowed: true}
		}
		denials = append(denials, d)
	}
	return mergeDenials(e.rules, denials)
}

// evalRule applies one Rule's four-axis AND filter to a command whose
// risk annotation has already been parsed by EvaluateOne (risk_invalid is
// handled there). cmdRankOk is false only when the command is unannotated
// (hasRisk=false); a present-but-invalid risk never reaches here. Returns
// Allowed=true only when the command clears every axis of this rule.
func evalRule(r *platform.Rule, path string, cmd *cobra.Command, hasRisk bool, cmdRisk platform.Risk, cmdRank int, cmdRankOk bool) Decision {
	// Unannotated gate: fail-closed unless THIS rule opts out. A command
	// with no risk_level annotation can still be granted by a rule that
	// sets AllowUnannotated=true (gradual-adoption opt-in); other rules in
	// the set reject it here and the OR moves on.
	if !hasRisk && !r.AllowUnannotated {
		return Decision{
			Allowed:    false,
			ReasonCode: "risk_not_annotated",
			Reason:     "command has no risk_level annotation; rule denies unannotated commands",
		}
	}

	// Axis 1: Deny has priority. Note OR semantics scope a rule's Deny to
	// that rule only -- it cannot veto another rule's Allow. A command to
	// block everywhere must be denied (or simply not allowed) by every rule.
	if matched, ok := firstMatch(r.Deny, path); ok {
		return Decision{
			Allowed:    false,
			ReasonCode: "command_denylisted",
			Reason:     fmt.Sprintf("command path %q matched deny pattern %q", path, matched),
		}
	}

	// Axis 2: Allow gate (empty allow means "no restriction").
	if len(r.Allow) > 0 && !matchesAny(r.Allow, path) {
		return Decision{
			Allowed:    false,
			ReasonCode: "domain_not_allowed",
			Reason:     fmt.Sprintf("command path %q not in allow list %v", path, r.Allow),
		}
	}

	// Axis 3: MaxRisk. Skipped when cmd risk is absent + AllowUnannotated:
	// the engine has no rank to compare against, and AllowUnannotated
	// is the explicit "allow this through" opt-in.
	if r.MaxRisk != "" && cmdRankOk {
		if limit, limitOk := r.MaxRisk.Rank(); limitOk && cmdRank > limit {
			return Decision{
				Allowed:    false,
				ReasonCode: reasonCodeForRisk(cmdRisk),
				Reason:     fmt.Sprintf("command risk %q exceeds rule max_risk %q", cmdRisk, r.MaxRisk),
			}
		}
	}

	// Axis 4: Identities. Unknown command identities is treated as ALLOW.
	if len(r.Identities) > 0 {
		cmdIdents := cmdmeta.Identities(cmd)
		if cmdIdents != nil && !hasIdentityIntersection(r.Identities, cmdIdents) {
			return Decision{
				Allowed:    false,
				ReasonCode: "identity_mismatch",
				Reason:     fmt.Sprintf("command supports identities %v; rule allows %v", cmdIdents, r.Identities),
			}
		}
	}

	return Decision{Allowed: true}
}

// mergeDenials collapses the per-rule rejections into a single Decision
// for a command that no rule granted. denials is parallel to rules (same
// order, one entry per rule, all Allowed=false).
//
// With exactly one rule the original rejection is returned verbatim, so
// single-rule envelopes are byte-for-byte identical to the pre-multi-rule
// behaviour (reason_code / reason unchanged). With multiple rules the
// rejection is the aggregate reason_code "no_matching_rule"; its Reason
// enumerates each rule's own rejection for debugging.
func mergeDenials(rules []*platform.Rule, denials []Decision) Decision {
	if len(denials) == 1 {
		return denials[0]
	}
	parts := make([]string, len(denials))
	for i, d := range denials {
		name := rules[i].Name
		if name == "" {
			name = fmt.Sprintf("#%d", i)
		}
		parts[i] = fmt.Sprintf("%s: %s", name, d.ReasonCode)
	}
	return Decision{
		Allowed:    false,
		ReasonCode: "no_matching_rule",
		Reason:     fmt.Sprintf("no rule grants this command (%s)", strings.Join(parts, "; ")),
	}
}

// BuildDeniedByPath converts engine Decisions to a deniedByPath map keyed
// by canonical path. It performs the parent-group aggregation defined in
// the tech doc: a non-runnable parent whose every runnable descendant is
// denied gets an aggregate denial (via AggregateChildren);
// hybrid commands (own RunE + children) get one only when both their own
// RunE and all children are denied.
//
// The root command (no parent) is never installed with a denyStub even if
// every child is denied -- the binary entry point must remain dispatchable
// so `--help` and similar remain available.
//
// source / ruleName populate PolicySource and RuleName on the produced
// Denial values, so envelope output can attribute denials.
func BuildDeniedByPath(root *cobra.Command, decisions map[string]Decision, source ResolveSource, ruleName string) map[string]Denial {
	out := map[string]Denial{}

	sourceLabel := policySourceLabel(source)
	for path, d := range decisions {
		if !d.Allowed {
			out[path] = Denial{
				Layer:        LayerPolicy,
				PolicySource: sourceLabel,
				RuleName:     ruleName,
				ReasonCode:   d.ReasonCode,
				Reason:       d.Reason,
			}
		}
	}

	aggregateParents(root, out)
	return out
}

// aggregateParents recursively examines each parent group. Returns true
// when every runnable descendant beneath cmd (including cmd itself when
// runnable) is denied; in that case the function also inserts an aggregate
// Denial for cmd, unless cmd is the binary root or cmd is already in the
// map (own RunE denial preserved).
//
// "Live" children are those with at least one runnable descendant; pure
// non-runnable placeholders neither count toward "all denied" nor block
// the aggregation.
func aggregateParents(cmd *cobra.Command, denied map[string]Denial) bool {
	if cmd == nil {
		return false
	}

	children := cmd.Commands()
	// A pure parent group decorated with the unknown-subcommand guard
	// looks Runnable() to cobra but is not a true hybrid: treat it
	// exactly like cobra-native parent groups so the aggregation pass
	// can still install an aggregate deny stub when every live child
	// is denied.
	cmdRunnable := cmd.Runnable() && !IsPureGroup(cmd)
	cmdPath := CanonicalPath(cmd)

	// Pure leaf
	if len(children) == 0 {
		if !cmdRunnable {
			return false // placeholder, doesn't contribute
		}
		_, ok := denied[cmdPath]
		return ok
	}

	// Has children: recurse first, collect direct-child denials for the
	// aggregation message.
	childDenials := make([]ChildDenial, 0, len(children))
	liveChildSeen := false
	allLiveChildrenDenied := true
	for _, child := range children {
		childDenied := aggregateParents(child, denied)
		if hasRunnableDescendant(child) {
			liveChildSeen = true
			if !childDenied {
				allLiveChildrenDenied = false
			}
		}
		if cp := CanonicalPath(child); cp != "" {
			if d, ok := denied[cp]; ok {
				childDenials = append(childDenials, ChildDenial{Path: cp, Denial: d})
			}
		}
	}

	if !liveChildSeen {
		// No reachable runnable descendant in children, but cmd itself
		// may still be a runnable hybrid (own RunE + placeholder
		// children). The contract is "every runnable descendant
		// beneath cmd (including cmd itself when runnable) is denied",
		// so when cmd is runnable, the answer depends on whether cmd
		// itself was denied. Returning false unconditionally here lost
		// that signal and blocked aggregation up the chain.
		if cmdRunnable {
			_, ownDenied := denied[cmdPath]
			return ownDenied
		}
		return false
	}

	// Hybrid: own RunE must also be denied for the group to count as denied.
	if cmdRunnable {
		if _, ownDenied := denied[cmdPath]; !ownDenied {
			return false
		}
	}

	if !allLiveChildrenDenied {
		return false
	}

	// Everything reachable below this command is denied. Install the
	// aggregate denyStub if there isn't already an own denial here, and
	// skip the binary root.
	if cmd.HasParent() && cmdPath != "" {
		if _, exists := denied[cmdPath]; !exists {
			SortChildren(childDenials)
			denied[cmdPath] = AggregateChildren(childDenials)
		}
	}
	return true
}

// hasRunnableDescendant reports whether cmd or any descendant has RunE.
// We use it to ignore pure placeholder branches when aggregating.
func hasRunnableDescendant(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if cmd.Runnable() && !IsPureGroup(cmd) {
		return true
	}
	for _, c := range cmd.Commands() {
		if hasRunnableDescendant(c) {
			return true
		}
	}
	return false
}

// policySourceLabel produces the "plugin:foo" / "yaml" / "" label that goes
// into CommandDeniedError.PolicySource and envelope.detail.policy_source.
//
// **Plugin name is included** because plugins live inside the binary and
// their names are part of the implementation contract; an integrator
// debugging a denial wants to know which plugin's Restrict() fired.
//
// **YAML file path is deliberately omitted** -- the envelope is observable
// by agents, CI logs, and other downstream systems, and the path leaks
// the user's home directory (e.g. /Users/alice/.lark-cli/policy.yml).
// The Denial.RuleName field already carries the human-identifier the user
// chose for their rule (yaml's "name:" field), which suffices for
// disambiguation. Use `config policy show` if the absolute path matters
// for a local debugging session.
func policySourceLabel(s ResolveSource) string {
	switch s.Kind {
	case SourcePlugin:
		return "plugin:" + s.Name
	case SourceYAML:
		return "yaml"
	}
	return ""
}

// reasonCodeForRisk picks the canonical reason_code for an exceeds-max-risk
// rejection.
func reasonCodeForRisk(risk platform.Risk) string {
	if risk == platform.RiskWrite || risk == platform.RiskHighRiskWrite {
		return "write_not_allowed"
	}
	return "risk_too_high"
}

// matchesAny reports whether path matches any of the doublestar globs.
// Invalid globs are skipped here -- they're rejected upstream by
// ValidateRule when the rule first enters the system.
func matchesAny(globs []string, path string) bool {
	_, ok := firstMatch(globs, path)
	return ok
}

// firstMatch returns the first glob in globs that matches path. Used by
// command_denylisted so the envelope can name the specific deny pattern
// that fired.
func firstMatch(globs []string, path string) (string, bool) {
	for _, g := range globs {
		if ok, err := doublestar.Match(g, path); err == nil && ok {
			return g, true
		}
	}
	return "", false
}

// hasIdentityIntersection reports whether the rule's typed identities
// share any value with the command's raw identity strings. Both slices
// are short (usually 1-2 identities) so a nested loop beats allocating
// a set.
func hasIdentityIntersection(rule []platform.Identity, cmd []string) bool {
	for _, x := range rule {
		for _, y := range cmd {
			if string(x) == y {
				return true
			}
		}
	}
	return false
}

// walkTree applies fn to every command in the tree, depth-first. Hidden
// commands are visited too -- they can still be invoked.
func walkTree(root *cobra.Command, fn func(*cobra.Command)) {
	if root == nil {
		return
	}
	fn(root)
	for _, c := range root.Commands() {
		walkTree(c, fn)
	}
}
