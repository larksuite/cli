// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/meta"
)

func TestServiceFlagGroups_AgentContract(t *testing.T) {
	method := map[string]interface{}{
		"path":       "chats/:chat_id/members",
		"httpMethod": "POST",
		"parameters": map[string]interface{}{
			"chat_id": map[string]interface{}{"type": "string", "location": "path", "required": true},
			"member_id_type": map[string]interface{}{
				"type": "string", "location": "query",
				"options": []interface{}{
					map[string]interface{}{"value": "open_id", "description": "以 open_id 标识用户"},
					map[string]interface{}{"value": "user_id", "description": "以 user_id 标识用户"},
				},
			},
		},
		// Documented body field -> --data belongs under Request Body.
		"requestBody": map[string]interface{}{
			"id_list": map[string]interface{}{"type": "list", "required": true},
		},
	}
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), meta.FromMap(method), "create", "chat.members", nil)
	out := renderServiceFlagGroups(cmd)

	idx := func(s string) int { return strings.Index(out, s) }

	// Section order: API Parameters → Request Body → Raw Parameter Input → Execution → Output.
	iParams, iBody, iRaw, iExec, iOut := idx("API Parameters:"), idx("Request Body:"), idx("Raw Parameter Input:"), idx("Execution:"), idx("Output:")
	for name, i := range map[string]int{"API Parameters": iParams, "Request Body": iBody, "Raw Parameter Input": iRaw, "Execution": iExec, "Output": iOut} {
		if i < 0 {
			t.Fatalf("missing section %q in:\n%s", name, out)
		}
	}
	if !(iParams < iBody && iBody < iRaw && iRaw < iExec && iExec < iOut) {
		t.Errorf("section order wrong:\n%s", out)
	}

	// Required/Optional subsections under API Parameters.
	if i := idx("  Required:"); i < iParams || i > iBody {
		t.Errorf("Required subsection misplaced:\n%s", out)
	}
	if i := idx("  Optional:"); i < iParams || i > iBody {
		t.Errorf("Optional subsection misplaced:\n%s", out)
	}

	// Typed flags are API Parameters; required path flag under Required, enum
	// flag under Optional with an inline "enum: ..." (not multi-line meanings).
	if i := idx("--chat-id"); i < iParams || i > iBody {
		t.Errorf("--chat-id not under API Parameters:\n%s", out)
	}
	// The redundant "<name>, required|optional." prefix is gone: required-ness is
	// carried by the Required:/Optional: subheadings, and the snake-case --params
	// key by the schema envelope — so it isn't echoed on every flag line.
	if strings.Contains(out, "chat_id, required") || strings.Contains(out, "member_id_type, optional") {
		t.Errorf("redundant <name>, required/optional prefix should not appear:\n%s", out)
	}
	if !strings.Contains(out, "enum: open_id=以 open_id 标识用户|user_id=以 user_id 标识用户") {
		t.Errorf("expected compact enum value=meaning inline:\n%s", out)
	}

	// --data is Request Body; --params is Raw Parameter Input (NOT API Parameters)
	// and carries the precedence rule.
	if i := idx("--data"); i < iBody || i > iRaw {
		t.Errorf("--data not under Request Body:\n%s", out)
	}
	if i := idx("--params"); i < iRaw || i > iExec {
		t.Errorf("--params not under Raw Parameter Input:\n%s", out)
	}
	if !strings.Contains(out, "typed flags override matching keys in --params") {
		t.Errorf("missing --params precedence rule:\n%s", out)
	}

	// Control flags land in Execution/Output.
	if i := idx("--dry-run"); i < iExec || i > iOut {
		t.Errorf("--dry-run not under Execution:\n%s", out)
	}
	if idx("--format") < iOut {
		t.Errorf("--format not under Output:\n%s", out)
	}

	// The usage template is wired to the grouped renderer (no flat Flags: list).
	if u := cmd.UsageString(); !strings.Contains(u, "API Parameters:") || strings.Contains(u, "\nFlags:\n") {
		t.Errorf("usage template not grouped:\n%s", u)
	}
}

// TestServiceFlagGroups_UndocumentedBodyIsRaw: a POST with no documented body
// fields still offers --data (escape hatch) but must NOT imply a declared body —
// it goes under Raw Parameter Input, not "Request Body".
func TestServiceFlagGroups_UndocumentedBodyIsRaw(t *testing.T) {
	method := map[string]interface{}{"path": "things/do", "httpMethod": "POST"} // POST, no requestBody, no params
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), meta.FromMap(method), "do", "things", nil)
	out := renderServiceFlagGroups(cmd)

	if strings.Contains(out, "Request Body:") {
		t.Errorf("undocumented body must not render a Request Body section:\n%s", out)
	}
	iRaw, iData := strings.Index(out, "Raw Parameter Input:"), strings.Index(out, "--data")
	if iRaw < 0 || iData < iRaw {
		t.Errorf("--data not under Raw Parameter Input:\n%s", out)
	}
	if !strings.Contains(out, "no documented fields") {
		t.Errorf("--data should be labeled a raw escape hatch:\n%s", out)
	}
}
