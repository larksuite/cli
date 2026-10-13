// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResizeSizeCompatibilityUsesExactTargets(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name, command, flag, payload, span, wire string
		targets                                  []string
		sizes                                    []interface{}
	}{
		{"column list crosses Z", "+cols-resize", "widths", `[80,"120","standard"]`, "Z:AB", "resize_width", []string{"Z:Z", "AA:AA", "AB:AB"}, []interface{}{float64(80), float64(120), "standard"}},
		{"row list uses absolute start", "+rows-resize", "heights", `[30,"auto",50]`, "7:9", "resize_height", []string{"7:7", "8:8", "9:9"}, []interface{}{float64(30), "auto", float64(50)}},
		{"bare size list", "+cols-resize", "widths", `80,120,160`, "B:D", "resize_width", []string{"B:B", "C:C", "D:D"}, []interface{}{float64(80), float64(120), float64(160)}},
		{"echoed map range", "+rows-resize", "heights", `{"8:9":40,"7":30}`, "7:9", "resize_height", []string{"7:7", "8:9"}, []interface{}{float64(30), float64(40)}},
		{"coordinate list keeps gaps", "+cols-resize", "widths", `[{"column":"Z","width":80},{"col":"B","size":120}]`, "", "resize_width", []string{"B:B", "Z:Z"}, []interface{}{float64(120), float64(80)}},
		{"explicit pixel entries", "+rows-resize", "heights", `[{"range":"31:32","type":"pixel","size":88}]`, "", "resize_height", []string{"31:32"}, []interface{}{float64(88)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"--url", testURL, "--sheet-name", "S", "--" + tc.flag, tc.payload}
			if tc.span != "" {
				args = append(args, "--range", tc.span)
			}
			api := parseDryRunAPI(t, shortcutFromRegistry(t, tc.command), args)
			body := api[0].(map[string]interface{})["body"].(map[string]interface{})
			require.Equal(t, "batch_update", body["tool_name"])
			ops := gjson.Get(body["input"].(string), "operations").Array()
			require.Len(t, ops, len(tc.targets))
			for i, target := range tc.targets {
				require.Equal(t, "resize_range", ops[i].Get("tool_name").String())
				input := ops[i].Get("input")
				require.Equal(t, testToken, input.Get("excel_id").String())
				require.Equal(t, "S", input.Get("sheet_name").String())
				require.Equal(t, target, input.Get("range").String())
				if mode, ok := tc.sizes[i].(string); ok {
					require.Equal(t, mode, input.Get(tc.wire+".type").String())
					require.False(t, input.Get(tc.wire+".value").Exists())
				} else {
					require.Equal(t, "pixel", input.Get(tc.wire+".type").String())
					require.Equal(t, tc.sizes[i], input.Get(tc.wire+".value").Value())
				}
			}
		})
	}
}

func TestResizeSizeCompatibilityRejectsAmbiguousOrUnsafeInputs(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		payload, span string
	}{
		{`[80,120]`, "A:C"},
		{`[80,120]`, ""},
		{`[80,120]`, "2:3"},
		{`[80,120]`, "B:A"},
		{`[80]`, "999999999999999999999999999999"},
		{`[80,12]`, "A:B"},
		{`[80,120.5]`, "A:B"},
		{`[80,null]`, "A:B"},
		{`[80,true]`, "A:B"},
		{`[80,"auto"]`, "A:B"},
		{`{"A":80,"C":120}`, "A:C"},
		{`{"A":80,"B:C":120}`, "A:B"},
		{`{"A":80,"A:B":120}`, "A:B"},
		{`[{"column":"A","width":80,"unknown":true}]`, ""},
		{`[{"column":"A","width":80},{"column":"A","width":120}]`, ""},
		{`[{"column":"A","width":80},{"column":"A:A","width":120}]`, ""},
		{`[{"column":"A","range":"B","width":80}]`, ""},
		{`[{"column":"A","width":80,"size":120}]`, ""},
		{`[{"row":"3","height":80}]`, ""},
		{`[{"column":"A","width":80,"type":"standard"}]`, ""},
		{`[{"column":"A","width":"standard","type":"pixel"}]`, ""},
		{`[{"column":"A","width":80},120]`, ""},
	} {
		t.Run(tc.payload+tc.span, func(t *testing.T) {
			args := []string{"--url", testURL, "--sheet-name", "S", "--widths", tc.payload, "--dry-run"}
			if tc.span != "" {
				args = append(args, "--range", tc.span)
			}
			stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cols-resize"), args)
			var validation *errs.ValidationError
			require.ErrorAs(t, err, &validation)
			require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
			if validation.Param == "" {
				require.Len(t, validation.Params, 2)
				require.Equal(t, "--widths", validation.Params[0].Name)
				require.Equal(t, "--range", validation.Params[1].Name)
			} else {
				require.Equal(t, "--widths", validation.Param)
			}
			require.Empty(t, stdout)
			if tc.payload == `[80,120.5]` {
				// The schema failure must survive the domain typed wrapper.
				require.NotNil(t, errors.Unwrap(validation))
			}
		})
	}
}

func TestResizeSizeListKeepsBatchLimit(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	list := make([]int, maxBatchOperations+1)
	for i := range list {
		list[i] = 80
	}
	payload, err := json.Marshal(list)
	require.NoError(t, err)
	_, _, err = runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cols-resize"), []string{
		"--url", testURL, "--sheet-name", "S", "--range", "A:CW", "--widths", string(payload), "--dry-run",
	})
	var validation *errs.ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, "--widths", validation.Param)
	require.Contains(t, validation.Message, "at most 100 entries")
}
