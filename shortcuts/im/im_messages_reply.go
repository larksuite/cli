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

var ImMessagesReply = common.Shortcut{
	Service:     "im",
	Command:     "+messages-reply",
	Description: "Reply to a message (supports thread replies); user/bot; content inputs cover text, Markdown, structured post JSON, and media (image/file/video/audio); reply-in-thread and idempotency key",
	Risk:        "write",
	Scopes:      []string{"im:message:send_as_bot"},
	UserScopes:  []string{"im:message.send_as_user", "im:message"},
	BotScopes:   []string{"im:message:send_as_bot"},
	AuthTypes:   []string{"bot", "user"},
	Flags: []common.Flag{
		{Name: "message-id", Desc: "message ID (om_xxx)", Required: true},
		{Name: "msg-type", Default: "text", Desc: "message type for --content JSON; when using --text/--markdown/--image/--file/--video/--audio, the effective type is inferred automatically", Enum: []string{"text", "post", "image", "file", "audio", "media", "interactive", "share_chat", "share_user"}},
		{Name: "content", Desc: "(one of --content/--text/--markdown/--image/--file/--video/--audio required) message content JSON"},
		{Name: "text", Desc: "plain text message (auto-wrapped as JSON)"},
		{Name: "markdown", Desc: "markdown text (auto-wrapped as post format with style optimization; image URLs auto-resolved)"},
		{Name: "mention", Type: "string_slice", Desc: "user_id or open_id to mention (repeatable or comma-separated; values are sent unchanged)"},
		{Name: "mention-all", Type: "bool", Desc: "mention all members using a structured at node"},
		{Name: "image", Desc: "image key (img_xxx), URL, or cwd-relative local path (absolute paths and .. are rejected)"},
		{Name: "file", Desc: "file key (file_xxx), URL, or cwd-relative local path (absolute paths and .. are rejected)"},
		{Name: "video", Desc: "video file key (file_xxx), URL, or cwd-relative local path (absolute paths and .. are rejected); must be used together with --video-cover"},
		{Name: "video-cover", Desc: "video cover image key (img_xxx), URL, or cwd-relative local path (absolute paths and .. are rejected); required when using --video"},
		{Name: "audio", Desc: audioMessageInputDesc},
		{Name: "reply-in-thread", Type: "bool", Desc: "reply in thread (message appears in thread stream instead of main chat)"},
		{Name: "idempotency-key", Desc: "idempotency key, max 50 characters (prevents duplicate sends)"},
	},
	Tips: []string{
		messageContentTip,
	},
	PostMount: installMessageFlagBehavior,
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		messageId := runtime.Str("message-id")
		msgType := runtime.Str("msg-type")
		content := runtime.Str("content")
		desc := ""
		text := runtime.Str("text")
		markdown := runtime.Str("markdown")
		imageKey := runtime.Str("image")
		fileKey := runtime.Str("file")
		videoKey := runtime.Str("video")
		videoCoverKey := runtime.Str("video-cover")
		audioKey := runtime.Str("audio")
		replyInThread := runtime.Bool("reply-in-thread")
		idempotencyKey := runtime.Str("idempotency-key")

		if markdown != "" {
			msgType = "post"
			content, desc = wrapMarkdownAsPostForDryRun(markdown)
		} else if mt, c, d := buildMediaContentFromKey(text, imageKey, fileKey, videoKey, videoCoverKey, audioKey); mt != "" {
			msgType, content, desc = mt, c, d
		}
		extra := map[string]interface{}{}
		if replyInThread {
			extra["reply_in_thread"] = true
		}
		if idempotencyKey != "" {
			extra["uuid"] = idempotencyKey
		}
		// Validate runs before DryRun in the shortcut pipeline, so request
		// construction cannot fail here.
		body, _ := buildMessageRequestBody(runtime, msgType, content, extra)

		d := common.NewDryRunAPI()
		if desc != "" {
			d.Desc(desc)
		}
		return d.
			POST("/open-apis/im/v1/messages/:message_id/reply").
			Body(body).
			Set("message_id", messageId)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		messageId := runtime.Str("message-id")
		msgType := runtime.Str("msg-type")
		content := runtime.Str("content")
		text := runtime.Str("text")
		markdown := runtime.Str("markdown")
		idempotencyKey := runtime.Str("idempotency-key")
		imageKey := runtime.Str("image")
		fileKey := runtime.Str("file")
		videoKey := runtime.Str("video")
		videoCoverKey := runtime.Str("video-cover")
		audioKey := runtime.Str("audio")

		fio := runtime.FileIO()
		for _, mf := range []struct{ flag, val string }{
			{"--image", imageKey}, {"--file", fileKey}, {"--video", videoKey},
			{"--video-cover", videoCoverKey}, {"--audio", audioKey},
		} {
			if err := validateMediaFlagPath(fio, mf.flag, mf.val); err != nil {
				return err
			}
		}
		if err := validateAudioMessageInput("--audio", audioKey); err != nil {
			return err
		}

		if messageId == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--message-id is required (om_xxx)").WithParam("--message-id")
		}
		if _, err := validateMessageID(messageId); err != nil {
			return err
		}

		if msg := validateContentFlags(text, markdown, content, imageKey, fileKey, videoKey, videoCoverKey, audioKey); msg != "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", msg)
		}
		if err := validateIdempotencyKey(idempotencyKey); err != nil {
			return err
		}
		if content != "" && !json.Valid([]byte(content)) {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--content is not valid JSON: %s\nexample: --content '{\"text\":\"hello\"}' or --text 'hello'", content).WithParam("--content")
		}
		if msg := validateExplicitMsgType(runtime.Cmd, msgType, text, markdown, imageKey, fileKey, videoKey, audioKey); msg != "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", msg).WithParam("--msg-type")
		}

		previewType, previewContent := msgType, content
		if markdown != "" {
			previewType = "post"
			previewContent, _ = wrapMarkdownAsPostForDryRun(markdown)
		} else if mt, c, _ := buildMediaContentFromKey(text, imageKey, fileKey, videoKey, videoCoverKey, audioKey); mt != "" {
			previewType, previewContent = mt, c
		}
		if _, err := buildMessageRequestBody(runtime, previewType, previewContent, nil); err != nil {
			return err
		}

		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		messageId := runtime.Str("message-id")
		msgType := runtime.Str("msg-type")
		content := runtime.Str("content")
		text := runtime.Str("text")
		markdown := runtime.Str("markdown")
		imageVal := runtime.Str("image")
		fileVal := runtime.Str("file")
		videoVal := runtime.Str("video")
		videoCoverVal := runtime.Str("video-cover")
		audioVal := runtime.Str("audio")
		replyInThread := runtime.Bool("reply-in-thread")
		idempotencyKey := runtime.Str("idempotency-key")
		fio := runtime.FileIO()
		for _, mf := range []struct{ flag, val string }{
			{"--image", imageVal}, {"--file", fileVal}, {"--video", videoVal},
			{"--video-cover", videoCoverVal}, {"--audio", audioVal},
		} {
			if err := validateMediaFlagPath(fio, mf.flag, mf.val); err != nil {
				return err
			}
		}

		if markdown != "" {
			post, err := resolveMarkdownAsPost(ctx, runtime, markdown)
			if err != nil {
				return err
			}
			msgType, content = "post", post
		} else if mt, c, err := resolveMediaContent(ctx, runtime, text, imageVal, fileVal, videoVal, videoCoverVal, audioVal); err != nil {
			return err
		} else if mt != "" {
			msgType, content = mt, c
		}

		extra := map[string]interface{}{}
		if replyInThread {
			extra["reply_in_thread"] = true
		}
		if idempotencyKey != "" {
			extra["uuid"] = idempotencyKey
		}
		data, err := buildMessageRequestBody(runtime, msgType, content, extra)
		if err != nil {
			return err
		}

		resData, err := runtime.DoWriteAPIJSONTyped(http.MethodPost,
			fmt.Sprintf("/open-apis/im/v1/messages/%s/reply", validate.EncodePathSegment(messageId)),
			nil, data)
		if err != nil {
			return err
		}

		result := map[string]interface{}{
			"message_id":  resData["message_id"],
			"chat_id":     resData["chat_id"],
			"create_time": common.FormatTimeWithSeconds(resData["create_time"]),
		}
		if err := addMessageMentionResult(runtime, resData, result); err != nil {
			return err
		}
		runtime.Out(result, nil)
		return nil
	},
}
