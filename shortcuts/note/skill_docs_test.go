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

	scene := readSkillDoc(t, "skills/lark-meeting/scenes/query-note-and-artifacts.md")
	if !strings.Contains(scene, "`note +detail` 支持 `--as user` / `--as bot`") {
		t.Error("query-note-and-artifacts.md must state that note +detail supports both user and bot")
	}
	if !strings.Contains(scene, "`note +transcript` 仅支持 `--as user`") {
		t.Error("query-note-and-artifacts.md must state that note +transcript is user-only (matches NoteTranscript.AuthTypes)")
	}
}

// TestNoteUnifiedBotStopPathIsDocumented pins that every doc reachable from a
// `note +detail` -> `note_display_type=unified` routing decision explicitly
// calls out that `+transcript` can't continue under --as bot, rather than
// letting an agent silently drop --as and switch identity.
func TestNoteUnifiedBotStopPathIsDocumented(t *testing.T) {
	for _, path := range []string{
		"skills/lark-meeting/scenes/query-note-and-artifacts.md",
		"skills/lark-meeting/references/lark-note-detail.md",
		"skills/lark-meeting/references/lark-note-transcript.md",
	} {
		content := readSkillDoc(t, path)
		if !strings.Contains(content, "bot") || !strings.Contains(content, "user") {
			t.Errorf("%s must document the bot+unified boundary for note +transcript (mention both user and bot)", path)
		}
	}
}

func TestLegacyNoteSkillRoutesToMeetingSkill(t *testing.T) {
	skill := readSkillDoc(t, "skills/lark-note/SKILL.md")
	for _, must := range []string{
		"本技能只用于兼容旧名称，不直接处理业务。",
		"../lark-meeting/SKILL.md",
	} {
		if !strings.Contains(skill, must) {
			t.Errorf("skills/lark-note/SKILL.md must preserve compatibility routing %q", must)
		}
	}
}

func TestMeetingNoteCoverHandlingIsReachable(t *testing.T) {
	noteScene := readSkillDoc(t, "skills/lark-meeting/scenes/query-note-and-artifacts.md")
	for _, must := range []string{
		"第一个 `<whiteboard",
		"docs +media-download",
		"--type whiteboard",
		"./notes/<note_id>/cover",
		"--as <source_identity>",
	} {
		if !strings.Contains(noteScene, must) {
			t.Errorf("query-note-and-artifacts.md must preserve note cover handling %q", must)
		}
	}

	meetingScene := readSkillDoc(t, "skills/lark-meeting/scenes/query-meeting-and-artifacts.md")
	if !strings.Contains(meetingScene, "query-note-and-artifacts.md") {
		t.Error("query-meeting-and-artifacts.md must route intelligent-note body handling to the owning Note scene")
	}
}
