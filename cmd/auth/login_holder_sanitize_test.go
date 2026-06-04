// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"strings"
	"testing"
)

// formatHolderLabel feeds the human-readable advisory written to stderr
// when the soft holder mismatch fires. The userName it embeds comes from
// app.Users[].UserName, which is persisted from a prior login's IdP
// user-info response. A maliciously- (or accidentally-) crafted name
// carrying ANSI escapes, C0 control bytes, or zero-width Unicode would
// otherwise reach a tailing terminal and could rewrite preceding output,
// inject a fake `[lark-cli]` prefix, or hide a phishing message under a
// cursor-back sequence. validate.SanitizeForTerminal strips all of those.
//
// Threat model: a previously-authorized account whose display name was
// poisoned (compromised IdP, MITM during an earlier login on a different
// device, hand-edit of the on-disk MultiAppConfig) re-surfaces during a
// soft re-login by a different user. Without sanitization the WARN line
// can be hijacked. The fresh user's UserName comes from THIS login's
// user-info call, so the same poisoning vector applies if that response
// is attacker-controlled.
func TestFormatHolderLabel_StripsTerminalEscapes(t *testing.T) {
	cases := []struct {
		name     string
		userName string
		openId   string
		// wantNotContain pins what MUST NOT survive into the label —
		// asserting on absence catches new escape categories the
		// sanitizer learns to strip later.
		wantNotContain []string
		// wantContain pins what survives — the brand prefix still
		// embedded by the caller, the open_id verbatim (grep-bait),
		// and the printable name characters.
		wantContain []string
	}{
		{
			name:     "ansi-color escape gets stripped",
			userName: "\x1b[31mAlice\x1b[0m",
			openId:   "ou_alice",
			// \x1b is the lead byte of every CSI escape; if any bit
			// of it survives, a downstream terminal can still parse a
			// truncated escape and mis-render.
			wantNotContain: []string{"\x1b", "[31m", "[0m"},
			wantContain:    []string{"Alice", "ou_alice"},
		},
		{
			name:     "BEL and other C0 control bytes get stripped",
			userName: "Al\x07ice\x08",
			openId:   "ou_alice",
			wantNotContain: []string{"\x07", "\x08"},
			wantContain:    []string{"Alice", "ou_alice"},
		},
		{
			name:     "carriage-return cannot rewind the line",
			userName: "Alice\rEVIL",
			openId:   "ou_alice",
			// \r alone is enough to repaint the prefix; the sanitizer
			// drops it because it is < 0x20 and not in the {\n,\t}
			// allow-list.
			wantNotContain: []string{"\r"},
			wantContain:    []string{"Alice", "EVIL", "ou_alice"},
		},
		{
			name:     "DEL byte gets stripped",
			userName: "Alice\x7f",
			openId:   "ou_alice",
			wantNotContain: []string{"\x7f"},
			wantContain:    []string{"Alice", "ou_alice"},
		},
		{
			name:     "OSC sequence gets stripped",
			userName: "\x1b]0;evil-title\x07Alice",
			openId:   "ou_alice",
			// OSC starts with \x1b] and is terminated by BEL or
			// ST. SanitizeForTerminal's regex covers CSI; any
			// stray \x1b lead byte the regex misses is then caught
			// by the C0 sweep.
			wantNotContain: []string{"\x1b", "\x07"},
			wantContain:    []string{"Alice", "ou_alice"},
		},
		{
			// Defense-in-depth for the threat-model docstring's
			// "zero-width Unicode" line. SanitizeForTerminal routes
			// these runes through charcheck.IsDangerousUnicode (a
			// branch separate from CSI / C0 / DEL); we want a test
			// at the formatHolderLabel boundary that fails if a
			// future refactor swaps in a CSI-only stripper.
			//
			// U+200B = zero-width space (invisible joiner — could hide
			//          a name boundary so "Al​ice" reads as "Alice"
			//          but fails an exact-match comparison)
			// U+202E = right-to-left override (visually flips the
			//          tail of the name; classic "report.txt" disguise
			//          for "report.exe")
			name:     "zero-width and bidi override Unicode get stripped",
			userName: "Al​ice‮EVIL",
			openId:   "ou_alice",
			wantNotContain: []string{"​", "‮"},
			wantContain:    []string{"Alice", "EVIL", "ou_alice"},
		},
		{
			name:     "empty userName falls through to bare open_id (no sanitize call)",
			userName: "",
			openId:   "ou_alice",
			wantContain: []string{"ou_alice"},
		},
		{
			name:     "clean userName survives unmodified",
			userName: "Alice",
			openId:   "ou_alice",
			wantContain: []string{"Alice", "ou_alice", "Alice (ou_alice)"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatHolderLabel(tc.userName, tc.openId)
			for _, s := range tc.wantNotContain {
				if strings.Contains(got, s) {
					t.Errorf("label contained banned substring %q (bytes: %x)\nlabel: %q", s, []byte(s), got)
				}
			}
			for _, s := range tc.wantContain {
				if !strings.Contains(got, s) {
					t.Errorf("label missing required substring %q\nlabel: %q", s, got)
				}
			}
		})
	}
}

