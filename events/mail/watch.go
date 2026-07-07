// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/internal/event"
	shortmail "github.com/larksuite/cli/shortcuts/mail"
)

func normalizeWatchParams(ctx context.Context, rt event.APIClient, params map[string]string) error {
	defaultParam(params, "mailbox", "me")
	defaultParam(params, "format", "data")
	defaultParam(params, "msg_format", "metadata")
	params["mailbox"] = strings.TrimSpace(params["mailbox"])
	params["format"] = strings.TrimSpace(params["format"])
	params["msg_format"] = strings.TrimSpace(params["msg_format"])
	params["mailbox_api"] = params["mailbox"]

	mailboxEmail := params["mailbox"]
	if strings.EqualFold(params["mailbox"], "me") {
		resolved, err := fetchMailboxPrimaryEmail(ctx, rt, "me")
		if err != nil {
			return shortmail.EnhanceProfileError(err)
		}
		mailboxEmail = resolved
	}
	params["mailbox_email"] = mailboxEmail
	params["mailbox"] = mailboxEmail

	labelIDs, err := resolveWatchFilterIDs(ctx, rt, params["mailbox_api"], params["label_ids"], params["labels"], "label_ids", "labels", "label")
	if err != nil {
		return err
	}
	folderIDs, err := resolveWatchFilterIDs(ctx, rt, params["mailbox_api"], params["folder_ids"], params["folders"], "folder_ids", "folders", "folder")
	if err != nil {
		return err
	}
	params["label_ids"] = encodeList(labelIDs)
	params["folder_ids"] = encodeList(folderIDs)
	return nil
}

func defaultParam(params map[string]string, key, value string) {
	if strings.TrimSpace(params[key]) == "" {
		params[key] = value
	}
}

func matchWatchMailbox(raw *event.RawEvent, params map[string]string) bool {
	eventBody := extractMailEventBody(raw.Payload)
	mailbox := params["mailbox_email"]
	if mailbox == "" {
		mailbox = params["mailbox"]
	}
	mailAddr, _ := eventBody["mail_address"].(string)
	return mailbox != "" && mailAddr != "" && strings.EqualFold(mailAddr, mailbox)
}

func processWatchEvent(ctx context.Context, rt event.APIClient, raw *event.RawEvent, params map[string]string) (json.RawMessage, error) {
	payload := payloadMap(raw.Payload)
	eventBody := shortmail.ExtractMailEventBody(payload)
	messageID, _ := eventBody["message_id"].(string)
	if messageID == "" {
		return nil, nil
	}

	msgFormat := params["msg_format"]
	if msgFormat == "" {
		msgFormat = "metadata"
	}
	outFormat := params["format"]
	if outFormat == "" {
		outFormat = "data"
	}
	labelIDSet := stringSet(parseList(params["label_ids"]))
	folderIDSet := stringSet(parseList(params["folder_ids"]))

	fetchMailbox := params["mailbox_api"]
	if fetchMailbox == "" {
		fetchMailbox = params["mailbox"]
	}
	if eventAddr, _ := eventBody["mail_address"].(string); eventAddr != "" {
		fetchMailbox = eventAddr
	}

	needMessage := msgFormat != "event" || len(labelIDSet) > 0 || len(folderIDSet) > 0 || params["output_dir"] != ""
	var message map[string]interface{}
	if needMessage {
		fetchFormat := shortmail.WatchFetchFormat(msgFormat, len(labelIDSet) > 0 || len(folderIDSet) > 0)
		if params["output_dir"] != "" {
			fetchFormat = "full"
		}
		var err error
		message, err = fetchMessageForWatch(ctx, rt, fetchMailbox, messageID, fetchFormat)
		if err != nil {
			failure := shortmail.WatchFetchFailureValue(messageID, fetchFormat, err, eventBody)
			return marshalWatchOutput(failure, outFormat)
		}
	}

	if len(folderIDSet) > 0 {
		folderID, _ := message["folder_id"].(string)
		if !folderIDSet[folderID] {
			return nil, nil
		}
	}
	if len(labelIDSet) > 0 && !shortmail.MessageHasLabel(message, labelIDSet) {
		return nil, nil
	}

	var outputData interface{} = payload
	if msgFormat != "event" && message != nil {
		if msgFormat == "minimal" {
			message = shortmail.MinimalWatchMessage(message)
		}
		outputData = map[string]interface{}{"message": message}
	}
	return marshalWatchOutput(outputData, outFormat)
}

func marshalWatchOutput(data interface{}, outFormat string) (json.RawMessage, error) {
	if outFormat == "json" {
		return json.Marshal(map[string]interface{}{
			"ok":       true,
			"identity": "user",
			"data":     data,
		})
	}
	return json.Marshal(data)
}

func payloadMap(raw json.RawMessage) map[string]interface{} {
	var data map[string]interface{}
	if len(raw) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.UseNumber()
		_ = dec.Decode(&data)
	}
	if data == nil {
		data = map[string]interface{}{}
	}
	return data
}

func extractMailEventBody(raw json.RawMessage) map[string]interface{} {
	return shortmail.ExtractMailEventBody(payloadMap(raw))
}

func encodeList(values []string) string {
	if len(values) == 0 {
		return ""
	}
	b, _ := json.Marshal(values)
	return string(b)
}

func parseList(input string) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	var values []string
	if strings.HasPrefix(input, "[") {
		if err := json.Unmarshal([]byte(input), &values); err == nil {
			return values
		}
	}
	for _, part := range strings.Split(input, ",") {
		if v := strings.TrimSpace(part); v != "" {
			values = append(values, v)
		}
	}
	return values
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		if v != "" {
			set[v] = true
		}
	}
	return set
}
