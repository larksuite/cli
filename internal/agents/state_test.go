// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import "testing"

func TestIsTerminal(t *testing.T) {
	cases := map[TaskState]bool{
		StateSubmitted: false, StateWorking: false, StateInputRequired: false,
		StateAuthRequired: false, StateCompleted: true, StateFailed: true,
		StateCanceled: true, StateRejected: true, StateUnknown: false,
	}
	for s, want := range cases {
		if got := s.IsTerminal(); got != want {
			t.Errorf("%s.IsTerminal()=%v want %v", s, got, want)
		}
	}
}

func TestShouldStopPolling(t *testing.T) {
	stop := []TaskState{StateCompleted, StateFailed, StateCanceled, StateRejected, StateInputRequired, StateAuthRequired}
	cont := []TaskState{StateSubmitted, StateWorking, StateUnknown}
	for _, s := range stop {
		if !s.ShouldStopPolling() {
			t.Errorf("%s should stop polling", s)
		}
	}
	for _, s := range cont {
		if s.ShouldStopPolling() {
			t.Errorf("%s should keep polling", s)
		}
	}
}
