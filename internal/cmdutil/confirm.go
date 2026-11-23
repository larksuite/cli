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
// "drive.files.delete"). The hint is deliberately NOT a pre-built retry
// command: argv cannot faithfully reproduce the original invocation (pipeline
// producers, stdin bytes, redirections, inline env and the executable's real
// path are all gone), POSIX quoting does not survive PowerShell/cmd.exe, and
// echoing argv values can copy credentials or free-form payloads (--sql,
// --json) into the error envelope and every log that captures it. Per the
// lark-shared approval protocol, the caller that obtained the user's consent
// appends --yes to its own saved argv array and re-executes.
func RequireConfirmation(action string) error {
	err := errs.NewConfirmationRequiredError(errs.RiskHighRiskWrite, action,
		"%s requires confirmation", action)
	return err.WithHint("add --yes to confirm")
}
