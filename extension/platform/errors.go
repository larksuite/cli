// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

import "fmt"

// CommandDeniedError is the structured error returned by a denyStub. Every
// pruned-command execution path -- direct invocation, alias expansion,
// internal call -- returns this exact type. The dispatcher converts it to a
// typed errs.* error; the Layer field carries the denial layer for the
// envelope.
//
// Layer values:
//
//   - "strict_mode" -- credential strict-mode rejected the command
//   - "policy"      -- user-layer Rule rejected the command
//
// PolicySource is a free-form identifier such as "plugin:secaudit",
// "yaml:mywork", or "strict-mode". Reason fields:
//
//   - ReasonCode -- closed enum, see tech-doc 5.3 (e.g. write_not_allowed,
//     all_children_denied, identity_not_supported)
//   - Reason     -- human-readable text
type CommandDeniedError struct {
	Path         string
	Layer        string
	PolicySource string
	RuleName     string
	ReasonCode   string
	Reason       string
}

// Error implements the standard error interface.
func (e *CommandDeniedError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("command %q denied: %s", e.Path, e.Reason)
	}
	return fmt.Sprintf("command %q denied (%s/%s)", e.Path, e.Layer, e.ReasonCode)
}
