// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package agentsupport carries the shipped verdict on whether an Agent Employee
// principal may run a command.
//
// It is a leaf on purpose. The verdict is produced in two places -- the
// hand-maintained shortcut table and the generated API build -- and read by the
// command gates in internal/cmdutil, which cannot import either without a
// cycle. Without a shared home the gates compared a bare string literal, where
// a typo reads as no verdict recorded and quietly keeps the command.
package agentsupport

// Verdict is the recorded answer for one command. The zero value means no
// verdict was recorded.
type Verdict string

const (
	// Unknown is the zero value: nothing recorded, so the command stays
	// available. The open platform decides; passing an undecided call beats
	// refusing one it would have served.
	Unknown Verdict = ""
	// Allowed means every unconditional OpenAPI dependency accepts an Agent
	// Employee principal.
	Allowed Verdict = "allowed"
	// Denied means at least one unconditional dependency does not.
	Denied Verdict = "denied"
)

// Denies reports the one verdict that withholds a command from an Agent
// Employee. Every other value, Unknown included, keeps it.
func (v Verdict) Denies() bool { return v == Denied }
