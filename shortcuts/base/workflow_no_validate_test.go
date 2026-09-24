// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestWorkflowNoValidate(t *testing.T) {
	body := apiHubFixture("future_component", `[]`)
	for _, update := range []bool{false, true} {
		for _, dry := range []bool{false, true} {
			for _, tc := range []struct {
				name, body    string
				skip, wantErr bool
			}{
				{"default rejects unknown", body, false, true},
				{"skip accepts unknown", body, true, false},
				{"skip rejects malformed JSON", `{"steps":`, true, true},
				{"skip rejects non-object", `[]`, true, true},
			} {
				command, shortcut, method, url := "+workflow-create", BaseWorkflowCreate, "POST", "/open-apis/base/v3/bases/app_x/workflows"
				if update {
					command, shortcut, method, url = "+workflow-update", BaseWorkflowUpdate, "PUT", url+"/wkf_1"
				}
				mode := "/execute/"
				if dry {
					mode = "/dry-run/"
				}
				t.Run(command+mode+tc.name, func(t *testing.T) {
					factory, stdout, reg := newExecuteFactory(t)
					args := []string{command, "--base-token", "app_x", "--json", tc.body}
					if update {
						args = append(args, "--workflow-id", "wkf_1")
					}
					if tc.skip {
						args = append(args, "--no-validate")
					}
					if dry {
						args = append(args, "--dry-run", "--format", "pretty")
					}
					stub := &httpmock.Stub{Method: method, URL: url, Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"workflow_id": "wkf_1"}}}
					if !dry && !tc.wantErr {
						reg.Register(stub)
					}
					err := runShortcut(t, shortcut, args, factory, stdout)
					if tc.wantErr {
						if err == nil {
							t.Fatal("expected validation error")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if dry {
						if !strings.Contains(stdout.String(), method+" "+url) || !strings.Contains(stdout.String(), "future_component") {
							t.Fatalf("unexpected preview: %s", stdout.String())
						}
						return
					}
					var got, want interface{}
					if err := json.Unmarshal(stub.CapturedBody, &got); err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal([]byte(tc.body), &want); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("payload changed: %s", stub.CapturedBody)
					}
				})
			}
		}
	}
}

func TestWorkflowValueTypeAliases(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		wantErr      bool
	}{
		{"snake", `"value_type":"text"`, false},
		{"camel", `"valueType":"text"`, false},
		{"equal", `"value_type":"text","valueType":"text"`, false},
		{"conflict", `"value_type":"text","valueType":"ref"`, true},
		{"empty snake", `"value_type":"","valueType":"text"`, true},
		{"empty camel", `"value_type":"text","valueType":""`, true},
		{"wrong type", `"value_type":12,"valueType":"text"`, true},
		{"null", `"value_type":null,"valueType":"text"`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, kind := range []string{"api", "agent"} {
				body := apiHubFixture("Automation_Create_Task", `[{"key":"rich_summary","value":[{`+tc.fields+`,"value":"hello"}]}]`)
				if kind == "agent" {
					body = `{"steps":[{"id":"agent","type":"AIAgentAction","data":{"prompt":[{` + tc.fields + `,"value":"hello"}]}}]}`
				}
				var parsed map[string]interface{}
				if err := json.Unmarshal([]byte(body), &parsed); err != nil {
					t.Fatal(err)
				}
				err := validateWorkflowBodyForCLI(parsed)
				if (err != nil) != tc.wantErr {
					t.Errorf("%s: error=%v, wantErr=%v", kind, err, tc.wantErr)
				}
			}
		})
	}
}
