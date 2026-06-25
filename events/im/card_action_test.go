// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
)

func TestCardActionTriggerRegistered(t *testing.T) {
	def, ok := event.Lookup("card.action.trigger")
	if !ok {
		t.Fatal("card.action.trigger should be registered via Keys()")
	}
	if def.Schema.Custom == nil {
		t.Error("card.action.trigger must set Schema.Custom")
	}
	if def.Process == nil {
		t.Error("card.action.trigger must set Process")
	}
	if len(def.Scopes) == 0 {
		t.Error("Scopes must not be empty")
	}
}

func TestProcessCardAction_Button(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_btn_001",
			"event_type": "card.action.trigger",
			"create_time": "1776409469273"
		},
		"event": {
			"operator": {"open_id": "ou_operator"},
			"token": "c-token-btn",
			"host": "im_message",
			"action": {
				"tag": "button",
				"value": {"key": "approve"},
				"name": "approve_btn",
				"form_value": {},
				"options": [],
				"checked": false
			},
			"context": {
				"open_message_id": "om_msg_001",
				"open_chat_id": "oc_chat_001"
			}
		}
	}`
	out := runCardAction(t, payload, nil)

	if out.Type != "card.action.trigger" {
		t.Errorf("Type = %q, want card.action.trigger", out.Type)
	}
	if out.EventID != "ev_btn_001" {
		t.Errorf("EventID = %q", out.EventID)
	}
	if out.OperatorID != "ou_operator" {
		t.Errorf("OperatorID = %q", out.OperatorID)
	}
	if out.ActionTag != "button" {
		t.Errorf("ActionTag = %q, want button", out.ActionTag)
	}
	if out.ActionValue != `{"key":"approve"}` {
		t.Errorf("ActionValue = %q", out.ActionValue)
	}
	if out.ActionName != "approve_btn" {
		t.Errorf("ActionName = %q", out.ActionName)
	}
	if out.Token != "c-token-btn" {
		t.Errorf("Token = %q", out.Token)
	}
	if out.MessageID != "om_msg_001" {
		t.Errorf("MessageID = %q", out.MessageID)
	}
	if out.ChatID != "oc_chat_001" {
		t.Errorf("ChatID = %q", out.ChatID)
	}
	if out.Host != "im_message" {
		t.Errorf("Host = %q", out.Host)
	}
	if out.Timestamp != "1776409469273" {
		t.Errorf("Timestamp = %q", out.Timestamp)
	}
}

func TestProcessCardAction_FormSubmit(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_form_001",
			"event_type": "card.action.trigger",
			"create_time": "1776409469274"
		},
		"event": {
			"operator": {"open_id": "ou_form_user"},
			"token": "c-token-form",
			"host": "im_message",
			"action": {
				"tag": "button",
				"value": {},
				"name": "submit_btn",
				"form_value": {"name": "test-user", "reason": "testing"},
				"options": [],
				"checked": false
			},
			"context": {
				"open_message_id": "om_form_001",
				"open_chat_id": "oc_chat_002"
			}
		}
	}`
	out := runCardAction(t, payload, nil)

	if out.FormValue != `{"name":"test-user","reason":"testing"}` {
		t.Errorf("FormValue = %q", out.FormValue)
	}
	if out.ActionTag != "button" {
		t.Errorf("ActionTag = %q, want button", out.ActionTag)
	}
}

func TestProcessCardAction_MultiSelect(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_ms_001",
			"event_type": "card.action.trigger",
			"create_time": "1776409469275"
		},
		"event": {
			"operator": {"open_id": "ou_ms_user"},
			"token": "c-token-ms",
			"host": "im_message",
			"action": {
				"tag": "multi_select_static",
				"value": {},
				"name": "multi_select",
				"options": ["opt_1", "opt_3"],
				"checked": false
			},
			"context": {
				"open_message_id": "om_ms_001",
				"open_chat_id": "oc_chat_003"
			}
		}
	}`
	out := runCardAction(t, payload, nil)

	if out.Options != "opt_1,opt_3" {
		t.Errorf("Options = %q, want opt_1,opt_3", out.Options)
	}
	if out.ActionTag != "multi_select_static" {
		t.Errorf("ActionTag = %q", out.ActionTag)
	}
}

func TestProcessCardAction_Input(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_input_001",
			"event_type": "card.action.trigger",
			"create_time": "1776409469276"
		},
		"event": {
			"operator": {"open_id": "ou_input_user"},
			"token": "c-token-input",
			"host": "im_message",
			"action": {
				"tag": "input",
				"value": {},
				"name": "text_input",
				"input_value": "hello world",
				"options": [],
				"checked": false
			},
			"context": {
				"open_message_id": "om_input_001",
				"open_chat_id": "oc_chat_004"
			}
		}
	}`
	out := runCardAction(t, payload, nil)

	if out.InputValue != "hello world" {
		t.Errorf("InputValue = %q", out.InputValue)
	}
	if out.ActionTag != "input" {
		t.Errorf("ActionTag = %q", out.ActionTag)
	}
}

