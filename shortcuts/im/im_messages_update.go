// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var ImMessagesUpdate = common.Shortcut{
	Service:     "im",
	Command:     "+messages-update",
	Description: "Update a sent text or post message; bot-only; edits messages sent by the current app",
	Risk:        "write",
	Scopes:      []string{"im:message:update"},
	BotScopes:   []string{"im:message:update"},
	AuthTypes:   []string{"bot"},
	Flags: []common.Flag{
		{Name: "message-id", Desc: "message ID to update (om_xxx)", Required: true},
		{Name: "msg-type", Default: "text", Desc: "message type for --content JSON; --text forces text and --markdown forces post", Enum: []string{"text", "post"}},
		{Name: "content", Desc: "(one of --content/--text/--markdown required) message content JSON for text or post"},
		{Name: "text", Desc: "plain text message (auto-wrapped as JSON)"},
		{Name: "markdown", Desc: "markdown text (auto-wrapped as post format with style optimization; image URLs auto-resolved)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		messageID := runtime.Str("message-id")
		msgType, content, desc := buildUpdateMessageContentForDryRun(runtime)

		body := map[string]interface{}{"msg_type": msgType, "content": content}
		d := common.NewDryRunAPI()
		if desc != "" {
			d.Desc(desc)
		}
		return d.
			PUT("/open-apis/im/v1/messages/:message_id").
			Body(body).
			Set("message_id", messageID)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		messageID := runtime.Str("message-id")
		if messageID == "" {
			return output.ErrValidation("--message-id is required (om_xxx)")
		}
		if _, err := validateMessageID(messageID); err != nil {
			return err
		}

		msgType := runtime.Str("msg-type")
		content := runtime.Str("content")
		text := runtime.Str("text")
		markdown := runtime.Str("markdown")

		set := 0
		for _, v := range []string{content, text, markdown} {
			if v != "" {
				set++
			}
		}
		if set != 1 {
			return output.ErrValidation("exactly one of --content, --text, or --markdown is required")
		}
		if content != "" && !json.Valid([]byte(content)) {
			return output.ErrValidation("--content is not valid JSON: %s\nexample: --content '{\"text\":\"hello\"}' or --text 'hello'", content)
		}
		if msgType != "text" && msgType != "post" {
			return output.ErrValidation("--msg-type must be text or post; editing image, file, audio, video, sticker, and interactive card messages is not supported")
		}

		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		messageID := runtime.Str("message-id")
		msgType, content, err := buildUpdateMessageContent(ctx, runtime)
		if err != nil {
			return err
		}

		resData, err := runtime.DoAPIJSON(http.MethodPut,
			fmt.Sprintf("/open-apis/im/v1/messages/%s", validate.EncodePathSegment(messageID)),
			nil,
			map[string]interface{}{
				"msg_type": msgType,
				"content":  content,
			})
		if err != nil {
			return err
		}

		message := resData
		if nested, ok := resData["message"].(map[string]interface{}); ok {
			message = nested
		}
		runtime.Out(map[string]interface{}{
			"message_id":  message["message_id"],
			"chat_id":     message["chat_id"],
			"update_time": common.FormatTimeWithSeconds(message["update_time"]),
			"updated":     message["updated"],
		}, nil)
		return nil
	},
}

func buildUpdateMessageContentForDryRun(runtime *common.RuntimeContext) (msgType, content, desc string) {
	if markdown := runtime.Str("markdown"); markdown != "" {
		content, desc = wrapMarkdownAsPostForDryRun(normalizeAtMentions(markdown))
		return "post", content, desc
	}
	if text := runtime.Str("text"); text != "" {
		return "text", marshalUpdateTextContent(normalizeAtMentions(text)), ""
	}
	return runtime.Str("msg-type"), runtime.Str("content"), ""
}

func buildUpdateMessageContent(ctx context.Context, runtime *common.RuntimeContext) (msgType, content string, err error) {
	if markdown := runtime.Str("markdown"); markdown != "" {
		return "post", resolveMarkdownAsPost(ctx, runtime, normalizeAtMentions(markdown)), nil
	}
	if text := runtime.Str("text"); text != "" {
		return "text", marshalUpdateTextContent(normalizeAtMentions(text)), nil
	}
	return runtime.Str("msg-type"), runtime.Str("content"), nil
}

func marshalUpdateTextContent(text string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]string{"text": text})
	return strings.TrimSpace(buf.String())
}
