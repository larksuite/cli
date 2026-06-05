// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package deprecation carries a process-level notice that the command currently
// being executed is a backward-compatibility alias, kept alive for users whose
// skill predates a refactor. The notice is surfaced in JSON output envelopes via
// output.PendingNotice (wired in cmd/root.go), mirroring internal/skillscheck.
//
// A CLI process runs exactly one shortcut, so a single process-level slot is
// sufficient: the command's Execute records the notice before producing output,
// and the output layer reads it back when building the envelope.
package deprecation

import (
	"strings"
	"sync/atomic"
)

// Notice describes a deprecated command alias and the current command that
// replaces it. Replacement and Skill are optional.
type Notice struct {
	Command     string `json:"command"`
	Replacement string `json:"replacement,omitempty"`
	Skill       string `json:"skill,omitempty"`
}

// Message returns a single-line, AI-agent-parseable description of the alias
// plus the canonical fix (update the skill). Mirrors the style of
// internal/skillscheck.StaleNotice.Message ("..., run: lark-cli update").
func (n *Notice) Message() string {
	var b strings.Builder
	b.WriteString(n.Command)
	b.WriteString(" is a pre-refactor compatibility alias")
	if n.Replacement != "" {
		b.WriteString("; use ")
		b.WriteString(n.Replacement)
		b.WriteString(" instead")
	}
	if n.Skill != "" {
		b.WriteString("; update your ")
		b.WriteString(n.Skill)
		b.WriteString(" skill, run: lark-cli update")
	} else {
		b.WriteString("; update your skill, run: lark-cli update")
	}
	return b.String()
}

// pending stores the latest deprecation notice for the current process.
var pending atomic.Pointer[Notice]

// SetPending stores the notice for consumption by output decorators.
// Pass nil to clear.
func SetPending(n *Notice) { pending.Store(n) }

// GetPending returns the pending deprecation notice, or nil.
func GetPending() *Notice { return pending.Load() }
