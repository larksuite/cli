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

func TestWorkflowJSONValidation(t *testing.T) {
	t.Run("server owned configuration", func(t *testing.T) {
		cases := []struct{ name, body string }{
			{
				name: "ordinary message with supported header color and omitted body mode",
				body: `{"steps":[{"id":"m","type":"LarkMessageAction","data":{"title_template_color":"red","content":[{"value_type":"text","value":"Body"}]}}]}`,
			},
			{
				name: "rich text styles and heading levels",
				body: `{"steps":[{"id":"m","type":"LarkMessageAction","data":{"content_type":"rich_text","rich_text_content":[{"type":"paragraph","line_marker_list":[{"type":"heading","level":3}],"value":[{"value_type":"text","value":"Title","style":{"color":"gray"}}]}]}}]}`,
			},
			{
				name: "model and legal mode combination belongs to server",
				body: strings.ReplaceAll(strings.ReplaceAll(workflowAgentFixture, `"thinking_mode":"deep"`, `"thinking_mode":"auto"`), "doubao_chat_model", "deepseek_chat_model"),
			},
			{
				name: "MCP env keys are independent tool configuration",
				body: strings.Replace(workflowAgentFixture,
					`"env":{"API_KEY_NAME":"synthetic-not-a-secret"}`,
					`"env":{"API_KEY_NAME":"synthetic-not-a-secret","value_type":"setting-a","valueType":"setting-b","value":"tool-defined-value"}`, 1),
			},
			{
				name: "unknown button extension preserved",
				body: `{"steps":[{"id":"m","type":"LarkMessageAction","data":{"btn_list":[{"text":"Open","btn_action":"openLink","style":"default","link":[{"value_type":"text","value":"https://example.com"}],"only_first_click":true,"not_repeat_click":false}],"button_config":{"future_option":true}}}]}`,
			},
			{
				name: "screenshot names and future extension preserved",
				body: `{"steps":[{"id":"s","type":"AppScreenshotAction","data":{"app_name":"应用","page_name":"页面","future_option":true}}]}`,
			},
			{
				name: "webhook rules belong to server",
				body: `{"steps":[{"id":"w","type":"WebhookTrigger","data":{"reset_token":true,"ip_list":["server-validates-addresses"],"future_option":true}}]}`,
			},
			{
				name: "optional fields and explicit null containers",
				body: `{"steps":[{}, {"id":"","type":"","next":null,"data":null,"meta":null,"children":null}]}`,
			},
			{
				name: "aliases retain spelling and unknown values",
				body: `{"future":{"items":[{"value_type":"future-value"},{"valueType":"future-value"},{"value_type":"","valueType":""},{"value_type":"future-value","valueType":"future-value"}]}}`,
			},
		}
		for _, tc := range cases {
			for _, update := range []bool{false, true} {
				for _, dry := range []bool{false, true} {
					t.Run(tc.name+"/"+fmtWorkflowAgentCase(update, dry), func(t *testing.T) {
						assertWorkflowBodyForwarded(t, tc.body, update, dry, false)
					})
				}
			}
		}
	})
	t.Run("reference array order", func(t *testing.T) {
		// The links define t -> f -> m, even though m precedes f in the JSON array.
		// CLI transport must leave graph validation to the server and preserve both.
		for _, content := range []struct{ name, fields string }{
			{"plain", `"content":[{"value_type":"ref","value":"$.f.fieldRecords","extra_info":{"field_name_list":["Status"]}}]`},
			{"rich", `"content_type":"rich_text","rich_text_content":[{"type":"paragraph","value":[{"value_type":"ref","value":"$.f.fieldRecords","extra_info":{"field_name_list":["Status"]}}]}]`},
		} {
			body := `{"steps":[{"id":"t","type":"ButtonTrigger","next":"f","data":{"table_name":"Orders","button_type":"buttonField"}},{"id":"m","type":"LarkMessageAction","data":{` + content.fields + `}},{"id":"f","type":"FindRecordAction","next":"m","data":{"table_name":"Orders"}}]}`
			for _, update := range []bool{false, true} {
				for _, dry := range []bool{false, true} {
					t.Run(content.name+"/"+fmtWorkflowAgentCase(update, dry), func(t *testing.T) {
						assertWorkflowBodyForwarded(t, body, update, dry, false)
					})
				}
			}
		}
	})
	t.Run("invalid JSON structure", func(t *testing.T) {
		cases := []struct{ name, body string }{
			{"steps object", `{"steps":{}}`},
			{"steps null", `{"steps":null}`},
			{"steps string", `{"steps":"steps"}`},
			{"null step", `{"steps":[null]}`},
			{"scalar step", `{"steps":[1]}`},
			{"array step", `{"steps":[[]]}`},
			{"numeric id", `{"steps":[{"id":1}]}`},
			{"null id", `{"steps":[{"id":null}]}`},
			{"numeric type", `{"steps":[{"type":1}]}`},
			{"null type", `{"steps":[{"type":null}]}`},
			{"array next", `{"steps":[{"next":[]}]}`},
			{"numeric next", `{"steps":[{"next":1}]}`},
			{"array data", `{"steps":[{"data":[]}]}`},
			{"scalar data", `{"steps":[{"data":false}]}`},
			{"array meta", `{"steps":[{"meta":[]}]}`},
			{"scalar meta", `{"steps":[{"meta":1}]}`},
			{"array children", `{"steps":[{"children":[]}]}`},
			{"scalar children", `{"steps":[{"children":"child"}]}`},
			{"recursive snake alias type", `{"future":{"items":[{"value_type":false}]}}`},
			{"recursive camel alias type", `{"future":{"items":[{"valueType":[]}]}}`},
			{"recursive null alias", `{"future":{"items":[{"value_type":null}]}}`},
			{"recursive alias conflict", `{"future":{"items":[{"value_type":"text","valueType":"ref"}]}}`},
		}
		for _, tc := range cases {
			for _, update := range []bool{false, true} {
				for _, dry := range []bool{false, true} {
					t.Run(tc.name+"/"+fmtWorkflowAgentCase(update, dry), func(t *testing.T) {
						assertWorkflowJSONRejected(t, tc.body, update, dry, false)
					})
				}
			}
		}
	})
}

