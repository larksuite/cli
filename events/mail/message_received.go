// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/internal/event"
)

type MailMessageReceivedOutput struct {
	Message map[string]interface{} `json:"message,omitempty" desc:"Fetched message payload for metadata, minimal, plain_text_full, or full formats"`
	Event   map[string]interface{} `json:"event,omitempty"   desc:"Raw mail event body for msg_format=event"`
}

func matchMailMessageReceived(raw *event.RawEvent, params map[string]string) bool {
	body := extractMailEventBody(raw)
	mailbox := normalizedMailbox(params)
	if mailbox == "" {
		return true
	}
	mailAddress, _ := body["mail_address"].(string)
	return strings.EqualFold(mailAddress, mailbox)
}

func processMailMessageReceived(ctx context.Context, rt event.APIClient, raw *event.RawEvent, params map[string]string) (json.RawMessage, error) {
	body := extractMailEventBody(raw)
	msgFormat := params["msg_format"]
	if msgFormat == "" {
		msgFormat = "metadata"
	}
	messageID, _ := body["message_id"].(string)
	if messageID == "" {
		return nil, nil
	}
	if msgFormat == "event" && params["folder_ids"] == "" && params["label_ids"] == "" {
		return raw.Payload, nil
	}
	fetchFormat := watchFetchFormat(msgFormat, params["folder_ids"] != "" || params["label_ids"] != "")
	fetchMailbox := normalizedMailbox(params)
	if eventMailbox, _ := body["mail_address"].(string); eventMailbox != "" {
		fetchMailbox = eventMailbox
	}
	message, err := fetchMessage(ctx, rt, fetchMailbox, messageID, fetchFormat)
	if err != nil {
		return json.Marshal(mailMessageFetchFailure(messageID, fetchFormat, err, body))
	}
	if !messageMatchesFilters(message, params) {
		return nil, nil
	}
	if msgFormat == "event" {
		return raw.Payload, nil
	}
	if msgFormat == "minimal" {
		message = minimalWatchMessage(message)
	}
	return json.Marshal(MailMessageReceivedOutput{Message: message})
}

func mailMessageFetchFailure(messageID, fetchFormat string, err error, eventBody map[string]interface{}) map[string]interface{} {
	payload := map[string]interface{}{
		"ok": false,
		"error": map[string]interface{}{
			"type":       "fetch_message_failed",
			"message_id": messageID,
			"format":     fetchFormat,
			"message":    err.Error(),
		},
	}
	if len(eventBody) > 0 {
		payload["event"] = eventBody
	}
	return payload
}

func extractMailEventBody(raw *event.RawEvent) map[string]interface{} {
	if raw == nil {
		return nil
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw.Payload, &data); err != nil {
		return nil
	}
	if eventBody, ok := data["event"].(map[string]interface{}); ok {
		return eventBody
	}
	return data
}

func fetchMessage(ctx context.Context, rt event.APIClient, mailbox, messageID, format string) (map[string]interface{}, error) {
	raw, err := rt.CallAPI(ctx, "GET", mailboxPath(mailbox, "messages", messageID)+"?format="+format, nil)
	if err != nil {
		return nil, err
	}
	data := responseData(raw)
	if msg, ok := data["message"].(map[string]interface{}); ok {
		return msg, nil
	}
	return data, nil
}

func watchFetchFormat(msgFormat string, forceMetadata bool) string {
	if forceMetadata && msgFormat == "event" {
		return "metadata"
	}
	switch msgFormat {
	case "metadata", "plain_text_full", "full":
		return msgFormat
	case "minimal":
		return "metadata"
	default:
		return "metadata"
	}
}

func minimalWatchMessage(message map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, 6)
	for _, key := range []string{"message_id", "thread_id", "folder_id", "label_ids", "internal_date", "message_state"} {
		if value, ok := message[key]; ok {
			out[key] = value
		}
	}
	return out
}

func messageMatchesFilters(message map[string]interface{}, params map[string]string) bool {
	if ids := csvSet(params["folder_ids"]); len(ids) > 0 {
		folderID, _ := message["folder_id"].(string)
		if !ids[folderID] {
			return false
		}
	}
	if ids := csvSet(params["label_ids"]); len(ids) > 0 && !messageHasLabel(message, ids) {
		return false
	}
	return true
}

func messageHasLabel(message map[string]interface{}, labelIDSet map[string]bool) bool {
	labels, _ := message["label_ids"].([]interface{})
	for _, label := range labels {
		if id, ok := label.(string); ok && labelIDSet[id] {
			return true
		}
	}
	return false
}

func csvSet(input string) map[string]bool {
	if input == "" {
		return nil
	}
	set := make(map[string]bool)
	for _, part := range strings.Split(input, ",") {
		if part = strings.TrimSpace(part); part != "" {
			set[part] = true
		}
	}
	return set
}
