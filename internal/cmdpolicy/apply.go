// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy

import (
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/platform"
)

// Apply walks the command tree and installs denyStubs for every path in
// deniedByPath whose Denial.Layer == "policy". It is the user-layer
// counterpart to applyStrictModeDenials in cmd/prune.go; both consume the
// same deniedByPath map produced by the bootstrap pipeline, neither
// re-evaluates rules.
//
// Three things must happen for every denied command (hard-constraints 1-4
// in the tech doc):
//
//  1. cmd.Hidden = true                -- removes from help / completion
//  2. cmd.DisableFlagParsing = true    -- denial-wins invariant; otherwise
//     cobra would intercept the call
//     with "missing required flag"
//     before we can return our error
//  3. cmd.RunE = denyStub(denial)      -- returns a typed
//     *errs.ValidationError so
//     cmd/root.go's envelope writer
//     emits structured JSON; the
//     wrapped error chain still
//     exposes *platform.CommandDeniedError
//     via errors.As for in-process
//     consumers
//
// Apply must be called once during the Bootstrap pipeline BEFORE
// cobra.Execute. It mutates the command tree in place and is not safe to
// call concurrently with command dispatch. Returns the number of commands
// modified.
func Apply(root *cobra.Command, deniedByPath map[string]Denial) int {
	if root == nil || len(deniedByPath) == 0 {
		return 0
	}

	count := 0
	walkTree(root, func(c *cobra.Command) {
		// Never install a denyStub on the binary root itself. Even if the
		// aggregation pass somehow marked it (e.g. all-children-denied at
		// the top), the binary entry point must remain dispatchable so
		// cobra's own help / completion paths still work.
		if !c.HasParent() {
			return
		}
		path := CanonicalPath(c)
		if path == "" {
			return
		}
		d, ok := deniedByPath[path]
		if !ok || d.Layer != LayerPolicy {
			return
		}
		if installDenyStub(c, path, d) {
			count++
		}
	})
	return count
}

// AnnotationDenialLayer / AnnotationDenialSource carry the denial
// signal to internal/hook through cobra annotations, avoiding an
// import cycle between hook and cmdpolicy.
const (
	AnnotationDenialLayer  = "lark:policy_denied_layer"
	AnnotationDenialSource = "lark:policy_denied_source"

	// AnnotationPureGroup marks a cobra.Command that is logically a
	// parent-only group but had a RunE attached by the bootstrap-time
	// unknown-subcommand guard. The engine treats annotated commands
	// the same as un-annotated parent groups (no RunE): they are not
	// evaluated against the Rule, and aggregateParents does not treat
	// them as hybrids.
	//
	// Without this signal, a user enabling a policy.yml with
	// max_risk: read would see every group (`lark-cli drive --help`,
	// `lark-cli docs --help`) return exit 2 + risk_not_annotated,
	// because the guard's RunE flips Runnable()=true and the engine
	// then demands a risk_level annotation on the group itself.
	AnnotationPureGroup = "lark:cmd_pure_group"
)

// IsPureGroup reports whether cmd carries the AnnotationPureGroup marker.
// Used by the engine to skip evaluation and by the aggregator to treat the
// command as a parent-only group regardless of cobra's Runnable() answer.
func IsPureGroup(cmd *cobra.Command) bool {
	if cmd == nil || cmd.Annotations == nil {
		return false
	}
	return cmd.Annotations[AnnotationPureGroup] == "true"
}

// CommandDeniedFromDenial materialises the wrapped error type carried
// on ExitError.Err so errors.As works for in-process consumers.
func CommandDeniedFromDenial(path string, d Denial) *platform.CommandDeniedError {
	return &platform.CommandDeniedError{
		Path:         path,
		Layer:        d.Layer,
		PolicySource: d.PolicySource,
		RuleName:     d.RuleName,
		ReasonCode:   d.ReasonCode,
		Reason:       d.Reason,
	}
}

