// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package surface describes the build-local command surface presented by one
// Cobra tree. It is deliberately independent from policy enforcement: a denied
// command may remain visible, or an opt-in distribution may conceal it.
package surface

import "strings"

// CommandID is a command's canonical slash-separated path relative to the
// binary root, for example "auth/login" or "config/strict-mode".
type CommandID string

// Canonical framework command IDs. Centralizing these paths keeps recovery,
// notices, diagnostics, and flag projection on the same exact-path vocabulary.
const (
	CommandAuthLogin         CommandID = "auth/login"
	CommandConfig            CommandID = "config"
	CommandConfigInit        CommandID = "config/init"
	CommandConfigBind        CommandID = "config/bind"
	CommandConfigStrictMode  CommandID = "config/strict-mode"
	CommandConfigPolicyShow  CommandID = "config/policy/show"
	CommandConfigPluginsShow CommandID = "config/plugins/show"
	CommandProfile           CommandID = "profile"
	CommandProfileAdd        CommandID = "profile/add"
	CommandProfileList       CommandID = "profile/list"
	CommandSchema            CommandID = "schema"
	CommandUpdate            CommandID = "update"
	CommandSkills            CommandID = "skills"
	CommandSkillsRead        CommandID = "skills/read"
)

// CommandState describes how one command is presented by a distribution.
type CommandState uint8

const (
	// CommandAvailable is the default for paths absent from a Plan.
	CommandAvailable CommandState = iota
	// CommandDeniedVisible keeps the command and references to it visible while
	// policy enforcement rejects execution.
	CommandDeniedVisible
	// CommandConcealed presents the command as absent from this distribution.
	CommandConcealed
)

// Plan is an immutable, build-local snapshot of command presentation states.
//
// NewPlan defensively copies its input and Plan exposes no mutator, so a Plan
// can safely be captured by the renderers belonging to one Cobra tree. The
// zero value (and a nil *Plan) describes the default, fully visible surface.
type Plan struct {
	commands map[CommandID]CommandState
}

// NewPlan returns an immutable snapshot of states. Paths omitted from states
// are CommandAvailable.
func NewPlan(states map[CommandID]CommandState) *Plan {
	commands := make(map[CommandID]CommandState, len(states))
	for id, state := range states {
		commands[id] = state
	}
	return &Plan{commands: commands}
}

// State returns the state explicitly recorded for id. It intentionally does
// not infer a child state from an ancestor; use IsConcealed or CanReference
// when effective presentation is required.
func (p *Plan) State(id CommandID) CommandState {
	if p == nil {
		return CommandAvailable
	}
	if state, ok := p.commands[id]; ok {
		return state
	}
	return CommandAvailable
}

// IsConcealed reports whether id or any canonical ancestor is concealed.
// Ancestor concealment dominates an explicit child state: a child cannot be
// presented through a parent that the distribution presents as absent.
func (p *Plan) IsConcealed(id CommandID) bool {
	for {
		if p.State(id) == CommandConcealed {
			return true
		}
		parent, ok := parentOf(id)
		if !ok {
			return false
		}
		id = parent
	}
}

// CanReference reports whether user-facing output may point at id.
// Denied-visible commands remain valid recovery targets because following the
// reference produces the existing, policy-rich denial. Concealed commands do
// not.
func (p *Plan) CanReference(id CommandID) bool {
	return !p.IsConcealed(id)
}

func parentOf(id CommandID) (CommandID, bool) {
	path := string(id)
	i := strings.LastIndexByte(path, '/')
	if i < 0 {
		return "", false
	}
	return CommandID(path[:i]), true
}
