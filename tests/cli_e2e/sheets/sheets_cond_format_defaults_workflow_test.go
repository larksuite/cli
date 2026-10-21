// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSheets_CondFormatDefaultsWorkflow needs a bot with Sheets read/write and
// Drive create/delete access. createSpreadsheet registers workbook and folder
// cleanup on the parent, including when a rule assertion fails. This proves
// stored rule configuration and calculated color-scale endpoint colors.
// +cond-format-result-get exposes cell_styles, with no documented bar-length
// or icon field, so dataBar/iconSet rendering is outside this proof surface.
func TestSheets_CondFormatDefaultsWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	spreadsheetToken := createSpreadsheet(t, t, ctx, "lark-cli-e2e-cond-format-defaults-"+clie2e.GenerateSuffix(), "bot")
	info, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"sheets", "+workbook-info", "--spreadsheet-token", spreadsheetToken}, DefaultAs: "bot",
	})
	require.NoError(t, err)
	info.AssertExitCode(t, 0)
	info.AssertStdoutStatus(t, true)
	sheetID := gjson.Get(info.Stdout, "data.sheets.0.sheet_id")
	require.Equal(t, gjson.String, sheetID.Type)
	require.NotEmpty(t, sheetID.String())

	seed, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"sheets", "+cells-set", "--spreadsheet-token", spreadsheetToken, "--sheet-id", sheetID.String(),
			"--range", "A1:C6", "--cells", `[[{"value":-10},{"value":0},{"value":0}],[{"value":0},{"value":20},{"value":20}],[{"value":25},{"value":40},{"value":40}],[{"value":50},{"value":60},{"value":60}],[{"value":75},{"value":80},{"value":80}],[{"value":100},{"value":100},{"value":100}]]`},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	seed.AssertExitCode(t, 0)
	seed.AssertStdoutStatus(t, true)

	for _, rule := range condFormatDefaultCases {
		t.Run(rule.ruleType, func(t *testing.T) {
			var ruleID string
			for _, stage := range []struct{ operation, cellRange, attrs string }{
				{"create", rule.column + "1:" + rule.column + "6", rule.createAttrs},
				{"update", rule.column + "2:" + rule.column + "5", rule.updateAttrs},
			} {
				if !t.Run(stage.operation+" and read back", func(t *testing.T) {
					args := []string{"sheets", "+cond-format-" + stage.operation,
						"--spreadsheet-token", spreadsheetToken, "--sheet-id", sheetID.String(),
						"--rule-type", rule.ruleType, "--ranges", `["` + stage.cellRange + `"]`,
						"--properties", `{"attrs":` + stage.attrs + `}`}
					if stage.operation == "update" {
						require.NotEmpty(t, ruleID, "update must use the rule identifier read after create")
						args = append(args, "--rule-id", ruleID)
					}
					write, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
					require.NoError(t, err)
					write.AssertExitCode(t, 0)
					write.AssertStdoutStatus(t, true)

					// List the sheet after both writes so an update that accidentally
					// creates a second rule cannot pass a filtered read of the old ID.
					read, err := clie2e.RunCmd(ctx, clie2e.Request{
						Args:      []string{"sheets", "+cond-format-list", "--spreadsheet-token", spreadsheetToken, "--sheet-id", sheetID.String()},
						DefaultAs: "bot",
					})
					require.NoError(t, err)
					read.AssertExitCode(t, 0)
					read.AssertStdoutStatus(t, true)
					matches := condFormatRulesOfType(t, gjson.Get(read.Stdout, "data"), sheetID.String(), rule.ruleType)
					require.Len(t, matches, 1, "read-back must contain exactly one %s rule", rule.ruleType)
					stored := matches[0]
					require.Equal(t, gjson.String, stored.id.Type, "conditional_format_id must be a string")
					require.NotEmpty(t, stored.id.String())
					if stage.operation == "create" {
						ruleID = stored.id.String()
					} else {
						require.Equal(t, ruleID, stored.id.String(), "update must retain the rule identifier")
					}
					require.Equal(t, rule.ruleType, stored.properties.Get("rule_type").String())
					require.Equal(t, []any{stage.cellRange}, stored.properties.Get("ranges").Value())
					require.True(t, stored.properties.Get("style").IsObject(), "stored shared style must remain an object")
					require.Empty(t, stored.properties.Get("style").Map(), "omitted shared style must not invent formatting")
					attrs := stored.properties.Get("attrs")
					require.True(t, attrs.IsArray(), "stored rule attrs must be an array")
					actualAttrs := attrs.Array()
					wantAttrs := gjson.Parse(stage.attrs).Array()
					require.Len(t, actualAttrs, len(wantAttrs))
					for i, expected := range wantAttrs {
						// The server may add defaults; every supplied threshold, type,
						// operator and color still has to survive on its own entry.
						for key, value := range expected.Map() {
							require.Equal(t, value.Value(), actualAttrs[i].Get(key).Value(), "attrs[%d].%s", i, key)
						}
					}

					if rule.ruleType == "colorScale" {
						computed, err := clie2e.RunCmd(ctx, clie2e.Request{
							Args: []string{"sheets", "+cond-format-result-get", "--spreadsheet-token", spreadsheetToken,
								"--sheet-id", sheetID.String(), "--range", stage.cellRange},
							DefaultAs: "bot",
						})
						require.NoError(t, err)
						computed.AssertExitCode(t, 0)
						computed.AssertStdoutStatus(t, true)
						ranges := gjson.Get(computed.Stdout, "data.ranges").Array()
						require.Len(t, ranges, 1)
						require.False(t, ranges[0].Get("truncated").Bool())
						rows := ranges[0].Get("cells").Array()
						wantRows := 6
						if stage.operation == "update" {
							wantRows = 4
						}
						require.Len(t, rows, wantRows)
						// The fixture increases down the column: the first and last
						// cells in each applied range are the exact min/max sentinels.
						for _, endpoint := range []struct{ row, attr int }{{0, 0}, {wantRows - 1, len(wantAttrs) - 1}} {
							cells := rows[endpoint.row].Array()
							require.Len(t, cells, 1)
							wantColor := wantAttrs[endpoint.attr].Get("color").String()
							gotColor := cells[0].Get("cell_styles.background_color").String()
							require.True(t, condFormatOpaqueColorEqual(wantColor, gotColor),
								"color-scale endpoint row %d: want %s, got %s", endpoint.row, wantColor, gotColor)
						}
					}
				}) {
					return
				}
			}
		})
	}
}

