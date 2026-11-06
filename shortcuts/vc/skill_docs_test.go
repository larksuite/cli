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
	"regexp"
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
		{"+meeting-chat", VCMeetingChat.AuthTypes, "lark-vc-meeting-chat.md"},
		{"+recording", VCRecording.AuthTypes, "lark-vc-recording.md"},
		{"+meeting-countdown", VCMeetingCountdown.AuthTypes, "lark-vc-meeting-countdown.md"},
		{"+meeting-share", VCMeetingShare.AuthTypes, "lark-vc-meeting-share.md"},
		{"+meeting-share-status", VCMeetingShareStatus.AuthTypes, "lark-vc-meeting-share.md"},
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

func TestVCMeetingManagementDocsMatchAuthTypesAndContracts(t *testing.T) {
	skill := readSkillDoc(t, "skills/lark-meeting/SKILL.md")

	for _, cmd := range []struct {
		name         string
		authTypes    []string
		reference    string
		botReference string
		sceneNeed    string
	}{
		{
			name:         "+meeting-end",
			authTypes:    VCMeetingEnd.AuthTypes,
			reference:    "lark-vc-meeting-end.md",
			botReference: "lark-vc-agent-meeting-end.md",
			sceneNeed:    "vc +meeting-end",
		},
		{
			name:      "+meeting-participant-kickout",
			authTypes: VCMeetingParticipantKickout.AuthTypes,
			reference: "lark-vc-meeting-participant-kickout.md",
			sceneNeed: "vc +meeting-participant-kickout",
		},
		{
			name:      "+meeting-transfer-host",
			authTypes: VCMeetingTransferHost.AuthTypes,
			reference: "lark-vc-meeting-transfer-host.md",
			sceneNeed: "vc +meeting-transfer-host",
		},
	} {
		t.Run(cmd.name, func(t *testing.T) {
			wantBot := cmd.botReference != ""
			if !hasAuthType(cmd.authTypes, "user") || hasAuthType(cmd.authTypes, "bot") != wantBot {
				t.Fatalf("%s AuthTypes = %v, want user support with bot=%v", cmd.name, cmd.authTypes, wantBot)
			}
			if !strings.Contains(skill, "references/"+cmd.reference) {
				t.Fatalf("skills/lark-meeting/SKILL.md must link %s to %s", cmd.name, cmd.reference)
			}

			reference := readSkillDoc(t, "skills/lark-meeting/references/"+cmd.reference)
			if wantBot {
				if !strings.Contains(reference, "--as user") || strings.Contains(reference, "--as bot") {
					t.Fatalf("%s must document only the user identity for %s", cmd.reference, cmd.name)
				}
				if !strings.Contains(skill, "references/"+cmd.botReference) {
					t.Fatalf("skills/lark-meeting/SKILL.md must link %s to %s", cmd.name, cmd.botReference)
				}
				if !strings.Contains(reference, "("+cmd.botReference+")") {
					t.Fatalf("%s must link the bot identity reference %s", cmd.reference, cmd.botReference)
				}
				botReference := readSkillDoc(t, "skills/lark-meeting/references/"+cmd.botReference)
				if !strings.Contains(botReference, "--as bot") {
					t.Fatalf("%s must document bot identity support for %s", cmd.botReference, cmd.name)
				}
			} else if !strings.Contains(reference, "仅支持 `user` 身份") && !strings.Contains(reference, "必须显式使用 `--as user`") {
				t.Fatalf("%s must state that %s is user-only", cmd.reference, cmd.name)
			}
			if !strings.Contains(reference, "--dry-run") || !strings.Contains(reference, "--yes") {
				t.Fatalf("%s must document both --dry-run preview and --yes confirmation", cmd.reference)
			}
		})
	}

	scene := readSkillDoc(t, "skills/lark-meeting/scenes/live-meeting-interact.md")
	for _, needle := range []string{
		"vc +meeting-end",
		"vc +meeting-participant-kickout",
		"vc +meeting-transfer-host",
		"vc meeting get",
		"--yes",
	} {
		if !strings.Contains(scene, needle) {
			t.Errorf("live-meeting-interact.md must mention %q for meeting management routing", needle)
		}
	}

	sceneCommands := strings.ReplaceAll(scene, "\\\n", " ")
	for _, example := range []struct {
		prefix       string
		confirmation bool
	}{
		{"lark-cli vc +meeting-end --as user", true},
		{"lark-cli vc +meeting-end --as bot", true},
		{"lark-cli vc +meeting-participant-kickout --as user", true},
		{"lark-cli vc +meeting-transfer-host --as user", true},
		{"lark-cli vc +meeting-participant-mute --as <same_identity>", false},
		{"lark-cli vc +meeting-participant-unmute --as <same_identity>", false},
	} {
		command := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(example.prefix) + `[ \t][^\n]*$`).FindString(sceneCommands)
		if command == "" || strings.Contains(command, "--dry-run") || (example.confirmation && !strings.Contains(command, "--yes")) {
			t.Errorf("invalid default meeting control example for %q: %s", example.prefix, command)
		}
	}
}

