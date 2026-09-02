// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package recovery

import "strings"

// RenderContext carries invocation-local facts that affect recovery command
// rendering. It belongs to one command-tree build; business error producers do
// not receive it and therefore remain independent of CLI invocation details.
type RenderContext struct {
	// Profile is the explicit --profile override from this invocation. An empty
	// value means recovery commands retain their historical profile-free form.
	Profile string
}

// AuthLoginCommand returns the auth-login command for this invocation. suffix
// is a code-owned argument fragment (for example "--device-code <code>"); the
// invocation profile is always emitted as one shell-safe argv value.
func (c RenderContext) AuthLoginCommand(suffix string) string {
	command := "lark-cli auth login"
	if c.Profile != "" {
		// The equals form keeps a leading-dash profile value attached to its flag;
		// single-quote escaping prevents shell expansion or argument splitting.
		command += " --profile=" + shellQuote(c.Profile)
	}
	if suffix != "" {
		command += " " + suffix
	}
	return command
}

// InlineAuthLoginCommand wraps AuthLoginCommand in a Markdown code span. The
// default command keeps the historical single-backtick bytes; unusual profile
// values containing backticks receive a longer delimiter without changing the
// shell command itself.
func (c RenderContext) InlineAuthLoginCommand(suffix string) string {
	return inlineCode(c.AuthLoginCommand(suffix))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func inlineCode(value string) string {
	delimiter := "`"
	for strings.Contains(value, delimiter) {
		delimiter += "`"
	}
	if delimiter == "`" {
		return delimiter + value + delimiter
	}
	return delimiter + " " + value + " " + delimiter
}
