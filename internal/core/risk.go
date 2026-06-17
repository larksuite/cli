// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

// Risk levels — the three-tier convention used across the CLI. They live here,
// at the leaf, so the envelope renderer (internal/schema) and the command
// toolkit (internal/cmdutil) share one vocabulary without a renderer depending
// on command utilities. Framework confirmation gating acts only on
// RiskHighRiskWrite.
const (
	RiskRead          = "read"
	RiskWrite         = "write"
	RiskHighRiskWrite = "high-risk-write"
)
