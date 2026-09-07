// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"os"
	"testing"

	"github.com/larksuite/cli/internal/meta"
)

func TestVCAffordanceRoutesAgentActionReferences(t *testing.T) {
	previousSource := mdSource
	t.Cleanup(func() { SetSource(previousSource) })
	SetSource(os.DirFS("../../affordance"))

	tests := []struct {
		command string
		ref     string
	}{
		{command: "+meeting-invite", ref: "lark-meeting/references/lark-vc-agent-meeting-invite.md"},
		{command: "+meeting-end", ref: "lark-meeting/references/lark-vc-agent-meeting-end.md"},
	}
	for _, testCase := range tests {
		t.Run(testCase.command, func(t *testing.T) {
			raw, ok := For("vc", testCase.command)
			if !ok {
				t.Fatalf("For(vc, %s) ok=false", testCase.command)
			}
			affordance, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
			if !ok {
				t.Fatalf("vc %s affordance did not parse", testCase.command)
			}
			for _, skill := range affordance.Skills {
				if skill == testCase.ref {
					return
				}
			}
			t.Fatalf("vc %s skills = %v, want %q", testCase.command, affordance.Skills, testCase.ref)
		})
	}
}
