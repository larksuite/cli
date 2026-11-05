// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/meta"
)

func TestVCAffordanceRoutesAgentActionReferences(t *testing.T) {
	previousSource := mdSource
	t.Cleanup(func() { SetSource(previousSource) })
	SetSource(os.DirFS("../../affordance"))

	tests := []struct {
		service string
		command string
		ref     string
	}{
		{service: "vc", command: "+meeting-invite", ref: "lark-meeting/references/lark-vc-agent-meeting-invite.md"},
		{service: "vc", command: "+meeting-chat", ref: "lark-meeting/references/lark-vc-meeting-chat.md"},
		{service: "vc", command: "+meeting-end", ref: "lark-meeting/references/lark-vc-agent-meeting-end.md"},
		{service: "vc", command: "+meeting-share", ref: "lark-meeting/references/lark-vc-meeting-share.md"},
		{service: "vc", command: "+meeting-share-status", ref: "lark-meeting/references/lark-vc-meeting-share.md"},
		{service: "vc", command: "+meeting-transfer-host", ref: "lark-meeting/references/lark-vc-meeting-transfer-host.md"},
		{service: "recording", command: "+list", ref: "lark-meeting/references/lark-recording-list.md"},
	}
	for _, testCase := range tests {
		t.Run(testCase.command, func(t *testing.T) {
			raw, ok := For(apicatalog.Catalog{}, testCase.service, testCase.command)
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

func TestVCMeetingShareGuidanceScopesAgentStartRestrictions(t *testing.T) {
	previousSource := mdSource
	t.Cleanup(func() { SetSource(previousSource) })
	SetSource(os.DirFS("../../affordance"))

	raw, ok := For(apicatalog.Catalog{}, "vc", "+meeting-share")
	if !ok {
		t.Fatal("meeting share guidance is missing")
	}
	guidance, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
	if !ok {
		t.Fatal("meeting share guidance did not parse")
	}
	prerequisites := strings.Join(guidance.Prerequisites, "\n")
	for _, required := range []string{"start", "当前所有在会成员", "包括应用机器人", "同租户"} {
		if !strings.Contains(prerequisites, required) {
			t.Errorf("meeting share prerequisites must include %q", required)
		}
	}
	tips := strings.Join(guidance.Tips, "\n")
	for _, required := range []string{"权限错误", "实际接口响应", "不能仅凭", "log_id"} {
		if !strings.Contains(tips, required) {
			t.Errorf("meeting share recovery guidance must include %q", required)
		}
	}
	if strings.Contains(prerequisites, "游客") || strings.Contains(tips, "游客") {
		t.Error("meeting share guidance must not expose guest-specific terminology")
	}

	raw, ok = For(apicatalog.Catalog{}, "vc", "+meeting-share-status")
	if !ok {
		t.Fatal("meeting share status guidance is missing")
	}
	status, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
	if !ok {
		t.Fatal("meeting share status guidance did not parse")
	}
	if len(status.Prerequisites) != 0 {
		t.Fatalf("share status must not inherit start restrictions: %v", status.Prerequisites)
	}
}
