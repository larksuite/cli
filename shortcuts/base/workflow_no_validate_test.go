// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"testing"
)

func TestWorkflowNoValidate(t *testing.T) {
	for _, update := range []bool{false, true} {
		for _, dry := range []bool{false, true} {
			for _, tc := range []struct{ name, body string }{
				{"historical APIHub component", apiHubFixture("future_component", `[]`)},
				{"step array shape", `{"steps":"server-owned-shape"}`},
				{"step object shape", `{"steps":[null]}`},
				{"recursive alias type", `{"future":{"value_type":1}}`},
				{"recursive alias conflict", `{"future":{"value_type":"text","valueType":"ref"}}`},
			} {
				t.Run(tc.name+"/"+fmtWorkflowAgentCase(update, dry), func(t *testing.T) {
					assertWorkflowJSONRejected(t, tc.body, update, dry, false)
					assertWorkflowBodyForwarded(t, tc.body, update, dry, true)
				})
			}
			for _, tc := range []struct{ name, body string }{
				{"malformed JSON", `{"steps":`},
				{"array top level", `[]`},
				{"null top level", `null`},
				{"scalar top level", `"workflow"`},
			} {
				t.Run(tc.name+"/"+fmtWorkflowAgentCase(update, dry), func(t *testing.T) {
					assertWorkflowJSONRejected(t, tc.body, update, dry, false)
					assertWorkflowJSONRejected(t, tc.body, update, dry, true)
				})
			}
		}
	}
}

func TestWorkflowValueTypeAliases(t *testing.T) {
	for _, tc := range []struct {
		name, fields              string
		wantAliasErr, wantNodeErr bool
	}{
		{"snake", `"value_type":"text"`, false, false},
		{"camel", `"valueType":"text"`, false, false},
		{"equal", `"value_type":"text","valueType":"text"`, false, false},
		{"empty snake alone", `"value_type":""`, false, true},
		{"empty camel alone", `"valueType":""`, false, true},
		{"equal empty", `"value_type":"","valueType":""`, false, true},
		{"unknown value", `"value_type":"future-value"`, false, true},
		{"conflict", `"value_type":"text","valueType":"ref"`, true, true},
		{"empty snake conflict", `"value_type":"","valueType":"text"`, true, true},
		{"empty camel conflict", `"value_type":"text","valueType":""`, true, true},
		{"wrong snake type", `"value_type":12,"valueType":"text"`, true, true},
		{"wrong camel type", `"valueType":false`, true, true},
		{"null snake", `"value_type":null`, true, true},
		{"null camel", `"valueType":null`, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for bodyIndex, body := range []string{
				apiHubFixture("Automation_Create_Task", `[{"key":"rich_summary","value":[{`+tc.fields+`,"value":"hello"}]}]`),
				`{"steps":[{"id":"agent","type":"AIAgentAction","data":{"prompt":[{` + tc.fields + `,"value":"hello"}]}}]}`,
				`{"future":{"nested":[{"values":[{` + tc.fields + `,"value":"hello"}]}]}}`,
			} {
				var parsed map[string]interface{}
				if err := json.Unmarshal([]byte(body), &parsed); err != nil {
					t.Fatal(err)
				}
				err := validateWorkflowBodyForCLI("+workflow-create", parsed)
				wantErr := tc.wantAliasErr
				if bodyIndex < 2 {
					wantErr = tc.wantNodeErr
				}
				if (err != nil) != wantErr {
					t.Errorf("body=%s error=%v, wantErr=%v", body, err, wantErr)
				}
			}
		})
	}
}
