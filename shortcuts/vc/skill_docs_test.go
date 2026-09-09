// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// Capability source test: pins the identity (user/bot) claims made in
// skills/lark-vc against the AuthTypes actually declared on the shortcuts.
// PR #2278's review found the docs had already drifted from the code once
// (SKILL.md claimed `+search` supported bot while vc_search.go stayed
// user-only) — this test fails loudly the next time that happens instead of
// relying on a human re-reading both sides on every AuthTypes change.

package vc

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

// TestVCSearchIdentityDocsMatchAuthTypes pins the user/bot identity contract in
// code, the command reference, and the cross-command meeting workflow.
func TestVCSearchIdentityDocsMatchAuthTypes(t *testing.T) {
	skill := readSkillDoc(t, "skills/lark-meeting/SKILL.md")
	reference := readSkillDoc(t, "skills/lark-meeting/references/lark-vc-search.md")
	scene := readSkillDoc(t, "skills/lark-meeting/scenes/query-meeting-and-artifacts.md")

	for _, identity := range []string{"user", "bot"} {
		if !hasAuthType(VCSearch.AuthTypes, identity) {
			t.Errorf("VCSearch.AuthTypes = %v, want %s included", VCSearch.AuthTypes, identity)
		}
	}
	if !strings.Contains(skill, "references/lark-vc-search.md") {
		t.Error("skills/lark-meeting/SKILL.md must link to the vc +search reference")
	}
	for _, identity := range []string{"--as user", "--as bot"} {
		if !strings.Contains(reference, identity) {
			t.Errorf("lark-vc-search.md must document %s", identity)
		}
	}
	if strings.Contains(scene, "`vc +search` 仅支持用户身份") {
		t.Error("meeting artifact scene must not claim vc +search is user-only")
	}
}

// TestVCBotShortcutsIdentityDocsMatchAuthTypes pins that the VC shortcuts this
// PR opened to bot (`+detail`, `+recording`, `+meeting-countdown`) are all declared bot-capable in
// code and documented as such in their lark-meeting references.
func TestVCBotShortcutsIdentityDocsMatchAuthTypes(t *testing.T) {
	skill := readSkillDoc(t, "skills/lark-meeting/SKILL.md")

	for _, cmd := range []struct {
		name      string
		authTypes []string
		reference string
	}{
		{"+detail", VCDetail.AuthTypes, "lark-vc-detail.md"},
		{"+recording", VCRecording.AuthTypes, "lark-vc-recording.md"},
		{"+meeting-countdown", VCMeetingCountdown.AuthTypes, "lark-vc-meeting-countdown.md"},
	} {
		if !hasAuthType(cmd.authTypes, "bot") {
			t.Errorf("%s AuthTypes = %v, want bot included (this PR's contract)", cmd.name, cmd.authTypes)
			continue
		}
		if !strings.Contains(skill, "references/"+cmd.reference) {
			t.Errorf("skills/lark-meeting/SKILL.md must link %s to %s", cmd.name, cmd.reference)
		}
		reference := readSkillDoc(t, "skills/lark-meeting/references/"+cmd.reference)
		for _, identity := range []string{"--as user", "--as bot"} {
			if !strings.Contains(reference, identity) {
				t.Errorf("%s must document %s support for %s", cmd.reference, identity, cmd.name)
			}
		}
	}
}

func TestMeetingArtifactSceneDelegatesToDomainOwners(t *testing.T) {
	scene := readSkillDoc(t, "skills/lark-meeting/scenes/query-meeting-and-artifacts.md")
	for _, target := range []string{
		"query-note-and-artifacts.md",
		"query-minutes-and-artifacts.md",
	} {
		if !strings.Contains(scene, target) {
			t.Errorf("query-meeting-and-artifacts.md must delegate to %s", target)
		}
	}

	for _, duplicatedCommand := range []string{
		"lark-cli note +detail",
		"lark-cli minutes +detail",
	} {
		if strings.Contains(scene, duplicatedCommand) {
			t.Errorf("query-meeting-and-artifacts.md must not duplicate downstream command %q", duplicatedCommand)
		}
	}
}
