// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

// TaskState is the A2A task state, constant across all providers (9 states).
type TaskState string

const (
	StateSubmitted     TaskState = "submitted"
	StateWorking       TaskState = "working"
	StateInputRequired TaskState = "input_required"
	StateAuthRequired  TaskState = "auth_required"
	StateCompleted     TaskState = "completed"
	StateFailed        TaskState = "failed"
	StateCanceled      TaskState = "canceled"
	StateRejected      TaskState = "rejected"
	StateUnknown       TaskState = "unknown"
)

// IsTerminal reports whether the task has entered a terminal state.
func (s TaskState) IsTerminal() bool {
	switch s {
	case StateCompleted, StateFailed, StateCanceled, StateRejected:
		return true
	default:
		return false
	}
}

// ShouldStopPolling reports whether polling should stop: terminal state, or
// awaiting additional input / re-authentication.
func (s TaskState) ShouldStopPolling() bool {
	return s.IsTerminal() || s == StateInputRequired || s == StateAuthRequired
}
