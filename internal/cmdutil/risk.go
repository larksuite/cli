// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
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