// A mock success below proves only that the CLI submitted the unchanged JSON;
// server acceptance of business settings is outside these transport tests.
func assertWorkflowBodyForwarded(t *testing.T, body string, update, dry, noValidate bool) {
	t.Helper()
	factory, stdout, registry := newExecuteFactory(t)
	shortcut, method, path := BaseWorkflowCreate, "POST", "/open-apis/base/v3/bases/app_x/workflows"
	args := []string{"+workflow-create", "--base-token", "app_x", "--json", body}
	if update {
		shortcut, method = BaseWorkflowUpdate, "PUT"
		path += "/wkf_1"
		args[0] = "+workflow-update"
		args = append(args, "--workflow-id", "wkf_1")
	}
	if noValidate {
		args = append(args, "--no-validate")
	}
	stub := &httpmock.Stub{Method: method, URL: path, Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"workflow_id": "wkf_1"}}}
	if dry {
		args = append(args, "--dry-run")
	} else {
		registry.Register(stub)
	}
	if err := runShortcut(t, shortcut, args, factory, stdout); err != nil {
		t.Fatal(err)
	}
	var got interface{}
	if dry {
		var preview struct {
			Data struct {
				API []struct {
					Method string      `json:"method"`
					URL    string      `json:"url"`
					Body   interface{} `json:"body"`
				} `json:"api"`
			} `json:"data"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
			t.Fatal(err)
		}
		if len(preview.Data.API) != 1 || preview.Data.API[0].Method != method || preview.Data.API[0].URL != path {
			t.Fatalf("unexpected requests: %s", stdout.String())
		}
		got = preview.Data.API[0].Body
	} else {
		registry.Verify(t)
		if err := json.Unmarshal(stub.CapturedBody, &got); err != nil {
			t.Fatal(err)
		}
	}
	if want := mustWorkflowJSON(t, body); !reflect.DeepEqual(got, want) {
		t.Fatalf("submitted JSON changed: got=%#v want=%#v", got, want)
	}
}

func assertWorkflowJSONRejected(t *testing.T, body string, update, dry, noValidate bool) {
	t.Helper()
	factory, stdout, _ := newExecuteFactory(t)
	shortcut := BaseWorkflowCreate
	args := []string{"+workflow-create", "--base-token", "app_x", "--json", body}
	if update {
		shortcut = BaseWorkflowUpdate
		args[0] = "+workflow-update"
		args = append(args, "--workflow-id", "wkf_1")
	}
	if dry {
		args = append(args, "--dry-run")
	}
	if noValidate {
		args = append(args, "--no-validate")
	}
	err := runShortcut(t, shortcut, args, factory, stdout)
	var validation *errs.ValidationError
	if !errors.As(err, &validation) || validation.Param != "--json" {
		t.Fatalf("expected local typed JSON validation error, got %T %v", err, err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("invalid JSON emitted output: %s", stdout.String())
	}
}
