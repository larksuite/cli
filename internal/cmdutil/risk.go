// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"fmt"

	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

const riskLevelAnnotationKey = "risk_level"

// Risk level constants — aliases of the canonical core.Risk* values, re-exported
// here so command code gets the risk vocabulary and the SetRisk/GetRisk helpers
// from one package. core is the single source of truth.
const (
	RiskRead          = core.RiskRead
	RiskWrite         = core.RiskWrite
	RiskHighRiskWrite = core.RiskHighRiskWrite
)

// SetRisk stores a command's static risk level on cobra annotations so the
// help renderer (cmd/root.go) can surface a Risk: line without importing
// shortcuts/common. Levels follow the three-tier convention: RiskRead |
// RiskWrite | RiskHighRiskWrite. Framework-level confirmation gating only
// acts on RiskHighRiskWrite.
func SetRisk(cmd *cobra.Command, level string) {
	if level == "" {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[riskLevelAnnotationKey] = level
}

// GetRisk returns the static risk level. ok is true when the command has a
// risk annotation.
func GetRisk(cmd *cobra.Command) (level string, ok bool) {
	if cmd.Annotations == nil {
		return "", false
	}
	level, ok = cmd.Annotations[riskLevelAnnotationKey]
	return level, ok && level != ""
}

// RiskLine renders the "Risk: <level>" line shown in help. ok is false when the
// command carries no risk annotation.
//
// The self-approval ban is keyed on the presence of a --yes flag, not on the
// risk level. The sentence asserts that passing --yes means the USER confirmed,
// so it belongs wherever that flag exists and the command checks it — and
// nowhere else. Both halves of that rule matter in practice:
//
//   - A command may carry a risk annotation without wiring a --yes gate (e.g.
//     `update`, which has no confirmation step at all). Warning those callers
//     about --yes would name a flag they cannot pass, so they get the bare
//     "Risk: <level>" line.
//   - A command may gate on --yes while declaring a level below
//     high-risk-write. `drive +push` and `drive +pull` take --yes to authorize
//     deleting remote or local files, and `apps +env-set` takes it to authorize
//     writing the online environment, all at write level. An agent that
//     self-approves --yes there destroys files just as surely as it would on a
//     high-risk-write command, so the ban must reach them too.
//
// The parenthetical differs between the two gate shapes because they mean
// different things: at high-risk-write the framework refuses the whole command
// without --yes, whereas at lower levels --yes authorizes one destructive step
// (a flag or an argument value) while the rest of the command runs unguarded.
// Claiming the whole command needs confirmation in the latter case would be
// false.
//
// The returned line has no surrounding whitespace; callers add their own
// separators.
func RiskLine(cmd *cobra.Command) (line string, ok bool) {
	level, ok := GetRisk(cmd)
	if !ok {
		return "", false
	}
	if cmd.Flags().Lookup("yes") == nil {
		return fmt.Sprintf("Risk: %s", level), true
	}
	if level == RiskHighRiskWrite {
		return fmt.Sprintf("Risk: %s (requires explicit user confirmation to execute; %s)", level, core.YesSelfApprovalBan), true
	}
	return fmt.Sprintf("Risk: %s (--yes authorizes a destructive step of this command; %s)", level, core.YesSelfApprovalBan), true
}
