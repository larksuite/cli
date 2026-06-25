// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/internal/event"
)

// CardActionTriggerOutput is the flattened shape for card.action.trigger.
type CardActionTriggerOutput struct {
	Type        string `json:"type"                    desc:"Event type; always card.action.trigger"`
	EventID     string `json:"event_id,omitempty"      desc:"Globally unique event ID"`
	Timestamp   string `json:"timestamp,omitempty"     desc:"Event delivery time (ms timestamp string)" kind:"timestamp_ms"`
	OperatorID  string `json:"operator_id,omitempty"   desc:"Operator open_id"                          kind:"open_id"`
	MessageID   string `json:"message_id,omitempty"    desc:"Message ID of the card"                    kind:"message_id"`
	ChatID      string `json:"chat_id,omitempty"       desc:"Chat ID"                                   kind:"chat_id"`
	Host        string `json:"host,omitempty"          desc:"Host type: im_message / im_top_notice"`
	Token       string `json:"token,omitempty"         desc:"Token for delay card update (valid 30 min, max 2 updates)"`
	ActionTag   string `json:"action_tag,omitempty"    desc:"Triggered element type: button/select_static/input/checker/etc"`
	ActionValue string `json:"action_value,omitempty"  desc:"Developer-defined action value as JSON string"`
	ActionName  string `json:"action_name,omitempty"   desc:"Element name attribute"`
	FormValue   string `json:"form_value,omitempty"    desc:"Form submission values as JSON string (only on form submit)"`
	InputValue  string `json:"input_value,omitempty"   desc:"Input field value (only for input elements)"`
	Option      string `json:"option,omitempty"        desc:"Selected option value (for single-select dropdown)"`
	Options     string `json:"options,omitempty"       desc:"Selected options, comma-separated (for multi-select)"`
	Checked     bool   `json:"checked"                 desc:"Checkbox state (for checkbox elements)"`
	Timezone    string `json:"timezone,omitempty"      desc:"User timezone for date/time picker interactions"`
	CardContent string `json:"card_content,omitempty"  desc:"Original card JSON content (body.content) auto-fetched via message get API at consume time using message_id; empty if message_id absent or fetch fails"`
}

func processCardAction(ctx context.Context, rt event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	var envelope struct {
		Header struct {
			EventID    string `json:"event_id"`
			EventType  string `json:"event_type"`
			CreateTime string `json:"create_time"`
		} `json:"header"`
		Event struct {
			Operator struct {
				OpenID string `json:"open_id"`
			} `json:"operator"`
			Token  string `json:"token"`
			Host   string `json:"host"`
			Action struct {
				Tag        string                 `json:"tag"`
				Value      map[string]interface{} `json:"value"`
				Name       string                 `json:"name"`
				FormValue  map[string]interface{} `json:"form_value"`
				InputValue string                 `json:"input_value"`
				Option     string                 `json:"option"`
				Options    []string               `json:"options"`
				Checked    bool                   `json:"checked"`
				Timezone   string                 `json:"timezone"`
			} `json:"action"`
			Context struct {
				OpenMessageID string `json:"open_message_id"`
				OpenChatID    string `json:"open_chat_id"`
			} `json:"context"`
		} `json:"event"`
	}
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		return raw.Payload, nil //nolint:nilerr // passthrough on malformed payload
	}

	actionValue := marshalToString(envelope.Event.Action.Value)
	formValue := marshalToString(envelope.Event.Action.FormValue)
	options := strings.Join(envelope.Event.Action.Options, ",")

	out := &CardActionTriggerOutput{
		Type:        envelope.Header.EventType,
		EventID:     envelope.Header.EventID,
		Timestamp:   envelope.Header.CreateTime,
		OperatorID:  envelope.Event.Operator.OpenID,
		MessageID:   envelope.Event.Context.OpenMessageID,
		ChatID:      envelope.Event.Context.OpenChatID,
		Host:        envelope.Event.Host,
		Token:       envelope.Event.Token,
		ActionTag:   envelope.Event.Action.Tag,
		ActionValue: actionValue,
		ActionName:  envelope.Event.Action.Name,
		FormValue:   formValue,
		InputValue:  envelope.Event.Action.InputValue,
		Option:      envelope.Event.Action.Option,
		Options:     options,
		Checked:     envelope.Event.Action.Checked,
		Timezone:    envelope.Event.Action.Timezone,
	}

	if out.MessageID != "" && rt != nil {
		out.CardContent = fetchCardUserDSL(ctx, rt, out.MessageID)
	}

	return json.Marshal(out)
}

// fetchCardUserDSL gets the card message content via message get API.
// Returns empty string on any failure — never blocks event consumption.
func fetchCardUserDSL(ctx context.Context, rt event.APIClient, messageID string) string {
	path := "/open-apis/im/v1/messages/" + messageID + "?card_msg_content_type=user_card_content"
	resp, err := rt.CallAPI(ctx, "GET", path, nil)
	if err != nil {
		return ""
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Items []struct {
				Body struct {
					Content string `json:"content"`
				} `json:"body"`
			} `json:"items"`
		} `json:"data"`
	}
	if json.Unmarshal(resp, &result) != nil || result.Code != 0 || len(result.Data.Items) == 0 {
		return ""
	}
	return result.Data.Items[0].Body.Content
}

func marshalToString(m map[string]interface{}) string {
	if len(m) == 0 {
		return ""
	}
	b, _ := json.Marshal(m)
	return string(b)
}
