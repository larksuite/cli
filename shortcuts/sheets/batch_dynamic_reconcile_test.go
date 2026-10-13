// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBatchOp_DynamicReconciliationsMatchStandaloneContracts(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name     string
		shortcut string
		input    map[string]interface{}
		want     map[string]string
	}{
		{
			name:     "delete end-index closes the inclusive span",
			shortcut: "+dim-delete",
			input: map[string]interface{}{
				"sheet_name":  "S",
				"start_index": 3,
				"end_index":   7,
			},
			want: map[string]string{"range": "3:7"},
		},
		{
			name:     "insert singleton range resolves to position",
			shortcut: "+dim-insert",
			input: map[string]interface{}{
				"sheet_name": "S",
				"range":      "3:3",
				"count":      float64(2),
			},
			want: map[string]string{"position": "3", "count": "2"},
		},
		{
			name:     "font bold reaches font weight",
			shortcut: "+cells-set-style",
			input: map[string]interface{}{
				"sheet_name": "S",
				"range":      "A1",
				"font_bold":  true,
			},
			want: map[string]string{"cells.0.0.cell_styles.font_weight": "bold"},
		},
		{
			name:     "plural style envelope keeps numeric fields",
			shortcut: "+cells-set-style",
			input: map[string]interface{}{
				"sheet_name": "S",
				"range":      "A1",
				"styles":     map[string]interface{}{"font_size": float64(14), "font_weight": "bold"},
			},
			want: map[string]string{"cells.0.0.cell_styles.font_size": "14", "cells.0.0.cell_styles.font_weight": "bold"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			translated, err := translateBatchOp(subOp(tc.shortcut, tc.input), testToken, 0)
			require.NoError(t, err)
			body, err := json.Marshal(translated["input"])
			require.NoError(t, err)
			for path, want := range tc.want {
				require.Equal(t, want, gjson.GetBytes(body, path).String(), string(body))
			}
		})
	}
}

func TestBatchOp_DeleteEndIndexSpellingsPreserveBoundary(t *testing.T) {
	for _, key := range []string{"end-index", "end_index", "endIndex"} {
		t.Run(key, func(t *testing.T) {
			translated, err := translateBatchOp(subOp("+dim-delete", map[string]interface{}{
				"sheet_name": "S", "start_index": float64(3), key: float64(7),
			}), testToken, 0)
			require.NoError(t, err)
			require.Equal(t, "modify_sheet_structure", translated["tool_name"])
			input := translated["input"].(map[string]interface{})
			require.Equal(t, "S", input["sheet_name"])
			require.Equal(t, "delete", input["operation"])
			require.Equal(t, "3:7", input["range"])
		})
	}
	for _, end := range []interface{}{float64(2), "bad"} {
		_, err := translateBatchOp(subOp("+dim-delete", map[string]interface{}{
			"sheet_name": "S", "start_index": float64(3), "endIndex": end,
		}), testToken, 0)
		validation := requireValidation(t, err, "end-index")
		require.Equal(t, "--operations", validation.Param)
	}
}

func TestBatchOp_DynamicReconciliationsRejectConflicts(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name     string
		shortcut string
		input    map[string]interface{}
		want     string
	}{
		{
			name:     "delete count disagrees with end index",
			shortcut: "+dim-delete",
			input: map[string]interface{}{
				"sheet_name":  "S",
				"start_index": 6,
				"end_index":   22,
				"count":       float64(16),
			},
			want: "disagrees with --range",
		},
		{
			name:     "insert range disagrees with position",
			shortcut: "+dim-insert",
			input: map[string]interface{}{
				"sheet_name": "S",
				"range":      "3:3",
				"position":   "4",
				"count":      float64(2),
			},
			want: "disagrees with --position",
		},
		{
			name:     "bold aliases disagree",
			shortcut: "+cells-set-style",
			input: map[string]interface{}{
				"sheet_name": "S",
				"range":      "A1",
				"bold":       false,
				"font_bold":  true,
			},
			want: "conflicting bold values",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := translateBatchOp(subOp(tc.shortcut, tc.input), testToken, 0)
			requireValidation(t, err, tc.want)
		})
	}
}
