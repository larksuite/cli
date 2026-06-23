// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"github.com/larksuite/cli/errs"
)

// RequireConfirmation constructs a typed *errs.ConfirmationRequiredError
// (exit code ExitConfirmationRequired) carrying the risk level and action as
// typed extension fields. Used by both shortcut and service command execution
// paths when a statically high-risk-write operation has not been confirmed
// with --yes.
//
// action identifies the operation for the agent (e.g. "mail +send",
// "drive.files.delete"). The envelope does not carry a pre-built retry
// command: agents already know their original invocation and only need to
// append --yes per the hint, which keeps the protocol free of shell-quoting
// pitfalls.
func RequireConfirmation(action string) error {
	return errs.NewConfirmationRequiredError(errs.RiskHighRiskWrite, action,
		"%s requires confirmation", action).
		WithHint("add --yes to confirm")
}
