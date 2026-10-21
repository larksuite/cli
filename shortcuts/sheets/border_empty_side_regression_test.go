// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type borderEmptySideInvocation struct {
	name, command, cellPath, param string
	args                           []string
}

// Exercise the mounted commands, including envelope reconciliation and the
// batch translator, so a pre-merge rejection cannot hide behind helper tests.
func borderEmptySideFlagInvocations(t *testing.T, border string, attributes []string) []borderEmptySideInvocation {
	t.Helper()
	var invocations []borderEmptySideInvocation
	for _, carrier := range []string{"border-styles", "style", "border", "borders", "border_side", "side_border"} {
		value := border
		var decoded map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(border), &decoded))
		flag := "style"
		switch carrier {
		case "border-styles":
			flag = carrier
		case "style":
			value = `{"border_styles":` + border + `}`
		case "border", "borders":
			value = `{"` + carrier + `":` + border + `}`
		default:
			style := map[string]interface{}{}
			for side, spec := range decoded {
				key := "border_" + side
				if carrier == "side_border" {
					key = side + "_border"
				}
				style[key] = spec
			}
			encoded, err := json.Marshal(style)
			require.NoError(t, err)
			value = string(encoded)
		}
		for _, invocation := range borderEmptySideInputInvocations(t, flag, value, attributes) {
			invocation.name += "/" + carrier
			invocations = append(invocations, invocation)
		}
	}
	return invocations
}

func borderEmptySideInputInvocations(t *testing.T, flag, value string, attributes []string) []borderEmptySideInvocation {
	t.Helper()
	var invocations []borderEmptySideInvocation
	for _, command := range []string{"+cells-set-style", "+cells-batch-set-style"} {
		args := []string{"--sheet-name", "S", "--range", "B2"}
		cellPath := "cells.0.0"
		if command == "+cells-batch-set-style" {
			args = []string{"--ranges", `["S!B2","S!D4"]`}
			cellPath = "operations.0.input.cells.0.0"
		}
		args = append(args, "--"+flag, value)
		args = append(args, attributes...)
		invocations = append(invocations, borderEmptySideInvocation{
			name: command, command: command, cellPath: cellPath,
			param: "--border-styles", args: args,
		})
	}
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(value), &decoded))
	input := map[string]interface{}{"sheet_name": "S", "range": "B2", strings.ReplaceAll(flag, "-", "_"): decoded}
	for i := 0; i < len(attributes); i += 2 {
		key := strings.ReplaceAll(strings.TrimPrefix(attributes[i], "--"), "-", "_")
		input[key] = attributes[i+1]
	}
	operations, err := json.Marshal([]interface{}{map[string]interface{}{
		"shortcut": "+cells-set-style", "input": input,
	}})
	require.NoError(t, err)
	invocations = append(invocations, borderEmptySideInvocation{
		name: "+batch-update", command: "+batch-update",
		cellPath: "operations.0.input.cells.0.0", param: "--operations",
		args: []string{"--operations", string(operations)},
	})
	return invocations
}

