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

// TestVCSearchIdentityDocsMatchAuthTypes pins that `+search` stays user-only
// in both code and the reference owned by lark-meeting. If AuthTypes ever
// gains "bot", this test forces a deliberate documentation update instead of
// letting the docs silently fall out of sync.
func TestVCSearchIdentityDocsMatchAuthTypes(t *testing.T) {
	skill := readSkillDoc(t, "skills/lark-meeting/SKILL.md")
	reference := readSkillDoc(t, "skills/lark-meeting/references/lark-vc-search.md")

	if hasAuthType(VCSearch.AuthTypes, "bot") {
		t.Fatalf("VCSearch.AuthTypes = %v now includes bot; update skills/lark-meeting/references/lark-vc-search.md wording (and this test) to reflect the new support instead of leaving the user-only claim below", VCSearch.AuthTypes)
	}
	if !strings.Contains(skill, "references/lark-vc-search.md") {
		t.Error("skills/lark-meeting/SKILL.md must link to the vc +search reference")
	}
	if !strings.Contains(reference, "仅支持 `user` 身份") && !strings.Contains(reference, "仅 `--as user`") {
		t.Error("lark-vc-search.md must state that +search only supports user identity (matches VCSearch.AuthTypes)")
	}
}

func TestVCMeetingLeaveDocsMatchBotOnlyAuthTypes(t *testing.T) {
	if len(VCMeetingLeave.AuthTypes) != 1 || VCMeetingLeave.AuthTypes[0] != "bot" {
		t.Fatalf("VCMeetingLeave.AuthTypes = %v, want [bot]", VCMeetingLeave.AuthTypes)
	}

	reference := readSkillDoc(t, "skills/lark-meeting/references/lark-vc-agent-meeting-leave.md")
	if !strings.Contains(reference, "仅支持 `bot` 身份") {
		t.Error("lark-vc-agent-meeting-leave.md must state that +meeting-leave only supports bot identity")
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

func TestVCMeetingJoinDocsRouteCalendarStart(t *testing.T) {
	skill := readSkillDoc(t, "skills/lark-meeting/SKILL.md")
	scene := readSkillDoc(t, "skills/lark-meeting/scenes/live-meeting-attend.md")
	reference := readSkillDoc(t, "skills/lark-meeting/references/lark-vc-agent-meeting-join.md")

	if !strings.Contains(reference, "# vc +meeting-join：发起日程会议或加入会议") {
		t.Error("lark-vc-agent-meeting-join.md must identify Calendar meeting starts")
	}
	for _, want := range []string{
		"机器人发起日程会议或参与会议",
		"让应用机器人发起日程会议或加入会议",
	} {
		if !strings.Contains(skill, want) {
			t.Errorf("SKILL.md must document %q", want)
		}
	}
	for name, content := range map[string]string{
		"live-meeting-attend.md":        scene,
		"lark-vc-agent-meeting-join.md": reference,
	} {
		if !strings.Contains(content, "应用机器人发起会议、参会与会中互动") {
			t.Errorf("%s must route application-bot meeting starts", name)
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
