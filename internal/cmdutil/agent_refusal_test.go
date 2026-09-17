// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

// allAgentRefusalReasons is every reason, in declaration order. Counting it
// against agentRefusalReasonCount fails when a reason is added without being
// listed here, which is what keeps the coverage of the two tests complete.
var allAgentRefusalReasons = []AgentRefusalReason{
	AgentRefusedShortcutVerdict,
	AgentRefusedUnmarkedMethod,
	AgentRefusedUnmarkedService,
	AgentRefusedHumanOnly,
}

// A reason whose Hint the switch does not answer returns "", which would ship a
// refusal telling the agent nothing about what to do instead.
func TestEveryAgentRefusalReasonHasItsOwnHint(t *testing.T) {
	if got, want := len(allAgentRefusalReasons), int(agentRefusalReasonCount); got != want {
		t.Fatalf("listed %d reasons, %d are declared: list the new one", got, want)
	}
	seen := map[string]AgentRefusalReason{}
	for _, reason := range allAgentRefusalReasons {
		hint := reason.Hint()
		if strings.TrimSpace(hint) == "" {
			t.Errorf("reason %d has no hint", reason)
			continue
		}
		if first, dup := seen[hint]; dup {
			t.Errorf("reason %d repeats the hint of reason %d: %q", reason, first, hint)
			continue
		}
		seen[hint] = reason
	}
}

// The refusal is decided locally, before any request goes out, so it must carry
// the precondition subtype rather than anything the server could have said.
func TestAgentRefusalCarriesLabelHintAndSubtype(t *testing.T) {
	for _, reason := range allAgentRefusalReasons {
		err := AgentRefusal("im +send", reason)
		var vErr *errs.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("reason %d: want *errs.ValidationError, got %T", reason, err)
		}
		if vErr.Subtype != errs.SubtypeFailedPrecondition {
			t.Errorf("reason %d: subtype = %q, want %q", reason, vErr.Subtype, errs.SubtypeFailedPrecondition)
		}
		if !strings.Contains(vErr.Error(), `"im +send"`) {
			t.Errorf("reason %d: message does not name the command: %q", reason, vErr.Error())
		}
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("reason %d: expected a problem envelope", reason)
		}
		if p.Hint != reason.Hint() {
			t.Errorf("reason %d: hint = %q, want %q", reason, p.Hint, reason.Hint())
		}
	}
}
