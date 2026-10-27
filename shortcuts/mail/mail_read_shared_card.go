// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const (
	sharedCardIDsPath             = "/open-apis/mail/v1/user_mailboxes/me/messages/get_by_card"
	sharedCardDetailsPath         = "/open-apis/mail/v1/messages/by_card"
	sharedCardAttachmentURLsPath  = "/open-apis/mail/v1/messages/attachments/download_url_by_card"
	sharedCardMessageBatchSize    = 50
	sharedCardAttachmentBatchSize = 100
)

type sharedCardOutput struct {
	Format     string                   `json:"format"`
	MessageIDs []string                 `json:"message_ids,omitempty"`
	Messages   []map[string]interface{} `json:"messages,omitempty"`
}

var MailReadSharedCard = common.Shortcut{
	Service:     "mail",
	Command:     "+read-shared-card",
	Description: "Read messages authorized by an IM shared-mail card. Attachment download URLs are returned only when explicitly requested.",
	Risk:        "read",
	Scopes: []string{
		"mail:user_mailbox.message:readonly",
	},
	ConditionalUserScopes: []string{
		"mail:user_mailbox.message.address:read",
		"mail:user_mailbox.message.subject:read",
		"mail:user_mailbox.message.body:read",
	},
	AuthTypes: []string{"user"},
	Flags: []common.Flag{
		{Name: "owner-id", Desc: "Required. ownerId from the IM shared-mail card.", Required: true},
		{Name: "card-id", Desc: "Required. cardId from the IM shared-mail card.", Required: true},
		{Name: "message-format", Default: "summary", Enum: []string{"summary", "id", "full", "plain_text_full", "metadata"}, Desc: "message view: summary | id | full | plain_text_full | metadata"},
		{Name: "include-attachment-download-urls", Type: "bool", Default: "false", Desc: "Request short-lived attachment download URLs (default: false)."},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("owner-id")) == "" {
			return mailValidationParamError("--owner-id", "--owner-id must not be empty")
		}
		if strings.TrimSpace(runtime.Str("card-id")) == "" {
			return mailValidationParamError("--card-id", "--card-id must not be empty")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		params := sharedCardParams(runtime, "")
		plan := common.NewDryRunAPI().
			Desc("Resolve card-authorized message IDs, read details through the dedicated card API, and optionally request attachment URLs").
			GET(sharedCardIDsPath).
			Params(map[string]interface{}{
				"owner_id": params["owner_id"],
				"card_id":  params["card_id"],
			})
		if runtime.Str("message-format") == "id" {
			return plan
		}
		plan.POST(sharedCardDetailsPath).
			Params(sharedCardParams(runtime, detailFormat(runtime.Str("message-format")))).
			Body(map[string]interface{}{"message_ids": []string{"<card_authorized_message_id>"}})
		if runtime.Bool("include-attachment-download-urls") {
			plan.POST(sharedCardAttachmentURLsPath).
				Params(map[string]interface{}{
					"owner_id":   params["owner_id"],
					"card_id":    params["card_id"],
					"message_id": "<card_authorized_message_id>",
				}).
				Body(map[string]interface{}{"attachment_ids": []string{"<attachment_id>"}})
		}
		return plan
	},
	Execute: executeReadSharedCard,
}

func executeReadSharedCard(ctx context.Context, runtime *common.RuntimeContext) error {
	ownerID := runtime.Str("owner-id")
	cardID := runtime.Str("card-id")
	idsData, err := runtime.CallAPITyped("GET", sharedCardIDsPath, map[string]interface{}{
		"owner_id": ownerID,
		"card_id":  cardID,
	}, nil)
	if err != nil {
		return err
	}
	rawMessageIDs, ok := idsData["message_ids"].([]interface{})
	if !ok {
		return mailInvalidResponseError("shared-card message lookup response missing message_ids")
	}
	messageIDs, err := sharedCardStringSlice(rawMessageIDs)
	if err != nil {
		return err
	}
	messageFormat := runtime.Str("message-format")
	if messageFormat == "id" {
		runtime.Out(sharedCardOutput{Format: messageFormat, MessageIDs: messageIDs}, nil)
		return nil
	}

	requestMessageIDs := stableUniqueStrings(messageIDs)
	messages := make([]map[string]interface{}, 0, len(requestMessageIDs))
	for start := 0; start < len(requestMessageIDs); start += sharedCardMessageBatchSize {
		end := min(start+sharedCardMessageBatchSize, len(requestMessageIDs))
		detailData, err := runtime.CallAPITyped("POST", sharedCardDetailsPath, sharedCardParams(runtime, detailFormat(messageFormat)), map[string]interface{}{
			"message_ids": requestMessageIDs[start:end],
		})
		if err != nil {
			return err
		}
		rawMessages, ok := detailData["messages"].([]interface{})
		if !ok {
			return mailInvalidResponseError("shared-card message detail response missing messages")
		}
		batch, err := sharedCardMapSlice(rawMessages)
		if err != nil {
			return err
		}
		messages = append(messages, batch...)
	}
	if runtime.Bool("include-attachment-download-urls") {
		for _, message := range messages {
			if err := addSharedCardAttachmentURLs(runtime, ownerID, cardID, message); err != nil {
				return err
			}
		}
	}
	switch messageFormat {
	case "full", "plain_text_full":
		normalized := make([]map[string]interface{}, 0, len(messages))
		for _, message := range messages {
			normalized = append(normalized, buildMessageOutput(message, messageFormat == "full"))
		}
		messages = normalized
	case "summary":
		messages = sharedCardSummaries(messages)
	}
	runtime.Out(sharedCardOutput{Format: messageFormat, Messages: messages}, nil)
	return nil
}

