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

// YesSelfApprovalBan is the sentence every surface must carry when it mentions
// --yes: the flag asserts that the USER confirmed, so an agent may never add it
// on its own initiative. Each surface embeds this in its own context (help line,
// schema property, exit-10 hint) instead of wording its own version — three
// independently-worded copies are what let the weakest one quietly become the
// rule agents actually follow.
const YesSelfApprovalBan = "the agent must NOT add --yes on its own — only pass --yes after the user has confirmed"
