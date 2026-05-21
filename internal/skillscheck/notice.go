// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package skillscheck verifies that the locally installed lark-cli
// skills are in sync with the running binary version, by comparing
// the current binary version against a stamp file written when skills
// are last synced (by `lark-cli update`). On mismatch it stores a
// notice for injection into JSON envelopes via output.PendingNotice.
package skillscheck

import (
	"fmt"
	"sync/atomic"
)

// StaleNotice signals that the locally synced skills version does not
// match the running binary. Current is the last successfully synced
// version (always non-empty — Init no longer emits a notice on cold
// start). Target is the running binary version. Mirrors
// internal/update.UpdateInfo's pending-notice pattern.
type StaleNotice struct {
	Current string `json:"current"`
	Target  string `json:"target"`
}

// Message returns a single-line, AI-agent-parseable description of the
// drift plus the canonical fix command. Mirrors internal/update.UpdateInfo.Message
// in style ("..., run: lark-cli update" suffix). Current is guaranteed
// non-empty because Init only emits a StaleNotice for the drift case
// (stamp present and != binary version).
func (s *StaleNotice) Message() string {
	return fmt.Sprintf(
		"lark-cli skills %s out of sync with binary %s, run: lark-cli update",
		s.Current, s.Target,
	)
}

// pending stores the latest stale notice for the current process.
var pending atomic.Pointer[StaleNotice]

// SetPending stores the stale notice for consumption by output decorators.
// Pass nil to clear.
func SetPending(n *StaleNotice) { pending.Store(n) }

// GetPending returns the pending stale notice, or nil.
func GetPending() *StaleNotice { return pending.Load() }
