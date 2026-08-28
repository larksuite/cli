// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var ImMessagesEdit = common.Shortcut{
	Service:     "im",
	Command:     "+messages-edit",
	Description: "Edit a sent text or rich-text message, optionally updating its attachment zone; bot only (the edit API does not accept user tokens)",
	Risk:        "write",
	Scopes:      []string{"im:message:send_as_bot"},
	BotScopes:   []string{"im:message:send_as_bot"},
	AuthTypes:   []string{"bot"},
	Flags: []common.Flag{
		{Name: "message-id", Desc: "message ID (om_xxx)", Required: true},
		{Name: "msg-type", Default: "text", Desc: "message type for --content JSON; when using --markdown/--text the effective type is inferred automatically", Enum: []string{"text", "post"}},
		{Name: "content", Desc: "(one of --content/--text/--markdown/--set-attachments required) message content JSON"},
		{Name: "text", Desc: "plain text message (auto-wrapped as JSON)"},
		{Name: "markdown", Desc: "markdown text (auto-wrapped as post format with style optimization; image URLs auto-resolved)"},
		{Name: "set-attachments", Type: "string_slice", Desc: "file/folder key (file_xxx), repeatable; sets the post message's attachment zone (requires --markdown or --msg-type post)"},
		{Name: "clear-attachments", Type: "bool", Desc: "clear the post message attachment zone (sets files:[]); cannot be used with --set-attachments"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		messageId := runtime.Str("message-id")
		msgType := runtime.Str("msg-type")
		content := runtime.Str("content")
		desc := ""
		text := runtime.Str("text")
		markdown := runtime.Str("markdown")

		clearAttachments := runtime.Bool("clear-attachments")

		if markdown != "" {
			msgType = "post"
			content, desc = wrapMarkdownAsPostForDryRun(markdown)
		} else if text != "" {
			jsonBytes, _ := json.Marshal(map[string]string{"text": text})
			content = string(jsonBytes)
		}

		// Attachment zone: merge --set-attachments files into the post content.
		attachments := runtime.StrSlice("set-attachments")
		if clearAttachments {
			if len(attachments) > 0 {
				return common.NewDryRunAPI().Set("error", errs.NewValidationError(errs.SubtypeInvalidArgument, "--clear-attachments cannot be used with --set-attachments").WithParam("--clear-attachments").Error())
			}
			msgType = "post"
			if content == "" {
				content = `{"zh_cn":{"content":[]}}`
			}
			if cleared, err := clearAttachmentsInPostContent(content); err == nil {
				content = cleared
			} else {
				return common.NewDryRunAPI().Set("error", errs.NewValidationError(errs.SubtypeInvalidArgument, "--clear-attachments: %v", err).WithParam("--clear-attachments").Error())
			}
			if desc != "" {
				desc += "; "
			}
			desc += "--clear-attachments sets files:[]"
		} else if len(attachments) > 0 {
			msgType = "post"
			if items, err := parseAttachments(attachments, "--set-attachments"); err == nil {
				if content == "" {
					content = `{"zh_cn":{"content":[]}}`
				}
				if merged, err := replaceAttachmentsIntoPostContent(content, items); err == nil {
					content = merged
				}
			}
			if desc != "" {
				desc += "; "
			}
			desc += "--set-attachments sets files in the post attachment zone"
		}

		body := map[string]interface{}{"msg_type": msgType, "content": content}
		d := common.NewDryRunAPI()
		if desc != "" {
			d.Desc(desc)
		}
		return d.
			PUT(fmt.Sprintf("/open-apis/im/v1/messages/%s", validate.EncodePathSegment(messageId))).
			Body(body).
			Set("message_id", messageId)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		messageId := runtime.Str("message-id")
		msgType := runtime.Str("msg-type")
		content := runtime.Str("content")
		text := runtime.Str("text")
		markdown := runtime.Str("markdown")

		if messageId == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--message-id is required (om_xxx)").WithParam("--message-id")
		}
		if _, err := validateMessageID(messageId); err != nil {
			return err
		}

		attachments, err := validateAttachmentFlags(runtime.StrSlice("set-attachments"), msgType, markdown, "--set-attachments", runtime.Cmd != nil && runtime.Cmd.Flags().Changed("msg-type"), text)
		if err != nil {
			return err
		}

		clearAttachments := runtime.Bool("clear-attachments")
		if clearAttachments && len(attachments) > 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--clear-attachments cannot be used with --set-attachments").WithParam("--clear-attachments")
		}
		if clearAttachments && markdown == "" && msgType != "post" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--clear-attachments only applies to post messages; use --markdown or --msg-type post with --content").WithParam("--clear-attachments")
		}
		// Mutual exclusion: --content already declares a files array, so the
		// attachment flags (--set-attachments / --clear-attachments) are another
		// attachment-zone source and must not be combined with it.
		if hasContentFiles(content) && (len(attachments) > 0 || clearAttachments) {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--set-attachments/--clear-attachments cannot be used with --content that already contains a files array; either edit files via --content or via the attachment flags, not both").WithParam("--set-attachments")
		}

		if msg := validateEditContentFlags(text, markdown, content, attachments, clearAttachments); msg != "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", msg)
		}
		if content != "" && !json.Valid([]byte(content)) {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--content is not valid JSON: %s\nexample: --content '{\"text\":\"hello\"}' or --text 'hello'", content).WithParam("--content")
		}
		if msg := validateExplicitMsgType(runtime.Cmd, msgType, text, markdown, "", "", "", ""); msg != "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", msg).WithParam("--msg-type")
		}

		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		messageId := runtime.Str("message-id")
		msgType := runtime.Str("msg-type")
		content := runtime.Str("content")
		text := runtime.Str("text")
		markdown := runtime.Str("markdown")

		if markdown != "" {
			msgType, content = "post", resolveMarkdownAsPost(ctx, runtime, markdown)
		} else if text != "" {
			jsonBytes, _ := json.Marshal(map[string]string{"text": text})
			content = string(jsonBytes)
		}

		// Attachment zone: merge --set-attachments files into the post content.
		attachments := runtime.StrSlice("set-attachments")
		clearAttachments := runtime.Bool("clear-attachments")
		if clearAttachments {
			if len(attachments) > 0 {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--clear-attachments cannot be used with --set-attachments").WithParam("--clear-attachments")
			}
			msgType = "post"
			if content == "" {
				content = `{"zh_cn":{"content":[]}}`
			}
			cleared, err := clearAttachmentsInPostContent(content)
			if err != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--clear-attachments: %v", err).WithParam("--clear-attachments")
			}
			content = cleared
		} else if items, err := parseAttachments(attachments, "--set-attachments"); err == nil && len(items) > 0 {
			msgType = "post"
			if content == "" {
				content = `{"zh_cn":{"content":[]}}`
			}
			merged, err := replaceAttachmentsIntoPostContent(content, items)
			if err != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--set-attachments: %v", err).WithParam("--set-attachments")
			}
			content = merged
		}

		normalizedContent := content
		if msgType == "text" || msgType == "post" {
			normalizedContent = normalizeAtMentions(content)
		}

		data := map[string]interface{}{
			"msg_type": msgType,
			"content":  normalizedContent,
		}

		resData, err := runtime.DoAPIJSONTyped(http.MethodPut,
			fmt.Sprintf("/open-apis/im/v1/messages/%s", validate.EncodePathSegment(messageId)),
			nil, data)
		if err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{
			"message_id":  resData["message_id"],
			"chat_id":     resData["chat_id"],
			"update_time": common.FormatTimeWithSeconds(resData["update_time"]),
		}, nil)
		return nil
	},
}

// validateEditContentFlags checks the edit content flags: text/markdown/content
// are mutually exclusive; --set-attachments counts as a content source so a post with
// only an attachment zone is allowed. --clear-attachments also counts as a content
// source: a clear-only edit (no body flag) is legal — Execute falls back to an empty
// post body and clears the attachment zone, matching the server's clear semantics.
func validateEditContentFlags(text, markdown, content string, attachments []attachmentItem, clearAttachments bool) string {
	contentFlags := 0
	if text != "" {
		contentFlags++
	}
	if markdown != "" {
		contentFlags++
	}
	if content != "" {
		contentFlags++
	}
	if contentFlags > 1 {
		return "--text, --markdown, and --content cannot be specified together"
	}
	if contentFlags == 0 && len(attachments) == 0 && !clearAttachments {
		return "specify --content <json>, --text <plain text>, --markdown <markdown text>, --set-attachments <file_key>, or --clear-attachments"
	}
	return ""
}
