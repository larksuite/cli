// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"fmt"

	"github.com/larksuite/cli/errs"
)

// AgentRefusalReason is why a command is withheld from an Agent Employee.
//
// It is the only thing that varies between the refusals: the sentence is always
// the same and the advice follows from the reason. Both the build-time stub
// (cmd/prune.go, which withholds the command from the tree) and the run-time
// gates below take their wording from here, so one command cannot be explained
// two different ways depending on where it was stopped.
type AgentRefusalReason int

const (
	// AgentRefusedShortcutVerdict: the shipped verdict denies this shortcut.
	// The verdict is settled against the shortcut's OpenAPI dependencies, so no
	// capability grant on the Agent platform can change it.
	AgentRefusedShortcutVerdict AgentRefusalReason = iota
	// AgentRefusedUnmarkedMethod: the owning service has agent-marked methods
	// but this one is not among them, which a capability grant may yet fix.
	AgentRefusedUnmarkedMethod
	// AgentRefusedUnmarkedService: no method in the owning service carries the
	// marker at all, so nothing local or grantable helps until its metadata does.
	AgentRefusedUnmarkedService
	// AgentRefusedHumanOnly: the command only means something for a human
	// account -- an interactive login and the like.
	AgentRefusedHumanOnly

	// agentRefusalReasonCount must stay last. Hint answers each reason by hand
	// and returns "" for anything it does not know, so a reason added without a
	// wording would ship a refusal that says nothing about what to do instead;
	// this is what the exhaustiveness test counts against.
	agentRefusalReasonCount
)

// Hint returns the advice that goes with the reason. The wordings live together
// so the differences between them stay visible to whoever edits one.
func (r AgentRefusalReason) Hint() string {
	switch r {
	case AgentRefusedShortcutVerdict:
		return "one of the OpenAPIs this shortcut calls does not accept an Agent Employee " +
			"token, so no capability grant can unlock it; run this command with a profile backed by a human account"
	case AgentRefusedUnmarkedMethod:
		return "grant this Agent Employee the capability it needs on the Agent " +
			"platform, or run this command with a profile backed by a human account"
	case AgentRefusedUnmarkedService:
		return "this API is not yet marked as Agent-Employee-callable in its " +
			"service's OpenAPI metadata; this is not something the current profile can fix — ask the service's " +
			"owning team to add the `agent` marker and regenerate api_definition on the open platform"
	case AgentRefusedHumanOnly:
		return "Manage this Agent on the Agent platform, or select a profile backed by a human account."
	}
	return ""
}

// AgentRefusalMessage is the sentence every agent refusal leads with. The stub
// needs it as a string for the denial record; the gates get it inside an error
// through AgentRefusal.
func AgentRefusalMessage(label string) string {
	return fmt.Sprintf("%q is not available for an Agent Employee profile", label)
}

// AgentRefusal is the refusal itself: one message, the reason's advice, and the
// subtype that says this was decided locally before any request went out.
func AgentRefusal(label string, reason AgentRefusalReason) error {
	return errs.NewValidationError(errs.SubtypeFailedPrecondition, "%s", AgentRefusalMessage(label)).
		WithHint("%s", reason.Hint())
}
