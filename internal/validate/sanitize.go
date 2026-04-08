// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package validate

import (
	"regexp"
	"strings"

	"github.com/larksuite/cli/internal/charcheck"
)

// ansiEscape matches ANSI CSI sequences (ESC[ ... letter) and OSC sequences (ESC] ... BEL).
// Private CSI sequences (e.g. ESC[?25l) use the extended parameter byte range [0-9;?>=!].
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?>=!]*[a-zA-Z]|\x1b\][^\x07]*\x07`)

// SanitizeForTerminal strips ANSI escape sequences, C0 control characters
// (except \n and \t), and dangerous Unicode from text, preserving the actual
// readable content. It should be applied to table format output and stderr
// messages, but NOT to json/ndjson output where programmatic consumers need
// the raw data.
//
// API responses may contain injected ANSI sequences that clear the screen,
// fake a colored "OK" status, or change the terminal title. In AI Agent
// scenarios, such injections can also pollute the LLM's context window
// with misleading output.
func SanitizeForTerminal(text string) string {
	if strings.ContainsRune(text, '\x1b') {
		text = ansiEscape.ReplaceAllString(text, "")
	}
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			continue
		case charcheck.IsDangerousUnicode(r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
