// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

func TestSearchReplaceShortcuts_DryRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sc          common.Shortcut
		args        []string
		toolName    string
		wantInput   map[string]interface{}
		wantOptions map[string]interface{}
	}{
		{
			name:     "+cells-search regex + match-case",
			sc:       CellsSearch,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--find", "foo", "--regex", "--match-case"},
			toolName: "search_data",
			wantInput: map[string]interface{}{
				"excel_id":    testToken,
				"sheet_id":    testSheetID,
				"search_term": "foo",
			},
			wantOptions: map[string]interface{}{
				"match_case": true,
				"use_regex":  true,
			},
		},
		{
			name:     "+cells-search all four options",
			sc:       CellsSearch,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--find", "x", "--match-case", "--match-entire-cell", "--regex", "--include-formulas"},
			toolName: "search_data",
			wantInput: map[string]interface{}{
				"excel_id":    testToken,
				"sheet_id":    testSheetID,
				"search_term": "x",
			},
			wantOptions: map[string]interface{}{
				"match_case":        true,
				"match_entire_cell": true,
				"use_regex":         true,
				"match_formulas":    true,
			},
		},
		{
			name:     "+cells-replace empty replace deletes match",
			sc:       CellsReplace,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--find", "foo", "--replacement", ""},
			toolName: "replace_data",
			wantInput: map[string]interface{}{
				"excel_id":     testToken,
				"sheet_id":     testSheetID,
				"search_term":  "foo",
				"replace_term": "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := parseDryRunBody(t, tt.sc, tt.args)
			got := decodeToolInput(t, body, tt.toolName)
			assertInputEquals(t, got, tt.wantInput)
			if tt.wantOptions != nil {
				opts, _ := got["options"].(map[string]interface{})
				if opts == nil {
					t.Fatalf("options missing: %#v", got)
				}
				for k, want := range tt.wantOptions {
					if opts[k] != want {
						t.Errorf("options[%q] = %v, want %v", k, opts[k], want)
					}
				}
			}
		})
	}
}

func TestCellsReplace_RequireFlag(t *testing.T) {
	t.Parallel()
	// --replace not passed at all (vs empty string) should error. This trips
	// cobra's required-flag gate before our Validate hook runs, so the error
	// is cobra's plain `required flag(s) "replacement" not set` rather than a
	// typed *errs.ValidationError — keep this assertion as a substring check.
	stdout, stderr, err := runShortcutCapturingErr(t, CellsReplace, []string{
		"--url", testURL, "--sheet-id", testSheetID, "--find", "foo", "--dry-run",
	})
	if err == nil {
		t.Fatalf("expected error when --replacement omitted; stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(err.Error(), "replacement") {
		t.Errorf("expected message about --replacement; got=%s|%s|%v", stdout, stderr, err)
	}
}
