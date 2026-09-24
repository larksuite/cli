// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

const workflowAgentFixture = `{"title":"Agent test","client_token":"agent-test","steps":[{"id":"trigger","type":"AddRecordTrigger","next":"agent","data":{"table_name":"Source","watched_field_name":"Text"}},{"id":"agent","type":"AIAgentAction","next":"after","children":{"links":[{"kind":"slot","label":"llm","to":""},{"kind":"slot","label":"memory","to":""},{"kind":"slot","label":"tool","to":"tool"}]},"data":{"prompt":[{"value_type":"ref","value":"$.trigger.field_id"}],"system_message":[{"value_type":"text","value":"Keep keys"}],"model":{"name":"doubao_chat_model","temperature":0,"max_token":1024,"timeout":60000},"max_iterations":5,"output_format_type":"json","json_output":"{\"Case_Key\":\"text\"}","enable_streaming_reply":false,"future_setting":true}},{"id":"tool","type":"AIAgentMCPAction","meta":{"component_id":"search_mcp"},"data":{"tool_list":["web_search"],"identity_type":"maker","env":{"API_KEY_NAME":"synthetic-not-a-secret"}}},{"id":"after","type":"AddRecordAction","data":{"table_name":"Results","field_values":[]}}]}`

func TestWorkflowAIAgentCreateUpdatePreservesPayload(t *testing.T) {
	for _, update := range []bool{false, true} {
		for _, dry := range []bool{false, true} {
			t.Run(fmtWorkflowAgentCase(update, dry), func(t *testing.T) {
				factory, stdout, reg := newExecuteFactory(t)
				shortcut, method, url := BaseWorkflowCreate, "POST", "/open-apis/base/v3/bases/app_x/workflows"
				args := []string{"+workflow-create", "--base-token", "app_x"}
				if update {
					shortcut = BaseWorkflowUpdate
					method = "PUT"
					url += "/wkf_1"
					args = []string{"+workflow-update", "--base-token", "app_x", "--workflow-id", "wkf_1"}
				}
				args = append(args, "--json", workflowAgentFixture)
				if dry {
					args = append(args, "--dry-run", "--format", "pretty")
				}
				var stub *httpmock.Stub
				if !dry {
					stub = &httpmock.Stub{Method: method, URL: url, Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"workflow_id": "wkf_1"}}}
					reg.Register(stub)
				}
				if err := runShortcut(t, shortcut, args, factory, stdout); err != nil {
					t.Fatal(err)
				}
				if dry {
					if !strings.Contains(stdout.String(), "AIAgentAction") || !strings.Contains(stdout.String(), "search_mcp") {
						t.Fatal(stdout.String())
					}
					return
				}
				var got, want interface{}
				if err := json.Unmarshal(stub.CapturedBody, &got); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal([]byte(workflowAgentFixture), &want); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("wire payload changed: %s", stub.CapturedBody)
				}
			})
		}
	}
}
func fmtWorkflowAgentCase(update, dry bool) string {
	name := "create"
	if update {
		name = "update"
	}
	if dry {
		name += "/dry-run"
	}
	return name
}

