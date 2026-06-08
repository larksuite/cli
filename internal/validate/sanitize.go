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
// (except \n and \t), DEL (0x7f), and dangerous Unicode (zero-width joiners,
// RTL overrides, etc.) from text, preserving the readable content.
//
// Apply this anywhere a string is destined for a terminal — table-format
// stdout, every stderr message, and ALSO any human-readable string that is
// embedded inside a JSON/NDJSON payload for compatibility (e.g. a `message`
// field that mirrors what stderr just printed). The rule is "sanitize on
// write to a TTY," not "sanitize before json.Marshal": a JSON consumer that
// pretty-prints the payload still surfaces those bytes to a terminal, and
// the sanitization is a one-way transform that strips only renderer-control
// codes — readable characters, including Unicode letters, are preserved.
//
// Do NOT apply to typed identity / data fields whose programmatic consumers
// need raw bytes (e.g. `holder_user_name`, `requested[]`, `granted[]`):
// those carry data, and any escape-stripping there is the consumer's job
// once they know what context they will render to. JSON's own escaping
// already protects byte-level transport for the wire format.
//
// API responses (and persisted MultiAppConfig values originally sourced
// from API responses) may contain injected ANSI sequences that clear the
// screen, fake a colored "OK" status, or change the terminal title. In AI
// Agent scenarios, such injections can also pollute the LLM's context
// window with misleading output. The sanitize-on-write boundary keeps the
// data layer pristine while preventing any TTY-bound rendering surface
// from being hijacked.
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
