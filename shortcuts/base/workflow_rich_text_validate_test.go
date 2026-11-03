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

const workflowRichTextFixture = `{
  "title":"Rich text",
  "steps":[{
    "id":"notify",
    "type":"LarkMessageAction",
    "data":{
      "receiver":[],
      "send_to_everyone":false,
      "content_type":"rich_text",
      "rich_text_content":[
        {"type":"paragraph","value":[
          {"value_type":"text","value":"状态：","style":{"bold":true,"color":"blue"}},
          {"value_type":"ref","value":"$.find_1.fieldRecords","extra_info":{"display_type":"list","field_name_list":["状态"]}},
          {"value_type":"link","value":{"text":[{"value_type":"text","value":"详情"}],"link":[{"value_type":"ref","value":"$.find_1.firstfieldsRecord.detail"}]}}
        ]},
        {"type":"horizontalLine"},
        {"type":"paragraph","line_marker_list":[{"type":"orderedList","level":1,"order_id":"list_1","order":1}],"value":[]}
      ],
      "btn_list":[]
    }
  }]
}`

func TestWorkflowRichTextValidationAcceptsPublicShape(t *testing.T) {
	body := mustWorkflowJSON(t, workflowRichTextFixture)
	if err := validateWorkflowBodyForCLI("+workflow-create", body); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowRichTextLinkChildStylesRejectedAcrossCommands(t *testing.T) {
	body := strings.Replace(workflowRichTextFixture, `"text":[{"value_type":"text","value":"详情"}]`, `"text":[{"value_type":"text","value":"查看","style":{"italic":true}},{"value_type":"text","value":"详情","style":{"bold":true}}]`, 1)
	for _, update := range []bool{false, true} {
		for _, dry := range []bool{false, true} {
			t.Run(fmtWorkflowAgentCase(update, dry), func(t *testing.T) {
				assertWorkflowJSONRejected(t, body, update, dry, false)
				assertWorkflowBodyForwarded(t, body, update, dry, true)
			})
		}
	}
}

// GET annotates both record and scalar references, including references inside
// link labels and targets. Reusing that definition must preserve the annotations.
const workflowRichTextGETFixture = `{
  "title":"Rich text GET",
  "steps":[{"id":"notify","type":"LarkMessageAction","data":{
    "receiver":[],"send_to_everyone":false,"btn_list":[],
    "content_type":"rich_text","rich_text_content":[{
      "type":"paragraph","line_marker_list":[],"value":[
        {"value_type":"ref","value":"$.find_1.firstfieldsRecord.fld_status","extra_info":{"table_name":"Source"},"style":{"bold":false,"color":"gray"}},
        {"value_type":"ref","value":"$.find_1.fieldRecords","extra_info":{"table_name":"Source","display_type":"table","field_name_list":["Status","Title"]}},
        {"value_type":"link","style":{"italic":true,"bold":false},"value":{
          "text":[{"value_type":"ref","value":"$.find_1.firstfieldsRecord.fld_title","extra_info":{"table_name":"Source"}}, {"value_type":"text","value":" details"}],
          "link":[{"value_type":"ref","value":"$.find_1.firstfieldsRecord.fld_url","extra_info":{"table_name":"Source"}}]
        }}
      ]
    }]
  }}]
}`

func TestWorkflowRichTextGETReferencesPreservedAcrossCommands(t *testing.T) {
	for _, action := range []string{"LarkMessageAction", "LarkEmailAction"} {
		body := strings.Replace(workflowRichTextGETFixture, "LarkMessageAction", action, 1)
		if action == "LarkEmailAction" {
			body = strings.Replace(body, `"receiver":[],"send_to_everyone":false,"btn_list":[],`, `"to":[],"cc":[],"bcc":[],"subject":[],"attachments":[],"show_source":false,`, 1)
		}
		for _, update := range []bool{false, true} {
			for _, dry := range []bool{false, true} {
				t.Run(action+"/"+fmtWorkflowAgentCase(update, dry), func(t *testing.T) {
					factory, stdout, registry := newExecuteFactory(t)
					shortcut, method, url := BaseWorkflowCreate, "POST", "/open-apis/base/v3/bases/app_x/workflows"
					args := []string{"+workflow-create", "--base-token", "app_x", "--json", body}
					if update {
						shortcut, method = BaseWorkflowUpdate, "PUT"
						url += "/wkf_1"
						args[0] = "+workflow-update"
						args = append(args, "--workflow-id", "wkf_1")
					}
					stub := &httpmock.Stub{Method: method, URL: url, Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"workflow_id": "wkf_1"}}}
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
						var result struct {
							DryRun bool `json:"dry_run"`
							Data   struct {
								API []struct {
									Method string      `json:"method"`
									URL    string      `json:"url"`
									Body   interface{} `json:"body"`
								} `json:"api"`
							} `json:"data"`
						}
						if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
							t.Fatal(err)
						}
						if !result.DryRun || len(result.Data.API) != 1 || result.Data.API[0].Method != method || result.Data.API[0].URL != url {
							t.Fatalf("unexpected dry-run request: %s", stdout.String())
						}
						got = result.Data.API[0].Body
					} else {
						registry.Verify(t)
						if err := json.Unmarshal(stub.CapturedBody, &got); err != nil {
							t.Fatal(err)
						}
					}
					if !reflect.DeepEqual(got, mustWorkflowJSON(t, body)) {
						t.Fatalf("GET definition changed on submission: %#v", got)
					}
				})
			}
		}
	}
}

