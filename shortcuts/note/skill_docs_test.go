// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// Capability source test: pins the identity (user/bot) claims made in
// skills/lark-note against the AuthTypes actually declared on the
// shortcuts, and pins the bot+unified stop-path guidance PR #2278's review
// found missing: NoteTranscript stays user-only, so any doc reachable from a
// `+detail --as bot` -> unified routing decision must say so explicitly
// instead of silently falling back to --as user.

package note

import (
	"os"
	"strings"
	"testing"
)

func hasAuthType(authTypes []string, want string) bool {
	for _, a := range authTypes {
		if a == want {
			return true
		}
	}
	return false
}

func readSkillDoc(t *testing.T, relPath string) string {
	t.Helper()
	data, err := os.ReadFile("../../" + relPath)
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	return string(data)
}

func TestNoteIdentityDocsMatchAuthTypes(t *testing.T) {
	if !hasAuthType(NoteDetail.AuthTypes, "bot") {
		t.Fatalf("NoteDetail.AuthTypes = %v, want bot included (this PR's contract)", NoteDetail.AuthTypes)
	}
	if hasAuthType(NoteTranscript.AuthTypes, "bot") {
		t.Fatalf("NoteTranscript.AuthTypes = %v now includes bot; update the bot+unified stop-path guidance below (and this test) instead of leaving it stale", NoteTranscript.AuthTypes)
	}

	skill := readSkillDoc(t, "skills/lark-note/SKILL.md")
	if !strings.Contains(skill, "`+detail` 支持 `--as user` / `--as bot`") {
		t.Error("skills/lark-note/SKILL.md must state that +detail supports both user and bot")
	}
	if !strings.Contains(skill, "`+transcript` 仅支持 `--as user`") {
		t.Error("skills/lark-note/SKILL.md must state that +transcript is user-only (matches NoteTranscript.AuthTypes)")
	}
}

// TestNoteUnifiedBotStopPathIsDocumented pins that every doc reachable from a
// `note +detail` -> `note_display_type=unified` routing decision explicitly
// calls out that `+transcript` can't continue under --as bot, rather than
// letting an agent silently drop --as and switch identity.
func TestNoteUnifiedBotStopPathIsDocumented(t *testing.T) {
	for _, path := range []string{
		"skills/lark-note/SKILL.md",
		"skills/lark-note/references/lark-note-detail.md",
		"skills/lark-note/references/lark-note-transcript.md",
	} {
		content := readSkillDoc(t, path)
		if !strings.Contains(content, "bot") || !strings.Contains(content, "user") {
			t.Errorf("%s must document the bot+unified boundary for note +transcript (mention both user and bot)", path)
		}
	}
}
