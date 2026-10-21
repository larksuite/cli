// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// The same contract-valid attrs drive dry-run and live read-back coverage.
var condFormatDefaultCases = []struct {
	ruleType, column, createAttrs, updateAttrs string
}{
	{
		ruleType: "dataBar", column: "A",
		createAttrs: `[{"value_type":"minValue","color":"#63BE7B"},{"value_type":"maxValue","color":"#F8696B"}]`,
		updateAttrs: `[{"value_type":"minValue","color":"#5A8AC6"},{"value_type":"maxValue","color":"#FFAA00"}]`,
	},
	{
		ruleType: "colorScale", column: "B",
		createAttrs: `[{"value_type":"minValue","color":"#FFFFFF"},{"value_type":"maxValue","color":"#FF0000"}]`,
		updateAttrs: `[{"value_type":"minValue","color":"#F8696B"},{"value_type":"percent","value":50,"color":"#FFEB84"},{"value_type":"maxValue","color":"#63BE7B"}]`,
	},
	{
		ruleType: "iconSet", column: "C",
		// Three icons partition values using iconCount-1 thresholds. The
		// greaterThanOrEqual thresholds must be supplied in descending order.
		createAttrs: `[{"icon_type":"3Arrows","value_type":"percent","operator":"greaterThanOrEqual","value":67},{"icon_type":"3Arrows","value_type":"percent","operator":"greaterThanOrEqual","value":33}]`,
		updateAttrs: `[{"icon_type":"3Circles","value_type":"percent","operator":"greaterThanOrEqual","value":75},{"icon_type":"3Circles","value_type":"percent","operator":"greaterThanOrEqual","value":25}]`,
	},
}

func TestSheets_CondFormatDefaultsBatchDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	for _, operation := range []string{"create", "update"} {
		for _, style := range []struct{ name, json string }{
			{"omitted style", ""},
			{"explicit empty style", `{}`},
			{"explicit style preserved", `{"back_color":"#123456","fore_color":"#654321","font":"bold"}`},
		} {
			t.Run(operation+"/"+style.name, func(t *testing.T) {
				var operations []map[string]any
				for _, rule := range condFormatDefaultCases {
					props := map[string]any{"attrs": json.RawMessage(rule.createAttrs)}
					if style.json != "" {
						props["style"] = json.RawMessage(style.json)
					}
					input := map[string]any{
						"sheet_id": "shDryRun", "rule_type": rule.ruleType,
						"ranges": []string{rule.column + "1:" + rule.column + "6"}, "properties": props,
					}
					if operation == "update" {
						input["rule_id"] = "rule-" + rule.ruleType
					}
					operations = append(operations, map[string]any{"shortcut": "+cond-format-" + operation, "input": input})
				}
				payload, err := json.Marshal(operations)
				require.NoError(t, err)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				t.Cleanup(cancel)
				result, err := clie2e.RunCmd(ctx, clie2e.Request{
					Args:      []string{"sheets", "+batch-update", "--spreadsheet-token", "shtDryRun", "--operations", string(payload), "--dry-run"},
					DefaultAs: "bot",
				})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				result.AssertStdoutStatus(t, true)
				require.Len(t, clie2e.DryRunGet(result.Stdout, "api").Array(), 1)
				require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
				require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
				require.Equal(t, "batch_update", clie2e.DryRunGet(result.Stdout, "api.0.body.tool_name").String())
				batch := gjson.Parse(clie2e.DryRunGet(result.Stdout, "api.0.body.input").String())
				require.Equal(t, "shtDryRun", batch.Get("excel_id").String())
				actual := batch.Get("operations").Array()
				require.Len(t, actual, len(condFormatDefaultCases))
				for i, rule := range condFormatDefaultCases {
					require.Equal(t, "manage_conditional_format_object", actual[i].Get("tool_name").String())
					input := actual[i].Get("input")
					require.Equal(t, "shtDryRun", input.Get("excel_id").String())
					require.Equal(t, "shDryRun", input.Get("sheet_id").String())
					require.Equal(t, operation, input.Get("operation").String())
					require.Equal(t, rule.ruleType, input.Get("properties.rule_type").String())
					require.Equal(t, []any{rule.column + "1:" + rule.column + "6"}, input.Get("properties.ranges").Value())
					require.JSONEq(t, rule.createAttrs, input.Get("properties.attrs").Raw)
					wantStyle := style.json
					if wantStyle == "" {
						wantStyle = `{}`
					}
					require.True(t, input.Get("properties.style").IsObject())
					require.JSONEq(t, wantStyle, input.Get("properties.style").Raw)
					if operation == "update" {
						require.Equal(t, "rule-"+rule.ruleType, input.Get("conditional_format_id").String())
					} else {
						require.False(t, input.Get("conditional_format_id").Exists())
					}
				}
			})
		}
	}
}

