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
)

func TestValidateWorkflowBodyForCLI(t *testing.T) {
	message := func(data string) string {
		return `{"steps":[{"id":"m","type":"LarkMessageAction","data":` + data + `}]}`
	}
	rich := func(block string) string {
		return message(`{"content_type":"rich_text","rich_text_content":[` + block + `]}`)
	}
	button := func(value string) string { return message(`{"btn_list":[` + value + `]}`) }
	webhook := func(data string) string { return `{"steps":[{"id":"w","type":"WebhookTrigger","data":` + data + `}]}` }
	agentMode := func(value string) string {
		return strings.Replace(workflowAgentFixture, `"thinking_mode":"deep"`, `"thinking_mode":`+value, 1)
	}
	findButton := func(info string) string {
		return `{"steps":[{"id":"t","type":"ButtonTrigger","next":"f"},{"id":"m","type":"LarkMessageAction","data":{"btn_list":[{"text":"完成","btn_action":"setRecord","record_type":"Ref","record_info":` + info + `,"record_values":[{"field_name":"标题","value":[{"value_type":"text","value":"完成"}]}]}]}},{"id":"f","type":"FindRecordAction","next":"m"}]}`
	}
	cases := []struct{ name, body, path string }{
		{"thinking mode empty", agentMode(`""`), "thinking_mode"},
		{"thinking mode null", agentMode(`null`), "thinking_mode"},
		{"thinking mode boolean", agentMode(`true`), "thinking_mode"},
		{"thinking mode number", agentMode(`1`), "thinking_mode"},
		{"thinking mode array", agentMode(`[]`), "thinking_mode"},
		{"thinking mode object", agentMode(`{}`), "thinking_mode"},
		{"thinking mode unknown", agentMode(`"slow"`), "thinking_mode"},
		{"thinking mode uppercase", agentMode(`"Deep"`), "thinking_mode"},
		{"thinking mode whitespace", agentMode(`" deep "`), "thinking_mode"},
		{"thinking mode camel null", strings.Replace(agentMode(`null`), `"thinking_mode":null`, `"thinkingMode":null`, 1), "thinking_mode"},
		{"record trigger app screenshot", `{"steps":[{"id":"shot","type":"AppScreenshotAction","data":{"app_name":"应用","page_name":"页面"}},{"id":"t","type":"AddRecordTrigger","next":"shot"}]}`, "ReminderTrigger"},
		{"record trigger dashboard message", `{"steps":[{"id":"m","type":"LarkMessageAction","data":{"content":[{"value_type":"ref","value":"$.dashboard.image","extra_info":{"dashboard_name":"看板"}}]}},{"id":"t","type":"AddRecordTrigger","next":"m"}]}`, "ReminderTrigger"},
		{"record trigger dashboard email", `{"steps":[{"id":"t","type":"AddRecordTrigger","next":"m"},{"id":"m","type":"LarkEmailAction","data":{"content_type":"rich_text","rich_text_content":[{"type":"paragraph","value":[{"value_type":"ref","value":"$.dashboard.image","extra_info":{"dashboard_name":"看板"}}]}]}}]}`, "ReminderTrigger"},
		{"webhook trigger dashboard summary", `{"steps":[{"id":"t","type":"WebhookTrigger","next":"m"},{"id":"m","type":"LarkMessageAction","data":{"content":[{"value_type":"ref","value":"$.dashboard.aiAnalysis","extra_info":{"dashboard_name":"看板"}}]}}]}`, "ReminderTrigger"},
		{"find button all records", findButton(`{"step_id":"f","type":"allRecords"}`), "record_info.type"},
		{"find button missing kind", findButton(`{"step_id":"f"}`), "record_info.type"},
		{"find button null kind", findButton(`{"stepId":"f","type":null}`), "record_info.type"},
		{"heading range", rich(`{"type":"paragraph","line_marker_list":[{"type":"heading","level":4}],"value":[]}`), "level"},
		{"empty rich body", message(`{"content_type":"rich_text","rich_text_content":[]}`), "rich_text_content"},
		{"mixed body", message(`{"content_type":"rich_text","content":[],"rich_text_content":[]}`), "content"},
		{"wrong mode", message(`{"content_type":"html"}`), "content_type"},
		{"plain style", message(`{"content":[{"value_type":"text","value":"x","style":{"bold":true}}]}`), "style"},
		{"unknown body color", rich(`{"type":"paragraph","value":[{"value_type":"text","value":"x","style":{"color":"pink"}}]}`), "color"},
		{"unknown style", rich(`{"type":"paragraph","value":[{"value_type":"text","value":"x","style":{"underline":true}}]}`), "underline"},
		{"marker duplicate", rich(`{"type":"paragraph","value":[],"line_marker_list":[{"type":"heading","level":1},{"type":"heading","level":2}]}`), "line_marker_list"},
		{"fractional list level", rich(`{"type":"paragraph","value":[],"line_marker_list":[{"type":"orderedList","level":1.5}]}`), "level"},
		{"zero list order", rich(`{"type":"paragraph","value":[],"line_marker_list":[{"type":"orderedList","level":1,"order":0}]}`), "order"},
		{"inconsistent list continuation", rich(`{"type":"paragraph","value":[],"line_marker_list":[{"type":"orderedList","level":1,"order":3}]},{"type":"paragraph","value":[],"line_marker_list":[{"type":"orderedList","level":1,"order":8}]}`), "expected 4"},
		{"private list start", rich(`{"type":"paragraph","value":[],"line_marker_list":[{"type":"orderedList","level":1,"start_order":2}]}`), "start_order"},
		{"horizontal line data", rich(`{"type":"horizontalLine","value":[]}`), "horizontalLine"},
		{"link child style", rich(`{"type":"paragraph","value":[{"value_type":"link","value":{"text":[{"value_type":"text","value":"x","style":{"bold":true}}],"link":[{"value_type":"text","value":"https://example.com"}]}}]}`), "style"},
		{"empty link text", rich(`{"type":"paragraph","value":[{"value_type":"link","value":{"text":[{"value_type":"text","value":""}],"link":[{"value_type":"text","value":"https://example.com"}]}}]}`), "value"},
		{"whole first record", `{"steps":[{"id":"f","type":"FindRecordAction","next":"m"},{"id":"m","type":"LarkMessageAction","data":{"content_type":"rich_text","rich_text_content":[{"type":"paragraph","value":[{"value_type":"ref","value":"$.f.firstfieldsRecord"}]}]}}]}`, "firstfieldsRecord"},
		{"empty selected fields", rich(`{"type":"paragraph","value":[{"value_type":"ref","value":"$.f.fieldRecords","extra_info":{"field_name_list":[]}}]}`), "field_name_list"},
		{"duplicate selected fields", rich(`{"type":"paragraph","value":[{"value_type":"ref","value":"$.f.fieldRecords","extra_info":{"field_name_list":["标题","标题"]}}]}`), "field_name_list"},
		{"header palette", message(`{"title_template_color":"future-color"}`), "title_template_color"},
		{"conflicting rich body aliases", message(`{"content_type":"rich_text","rich_text_content":[{"type":"paragraph","value":[{"value_type":"text","value":"甲"}]}],"richTextContent":[{"type":"paragraph","value":[{"valueType":"text","value":"乙"}]}]}`), "conflicting rich_text_content/richTextContent"},
		{"button action", button(`{"text":"x","btn_action":"future"}`), "btn_action"},
		{"button style", button(`{"text":"x","btn_action":"openLink","style":"future","link":[]}`), "style"},
		{"link repeat click", button(`{"text":"x","btn_action":"openLink","not_repeat_click":true,"link":[]}`), "not_repeat_click"},
		{"button action conflict", button(`{"text":"x","btn_action":"triggerWorkflow","workflow_id":"wkf_x","link":[{"value_type":"text","value":"x"}]}`), "link"},
		{"missing workflow", button(`{"text":"x","btn_action":"triggerWorkflow"}`), "workflow_id"},
		{"missing table", button(`{"text":"x","btn_action":"addRecord","record_values":[]}`), "table_name"},
		{"record source", button(`{"text":"x","btn_action":"setRecord","record_type":"Ref","record_info":{},"record_values":[]}`), "record_info"},
		{"mutual mode", message(`{"button_config":{"not_repeat_click_mode":"OnlySelf","mutual_exclusion":true}}`), "not_repeat_click_mode"},
		{"mutual missing button", message(`{"button_config":{"mutual_exclusion":true}}`), "button_config"},
		{"screenshot names", `{"steps":[{"type":"AppScreenshotAction","data":{"app_name":"App"}}]}`, "page_name"},
		{"webhook private IP", webhook(`{"ip_allow_list":["127.0.0.1"]}`), "ip_allow_list"},
		{"webhook empty list", webhook(`{"ip_allow_list":[]}`), "ip_allow_list"},
		{"webhook bool", webhook(`{"enable_bearer_check":"true"}`), "enable_bearer_check"},
		{"webhook invalid JSON", webhook(`{"output_json":"{"}`), "output_json"},
		{"webhook managed token", webhook(`{"bearer_token":"synthetic"}`), "bearer_token"},
		{"webhook reset dependency", webhook(`{"reset_bearer_token":true,"enable_bearer_check":false}`), "reset_bearer_token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, command := range []string{"+workflow-create", "+workflow-update"} {
				body := mustWorkflowJSON(t, tc.body)
				before, _ := json.Marshal(body)
				err := validateWorkflowBodyForCLI(command, body)
				var validation *errs.ValidationError
				if !errors.As(err, &validation) || validation.Param != "--json" || !strings.Contains(err.Error(), tc.path) {
					t.Fatalf("%s: expected typed error containing %s, got %T %v", command, tc.path, err, err)
				}
				after, _ := json.Marshal(body)
				if !reflect.DeepEqual(before, after) {
					t.Fatal("validation mutated request")
				}
			}
		})
	}
	t.Run("command boundary and explicit bypass", func(t *testing.T) {
		body := cases[0].body
		for _, update := range []bool{false, true} {
			for _, dry := range []bool{false, true} {
				assertWorkflowJSONRejected(t, body, update, dry, false)
				assertWorkflowBodyForwarded(t, body, update, dry, true)
			}
		}
	})
	t.Run("valid configurations preserve exact request", func(t *testing.T) {
		for _, tc := range []struct{ name, body string }{
			{"headings and optional list numbering", rich(`{"type":"paragraph","value":[],"line_marker_list":[{"type":"heading","level":1},{"type":"orderedList","level":1}]},{"type":"paragraph","value":[],"line_marker_list":[{"type":"heading","level":2}]},{"type":"paragraph","value":[],"line_marker_list":[{"type":"heading","level":3},{"type":"unorderedList","level":2}]}`)},
			{"nested list returns to parent numbering", rich(`{"type":"paragraph","value":[],"line_marker_list":[{"type":"orderedList","level":1,"order":3,"order_id":"甲"}]},{"type":"paragraph","value":[],"line_marker_list":[{"type":"orderedList","level":2,"order_id":"甲"}]},{"type":"paragraph","value":[],"line_marker_list":[{"type":"orderedList","level":1,"order":4,"order_id":"甲"}]}`)},
			{"independent explicit groups", rich(`{"type":"paragraph","value":[],"line_marker_list":[{"type":"orderedList","level":1,"order":3,"order_id":"甲"}]},{"type":"paragraph","value":[],"line_marker_list":[{"type":"orderedList","level":1,"order":8,"order_id":"乙"}]}`)},
			{"camel aliases", message(`{"contentType":"rich_text","richTextContent":[{"type":"paragraph","lineMarkerList":[{"type":"orderedList","level":1,"orderId":"中文组","order":3}],"value":[{"valueType":"text","value":"内容","style":{"bold":false,"color":"gray"}}]}]}`)},
			{"first record concrete field", rich(`{"type":"paragraph","value":[{"value_type":"ref","value":"$.f.firstfieldsRecord.标题","extra_info":{"table_name":"订单","view_name":"默认视图","future_annotation":true}}]}`)},
			{"legacy plain first record", message(`{"content":[{"value_type":"ref","value":"$.f.firstfieldsRecord"}]}`)},
			{"null optional reference annotations", rich(`{"type":"paragraph","value":[{"value_type":"ref","value":"$.trigger.标题","extra_info":null}]}`)},
			{"button link annotations ignored", button(`{"text":"打开","btn_action":"openLink","link":[{"value_type":"text","value":"https://example.com","style":{"bold":true}}]}`)},
			{"button empty irrelevant fields", button(`{"text":"打开","btn_action":"openLink","link":[{"value_type":"text","value":"https://example.com"}],"not_repeat_click":false,"table_name":"","record_values":[],"record_info":null,"workflow_id":""}`)},
			{"setRecord table name remains ignored", button(`{"text":"完成","btn_action":"setRecord","table_name":"旧提示","record_values":[{"field_name":"标题","value":[{"value_type":"text","value":"完成"}]}]}`)},
			{"OnlySelf without restrictions accepted by model", message(`{"btn_list":null,"button_config":{"not_repeat_click_mode":"OnlySelf","mutual_exclusion":false}}`)},
			{"null default config fields", message(`{"button_config":{"not_repeat_click_mode":null,"mutual_exclusion":null}}`)},
			{"mutual action buttons", message(`{"btn_list":[{"text":"完成","btn_action":"triggerWorkflow","workflow_id":"wkf_x","not_repeat_click":true}],"button_config":{"mutual_exclusion":true}}`)},
			{"mutual action with link false ignored", message(`{"btn_list":[{"text":"完成","btn_action":"triggerWorkflow","workflow_id":"wkf_x","not_repeat_click":true},{"text":"打开","btn_action":"openLink","not_repeat_click":false,"link":[{"value_type":"text","value":"https://example.com"}]}],"button_config":{"mutual_exclusion":true}}`)},
			{"empty webhook", `{"steps":[{"type":"WebhookTrigger","data":null}]}`},
			{"public mapped IPv4 matches Go sanitizer", webhook(`{"ip_allow_list":["::ffff:203.0.113.10"],"output_json":"{}","enable_bearer_check":false}`)},
			{"future extension outside owned fields", message(`{"future_extension":{"nested":[true]},"content":[{"value_type":"text","value":"body"}]}`)},
			{"unrelated metadata aliases", `{"steps":[{"id":"m","type":"LarkMessageAction","meta":{"table_name":"注释甲","tableName":"注释乙"},"data":{"content":[{"value_type":"text","value":"正文"}]}}]}`},
			{"opaque extension aliases", message(`{"future_extension":{"table_name":"注释甲","tableName":"注释乙"},"content":[{"value_type":"text","value":"正文"}]}`)},
			{"readonly reference annotations", rich(`{"type":"paragraph","value":[{"value_type":"ref","value":"$.trigger.标题","extra_info":{"table_name":"注释甲","tableName":"注释乙"}}]}`)},
			{"equivalent rich body aliases", message(`{"content_type":"rich_text","rich_text_content":[{"type":"paragraph","value":[{"value_type":"text","value":"正文"}]}],"richTextContent":[{"type":"paragraph","value":[{"valueType":"text","value":"正文"}]}]}`)},
			{"thinking mode auto", agentMode(`"auto"`)},
			{"thinking mode quick", agentMode(`"quick"`)},
			{"thinking mode camel", strings.Replace(agentMode(`"deep"`), `"thinking_mode":"deep"`, `"thinkingMode":"deep"`, 1)},
			{"thinking mode omitted", strings.Replace(workflowAgentFixture, `,"thinking_mode":"deep"`, "", 1)},
			{"thinking mode model capability remains server owned", strings.Replace(agentMode(`"auto"`), "doubao_chat_model", "deepseek_chat_model", 1)},
			{"thinking mode leaves other agent fields untouched", strings.Replace(workflowAgentFixture, `"future_setting":true`, `"future_setting":true,"content_type":"plain_text","contentType":"rich_text"`, 1)},
			{"find button first record with reordered array", findButton(`{"step_id":"f","type":"firstRecord"}`)},
			{"trigger record button source", strings.Replace(findButton(`{"step_id":"t"}`), `"type":"FindRecordAction"`, `"type":"AddRecordAction"`, 1)},
			{"unknown source type remains server owned", strings.Replace(findButton(`{"step_id":"f","type":"futureRecord"}`), `"type":"FindRecordAction"`, `"type":"FutureRecordAction"`, 1)},
			{"timer screenshot", `{"steps":[{"id":"shot","type":"AppScreenshotAction","data":{"appName":"应用","pageName":"页面"}},{"id":"t","type":"TimerTrigger","next":"shot"}]}`},
			{"reminder dashboard", `{"steps":[{"id":"m","type":"LarkMessageAction","data":{"content":[{"value_type":"ref","value":"$.dashboard.aiAnalysis","extra_info":{"dashboard_name":"看板"}}]}},{"id":"t","type":"ReminderTrigger","next":"m"}]}`},
			{"button dashboard", `{"steps":[{"id":"t","type":"ButtonTrigger","next":"m"},{"id":"m","type":"LarkEmailAction","data":{"content":[{"value_type":"ref","value":"$.dashboard.image","extra_info":{"dashboard_name":"看板"}}]}}]}`},
			{"future trigger capability remains server owned", `{"steps":[{"id":"t","type":"FutureTrigger","next":"shot"},{"id":"shot","type":"AppScreenshotAction","data":{"app_name":"应用","page_name":"页面"}}]}`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				for _, update := range []bool{false, true} {
					for _, dry := range []bool{false, true} {
						assertWorkflowBodyForwarded(t, tc.body, update, dry, false)
					}
				}
			})
		}
	})
	t.Run("webhook reset command boundary", func(t *testing.T) {
		body := webhook(`{"reset_bearer_token":false,"enable_bearer_check":true}`)
		assertWorkflowJSONRejected(t, body, false, false, false)
		assertWorkflowBodyForwarded(t, body, true, false, false)
	})
}

