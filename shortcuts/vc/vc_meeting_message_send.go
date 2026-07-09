// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	meetingMessageTypeText     = "text"
	meetingMessageTypeReaction = "reaction"
	// Keep the client-side cap below the server-side content limit.
	meetingMessageMaxTextBytes = 48 * 1024
	meetingMessageMaxUUIDBytes = 128
)

// VCMeetingMessageSend sends an in-meeting text message or reaction emoji.
var VCMeetingMessageSend = common.Shortcut{
	Service:     "vc",
	Command:     "+meeting-message-send",
	Description: "Send an in-meeting text message or reaction emoji",
	Risk:        "write",
	Scopes:      []string{"vc:meeting.message:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "meeting-id", Required: true, Desc: "meeting ID to send into"},
		{Name: "msg-type", Desc: "message type: text or reaction"},
		{Name: "text", Desc: "text content when --msg-type text"},
		{Name: "emoji-type", Desc: "emoji key when --msg-type reaction, for example LOVE, THUMBSUP, VC_NoSound"},
		{Name: "uuid", Desc: "optional idempotency key"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateMeetingEventsMeetingID(runtime.Str("meeting-id")); err != nil {
			return err
		}
		_, err := validateMeetingMessagePayload(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, err := buildMeetingMessageSendBody(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST(buildMeetingMessageSendPath()).
			Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body, err := buildMeetingMessageSendBody(runtime)
		if err != nil {
			return err
		}
		data, err := runtime.CallAPITyped(http.MethodPost, buildMeetingMessageSendPath(), nil, body)
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]interface{}{}
		}
		runtime.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintln(w, "Meeting message sent.")
			if msgType := common.GetString(data, "msg_type"); msgType != "" {
				fmt.Fprintf(w, "  Type:  %s\n", msgType)
			} else if msgType, _ := body["msg_type"].(string); msgType != "" {
				fmt.Fprintf(w, "  Type:  %s\n", msgType)
			}
			if uuid := common.GetString(data, "uuid"); uuid != "" {
				fmt.Fprintf(w, "  UUID:  %s\n", uuid)
			}
		})
		return nil
	},
}

func buildMeetingMessageSendPath() string {
	return "/open-apis/vc/v1/bots/message"
}

func buildMeetingMessageSendBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	msgType, err := validateMeetingMessagePayload(runtime)
	if err != nil {
		return nil, err
	}
	body := map[string]interface{}{
		"meeting_id": strings.TrimSpace(runtime.Str("meeting-id")),
		"msg_type":   msgType,
	}
	switch msgType {
	case meetingMessageTypeText:
		body["content"] = strings.TrimSpace(runtime.Str("text"))
	case meetingMessageTypeReaction:
		body["content"] = strings.TrimSpace(runtime.Str("emoji-type"))
	}
	if uuid := strings.TrimSpace(runtime.Str("uuid")); uuid != "" {
		body["uuid"] = uuid
	}
	return body, nil
}

func validateMeetingMessagePayload(runtime *common.RuntimeContext) (string, error) {
	msgType, err := resolveMeetingMessageType(runtime)
	if err != nil {
		return "", err
	}
	if msgType == meetingMessageTypeText {
		text := strings.TrimSpace(runtime.Str("text"))
		if len(text) > meetingMessageMaxTextBytes {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, fmt.Sprintf("--text is too long; max %d bytes", meetingMessageMaxTextBytes)).WithParam("--text")
		}
	}
	if uuid := strings.TrimSpace(runtime.Str("uuid")); len(uuid) > meetingMessageMaxUUIDBytes {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, fmt.Sprintf("--uuid is too long; max %d bytes", meetingMessageMaxUUIDBytes)).WithParam("--uuid")
	}
	return msgType, nil
}

func resolveMeetingMessageType(runtime *common.RuntimeContext) (string, error) {
	msgType := strings.ToLower(strings.TrimSpace(runtime.Str("msg-type")))
	text := strings.TrimSpace(runtime.Str("text"))
	emojiType := strings.TrimSpace(runtime.Str("emoji-type"))

	if msgType == "" {
		switch {
		case text != "" && emojiType == "":
			msgType = meetingMessageTypeText
		case text == "" && emojiType != "":
			msgType = meetingMessageTypeReaction
		default:
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--msg-type is required when both --text and --emoji-type are empty or both are set").WithParam("--msg-type")
		}
	}

	switch msgType {
	case meetingMessageTypeText:
		if text == "" {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--text is required when --msg-type text").WithParam("--text")
		}
		if emojiType != "" {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--emoji-type cannot be used when --msg-type text").WithParam("--emoji-type")
		}
	case meetingMessageTypeReaction:
		if emojiType == "" {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--emoji-type is required when --msg-type reaction").WithParam("--emoji-type")
		}
		if text != "" {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--text cannot be used when --msg-type reaction").WithParam("--text")
		}
	default:
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--msg-type must be text or reaction").WithParam("--msg-type")
	}
	return msgType, nil
}
