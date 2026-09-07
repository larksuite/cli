// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/meta"
)

func TestMeetingDomainsDeclareUnifiedSkill(t *testing.T) {
	previousSource := mdSource
	t.Cleanup(func() { SetSource(previousSource) })
	SetSource(os.DirFS("../../affordance"))

	for _, domain := range []string{"vc", "minutes", "note"} {
		t.Run(domain, func(t *testing.T) {
			if got, ok := DomainSkill(domain); !ok || got != "lark-meeting" {
				t.Fatalf("DomainSkill(%s) = (%q, %v), want (lark-meeting, true)", domain, got, ok)
			}
			if got, ok := DomainSkills(domain); !ok || len(got) != 1 || got[0] != "lark-meeting" {
				t.Fatalf("DomainSkills(%s) = (%v, %v), want ([lark-meeting], true)", domain, got, ok)
			}
		})
	}
}

func TestMeetingScreenshotRoutesToItsReference(t *testing.T) {
	previousSource := mdSource
	t.Cleanup(func() { SetSource(previousSource) })
	SetSource(os.DirFS("../../affordance"))

	raw, ok := For("vc", "+meeting-screenshot")
	if !ok {
		t.Fatal("For(vc, +meeting-screenshot) ok=false")
	}
	guidance, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
	if !ok {
		t.Fatal("meeting screenshot affordance did not parse")
	}
	want := "lark-meeting/references/lark-vc-meeting-screenshot.md"
	for _, skill := range guidance.Skills {
		if skill == want {
			return
		}
	}
	t.Fatalf("meeting screenshot skills = %v, want %q", guidance.Skills, want)
}

func TestVCRecordingControlAffordanceMatchesSkillReference(t *testing.T) {
	previousSource := mdSource
	t.Cleanup(func() { SetSource(previousSource) })
	SetSource(os.DirFS("../../affordance"))

	const reference = "lark-meeting/references/lark-vc-recording-control.md"
	for _, command := range []string{"+meeting-recording-start", "+meeting-recording-stop"} {
		raw, ok := For("vc", command)
		if !ok {
			t.Fatalf("For(vc, %s) returned no affordance", command)
		}
		var affordance meta.Affordance
		if err := json.Unmarshal(raw, &affordance); err != nil {
			t.Fatalf("decode %s affordance: %v", command, err)
		}
		if len(affordance.Skills) != 2 || affordance.Skills[0] != "lark-meeting" || affordance.Skills[1] != reference {
			t.Fatalf("%s skills = %v, want [lark-meeting %s]", command, affordance.Skills, reference)
		}
	}

	source, err := os.ReadFile("../../skills/lark-meeting/references/lark-vc-recording-control.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"lark-cli vc +meeting-recording-start",
		"lark-cli vc +meeting-recording-stop",
		"仅支持 `--as user`",
		"不发送可选的 `timezone`",
	} {
		if !strings.Contains(string(source), required) {
			t.Errorf("recording control reference must contain %q", required)
		}
	}
}

func TestVCParticipantAudioAffordanceMatchesSkillReference(t *testing.T) {
	previousSource := mdSource
	t.Cleanup(func() { SetSource(previousSource) })
	SetSource(os.DirFS("../../affordance"))

	const reference = "lark-meeting/references/lark-vc-meeting-participant-audio.md"
	for _, command := range []string{"+meeting-participant-mute", "+meeting-participant-unmute"} {
		raw, ok := For("vc", command)
		if !ok {
			t.Fatalf("For(vc, %s) returned no affordance", command)
		}
		var affordance meta.Affordance
		if err := json.Unmarshal(raw, &affordance); err != nil {
			t.Fatalf("decode %s affordance: %v", command, err)
		}
		if len(affordance.Skills) != 2 || affordance.Skills[0] != "lark-meeting" || affordance.Skills[1] != reference {
			t.Fatalf("%s skills = %v, want [lark-meeting %s]", command, affordance.Skills, reference)
		}
	}

	referenceSource, err := os.ReadFile("../../skills/lark-meeting/references/lark-vc-meeting-participant-audio.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"lark-cli vc +meeting-participant-mute",
		"lark-cli vc +meeting-participant-unmute",
		"--target-user-id",
		"--user-id-type",
		"`--as user` 或 `--as bot`",
		"vc:meeting.bot.manage:write",
		"请求已发送",
		"不表示目标参会人已经开麦",
		"## 错误码",
		"`121101`",
		"`121102`",
		"`121103`",
		"`121104`",
		"`121105`",
		"`121107`",
		"`122003`",
		"`122005`",
		"operator is not in the meeting",
		"target participant is not in the meeting or is no longer active",
	} {
		if !strings.Contains(string(referenceSource), required) {
			t.Errorf("participant audio reference must contain %q", required)
		}
	}
	for _, shortCode := range []string{"`1101`", "`1102`", "`1103`", "`1104`", "`1105`", "`1107`", "`2003`", "`2005`"} {
		if strings.Contains(string(referenceSource), shortCode) {
			t.Errorf("participant audio reference must not document OGW short code %q as an external code", shortCode)
		}
	}

	for path, required := range map[string][]string{
		"../../skills/lark-meeting/SKILL.md": {
			"[lark-vc-meeting-participant-audio](references/lark-vc-meeting-participant-audio.md)",
		},
		"../../skills/lark-meeting/scenes/live-meeting-interact.md": {
			"lark-cli vc +meeting-participant-mute",
			"lark-cli vc +meeting-participant-unmute",
			"[会中闭麦与请求开麦](../references/lark-vc-meeting-participant-audio.md)",
		},
	} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range required {
			if !strings.Contains(string(source), want) {
				t.Errorf("%s must contain %q", path, want)
			}
		}
	}
}