func TestProcessCardAction_DatePicker(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_date_001",
			"event_type": "card.action.trigger",
			"create_time": "1776409469277"
		},
		"event": {
			"operator": {"open_id": "ou_date_user"},
			"token": "c-token-date",
			"host": "im_message",
			"action": {
				"tag": "date_picker",
				"value": {},
				"name": "date_selector",
				"option": "2024-04-01 +0800",
				"timezone": "Asia/Shanghai",
				"options": [],
				"checked": false
			},
			"context": {
				"open_message_id": "om_date_001",
				"open_chat_id": "oc_chat_005"
			}
		}
	}`
	out := runCardAction(t, payload, nil)

	if out.Option != "2024-04-01 +0800" {
		t.Errorf("Option = %q", out.Option)
	}
	if out.Timezone != "Asia/Shanghai" {
		t.Errorf("Timezone = %q", out.Timezone)
	}
}

func TestProcessCardAction_MalformedPayload(t *testing.T) {
	raw := &event.RawEvent{
		EventID:   "ev_bad",
		EventType: "card.action.trigger",
		Payload:   json.RawMessage(`not json`),
		Timestamp: time.Now(),
	}
	got, err := processCardAction(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("Process should swallow parse errors, got %v", err)
	}
	if string(got) != "not json" {
		t.Errorf("malformed fallback output = %q, want original bytes", string(got))
	}
}

func TestProcessCardAction_MessageGetSuccess(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_mg_ok",
			"event_type": "card.action.trigger",
			"create_time": "1776409469278"
		},
		"event": {
			"operator": {"open_id": "ou_mg_user"},
			"token": "c-token-mg",
			"host": "im_message",
			"action": {
				"tag": "button",
				"value": {"key": "click"},
				"name": "btn",
				"form_value": {},
				"options": [],
				"checked": false
			},
			"context": {
				"open_message_id": "om_mg_001",
				"open_chat_id": "oc_chat_mg"
			}
		}
	}`
	cardContent := `{"header":{"title":{"tag":"plain_text","content":"A card"}}}`
	mock := &mockAPIClient{resp: `{
		"code": 0,
		"msg": "success",
		"data": {
			"items": [{
				"body": {"content": "` + escapeJSON(cardContent) + `"}
			}]
		}
	}`}
	out := runCardAction(t, payload, mock)

	if out.CardContent == "" {
		t.Error("CardContent should not be empty when message get succeeds")
	}
}

func TestProcessCardAction_MessageGetErrorCode(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_mg_ec",
			"event_type": "card.action.trigger",
			"create_time": "1776409469279"
		},
		"event": {
			"operator": {"open_id": "ou_mg_user2"},
			"token": "c-token-mg2",
			"host": "im_message",
			"action": {
				"tag": "button",
				"value": {},
				"name": "btn",
				"form_value": {},
				"options": [],
				"checked": false
			},
			"context": {
				"open_message_id": "om_mg_002",
				"open_chat_id": "oc_chat_mg2"
			}
		}
	}`
	mock := &mockAPIClient{resp: `{"code": 1, "msg": "error", "data": {"items": []}}`}
	out := runCardAction(t, payload, mock)

	if out.CardContent != "" {
		t.Errorf("CardContent should be empty when code != 0, got %q", out.CardContent)
	}
}

func TestProcessCardAction_MessageGetFailure(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_mg_fail",
			"event_type": "card.action.trigger",
			"create_time": "1776409469280"
		},
		"event": {
			"operator": {"open_id": "ou_mg_user3"},
			"token": "c-token-mg3",
			"host": "im_message",
			"action": {
				"tag": "button",
				"value": {},
				"name": "btn",
				"form_value": {},
				"options": [],
				"checked": false
			},
			"context": {
				"open_message_id": "om_mg_003",
				"open_chat_id": "oc_chat_mg3"
			}
		}
	}`
	mock := &mockAPIClient{errResp: true}
	out := runCardAction(t, payload, mock)

	if out.CardContent != "" {
		t.Errorf("CardContent should be empty when message get fails, got %q", out.CardContent)
	}
}

func TestProcessCardAction_EmptyMessageID(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_no_msg",
			"event_type": "card.action.trigger",
			"create_time": "1776409469281"
		},
		"event": {
			"operator": {"open_id": "ou_no_msg"},
			"token": "c-token-nm",
			"host": "im_message",
			"action": {
				"tag": "button",
				"value": {},
				"name": "btn",
				"form_value": {},
				"options": [],
				"checked": false
			},
			"context": {
				"open_message_id": "",
				"open_chat_id": "oc_chat_nm"
			}
		}
	}`
	out := runCardAction(t, payload, nil)

	if out.CardContent != "" {
		t.Errorf("CardContent should be empty when message_id is absent, got %q", out.CardContent)
	}
}

type mockAPIClient struct {
	resp    string
	errResp bool
}

func (m *mockAPIClient) CallAPI(_ context.Context, _, _ string, _ interface{}) (json.RawMessage, error) {
	if m.errResp {
		return nil, context.DeadlineExceeded
	}
	return json.RawMessage(m.resp), nil
}

func runCardAction(t *testing.T, payload string, rt event.APIClient) CardActionTriggerOutput {
	t.Helper()
	raw := &event.RawEvent{
		EventID:   "ev_test",
		EventType: "card.action.trigger",
		Payload:   json.RawMessage(payload),
		Timestamp: time.Now(),
	}
	got, err := processCardAction(context.Background(), rt, raw, nil)
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	var out CardActionTriggerOutput
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("Process output is not valid CardActionTriggerOutput JSON: %v\nraw=%s", err, string(got))
	}
	return out
}

func escapeJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}