func sharedCardParams(runtime *common.RuntimeContext, format string) map[string]interface{} {
	params := map[string]interface{}{
		"owner_id": runtime.Str("owner-id"),
		"card_id":  runtime.Str("card-id"),
	}
	if format != "" {
		params["format"] = format
	}
	return params
}

func detailFormat(format string) string {
	if format == "summary" {
		return "metadata"
	}
	return format
}

func addSharedCardAttachmentURLs(runtime *common.RuntimeContext, ownerID, cardID string, message map[string]interface{}) error {
	attachmentIDs, err := attachmentIDs(message["attachments"])
	if err != nil {
		return err
	}
	attachmentIDs = stableUniqueStrings(attachmentIDs)
	message["attachment_download_urls"] = []interface{}{}
	message["attachment_failed_ids"] = []interface{}{}
	if len(attachmentIDs) == 0 {
		return nil
	}
	messageID := sharedCardFirstString(message, "message_id", "id")
	if messageID == "" {
		return mailInvalidResponseError("shared-card message detail response has attachments but no message_id")
	}
	downloadURLs := make([]interface{}, 0, len(attachmentIDs))
	failedIDs := make([]interface{}, 0)
	for start := 0; start < len(attachmentIDs); start += sharedCardAttachmentBatchSize {
		end := min(start+sharedCardAttachmentBatchSize, len(attachmentIDs))
		data, err := runtime.CallAPITyped("POST", sharedCardAttachmentURLsPath, map[string]interface{}{
			"owner_id":   ownerID,
			"card_id":    cardID,
			"message_id": messageID,
		}, map[string]interface{}{"attachment_ids": attachmentIDs[start:end]})
		if err != nil {
			return err
		}
		if urls, ok := data["download_urls"].([]interface{}); ok {
			downloadURLs = append(downloadURLs, urls...)
		}
		if failed, ok := data["failed_ids"].([]interface{}); ok {
			failedIDs = append(failedIDs, failed...)
		}
	}
	message["attachment_download_urls"] = downloadURLs
	message["attachment_failed_ids"] = failedIDs
	return nil
}

func sharedCardSummaries(messages []map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(messages))
	for _, message := range messages {
		summary := map[string]interface{}{
			"message_id":   sharedCardFirstString(message, "message_id", "id"),
			"body_preview": strVal(message["body_preview"]),
		}
		if urls, ok := message["attachment_download_urls"]; ok {
			summary["attachment_download_urls"] = urls
		}
		if failed, ok := message["attachment_failed_ids"]; ok {
			summary["attachment_failed_ids"] = failed
		}
		out = append(out, summary)
	}
	return out
}

func attachmentIDs(value interface{}) ([]string, error) {
	switch value.(type) {
	case nil:
		return nil, nil
	}
	items, ok := value.([]interface{})
	if !ok {
		return nil, mailInvalidResponseError("shared-card message attachments must be an array")
	}
	ids := make([]string, 0, len(items))
	for i, item := range items {
		attachment, ok := item.(map[string]interface{})
		if !ok {
			return nil, mailInvalidResponseError(fmt.Sprintf("shared-card attachment at index %d must be an object", i))
		}
		if id := sharedCardFirstString(attachment, "id", "attachment_id"); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func sharedCardStringSlice(value interface{}) ([]string, error) {
	items, ok := value.([]interface{})
	if !ok {
		return nil, mailInvalidResponseError("shared-card message_ids must be an array")
	}
	out := make([]string, 0, len(items))
	for i, item := range items {
		value, ok := item.(string)
		if !ok || value == "" {
			return nil, mailInvalidResponseError(fmt.Sprintf("shared-card message_id at index %d must be a non-empty string", i))
		}
		out = append(out, value)
	}
	return out, nil
}

func stableUniqueStrings(values []string) []string {
	unique := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func sharedCardMapSlice(value interface{}) ([]map[string]interface{}, error) {
	items, ok := value.([]interface{})
	if !ok {
		return nil, mailInvalidResponseError("shared-card messages must be an array")
	}
	out := make([]map[string]interface{}, 0, len(items))
	for i, item := range items {
		value, ok := item.(map[string]interface{})
		if !ok {
			return nil, mailInvalidResponseError(fmt.Sprintf("shared-card message at index %d must be an object", i))
		}
		out = append(out, value)
	}
	return out, nil
}

func sharedCardFirstString(values map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}
