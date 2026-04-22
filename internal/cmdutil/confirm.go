// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"fmt"
	"os"
	"strings"

	"github.com/larksuite/cli/internal/output"
)

// RequireConfirmation constructs a confirmation_required error with exit code
// ExitConfirmationRequired and a structured Risk envelope. Used by both
// shortcut and service command execution paths when a statically
// high-risk-write operation has not been confirmed with --yes.
//
// action identifies the operation for the agent (e.g. "mail +send",
// "drive.files.delete").
func RequireConfirmation(action string) error {
	return &output.ExitError{
		Code: output.ExitConfirmationRequired,
		Detail: &output.ErrDetail{
			Type:       "confirmation_required",
			Message:    fmt.Sprintf("%s requires confirmation", action),
			Hint:       "add --yes to confirm",
			FixCommand: buildFixCommand(),
			Risk: &output.RiskDetail{
				Level:  "high-risk-write",
				Action: action,
			},
		},
	}
}

// buildFixCommand returns the original invocation with --yes appended, so the
// agent can surface a concrete command to retry after user confirmation.
// Returns an empty string when --yes is already present or os.Args is empty.
func buildFixCommand() string {
	if len(os.Args) == 0 {
		return ""
	}
	for _, a := range os.Args[1:] {
		if a == "--yes" || a == "-y" {
			return ""
		}
	}
	parts := make([]string, 0, len(os.Args)+1)
	parts = append(parts, os.Args...)
	parts = append(parts, "--yes")
	return strings.Join(parts, " ")
}
