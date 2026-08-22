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
	for _, command := range []string{"+recording-start", "+recording-stop"} {
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
		"lark-cli vc +recording-start",
		"lark-cli vc +recording-stop",
		"仅支持 `--as user`",
		"不发送可选的 `timezone`",
	} {
		if !strings.Contains(string(source), required) {
			t.Errorf("recording control reference must contain %q", required)
		}
	}
}
