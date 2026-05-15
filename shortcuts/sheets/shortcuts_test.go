// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"testing"

	"github.com/larksuite/cli/internal/auth"
)

func TestShortcuts_NoUnrealReadScope(t *testing.T) {
	for _, s := range Shortcuts() {
		for _, scope := range s.Scopes {
			if scope == "sheets:spreadsheet:read" {
				t.Errorf("shortcut %s declares unreal scope %q; Feishu Open Platform uses sheets:spreadsheet:readonly", s.Command, scope)
			}
		}
	}
}

func TestShortcuts_PrecheckAcceptsFeishuSheetsScopes(t *testing.T) {
	granted := "sheets:spreadsheet:create sheets:spreadsheet:readonly sheets:spreadsheet:write_only"
	for _, s := range Shortcuts() {
		sheetsOnly := true
		for _, scope := range s.Scopes {
			if len(scope) < 7 || scope[:7] != "sheets:" {
				sheetsOnly = false
				break
			}
		}
		if !sheetsOnly || len(s.Scopes) == 0 {
			continue
		}
		if missing := auth.MissingScopes(granted, s.Scopes); len(missing) > 0 {
			t.Errorf("shortcut %s rejects a token granted real Feishu sheets scopes; missing=%v required=%v", s.Command, missing, s.Scopes)
		}
	}
}
