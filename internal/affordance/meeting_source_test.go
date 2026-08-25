// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"os"
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