func TestBorderEmptySideMergesBeforeValidation(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name, border, want string
		attributes         []string
	}{
		{
			name: "empty selection with weight", border: `{"left":{}}`,
			attributes: []string{"--border-weight", "medium"}, want: `{"left":{"weight":"medium"}}`,
		},
		{
			name: "empty selection with color", border: `{"left":{}}`,
			attributes: []string{"--border-color", "#123456"}, want: `{"left":{"color":"#123456"}}`,
		},
		{
			name: "empty selection with both attributes", border: `{"left":{}}`,
			attributes: []string{"--border-weight", "medium", "--border-color", "#123456"},
			want:       `{"left":{"weight":"medium","color":"#123456"}}`,
		},
		{
			name: "all empty sides with weight", border: `{"all":{}}`,
			attributes: []string{"--border-weight", "medium"},
			want:       `{"left":{"weight":"medium"},"right":{"weight":"medium"},"top":{"weight":"medium"},"bottom":{"weight":"medium"}}`,
		},
		{
			name: "mixed empty and styled sides", border: `{"left":{},"right":{"style":"dashed"}}`,
			attributes: []string{"--border-color", "#123456"},
			want:       `{"left":{"color":"#123456"},"right":{"style":"dashed","color":"#123456"}}`,
		},
		{
			name: "empty weight filled by weight flag", border: `{"left":{"weight":""}}`,
			attributes: []string{"--border-weight", "medium"}, want: `{"left":{"weight":"medium"}}`,
		},
		{
			name: "empty weight filled by color flag", border: `{"left":{"weight":""}}`,
			attributes: []string{"--border-color", "#123456"}, want: `{"left":{"color":"#123456"}}`,
		},
		{
			name: "empty weight and styled side both receive color", border: `{"left":{"weight":""},"right":{"style":"solid"}}`,
			attributes: []string{"--border-color", "#123456"},
			want:       `{"left":{"color":"#123456"},"right":{"style":"solid","color":"#123456"}}`,
		},
		{
			name: "whitespace weight and styled side both receive weight", border: `{"left":{"weight":" \t"},"right":{"style":"solid"}}`,
			attributes: []string{"--border-weight", "medium"},
			want:       `{"left":{"weight":"medium"},"right":{"style":"solid","weight":"medium"}}`,
		},
		{
			name: "empty weight flag does not write null", border: `{"left":{"style":"solid","weight":""}}`,
			attributes: []string{"--border-weight", ""}, want: `{"left":{"style":"solid"}}`,
		},
		{
			name: "empty weight flag preserves merged color", border: `{"left":{}}`,
			attributes: []string{"--border-weight", "", "--border-color", "#123456"}, want: `{"left":{"color":"#123456"}}`,
		},
		{
			name: "width survives empty weight flag", border: `{"left":{"weight":"","width":2}}`,
			attributes: []string{"--border-weight", ""}, want: `{"left":{"weight":"medium"}}`,
		},
		{
			name: "width and color retain both attributes", border: `{"left":{"weight":"","width":3}}`,
			attributes: []string{"--border-color", "#123456"}, want: `{"left":{"weight":"thick","color":"#123456"}}`,
		},
		{
			name: "empty optional weight with width only", border: `{"left":{"weight":"","width":3}}`,
			want: `{"left":{"weight":"thick"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, invocation := range borderEmptySideFlagInvocations(t, tc.border, tc.attributes) {
				t.Run(invocation.name, func(t *testing.T) {
					assertBorderEmptySideDryRun(t, invocation, tc.want)
				})
			}
		})
	}
}

func TestBorderEmptySideRejectsMissingEffectiveAttributes(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name, border string
		attributes   []string
	}{
		{name: "empty side", border: `{"left":{}}`},
		{name: "empty weight only", border: `{"left":{"weight":""}}`},
		{name: "whitespace weight only", border: `{"left":{"weight":" \t"}}`},
		{name: "all empty sides", border: `{"all":{}}`},
		{name: "another styled side cannot hide an empty side", border: `{"top":{"style":"solid"},"left":{}}`},
		{name: "another styled side cannot hide an empty weight", border: `{"left":{"weight":""},"right":{"style":"solid"}}`},
		{name: "empty weight flag leaves selection empty", border: `{"left":{}}`, attributes: []string{"--border-weight", ""}},
		{name: "empty flag and empty optional weight", border: `{"left":{"weight":""}}`, attributes: []string{"--border-weight", " \t"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, invocation := range borderEmptySideFlagInvocations(t, tc.border, tc.attributes) {
				t.Run(invocation.name, func(t *testing.T) {
					args := append([]string{"--url", testURL, "--dry-run"}, invocation.args...)
					out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, invocation.command), args)
					validation := requireValidation(t, err, "must include at least one border attribute")
					require.Equal(t, invocation.param, validation.Param)
					// The batch key-normalization boundary does not retain causes;
					// translator and standalone validation do.
					if invocation.command != "+batch-update" || len(tc.attributes) == 0 {
						require.NotNil(t, validation.Cause)
					}
					require.Empty(t, out)
				})
			}
		})
	}
}

func TestBorderEmptySideJSONPayloadsValidateFinalAttributes(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name, border, want string
	}{
		{name: "empty side", border: `{"left":{}}`},
		{name: "empty weight only", border: `{"left":{"weight":""}}`},
		{name: "empty weight with style", border: `{"left":{"style":"solid","weight":""}}`, want: `{"left":{"style":"solid"}}`},
		{name: "empty weight with width", border: `{"left":{"weight":"","width":2}}`, want: `{"left":{"weight":"medium"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			style := `{"border_styles":` + tc.border + `}`
			for _, invocation := range []borderEmptySideInvocation{
				{
					name: "styles item", command: "+styles-put", cellPath: "operations.0.input.cells.0.0", param: "--styles",
					args: []string{"--styles", `{"styles":[{"name":"S","cell_styles":[{"range":"B2","border_styles":` + tc.border + `}]}]}`},
				},
				{
					name: "nested style item", command: "+styles-put", cellPath: "operations.0.input.cells.0.0", param: "--styles",
					args: []string{"--styles", `{"styles":[{"name":"S","cell_styles":[{"range":"B2","style":` + style + `}]}]}`},
				},
				{
					name: "typed cell", command: "+cells-set", cellPath: "cells.0.0", param: "--cells",
					args: []string{"--sheet-name", "S", "--range", "B2", "--cells", `[[` + style + `]]`},
				},
				{
					name: "nested typed cell", command: "+cells-set", cellPath: "cells.0.0", param: "--cells",
					args: []string{"--sheet-name", "S", "--range", "B2", "--cells", `[[{"cell_styles":` + style + `}]]`},
				},
				{
					name: "batch typed cell", command: "+batch-update", cellPath: "operations.0.input.cells.0.0", param: "--operations",
					args: []string{"--operations", `[{"shortcut":"+cells-set","input":{"sheet_name":"S","range":"B2","cells":[[` + style + `]]}}]`},
				},
			} {
				t.Run(invocation.name, func(t *testing.T) {
					if tc.want != "" {
						assertBorderEmptySideDryRun(t, invocation, tc.want)
						return
					}
					args := append([]string{"--url", testURL, "--dry-run"}, invocation.args...)
					out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, invocation.command), args)
					validation := requireValidation(t, err, "must include at least one border attribute")
					require.Equal(t, invocation.param, validation.Param)
					require.NotNil(t, validation.Cause)
					require.Empty(t, out)
				})
			}
		})
	}
}