func TestVCUserIdentityLifecycleDocsMatchShortcutContracts(t *testing.T) {
	skill := readSkillDoc(t, "skills/lark-meeting/SKILL.md")
	scene := readSkillDoc(t, "skills/lark-meeting/scenes/live-meeting-attend.md")
	joinReference := readSkillDoc(t, "skills/lark-meeting/references/lark-vc-agent-meeting-join.md")
	inviteReference := readSkillDoc(t, "skills/lark-meeting/references/lark-vc-agent-meeting-invite.md")
	leaveReference := readSkillDoc(t, "skills/lark-meeting/references/lark-vc-agent-meeting-leave.md")
	endReference := readSkillDoc(t, "skills/lark-meeting/references/lark-vc-meeting-end.md")

	for name, authTypes := range map[string][]string{
		"+meeting-join":   VCMeetingJoin.AuthTypes,
		"+meeting-invite": VCMeetingInvite.AuthTypes,
		"+meeting-end":    VCMeetingEnd.AuthTypes,
		"+meeting-leave":  VCMeetingLeave.AuthTypes,
	} {
		if !hasAuthType(authTypes, "user") {
			t.Fatalf("%s AuthTypes = %v, want user identity support", name, authTypes)
		}
	}

	for name, content := range map[string]string{
		"SKILL.md":                     skill,
		"live-meeting-attend.md":       scene,
		"lark-vc-agent-meeting-join":   joinReference,
		"lark-vc-agent-meeting-invite": inviteReference,
		"lark-vc-agent-meeting-leave":  leaveReference,
		"lark-vc-meeting-end":          endReference,
	} {
		if !strings.Contains(content, "--as user") {
			t.Errorf("%s must document user identity support", name)
		}
		for _, internalIdentity := range []string{"AAT", "TAT", "UAT", "Agent Employee"} {
			if strings.Contains(content, internalIdentity) {
				t.Errorf("%s must not expose internal identity %q", name, internalIdentity)
			}
		}
	}
	if !strings.Contains(inviteReference, "SELECTED") ||
		!strings.Contains(inviteReference, "ALL_SUGGESTED") ||
		!strings.Contains(inviteReference, "不支持") {
		t.Error("invite reference must document user SELECTED support and ALL_SUGGESTED rejection")
	}
	for _, want := range []string{"vc +meeting-end --as user", "vc +meeting-end --as bot", "当前用户或应用机器人都必须是当前 Host", "不得为了执行成功在 `--as user` 与 `--as bot` 之间静默切换"} {
		if !strings.Contains(scene, want) {
			t.Errorf("live-meeting-attend.md must document %q", want)
		}
	}
	if strings.Contains(scene, "不支持 `meeting-end`") {
		t.Error("live-meeting-attend.md must not reject the user identity meeting-end path")
	}
	for _, want := range []string{
		"入会失败的解释规则",
		"以本次 CLI 返回的错误消息为依据。",
		"`AI Summary` =「AI 总结」",
		"`Allow agents to join meetings` =「允许智能体加入会议」",
		"只转述消息明确表达的失败原因和操作要求，不自行补充；消息中的帮助链接按原样保留。",
		"`--action join\\|start`",
		"默认 `join` 不会向请求体写入 `action` 字段",
		"`--action start` 才写入 `action: 2`",
	} {
		if !strings.Contains(joinReference, want) {
			t.Errorf("入会reference必须保留规则 %q", want)
		}
	}
	if regexp.MustCompile(`\b121\d{3}\b`).MatchString(joinReference) ||
		regexp.MustCompile(`(?m)^\s*\|[^\n]*(?:错误码|error code)[^\n]*\|`).MatchString(joinReference) {
		t.Error("入会reference不得保留旧业务错误码或建立新的逐码错误表")
	}
}

func TestVCMeetingScreenshotDocsDocumentFeatureUnavailable(t *testing.T) {
	reference := readSkillDoc(t, "skills/lark-meeting/references/lark-vc-meeting-screenshot.md")
	for _, want := range []string{
		"128006",
		"meeting screenshot is currently in limited availability and is not available for this meeting.",
		"不要通过切换 `--as` 身份或新增 scope 绕过",
	} {
		if !strings.Contains(reference, want) {
			t.Errorf("lark-vc-meeting-screenshot must document %q", want)
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