func TestSheets_CondFormatDefaultsBatchValidationDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	const cellAttrs = `[{"compare_type":"greaterThan","value":"0"}]`
	for _, operation := range []string{"create", "update"} {
		for _, rule := range condFormatDefaultCases {
			cases := []struct {
				name, ruleType, properties, wantError string
			}{
				{"explicit null style", rule.ruleType, `{"style":null,"attrs":` + rule.createAttrs + `}`, "style"},
				{"wrong attrs for effective type", rule.ruleType, `{"attrs":` + cellAttrs + `}`, "attrs[0]"},
				{"flat visual type overrides nested cellIs", rule.ruleType, `{"rule_type":"cellIs","attrs":` + rule.createAttrs + `}`, ""},
				{"flat cellIs still requires style", "cellIs", `{"rule_type":"` + rule.ruleType + `","attrs":` + cellAttrs + `}`, `required property "style" is missing`},
				{"flat cellIs preserves explicit style", "cellIs", `{"rule_type":"` + rule.ruleType + `","style":{"back_color":"#123456"},"attrs":` + cellAttrs + `}`, ""},
			}
			if rule.ruleType == condFormatDefaultCases[0].ruleType {
				cases = append(cases, struct{ name, ruleType, properties, wantError string }{
					"ordinary cellIs missing style", "cellIs", `{"attrs":` + cellAttrs + `}`, `required property "style" is missing`,
				})
			}
			for _, tc := range cases {
				t.Run(operation+"/"+rule.ruleType+"/"+tc.name, func(t *testing.T) {
					input := map[string]any{
						"sheet_id": "shDryRun", "rule_type": tc.ruleType,
						"ranges": []string{"A1:A6"}, "properties": json.RawMessage(tc.properties),
					}
					if operation == "update" {
						input["rule_id"] = "ruleDryRun"
					}
					payload, err := json.Marshal([]any{map[string]any{"shortcut": "+cond-format-" + operation, "input": input}})
					require.NoError(t, err)
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					t.Cleanup(cancel)
					result, err := clie2e.RunCmd(ctx, clie2e.Request{
						Args:      []string{"sheets", "+batch-update", "--spreadsheet-token", "shtDryRun", "--operations", string(payload), "--dry-run"},
						DefaultAs: "bot",
					})
					require.NoError(t, err)
					if tc.wantError != "" {
						result.AssertExitCode(t, 2)
						require.Empty(t, result.Stdout)
						require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
						require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
						require.Equal(t, "--operations", gjson.Get(result.Stderr, "error.param").String())
						require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), tc.wantError)
						return
					}
					result.AssertExitCode(t, 0)
					result.AssertStdoutStatus(t, true)
					batch := gjson.Parse(clie2e.DryRunGet(result.Stdout, "api.0.body.input").String())
					props := batch.Get("operations.0.input.properties")
					require.Equal(t, tc.ruleType, props.Get("rule_type").String())
					require.JSONEq(t, gjson.Get(tc.properties, "attrs").Raw, props.Get("attrs").Raw)
					wantStyle := `{}`
					if tc.ruleType == "cellIs" {
						wantStyle = `{"back_color":"#123456"}`
					}
					require.JSONEq(t, wantStyle, props.Get("style").Raw)
				})
			}
		}
	}
}
