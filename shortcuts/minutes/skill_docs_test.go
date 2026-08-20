// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// Capability source test: pins the identity (user/bot) claims made in
// skills/lark-minutes against the AuthTypes actually declared on the
// shortcuts, and pins that the write shortcut this PR opened to bot
// (`+apply-permission`) ships with a reference doc — PR #2278's review found
// it had none, which made the main SKILL.md's own "read the reference before
// using a shortcut" contract unsatisfiable.

package minutes

import (
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
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

// TestMinutesBotShortcutsIdentityDocsMatchAuthTypes pins that every minutes
// shortcut declared bot-capable in code is documented as such in the main
// SKILL.md identity line, so the two sources can't silently diverge again.
func TestMinutesBotShortcutsIdentityDocsMatchAuthTypes(t *testing.T) {
	skill := readSkillDoc(t, "skills/lark-minutes/SKILL.md")

	for _, cmd := range []struct {
		name      string
		authTypes []string
	}{
		{"+search", MinutesSearch.AuthTypes},
		{"+detail", MinutesDetail.AuthTypes},
		{"+download", MinutesDownload.AuthTypes},
		{"+apply-permission", MinutesApplyPermission.AuthTypes},
	} {
		if !hasAuthType(cmd.authTypes, "bot") {
			t.Errorf("%s AuthTypes = %v, want bot included", cmd.name, cmd.authTypes)
			continue
		}
		token := "`" + cmd.name + "`"
		if !strings.Contains(skill, token) {
			t.Errorf("skills/lark-minutes/SKILL.md identity section must mention %s alongside its bot support", token)
		}
	}
	if !strings.Contains(skill, "也支持 `--as bot`") {
		t.Error("skills/lark-minutes/SKILL.md identity section must state which commands also support --as bot")
	}
}

// TestMinutesApplyPermissionHasReference pins that `+apply-permission` (this
// PR's new write-capable-by-bot shortcut) has a dedicated reference doc that
// the Shortcuts table links to, satisfying the SKILL.md-wide
// "read the reference before using any shortcut" contract.
func TestMinutesApplyPermissionHasReference(t *testing.T) {
	if !hasAuthType(MinutesApplyPermission.AuthTypes, "bot") {
		t.Fatalf("MinutesApplyPermission.AuthTypes = %v, want bot included (this PR's contract)", MinutesApplyPermission.AuthTypes)
	}

	reference := readSkillDoc(t, "skills/lark-minutes/references/lark-minutes-apply-permission.md")
	for _, must := range []string{"--as bot", "--as user", "身份", "missing scope"} {
		if !strings.Contains(reference, must) {
			t.Errorf("lark-minutes-apply-permission.md must cover %q (user/bot identity + scope-vs-ACL guidance)", must)
		}
	}

	skill := readSkillDoc(t, "skills/lark-minutes/SKILL.md")
	if !strings.Contains(skill, "[`+apply-permission`](references/lark-minutes-apply-permission.md)") {
		t.Error("skills/lark-minutes/SKILL.md Shortcuts table must link +apply-permission to its reference doc")
	}
}

func hasFlag(flags []common.Flag, want string) bool {
	for _, f := range flags {
		if f.Name == want {
			return true
		}
	}
	return false
}

// TestMeetingSummaryWorkflowDocumentsMinuteTokenFallback pins the fallback that
// issue #2379 found missing: the meeting-summary workflow used to tell the agent
// to report "无纪要" whenever a meeting had no note_id, even though such meetings
// often still carry a readable minute_token. It also pins the fallback commands
// against the flag names the shortcuts actually declare, since +detail takes the
// plural --minute-tokens while +apply-permission takes the singular form.
func TestMeetingSummaryWorkflowDocumentsMinuteTokenFallback(t *testing.T) {
	if !hasFlag(MinutesDetail.Flags, "minute-tokens") {
		t.Fatalf("MinutesDetail flags = %v, want minute-tokens", MinutesDetail.Flags)
	}
	if !hasFlag(MinutesApplyPermission.Flags, "minute-token") {
		t.Fatalf("MinutesApplyPermission flags = %v, want minute-token", MinutesApplyPermission.Flags)
	}

	workflow := readSkillDoc(t, "skills/lark-workflow-meeting-summary/SKILL.md")
	for _, must := range []string{
		"minutes +detail --minute-tokens",
		"minutes +apply-permission --minute-token ",
		"--transcript",
		"--output-dir",
		"No read permission",
	} {
		if !strings.Contains(workflow, must) {
			t.Errorf("lark-workflow-meeting-summary/SKILL.md must cover %q so a missing note_id is not reported as 无纪要", must)
		}
	}
}