// End-to-end through verifyHolder's soft-mismatch branch: the Message
// field on the returned holderMismatchWarning is what gets written to
// stderr. It MUST have the poisoned name stripped. The typed
// HolderUserName / FreshUserName fields, by contrast, carry raw bytes
// for JSON consumers (per validate.SanitizeForTerminal's documented
// contract: not for json/ndjson output, programmatic consumers need
// the raw data and apply their own escape rules).
func TestVerifyHolder_SoftMismatch_SanitizesMessageButPreservesTypedFields(t *testing.T) {
	const poisonedHolder = "Al\x1b[31mice\x1b[0m"
	const poisonedFresh = "Bo\x07b"

	warning, abortErr := verifyHolder(
		"ou_alice", poisonedHolder, "", // implied holder source -> soft branch
		"ou_bob", poisonedFresh,
	)
	if abortErr != nil {
		t.Fatalf("expected soft warning, got abortErr: %v", abortErr)
	}
	if warning == nil {
		t.Fatal("expected non-nil holderMismatchWarning")
	}

	// Stderr-bound Message must NOT contain the escapes — that is the
	// whole point of M1.
	for _, banned := range []string{"\x1b", "[31m", "[0m", "\x07"} {
		if strings.Contains(warning.Message, banned) {
			t.Errorf("warning.Message leaked %q (bytes: %x): %q", banned, []byte(banned), warning.Message)
		}
	}
	// And it must still contain the readable parts so a human tailing
	// stderr can map open_ids to people.
	for _, want := range []string{"Alice", "Bob", "ou_alice", "ou_bob", "[lark-cli]", "[WARN]"} {
		if !strings.Contains(warning.Message, want) {
			t.Errorf("warning.Message lost expected fragment %q: %q", want, warning.Message)
		}
	}

	// Typed fields preserve raw bytes — JSON consumers need that, and
	// the sanitize-on-write convention is documented at
	// validate.SanitizeForTerminal. Asserting equality (not just
	// "contains") here proves no copy of the sanitizer leaked into the
	// typed-field path.
	if warning.HolderUserName != poisonedHolder {
		t.Errorf("HolderUserName was sanitized but should stay raw for JSON consumers; got %q want %q",
			warning.HolderUserName, poisonedHolder)
	}
	if warning.FreshUserName != poisonedFresh {
		t.Errorf("FreshUserName was sanitized but should stay raw for JSON consumers; got %q want %q",
			warning.FreshUserName, poisonedFresh)
	}
	// Open_ids are regex-validated upstream at the IdP boundary, so a
	// poisoned holder.UserName cannot bleed into them. Pin the
	// invariant so a future refactor cannot accidentally mix them.
	if warning.HolderOpenId != "ou_alice" || warning.FreshOpenId != "ou_bob" {
		t.Errorf("open_id fields corrupted: holder=%q fresh=%q", warning.HolderOpenId, warning.FreshOpenId)
	}
}
