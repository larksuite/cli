// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func workflowRichMediaBody(action, render string) string {
	attribute := "image"
	if render == "smart_summary" {
		attribute = "aiAnalysis"
	}
	fields := `"receiver":[],"send_to_everyone":false,"title_template_color":"red","btn_list":[],`
	image := `{"value_type":"ref","value":"$.shot.Image"},`
	if action == "LarkEmailAction" {
		fields = `"to":[],"subject":[],"show_source":false,`
		image = ""
	}
	return fmt.Sprintf(`{"title":"Media","steps":[
        {"id":"shot","type":"AppScreenshotAction","data":{"app_name":"销售应用","page_name":"订单看板"},"next":"notify"},
        {"id":"notify","type":%q,"data":{%s
            "content_type":"rich_text","rich_text_content":[{"type":"paragraph","value":[
                {"value_type":"text","value":"before","style":{"bold":true}},
                {"value_type":"ref","value":%q,"extra_info":{"dashboard_name":"销售看板"}},
                %s
                {"value_type":"text","value":"after","style":{"italic":true}}
            ]}],"attachments":[{"value_type":"ref","value":"$.shot.Image"}]
        }}]}`, action, fields, "$.dashboard."+attribute, image)
}

func TestWorkflowRichMediaPreservedAcrossCommands(t *testing.T) {
	for _, action := range []string{"LarkMessageAction", "LarkEmailAction"} {
		renders := []string{"screenshot"}
		if action == "LarkMessageAction" {
			renders = append(renders, "smart_summary")
		}
		for _, render := range renders {
			for _, update := range []bool{false, true} {
				for _, dry := range []bool{false, true} {
					t.Run(action+"/"+render+"/"+fmtWorkflowAgentCase(update, dry), func(t *testing.T) {
						body := workflowRichMediaBody(action, render)
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
								Data struct {
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
							if len(result.Data.API) != 1 || result.Data.API[0].Method != method || result.Data.API[0].URL != url {
								t.Fatal("unexpected request")
							}
							got = result.Data.API[0].Body
						} else {
							registry.Verify(t)
							if err := json.Unmarshal(stub.CapturedBody, &got); err != nil {
								t.Fatal(err)
							}
						}
						if !reflect.DeepEqual(got, mustWorkflowJSON(t, body)) {
							t.Fatal("media, order or other configuration changed")
						}
					})
				}
			}
		}
	}
}

func TestWorkflowImageNamedScalarReferenceIsNotMedia(t *testing.T) {
	for _, action := range []string{"LarkMessageAction", "LarkEmailAction"} {
		t.Run(action, func(t *testing.T) {
			body := mustWorkflowJSON(t, workflowAgentFixture)
			steps := body["steps"].([]interface{})
			agent := steps[1].(map[string]interface{})
			agent["data"].(map[string]interface{})["json_output"] = `{"Image":"https://example.com/image"}`
			steps = append(steps, map[string]interface{}{
				"id":   "notify",
				"type": action,
				"data": map[string]interface{}{
					"content_type": "rich_text",
					"rich_text_content": []interface{}{map[string]interface{}{
						"type": "paragraph",
						"value": []interface{}{map[string]interface{}{
							"value_type": "link",
							"value": map[string]interface{}{
								"text": []interface{}{map[string]interface{}{"value_type": "ref", "value": "$.agent.Image"}},
								"link": []interface{}{map[string]interface{}{"value_type": "ref", "value": "$.agent.Image"}},
							},
						}},
					}},
				},
			})
			body["steps"] = steps
			if err := validateWorkflowBodyForCLI("+workflow-create", body); err != nil {
				t.Fatalf("Image-named scalar ref should not be treated as screenshot media: %v", err)
			}
		})
	}
}
