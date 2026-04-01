// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

var ImMessagesSend = common.Shortcut{
	Service:     "im",
	Command:     "+messages-send",
	Description: "Send a message to a chat or direct message with bot identity; bot-only; sends to chat-id or user-id with text/markdown/post/media, supports idempotency key",
	Risk:        "write",
	Scopes:      []string{"im:message:send_as_bot"},
	AuthTypes:   []string{"bot"},
	Flags: []common.Flag{
		{Name: "chat-id", Desc: "(required, mutually exclusive with --user-id) chat ID (oc_xxx)"},
		{Name: "user-id", Desc: "(required, mutually exclusive with --chat-id) user open_id (ou_xxx)"},
		{Name: "msg-type", Default: "text", Desc: "message type for --content JSON; when using --text/--markdown/--image/--file/--video/--audio, the effective type is inferred automatically", Enum: []string{"text", "post", "image", "file", "audio", "media", "interactive", "share_chat", "share_user"}},
		{Name: "content", Desc: "(one of --content/--text/--markdown/--image/--file/--video/--audio required) message content JSON"},
		{Name: "text", Desc: "plain text message (auto-wrapped as JSON)"},
		{Name: "markdown", Desc: "markdown text (auto-wrapped as post format with style optimization; image URLs auto-resolved)"},
		{Name: "idempotency-key", Desc: "idempotency key (prevents duplicate sends)"},
		{Name: "image", Desc: "image_key, local file path"},
		{Name: "file", Desc: "file_key, local file path"},
		{Name: "video", Desc: "video file_key, local file path; must be used together with --video-cover"},
		{Name: "video-cover", Desc: "video cover image_key, local file path; required when using --video"},
		{Name: "audio", Desc: "audio file_key, local file path"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		chatFlag := runtime.Str("chat-id")
		userFlag := runtime.Str("user-id")
		msgType := runtime.Str("msg-type")
		content := runtime.Str("content")
		desc := ""
		text := runtime.Str("text")
		markdown := runtime.Str("markdown")
		idempotencyKey := runtime.Str("idempotency-key")
		imageKey := runtime.Str("image")
		fileKey := runtime.Str("file")
		videoKey := runtime.Str("video")
		videoCoverKey := runtime.Str("video-cover")
		audioKey := runtime.Str("audio")

		if markdown != "" {
			msgType = "post"
			content, desc = wrapMarkdownAsPostForDryRun(markdown)
		} else if mt, c, d := buildMediaContentFromKey(text, imageKey, fileKey, videoKey, videoCoverKey, audioKey); mt != "" {
			msgType, content, desc = mt, c, d
		}

		receiveIdType := "chat_id"
		receiveId := chatFlag
		if userFlag != "" {
			receiveIdType = "open_id"
			receiveId = userFlag
		}

		if msgType == "text" || msgType == "post" {
			content = normalizeAtMentions(content)
		}

		body := map[string]interface{}{"receive_id": receiveId, "msg_type": msgType, "content": content}
		if idempotencyKey != "" {
			body["uuid"] = idempotencyKey
		}

		d := common.NewDryRunAPI()
		if desc != "" {
			d.Desc(desc)
		}
		return d.
			POST("/open-apis/im/v1/messages").
			Params(map[string]interface{}{"receive_id_type": receiveIdType}).
			Body(body)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		chatFlag := runtime.Str("chat-id")
		userFlag := runtime.Str("user-id")
		msgType := runtime.Str("msg-type")
		content := runtime.Str("content")
		text := runtime.Str("text")
		markdown := runtime.Str("markdown")
		imageKey := runtime.Str("image")
		fileKey := runtime.Str("file")
		videoKey := runtime.Str("video")
		videoCoverKey := runtime.Str("video-cover")
		audioKey := runtime.Str("audio")

		if !isMediaKey(imageKey) {
			if _, err := validate.SafeLocalFlagPath("--image", imageKey); err != nil {
				return output.ErrValidation("%v", err)
			}
		}
		if !isMediaKey(fileKey) {
			if _, err := validate.SafeLocalFlagPath("--file", fileKey); err != nil {
				return output.ErrValidation("%v", err)
			}
		}
		if !isMediaKey(videoKey) {
			if _, err := validate.SafeLocalFlagPath("--video", videoKey); err != nil {
				return output.ErrValidation("%v", err)
			}
		}
		if !isMediaKey(videoCoverKey) {
			if _, err := validate.SafeLocalFlagPath("--video-cover", videoCoverKey); err != nil {
				return output.ErrValidation("%v", err)
			}
		}
		if !isMediaKey(audioKey) {
			if _, err := validate.SafeLocalFlagPath("--audio", audioKey); err != nil {
				return output.ErrValidation("%v", err)
			}
		}

		if err := common.ExactlyOne(runtime, "chat-id", "user-id"); err != nil {
			return err
		}

		// Validate ID formats
		if chatFlag != "" {
			if _, err := common.ValidateChatID(chatFlag); err != nil {
				return err
			}
		}
		if userFlag != "" {
			if _, err := common.ValidateUserID(userFlag); err != nil {
				return err
			}
		}

		if msg := validateContentFlags(text, markdown, content, imageKey, fileKey, videoKey, videoCoverKey, audioKey); msg != "" {
			return common.FlagErrorf(msg)
		}
		if content != "" && !json.Valid([]byte(content)) {
			return common.FlagErrorf("--content is not valid JSON: %s\nexample: --content '{\"text\":\"hello\"}' or --text 'hello'", content)
		}
		if msg := validateExplicitMsgType(runtime.Cmd, msgType, text, markdown, imageKey, fileKey, videoKey, audioKey); msg != "" {
			return common.FlagErrorf(msg)
		}

		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		chatFlag := runtime.Str("chat-id")
		userFlag := runtime.Str("user-id")
		msgType := runtime.Str("msg-type")
		content := runtime.Str("content")
		text := runtime.Str("text")
		markdown := runtime.Str("markdown")
		idempotencyKey := runtime.Str("idempotency-key")
		imageVal := runtime.Str("image")
		fileVal := runtime.Str("file")
		videoVal := runtime.Str("video")
		videoCoverVal := runtime.Str("video-cover")
		audioVal := runtime.Str("audio")
		if !isMediaKey(imageVal) {
			if _, err := validate.SafeLocalFlagPath("--image", imageVal); err != nil {
				return output.ErrValidation("%v", err)
			}
		}
		if !isMediaKey(fileVal) {
			if _, err := validate.SafeLocalFlagPath("--file", fileVal); err != nil {
				return output.ErrValidation("%v", err)
			}
		}
		if !isMediaKey(videoVal) {
			if _, err := validate.SafeLocalFlagPath("--video", videoVal); err != nil {
				return output.ErrValidation("%v", err)
			}
		}
		if !isMediaKey(videoCoverVal) {
			if _, err := validate.SafeLocalFlagPath("--video-cover", videoCoverVal); err != nil {
				return output.ErrValidation("%v", err)
			}
		}
		if !isMediaKey(audioVal) {
			if _, err := validate.SafeLocalFlagPath("--audio", audioVal); err != nil {
				return output.ErrValidation("%v", err)
			}
		}
		// Resolve content type
		if markdown != "" {
			msgType, content = "post", resolveMarkdownAsPost(ctx, runtime, markdown)
		} else if mt, c, err := resolveMediaContent(ctx, runtime, text, imageVal, fileVal, videoVal, videoCoverVal, audioVal); err != nil {
			return err
		} else if mt != "" {
			msgType, content = mt, c
		}

		receiveIdType := "chat_id"
		receiveId := chatFlag
		if userFlag != "" {
			receiveIdType = "open_id"
			receiveId = userFlag
		}

		normalizedContent := content
		if msgType == "text" || msgType == "post" {
			normalizedContent = normalizeAtMentions(content)
		}

		data := map[string]interface{}{
			"receive_id": receiveId,
			"msg_type":   msgType,
			"content":    normalizedContent,
		}
		if idempotencyKey != "" {
			data["uuid"] = idempotencyKey
		}

		resData, err := runtime.DoAPIJSON(http.MethodPost, "/open-apis/im/v1/messages",
			larkcore.QueryParams{"receive_id_type": []string{receiveIdType}}, data)
		if err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{
			"message_id":  resData["message_id"],
			"chat_id":     resData["chat_id"],
			"create_time": common.FormatTimeWithSeconds(resData["create_time"]),
		}, nil)
		return nil
	},
}

// isMediaKey returns true if the value looks like an existing API key rather than a local file path.
func isMediaKey(value string) bool {
	return strings.HasPrefix(value, "img_") || strings.HasPrefix(value, "file_")
}
