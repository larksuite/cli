// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
)

// RequireConfirmation constructs a typed *errs.ConfirmationRequiredError
// (exit code ExitConfirmationRequired) carrying the risk level and action as
// typed extension fields. Used by both shortcut and service command execution
// paths when a statically high-risk-write operation has not been confirmed
// with --yes.
//
// action identifies the operation for the agent (e.g. "mail +send",
// "drive.files.delete"). When the original invocation can be re-run safely,
// the hint carries the complete retry command with --yes appended — eval
// traces show agents always self-heal by appending --yes, so handing them
// the exact line saves the reconstruction step. The retry line is omitted
// (falling back to the plain hint) when any argument reads stdin (a bare "-",
// as its own token or bundled onto a flag as --flag=-, whose piped data a
// bare re-run would not reproduce), when any argument is a credential-bearing
// flag (echoing its value would copy the secret from the process table into
// the error envelope and every log that captures it), or when the rendered
// command would be unreasonably long to echo back.
func RequireConfirmation(action string) error {
	return requireConfirmationFor(action, os.Args)
}

// requireConfirmationFor is RequireConfirmation with an injectable argv,
// separated so tests can pin the composed hint deterministically.
func requireConfirmationFor(action string, args []string) error {
	err := errs.NewConfirmationRequiredError(errs.RiskHighRiskWrite, action,
		"%s requires confirmation", action)
	if retry := retryCommandWithYes(args); retry != "" {
		return err.WithHint("add --yes to confirm; re-run: %s", retry)
	}
	return err.WithHint("add --yes to confirm")
}

// retryCommandMaxLen caps the rendered retry command: past this, echoing the
// full invocation back (e.g. a +batch-update with a large inline JSON)
// costs more context than it saves.
const retryCommandMaxLen = 300

// retryCommandWithYes renders args as a shell-safe command line with --yes
// appended, or "" when a safe rendering isn't possible (see
// RequireConfirmation).
func retryCommandWithYes(args []string) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, filepath.Base(args[0]))
	for _, a := range args[1:] {
		if argReadsStdin(a) || argCarriesSecret(a) {
			return ""
		}
		parts = append(parts, shellQuoteArg(a))
	}
	parts = append(parts, "--yes")
	line := strings.Join(parts, " ")
	if len(line) > retryCommandMaxLen {
		return ""
	}
	return line
}

// argReadsStdin reports whether an argument makes a flag read from stdin — the
// portable bare "-" value, whether passed as its own token (--flag -) or
// bundled onto the flag (--flag=- / -f=-). Piped stdin is one-shot data a bare
// re-run cannot reproduce, so any such argument suppresses the retry line.
func argReadsStdin(a string) bool {
	if a == "-" {
		return true
	}
	if strings.HasPrefix(a, "-") {
		if i := strings.IndexByte(a, '='); i >= 0 && a[i+1:] == "-" {
			return true
		}
	}
	return false
}

// argCarriesSecret reports whether an argument is a credential-bearing flag
// (--app-secret, --token, --password, --api-key and spelling variants). The
// retry line is suppressed rather than redacted: a line with a masked value
// would fail verbatim, so the honest fallback is the plain add-yes hint. Only
// flag tokens are inspected — a positional value containing "token" (a file
// named token.json, a sheet title) must not suppress the line.
func argCarriesSecret(a string) bool {
	if !strings.HasPrefix(a, "-") {
		return false
	}
	name := strings.TrimLeft(a, "-")
	if i := strings.IndexByte(name, '='); i >= 0 {
		name = name[:i]
	}
	name = strings.ToLower(strings.ReplaceAll(name, "_", "-"))
	for _, marker := range []string{"secret", "token", "password", "passwd", "api-key", "apikey", "access-key"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

// shellQuoteArg single-quotes an argument when it contains any character a
// POSIX shell could interpret, so the retry line is copy-paste safe.
func shellQuoteArg(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\"'\\$`!*?[](){}<>|&;#~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