func mustWorkflowJSON(t *testing.T, body string) map[string]interface{} {
	t.Helper()
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(body), &parsed)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestWorkflowViewLinkGETPreservedOnUpdate(t *testing.T) {
	for _, action := range []string{"LarkMessageAction", "LarkEmailAction"} {
		for _, location := range []string{"plain", "rich", "link"} {
			t.Run(action+"/"+location, func(t *testing.T) {
				viewRef := map[string]interface{}{
					"value_type": "ref", "value": "$.trigger.recordLink.vew_orders",
					"extra_info": map[string]interface{}{"table_name": "Orders", "view_name": "Pending orders"},
				}
				data := map[string]interface{}{"receiver": []interface{}{}, "send_to_everyone": false, "btn_list": []interface{}{}}
				if action == "LarkEmailAction" {
					data = map[string]interface{}{"to": []interface{}{}, "cc": []interface{}{}, "bcc": []interface{}{}, "subject": []interface{}{}, "attachments": []interface{}{}, "show_source": false}
				}
				if location == "plain" {
					data["content_type"], data["content"] = "plain_text", []interface{}{viewRef}
				} else {
					var item interface{} = viewRef
					if location == "link" {
						item = map[string]interface{}{"value_type": "link", "value": map[string]interface{}{
							"text": []interface{}{map[string]interface{}{"value_type": "text", "value": "Open orders"}},
							"link": []interface{}{viewRef},
						}}
					}
					data["content_type"] = "rich_text"
					data["rich_text_content"] = []interface{}{map[string]interface{}{"type": "paragraph", "value": []interface{}{item}}}
				}
				definition := map[string]interface{}{
					"title": "Before edit",
					"steps": []interface{}{
						map[string]interface{}{"id": "trigger", "type": "ButtonTrigger", "next": "notify", "data": map[string]interface{}{"table_name": "Orders", "button_type": "buttonField"}},
						map[string]interface{}{"id": "notify", "type": action, "data": data},
					},
				}
				getFactory, getOut, getRegistry := newExecuteFactory(t)
				getRegistry.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/base/v3/bases/app_x/workflows/wkf_1", Body: map[string]interface{}{"code": 0, "data": definition}})
				if err := runShortcut(t, BaseWorkflowGet, []string{"+workflow-get", "--base-token", "app_x", "--workflow-id", "wkf_1"}, getFactory, getOut); err != nil {
					t.Fatal(err)
				}
				getRegistry.Verify(t)
				var response struct {
					Data map[string]interface{} `json:"data"`
				}
				if err := json.Unmarshal(getOut.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(response.Data, definition) {
					t.Fatalf("GET changed view metadata: %#v", response.Data)
				}
				response.Data["title"] = "After edit"
				body, err := json.Marshal(response.Data)
				if err != nil {
					t.Fatal(err)
				}
				factory, stdout, registry := newExecuteFactory(t)
				stub := &httpmock.Stub{Method: "PUT", URL: "/open-apis/base/v3/bases/app_x/workflows/wkf_1", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"workflow_id": "wkf_1"}}}
				registry.Register(stub)
				if err := runShortcut(t, BaseWorkflowUpdate, []string{"+workflow-update", "--base-token", "app_x", "--workflow-id", "wkf_1", "--json", string(body)}, factory, stdout); err != nil {
					t.Fatal(err)
				}
				registry.Verify(t)
				var submitted map[string]interface{}
				if err := json.Unmarshal(stub.CapturedBody, &submitted); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(submitted, response.Data) {
					t.Fatalf("UPDATE changed the recordLink target or metadata: %#v", submitted)
				}
			})
		}
	}
}