// BuildDenialError is the default typed error for user-layer denials:
// Message comes from CommandDeniedError.Error(); the policy layer, source,
// rule name, and reason code are folded into the Hint. The
// *platform.CommandDeniedError is preserved as the Cause so errors.As
// works for in-process consumers.
func BuildDenialError(path string, d Denial) *errs.ValidationError {
	cd := CommandDeniedFromDenial(path, d)
	return errs.NewValidationError(errs.SubtypeFailedPrecondition, "%s", cd.Error()).
		WithHint("denied by %s policy (source %s, rule %q, reason_code %s); adjust the policy configuration to allow this command",
			cd.Layer, cd.PolicySource, cd.RuleName, cd.ReasonCode).
		WithCause(cd)
}

// installDenyStub mutates a cobra.Command in place. Unlike cmd/prune.go
// which does RemoveCommand+AddCommand (changing the pointer), we modify
// the existing node so any external reference (snapshots, alias targets)
// continues to point at the same cmd.
//
// Help fields (cmd.Short / cmd.Long / cmd.Flags()) are deliberately
// preserved so `--help` on a denied command still describes what the
// command was intended to do.
//
// Two cobra Annotations are set as a denial signal that internal/hook
// reads (without taking a dependency on this package):
//
//   - AnnotationDenialLayer  -> "policy" or "strict_mode"
//   - AnnotationDenialSource -> the PolicySource ("yaml", "plugin:foo", ...)
//
// Returns true when the stub was actually installed and false on the
// strict-mode early-return so callers can compute an accurate "commands
// modified" count.
func installDenyStub(cmd *cobra.Command, path string, d Denial) bool {
	// strict-mode wins over user-layer pruning. If the command was
	// already replaced by a strict-mode stub (cmd/prune.go::strictModeStubFrom
	// writes layer=strict_mode), do NOT overwrite -- the user-layer
	// rule cannot relax or relabel a credential-hard boundary.
	//
	// Behaviour without this guard (pre-fix): a user yaml rule matching
	// a strict-mode stub's path would replace the RunE with the pruning
	// denyStub, hiding the original strict-mode error message AND
	// re-labelling detail.layer from "strict_mode" to "policy".
	if cmd.Annotations != nil &&
		cmd.Annotations[AnnotationDenialLayer] == LayerStrictMode {
		return false
	}
	cmd.Hidden = true
	cmd.DisableFlagParsing = true

	// Bypass cobra's pre-RunE gates that would otherwise short-circuit
	// before the wrapped RunE (= where observers + denial guard live):
	//
	//   1. Args validator: original commands often declare cobra.NoArgs
	//      or a custom Args function. With DisableFlagParsing=true,
	//      `--doc xxx` looks like positional args; cobra.ValidateArgs
	//      fires BEFORE PersistentPreRunE / PreRunE / RunE and would
	//      surface a Cobra usage error instead of our pruning envelope.
	//      ArbitraryArgs accepts everything.
	//
	//   2. Parent's PersistentPreRunE: cobra's "first PersistentPreRunE
	//      wins" walks UP from the leaf. cmd/auth/auth.go declares a
	//      PersistentPreRunE that returns external_provider when env
	//      credentials are set; without our leaf-level override, that
	//      fires before pruning's RunE and the caller sees the wrong
	//      envelope. We set a no-op leaf PersistentPreRunE that just
	//      silences usage and returns nil, so dispatch proceeds to the
	//      wrapped RunE (which produces the real pruning envelope and
	//      lets Before/After observers fire).
	cmd.Args = cobra.ArbitraryArgs
	cmd.PersistentPreRunE = func(c *cobra.Command, _ []string) error {
		c.SilenceUsage = true
		return nil
	}
	cmd.PersistentPreRun = nil
	cmd.PreRunE = nil
	cmd.PreRun = nil

	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[AnnotationDenialLayer] = d.Layer
	cmd.Annotations[AnnotationDenialSource] = d.PolicySource

	denial := d // capture by value for the closure
	cmd.RunE = func(c *cobra.Command, args []string) error {
		// The typed message carries the user-facing semantic ("a command
		// was denied"); the hint carries the layer / source / rule
		// distinction ("policy" vs "strict_mode") for debugging.
		return BuildDenialError(path, denial)
	}
	// Clear any pre-existing Run hook: cobra prefers RunE when both are
	// set, but leaving a stale Run around is a foot-gun for future
	// maintainers.
	cmd.Run = nil
	return true
}
