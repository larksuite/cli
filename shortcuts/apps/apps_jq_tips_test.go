// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"
)

func TestListyCommandsHaveJqTip(t *testing.T) {
	wantCmds := map[string]bool{
		"+list": true, "+db-table-list": true, "+db-table-schema": true,
		"+db-sql": true, "+release-list": true, "+session-list": true,
	}
	for _, s := range Shortcuts() {
		if !wantCmds[s.Command] {
			continue
		}
		has := false
		for _, tip := range s.Tips {
			if strings.Contains(tip, "--jq") || strings.Contains(tip, "-q '") {
				has = true
			}
		}
		if !has {
			t.Errorf("%s should have a --jq filter tip", s.Command)
		}
	}
}