func TestWorkflowBodyRefUsesSourceStepType(t *testing.T) {
	cases := []struct {
		name       string
		sourceType string
		sourceData string
		ref        string
		plain      bool
		wantErr    bool
	}{
		{"agent JSON scalar", "AIAgentAction", `{"output_format_type":"json","json_output":"{\"firstfieldsRecord\":\"text\"}"}`, "$.source.firstfieldsRecord", false, false},
		{"agent form scalar", "AIAgentAction", `{"output_format_type":"form","form_output":[{"name":"firstfieldsRecord","type":"string"}]}`, "$.source.firstfieldsRecord", false, false},
		{"unknown source stays server owned", "FutureAction", `{}`, "$.source.firstfieldsRecord", false, false},
		{"find whole first record", "FindRecordAction", `{"table_name":"Source"}`, "$.source.firstfieldsRecord", false, true},
		{"find concrete first record field", "FindRecordAction", `{"table_name":"Source"}`, "$.source.firstfieldsRecord.Title", false, false},
		{"find all records", "FindRecordAction", `{"table_name":"Source"}`, "$.source.fieldRecords", false, false},
		{"find plain first record", "FindRecordAction", `{"table_name":"Source"}`, "$.source.firstfieldsRecord", true, false},
	}
	for _, tc := range cases {
		for _, action := range []string{"LarkMessageAction", "LarkEmailAction"} {
			for _, update := range []bool{false, true} {
				for _, dry := range []bool{false, true} {
					t.Run(tc.name+"/"+action+"/"+fmtWorkflowAgentCase(update, dry), func(t *testing.T) {
						items := []interface{}{map[string]interface{}{"value_type": "ref", "value": tc.ref}}
						data := map[string]interface{}{
							"content_type": "rich_text",
							"rich_text_content": []interface{}{map[string]interface{}{
								"type": "paragraph", "value": items,
							}},
						}
						if tc.plain {
							data = map[string]interface{}{"content": items}
						}
						// The source appears after its consumer to exercise ID-based lookup.
						payload := map[string]interface{}{"steps": []interface{}{
							map[string]interface{}{"id": "trigger", "type": "AddRecordTrigger", "next": "source"},
							map[string]interface{}{"id": "message", "type": action, "data": data},
							map[string]interface{}{"id": "source", "type": tc.sourceType, "next": "message", "data": json.RawMessage(tc.sourceData)},
						}}
						encoded, err := json.Marshal(payload)
						if err != nil {
							t.Fatal(err)
						}
						body := string(encoded)
						if tc.wantErr {
							assertWorkflowJSONRejected(t, body, update, dry, false)
							assertWorkflowBodyForwarded(t, body, update, dry, true)
							return
						}
						assertWorkflowBodyForwarded(t, body, update, dry, false)
					})
				}
			}
		}
	}
}
