// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"os"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/envvars"
)

// The runtime half of the risk-declaration defence. The type (core.Risk) stops
// a bare string from reaching a declaration, and the manifest CI rule rejects
// a value outside the enum before it ships — but neither covers a value that
// arrives at runtime through a string boundary: generated service metadata, a
// cobra annotation rewritten by a policy stub, a hand-built command in a test.
// This is where such a value is caught, and it fails closed: an unrecognised
// level is never treated as `read`.
//
// The rules:
//
//   - declared high-risk-write        → confirmation gate (--yes)
//   - declared outside the taxonomy   → refuse to execute; it is a declaration
//     bug, and running the command is the one outcome that cannot be undone
//   - refusal downgraded via env      → treat as the highest tier, i.e. still
//     gated on --yes, never allowed through silently
//   - absent / read / write           → no gate

// invalidRiskDeclaration reports whether level is present but outside the
// closed taxonomy. An absent level ("") is a legal state — it means the
// command is not annotated and defaults to read — and is deliberately not
// reported here, so "unannotated" and "misspelled" never collapse into one
// branch.
func invalidRiskDeclaration(level Risk) bool {
	return level != "" && !level.IsValid()
}

// RequiresConfirmation reports whether a declared level must pass the --yes
// gate. Invalid levels are included: if the refusal is downgraded they still
// have to be confirmed, and if it is not, EnforceRiskDeclaration has already
// rejected the call.
func RequiresConfirmation(level Risk) bool {
	return level == RiskHighRiskWrite || invalidRiskDeclaration(level)
}

// EnforceRiskDeclaration returns a non-nil error when a command must not run
// because its declared risk level is not in the closed taxonomy. Call it
// before the confirmation gate; a nil return means the declaration is usable
// (possibly after being downgraded to the highest tier by the env switch).
//
// action identifies the operation for the agent, in the same shape used by
// RequireConfirmation ("drive +delete", "drive.files.delete").
func EnforceRiskDeclaration(action string, level Risk) error {
	if !invalidRiskDeclaration(level) {
		return nil
	}
	if allowInvalidRisk() {
		return nil
	}
	return errs.NewInternalError(errs.SubtypeInvalidRiskDeclaration,
		"%s declares risk %q, which is not one of read|write|high-risk-write", action, level).
		WithHint("this is a bug in the command declaration; report it. Set %s=1 to run the command under the highest-risk confirmation gate instead", envvars.CliAllowInvalidRisk)
}

// allowInvalidRisk reports whether the operator opted into the downgrade. Any
// unrecognised value counts as "not set": an escape hatch that fails open on
// a typo would reintroduce the bug it exists to work around.
func allowInvalidRisk() bool {
	switch strings.TrimSpace(strings.ToLower(os.Getenv(envvars.CliAllowInvalidRisk))) {
	case "1", "true", "on", "yes", "y":
		return true
	default:
		return false
	}
}
