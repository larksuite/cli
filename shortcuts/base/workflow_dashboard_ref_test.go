// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestWorkflowDashboardRefRoundTrip(t *testing.T) {
	for _, rich := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "rich"}[rich], func(t *testing.T) {
			body := workflowRichMediaBody("LarkMessageAction", "screenshot")
			if !rich {
				var input map[string]interface{}
				if err := json.Unmarshal([]byte(body), &input); err != nil {
					t.Fatal(err)
				}
				data := input["steps"].([]interface{})[1].(map[string]interface{})["data"].(map[string]interface{})
				data["content"] = data["rich_text_content"].([]interface{})[0].(map[string]interface{})["value"]
				for _, item := range data["content"].([]interface{}) {
					delete(item.(map[string]interface{}), "style")
				}
				delete(data, "rich_text_content")
				delete(data, "content_type")
				encoded, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				body = string(encoded)
			}
			factory, stdout, registry := newExecuteFactory(t)
			stub := &httpmock.Stub{Method: "PUT", URL: "/open-apis/base/v3/bases/app_x/workflows/wkf_1", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"workflow_id": "wkf_1"}}}
			registry.Register(stub)
			if err := runShortcut(t, BaseWorkflowUpdate, []string{"+workflow-update", "--base-token", "app_x", "--workflow-id", "wkf_1", "--json", body}, factory, stdout); err != nil {
				t.Fatal(err)
			}
			registry.Verify(t)
			var got interface{}
			if err := json.Unmarshal(stub.CapturedBody, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, mustWorkflowJSON(t, body)) {
				t.Fatal("dashboard reference changed in request")
			}
		})
	}
}
