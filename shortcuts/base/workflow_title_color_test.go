// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestWorkflowTitleColorAcrossCreateUpdateAndDryRun(t *testing.T) {
	for _, color := range []string{"none", "blue", "wathet", "turquoise", "green", "yellow", "orange", "red", "carmine", "violet", "purple", "indigo", "grey"} {
		for _, rich := range []bool{false, true} {
			body := mustWorkflowJSON(t, workflowRichTextFixture)
			data := body["steps"].([]interface{})[0].(map[string]interface{})["data"].(map[string]interface{})
			data["title_template_color"] = color
			if !rich {
				data["content_type"] = "plain_text"
				data["content"] = []interface{}{map[string]interface{}{"value_type": "text", "value": "Body"}}
				delete(data, "rich_text_content")
			}
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			for _, update := range []bool{false, true} {
				for _, dry := range []bool{false, true} {
					t.Run(color+"/"+data["content_type"].(string)+"/"+fmtWorkflowAgentCase(update, dry), func(t *testing.T) {
						factory, stdout, registry := newExecuteFactory(t)
						shortcut, method, url := BaseWorkflowCreate, "POST", "/open-apis/base/v3/bases/app_x/workflows"
						args := []string{"+workflow-create", "--base-token", "app_x", "--json", string(encoded)}
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
										Body interface{} `json:"body"`
									} `json:"api"`
								} `json:"data"`
							}
							if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
								t.Fatal(err)
							}
							if len(result.Data.API) != 1 {
								t.Fatalf("unexpected requests: %s", stdout.String())
							}
							got = result.Data.API[0].Body
						} else {
							registry.Verify(t)
							if err := json.Unmarshal(stub.CapturedBody, &got); err != nil {
								t.Fatal(err)
							}
						}
						if !reflect.DeepEqual(got, body) {
							t.Fatal("title color or body changed on submission")
						}
					})
				}
			}
		}
	}
}
