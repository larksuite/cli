// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package beforeyouedit carries a process-level "read this before you edit"
// pointer for sheet-mutating commands. The notice is surfaced in JSON output
// envelopes under "_notice.before_you_edit", mirroring internal/deprecation.
//
// Unlike the update/skills notices wired in cmd/root.go, this one registers
// itself with the output package directly (see init), so it also surfaces in
// distributions that assemble the command tree via cmd.Build without running
// the root entry point's setupNotices — e.g. embedder binaries.
//
// A CLI process runs exactly one shortcut, so a single process-level slot is
// sufficient: the command's hooks record the notice before producing output,
// and the output layer reads it back when building the envelope.
package beforeyouedit

import (
	"sync/atomic"

	"github.com/larksuite/cli/internal/output"
)

// Notice points the calling agent at the skill reference that governs the
// edit it is about to make.
type Notice struct {
	Command string `json:"command"`
	Message string `json:"message"`
}

// pending stores the latest notice for the current process.
var pending atomic.Pointer[Notice]

// SetPending stores the notice for consumption by the output layer.
// Pass nil to clear.
func SetPending(n *Notice) { pending.Store(n) }

// GetPending returns the pending notice, or nil.
func GetPending() *Notice { return pending.Load() }

func init() {
	output.RegisterBuiltinNotice(func() (string, interface{}) {
		n := GetPending()
		if n == nil {
			return "", nil
		}
		return "before_you_edit", map[string]interface{}{
			"command": n.Command,
			"message": n.Message,
		}
	})
}
