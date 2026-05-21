// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package lintapi defines the shared types every lint domain returns from
// its scan entry point. New lint domains (sibling packages under lint/)
// MUST return []lintapi.Violation so cmd/main can aggregate and report
// uniformly. The domain may add its own private types for internal use.
package lintapi

// Action enumerates the response modes for a violation.
type Action string

const (
	// ActionReject hard-fails CI. Only REJECT contributes to a nonzero
	// lintcheck exit code.
	ActionReject Action = "REJECT"
	// ActionLabel emits a diagnostic so CI can label the PR but does not fail.
	ActionLabel Action = "LABEL"
	// ActionWarning surfaces a reviewer-attention note without failing CI.
	// CI does NOT exit nonzero on warnings; they are reviewer signal only.
	ActionWarning Action = "WARNING"
)

// Violation describes a single lint hit. Rule identifies which check
// produced it; the domain package owns the rule namespace.
type Violation struct {
	Rule       string
	Action     Action
	File       string
	Line       int
	Message    string
	Suggestion string
}
