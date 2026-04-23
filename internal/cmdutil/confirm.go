// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"fmt"

	"github.com/larksuite/cli/internal/output"
)

// RequireConfirmation constructs a confirmation_required error with exit code
// ExitConfirmationRequired and a structured Risk envelope. Used by both
// shortcut and service command execution paths when a statically
// high-risk-write operation has not been confirmed with --yes.
//
// action identifies the operation for the agent (e.g. "mail +send",
// "drive.files.delete"). The envelope does not carry a pre-built retry
// command: agents already know their original invocation and only need to
// append --yes per the hint, which keeps the protocol free of shell-quoting
// pitfalls.
func RequireConfirmation(action string) error {
	return &output.ExitError{
		Code: output.ExitConfirmationRequired,
		Detail: &output.ErrDetail{
			Type:    "confirmation_required",
			Message: fmt.Sprintf("%s requires confirmation", action),
			Hint:    "add --yes to confirm",
			Risk: &output.RiskDetail{
				Level:  "high-risk-write",
				Action: action,
			},
		},
	}
}