func TestWorkflowAIAgentRejectsIgnoredAndMalformedSettings(t *testing.T) {
	cases := []struct{ name, old, replacement string }{
		{"ignored thinking mode", `"temperature":0`, `"temperature":0,"thinking_mode":"deep"`},
		{"ineffective memory", `"max_iterations":5`, `"max_iterations":5,"memory":{"history_rounds":3}`},
		{"internal model link", `"label":"llm","to":""`, `"label":"llm","to":"tool"`},
		{"missing tool", `"label":"tool","to":"tool"`, `"label":"tool","to":"missing"`},
		{"tool in linear chain", `"id":"tool","type"`, `"id":"tool","next":"after","type"`},
		{"orphan MCP", `"label":"tool","to":"tool"`, `"label":"tool","to":""`},
		{"custom MCP", `"type":"AIAgentMCPAction"`, `"type":"AIAgentCustomMCPAction"`},
		{"unknown package", `"component_id":"search_mcp"`, `"component_id":"not_supported"`},
		{"tool list shape", `"tool_list":["web_search"]`, `"tool_list":"web_search"`},
		{"incompatible identity", `"identity_type":"maker"`, `"identity_type":"triggerPersonal"`},
		{"non-string env", `"API_KEY_NAME":"synthetic-not-a-secret"`, `"API_KEY_NAME":42`},
		{"bad prompt", `"prompt":[{"value_type":"ref","value":"$.trigger.field_id"}]`, `"prompt":"hello"`},
		{"bad reference", `"value":"$.trigger.field_id"`, `"value":"trigger.field_id"`},
		{"zero iterations", `"max_iterations":5`, `"max_iterations":0`},
		{"fractional iteration", `"max_iterations":5`, `"max_iterations":1.5`},
		{"bad model", `"name":"doubao_chat_model"`, `"name":""`},
		{"bad output format", `"output_format_type":"json"`, `"output_format_type":"invalid"`},
		{"bad streaming flag", `"enable_streaming_reply":false`, `"enable_streaming_reply":"false"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Replace(workflowAgentFixture, tc.old, tc.replacement, 1)
			if body == workflowAgentFixture {
				t.Fatal("fixture replacement did not match")
			}
			factory, stdout, _ := newExecuteFactory(t)
			err := runShortcut(t, BaseWorkflowCreate, []string{"+workflow-create", "--base-token", "app_x", "--json", body, "--dry-run"}, factory, stdout)
			if err == nil {
				t.Fatal("invalid config was accepted")
			}
		})
	}
}

func TestWorkflowAIAgentDelayOnlyAllowedInMainChain(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantErr    bool
	}{
		{
			name:    "Delay attached as Agent tool",
			body:    `{"steps":[{"id":"agent","type":"AIAgentAction","children":{"links":[{"kind":"slot","label":"tool","to":"wait"}]}},{"id":"wait","type":"Delay"}]}`,
			wantErr: true,
		},
		{
			name: "Delay on main chain",
			body: `{"steps":[{"id":"trigger","type":"AddRecordTrigger","next":"wait"},{"id":"wait","type":"Delay","next":"agent"},{"id":"agent","type":"AIAgentAction"}]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			err := runShortcut(t, BaseWorkflowCreate, []string{"+workflow-create", "--base-token", "app_x", "--json", tc.body, "--dry-run"}, factory, stdout)
			if tc.wantErr {
				var validationErr *errs.ValidationError
				if !errors.As(err, &validationErr) || !strings.Contains(err.Error(), "Delay is not a supported AI Agent tool type") {
					t.Fatalf("expected Delay tool validation error, got %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestWorkflowAIAgentAllowsServerOwnedDefaultsAndDynamicCatalog(t *testing.T) {
	cases := []string{
		strings.Replace(workflowAgentFixture, `"tool_list":["web_search"]`, `"tool_list":[]`, 1),
		strings.Replace(workflowAgentFixture, `"tool_list":["web_search"]`, `"tool_list":["new_server_tool"]`, 1),
		strings.Replace(strings.Replace(workflowAgentFixture, `"type":"AddRecordTrigger"`, `"type":"LarkMessageTrigger"`, 1), `"max_iterations":5`, `"max_iterations":5,"memory":{"history_rounds":3}`, 1),
		strings.Replace(workflowAgentFixture, `"tool_list":["web_search"],`, ``, 1),
	}
	for _, body := range cases {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(body), &parsed); err != nil {
			t.Fatal(err)
		}
		if err := validateWorkflowBodyForCLI(parsed); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkflowAIAgentToolInfoPreservesWireSemantics(t *testing.T) {
	for _, toolType := range []string{"LarkEmailAction", "APIHubAction", "AIAgentMCPAction"} {
		for _, description := range []string{"omitted", "null", `{"description":""}`, `{"description":"Only send after approval: Case_Key"}`} {
			for _, update := range []bool{false, true} {
				t.Run(toolType+"/"+description+"/"+fmtWorkflowAgentCase(update, false), func(t *testing.T) {
					var body map[string]interface{}
					if err := json.Unmarshal([]byte(workflowAgentFixture), &body); err != nil {
						t.Fatal(err)
					}
					tool := body["steps"].([]interface{})[2].(map[string]interface{})
					tool["type"] = toolType
					data := tool["data"].(map[string]interface{})
					switch toolType {
					case "LarkEmailAction":
						delete(tool, "meta")
						data = map[string]interface{}{
							"to":          []interface{}{map[string]interface{}{"value_type": "text", "value": "test@example.com"}},
							"subject":     []interface{}{map[string]interface{}{"value_type": "text", "value": "Test"}},
							"content":     []interface{}{map[string]interface{}{"value_type": "ref", "value": "$.trigger.field_id"}},
							"show_source": false,
						}
					case "APIHubAction":
						tool["meta"] = map[string]interface{}{"component_id": "Automation_Create_Task"}
						data = map[string]interface{}{"params": []interface{}{map[string]interface{}{"key": "rich_summary", "value": []interface{}{map[string]interface{}{"value_type": "text", "value": "Test"}}}}}
					}
					tool["data"] = data
					if description != "omitted" {
						var info interface{}
						if err := json.Unmarshal([]byte(description), &info); err != nil {
							t.Fatal(err)
						}
						data["tool_info"] = info
					}
					payload, err := json.Marshal(body)
					if err != nil {
						t.Fatal(err)
					}
					factory, stdout, reg := newExecuteFactory(t)
					shortcut, method, url := BaseWorkflowCreate, "POST", "/open-apis/base/v3/bases/app_x/workflows"
					args := []string{"+workflow-create", "--base-token", "app_x"}
					if update {
						shortcut, method = BaseWorkflowUpdate, "PUT"
						url += "/wkf_1"
						args = []string{"+workflow-update", "--base-token", "app_x", "--workflow-id", "wkf_1"}
					}
					args = append(args, "--json", string(payload))
					// Dry-run and HTTP execution must accept the same shape.
					if err := runShortcut(t, shortcut, append(append([]string{}, args...), "--dry-run"), factory, stdout); err != nil {
						t.Fatal(err)
					}
					stub := &httpmock.Stub{Method: method, URL: url, Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"workflow_id": "wkf_1"}}}
					reg.Register(stub)
					if err := runShortcut(t, shortcut, args, factory, stdout); err != nil {
						t.Fatal(err)
					}
					var got interface{}
					if err := json.Unmarshal(stub.CapturedBody, &got); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, body) {
						t.Fatalf("tool metadata or business parameters changed on wire: %s", stub.CapturedBody)
					}
				})
			}
		}
	}
}

func TestWorkflowAIAgentToolInfoRejectsMalformedDescriptions(t *testing.T) {
	for _, value := range []string{`[]`, `"description"`, `true`, `1`, `{}`, `{"description":null}`, `{"description":3}`} {
		for _, update := range []bool{false, true} {
			t.Run(value+"/"+fmtWorkflowAgentCase(update, true), func(t *testing.T) {
				body := strings.Replace(workflowAgentFixture, `"tool_list":["web_search"]`, `"tool_list":["web_search"],"tool_info":`+value, 1)
				factory, stdout, _ := newExecuteFactory(t)
				shortcut := BaseWorkflowCreate
				args := []string{"+workflow-create", "--base-token", "app_x"}
				if update {
					shortcut = BaseWorkflowUpdate
					args = []string{"+workflow-update", "--base-token", "app_x", "--workflow-id", "wkf_1"}
				}
				err := runShortcut(t, shortcut, append(args, "--json", body, "--dry-run"), factory, stdout)
				var validationErr *errs.ValidationError
				if !errors.As(err, &validationErr) || !strings.Contains(err.Error(), "data.tool_info") {
					t.Fatalf("expected typed tool_info validation error, got %v", err)
				}
			})
		}
	}
}
