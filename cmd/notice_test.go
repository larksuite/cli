// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/deprecation"
)

// composePendingNotice must surface a deprecated-command alias under the
// "deprecated_command" key, with the migration target and a skill-update hint,
// so the JSON "_notice" envelope reaches users who run pre-refactor commands
// without ever reading --help.
func TestComposePendingNoticeDeprecatedCommand(t *testing.T) {
	t.Cleanup(func() { deprecation.SetPending(nil) })

	deprecation.SetPending(&deprecation.Notice{
		Command:     "+read",
		Replacement: "+cells-get",
		Skill:       "lark-sheets",
	})

	got := composePendingNotice()
	if got == nil {
		t.Fatal("composePendingNotice() = nil, want deprecated_command entry")
	}
	entry, ok := got["deprecated_command"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing deprecated_command key: %#v", got)
	}
	if entry["command"] != "+read" {
		t.Errorf("command = %v, want +read", entry["command"])
	}
	if entry["replacement"] != "+cells-get" {
		t.Errorf("replacement = %v, want +cells-get", entry["replacement"])
	}
	if entry["skill"] != "lark-sheets" {
		t.Errorf("skill = %v, want lark-sheets", entry["skill"])
	}
	if msg, _ := entry["message"].(string); !strings.Contains(msg, "update your lark-sheets skill") {
		t.Errorf("message missing skill-update hint: %q", msg)
	}
}

// With nothing pending, the provider returns nil so no "_notice" field is
// emitted on a clean run.
func TestComposePendingNoticeEmpty(t *testing.T) {
	t.Cleanup(func() { deprecation.SetPending(nil) })
	deprecation.SetPending(nil)

	if got := composePendingNotice(); got != nil {
		// update/skills pending are process-global; only assert the absence of
		// our own key to stay robust against unrelated pending state.
		if _, ok := got["deprecated_command"]; ok {
			t.Fatalf("deprecated_command present after clear: %#v", got)
		}
	}
}