// The service can serialize an opaque endpoint color as hex, rgb or rgba.
// Accept only integer RGB channels and an exactly opaque alpha; this is not
// a general CSS parser and must not round a different color into a match.
func condFormatOpaqueColorEqual(expectedHex, actual string) bool {
	if len(expectedHex) != 7 || expectedHex[0] != '#' {
		return false
	}
	rgb, err := strconv.ParseUint(expectedHex[1:], 16, 24)
	if err != nil {
		return false
	}
	actual = strings.TrimSpace(actual)
	if strings.EqualFold(expectedHex, actual) {
		return true
	}
	function, args, ok := strings.Cut(strings.ToLower(actual), "(")
	if !ok || !strings.HasSuffix(args, ")") {
		return false
	}
	channels := strings.Split(strings.TrimSuffix(args, ")"), ",")
	switch function {
	case "rgb":
		if len(channels) != 3 {
			return false
		}
	case "rgba":
		if len(channels) != 4 {
			return false
		}
		alpha := strings.TrimSpace(channels[3])
		if alpha != "1" {
			fraction, ok := strings.CutPrefix(alpha, "1.")
			if !ok || fraction == "" || strings.Trim(fraction, "0") != "" {
				return false
			}
		}
	default:
		return false
	}
	for i, shift := range []uint{16, 8, 0} {
		channel, err := strconv.ParseUint(strings.TrimSpace(channels[i]), 10, 8)
		if err != nil || channel != (rgb>>shift)&0xff {
			return false
		}
	}
	return true
}

