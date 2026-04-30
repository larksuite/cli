// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import "github.com/spf13/cobra"

const riskLevelAnnotationKey = "risk_level"

// SetRisk stores a command's static risk level on cobra annotations so the
// help renderer (cmd/root.go) can surface a Risk: line without importing
// shortcuts/common. Levels follow the three-tier convention: "read" | "write"
// | "high-risk-write". Framework-level confirmation gating only acts on
// "high-risk-write".
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