func assertBorderEmptySideDryRun(t *testing.T, invocation borderEmptySideInvocation, want string) {
	t.Helper()
	calls := parseDryRunAPI(t, shortcutFromRegistry(t, invocation.command), append([]string{"--url", testURL}, invocation.args...))
	require.Len(t, calls, 1)
	call := calls[0].(map[string]interface{})
	require.Equal(t, "POST", call["method"])
	require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/"+testToken+"/tools/invoke_write", call["url"])
	body := call["body"].(map[string]interface{})
	tool := "set_cell_range"
	if strings.HasPrefix(invocation.cellPath, "operations.") {
		tool = "batch_update"
	}
	input := decodeToolInput(t, body, tool)
	require.Equal(t, testToken, input["excel_id"])
	raw := body["input"].(string)
	cell := gjson.Get(raw, invocation.cellPath)
	require.JSONEq(t, want, cell.Get("border_styles").Raw)
	require.False(t, cell.Get("value").Exists())
	if invocation.command == "+cells-batch-set-style" {
		require.JSONEq(t, want, gjson.Get(raw, "operations.1.input.cells.0.0.border_styles").Raw)
	}
}

func TestBorderEmptySideStillRejectsConflictingWeight(t *testing.T) {
	_, _, err := borderAttributeFlags(newMapFlagViewForCommand("+cells-set-style", map[string]interface{}{
		"border-styles": `{"left":{"weight":"","width":2}}`,
		"border-weight": "thick",
	}))
	var validation *errs.ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
	require.Equal(t, "--border-weight", validation.Param)
}

func TestBorderEmptySideAliasCanonicalMergeRegression(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name, style, want string
	}{
		{
			name:  "empty alias preserves canonical attributes",
			style: `{"border":{"left":{"weight":""},"right":{"style":"solid"}},"border_styles":{"left":{"style":"dashed","weight":"thick","color":"#123456"}}}`,
			want:  `{"left":{"style":"dashed","weight":"thick","color":"#123456"},"right":{"style":"solid"}}`,
		},
		{
			name:  "disjoint alias and canonical attributes merge",
			style: `{"left_border":{"weight":" \t","color":"#123456"},"border_styles":{"left":{"style":"dashed","weight":"medium"}}}`,
			want:  `{"left":{"style":"dashed","weight":"medium","color":"#123456"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, invocation := range borderEmptySideInputInvocations(t, "style", tc.style, nil) {
				t.Run(invocation.name, func(t *testing.T) {
					assertBorderEmptySideDryRun(t, invocation, tc.want)
				})
			}
		})
	}
}

func TestBorderEmptySideAliasRejectsConflictsAndInvalidTypesRegression(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name, style, message string
	}{
		{"canonical side scalar", `{"border":{"left":{"weight":""}},"border_styles":{"left":"solid"}}`, "conflicts"},
		{"canonical side null", `{"borders":{"left":{"weight":" \t"}},"border_styles":{"left":null}}`, "conflicts"},
		{"canonical side array", `{"border_left":{},"border_styles":{"left":[]}}`, "conflicts"},
		{"canonical border nonobject", `{"left_border":{"weight":""},"border_styles":[]}`, "must be an object"},
		{"canonical color collision", `{"border":{"left":{"weight":"","color":"#123456"}},"border_styles":{"left":{"color":"#654321"}}}`, "conflicts"},
		{"canonical weight collision", `{"border_left":{"weight":"","width":2},"border_styles":{"left":{"weight":"thick"}}}`, "conflicts"},
		{"duplicate attribute still rejected", `{"left_border":{"weight":"","style":"solid"},"border_styles":{"left":{"style":"solid"}}}`, "conflicts"},
		{"invalid weight type", `{"border":{"left":{"weight":false}}}`, "weight"},
		{"null weight remains invalid", `{"border":{"left":{"weight":null}}}`, "weight"},
		{"invalid color type", `{"borders":{"left":{"weight":"","color":false}}}`, "color"},
		{"invalid style type", `{"border_left":{"weight":"","style":[]}}`, "style"},
		{"unknown attribute", `{"left_border":{"weight":"","unexpected":true}}`, "not a border attribute"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, invocation := range borderEmptySideInputInvocations(t, "style", tc.style, nil) {
				t.Run(invocation.name, func(t *testing.T) {
					args := append([]string{"--url", testURL, "--dry-run"}, invocation.args...)
					out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, invocation.command), args)
					validation := requireValidation(t, err, tc.message)
					param := "--style"
					if invocation.command == "+batch-update" {
						param = "--operations"
					}
					require.Equal(t, param, validation.Param)
					require.Empty(t, out)
				})
			}
		})
	}
}