func TestSheets_CondFormatEndpointColorRegression(t *testing.T) {
	for _, tc := range []struct {
		name, expected, actual string
		equal                  bool
	}{
		{"hex case", "#FF0000", "#ff0000", true},
		{"rgb", "#FF0000", "rgb(255, 0, 0)", true},
		{"live rgba", "#FF0000", "rgba(255, 0, 0, 1)", true},
		{"mixed channels and decimal alpha", "#63BE7B", "rgba(99,190,123,1.000)", true},
		{"different hex", "#FF0000", "#00ff00", false},
		{"different rgb", "#FF0000", "rgb(254, 0, 0)", false},
		{"different rgba", "#FF0000", "rgba(255, 0, 1, 1)", false},
		{"translucent", "#FF0000", "rgba(255, 0, 0, 0.5)", false},
		{"alpha must not round to one", "#FF0000", "rgba(255, 0, 0, 0.999999999999999999)", false},
		{"alpha above one", "#FF0000", "rgba(255, 0, 0, 1.000000000000000001)", false},
		{"missing alpha", "#FF0000", "rgba(255, 0, 0)", false},
		{"extra channel", "#FF0000", "rgb(255, 0, 0, 1)", false},
		{"out of range channel", "#FF0000", "rgb(256, 0, 0)", false},
		{"fractional channel", "#FF0000", "rgb(254.999999999999999999, 0, 0)", false},
		{"invalid channel", "#FF0000", "rgb(red, 0, 0)", false},
		{"trailing content", "#FF0000", "rgba(255, 0, 0, 1) extra", false},
		{"empty color", "#FF0000", "", false},
		{"invalid hex", "#GG0000", "#GG0000", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.equal, condFormatOpaqueColorEqual(tc.expected, tc.actual),
				"expected %q, actual %q", tc.expected, tc.actual)
		})
	}
}

type condFormatStoredRule struct {
	id         gjson.Result
	properties gjson.Result
}

// get_conditional_format_objects returns sheets[].conditional_formats[],
// with conditional_format_id beside details (not inside the rule details).
func condFormatRulesOfType(t *testing.T, data gjson.Result, sheetID, ruleType string) []condFormatStoredRule {
	t.Helper()
	sheets := data.Get("sheets")
	require.True(t, sheets.IsArray(), "conditional-format read-back must contain data.sheets")
	var matchingSheets []gjson.Result
	for _, sheet := range sheets.Array() {
		if sheet.Get("sheet_id").String() == sheetID {
			matchingSheets = append(matchingSheets, sheet)
		}
	}
	require.Len(t, matchingSheets, 1, "conditional-format read-back must contain the requested sheet exactly once")
	sheet := matchingSheets[0]
	require.Equal(t, gjson.String, sheet.Get("sheet_id").Type)
	require.Equal(t, sheetID, sheet.Get("sheet_id").String())
	formats := sheet.Get("conditional_formats")
	require.True(t, formats.IsArray(), "requested sheet must contain conditional_formats")
	var rules []condFormatStoredRule
	for _, format := range formats.Array() {
		details := format.Get("details")
		require.True(t, details.IsObject(), "conditional format must contain details")
		if details.Get("rule_type").String() == ruleType {
			rules = append(rules, condFormatStoredRule{format.Get("conditional_format_id"), details})
		}
	}
	return rules
}

func TestSheets_CondFormatListReadbackShape(t *testing.T) {
	data := gjson.Parse(`{
		"sheets": [
			{"sheet_id":"other-sheet","sheet_name":"Other","conditional_formats":[
				{"conditional_format_id":"other-rule","details":{"rule_type":"dataBar","ranges":["A1:A6"],"style":{},"attrs":[{"value_type":"minValue","color":"#FFFFFF"},{"value_type":"maxValue","color":"#000000"}]}}
			]},
			{"sheet_id":"target-sheet","sheet_name":"Target","conditional_formats":[
				{"conditional_format_id":"target-rule","details":{"rule_type":"dataBar","ranges":["A2:A5"],"style":{},"attrs":[{"value_type":"minValue","color":"#63BE7B"},{"value_type":"maxValue","color":"#F8696B"}]}}
			]}
		],
		"total":2
	}`)
	rules := condFormatRulesOfType(t, data, "target-sheet", "dataBar")
	require.Len(t, rules, 1)
	require.Equal(t, gjson.String, rules[0].id.Type)
	require.Equal(t, "target-rule", rules[0].id.String())
	require.JSONEq(t, `{"rule_type":"dataBar","ranges":["A2:A5"],"style":{},"attrs":[{"value_type":"minValue","color":"#63BE7B"},{"value_type":"maxValue","color":"#F8696B"}]}`, rules[0].properties.Raw)
	require.Empty(t, condFormatRulesOfType(t, data, "target-sheet", "iconSet"))
}
