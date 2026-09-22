// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const meetingChatAPIPath = "/open-apis/vc/v1/bots/chat"

type meetingChatRequest struct {
	MeetingID string `json:"meeting_id"`
}

type meetingChatResponse struct {
	ChatID string `json:"chat_id"`
}

// VCMeetingChat explicitly creates or reuses a meeting chat for the caller.
var VCMeetingChat = common.Shortcut{
	Service:     "vc",
	Command:     "+meeting-chat",
	Description: "Create or reuse a meeting chat and return its Chat ID",
	Risk:        "write",
	Scopes:      []string{"vc:meeting.interaction:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "meeting-id", Required: true, Desc: "long numeric meeting ID (not the 9-digit meeting number)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateMeetingEventsMeetingID(runtime.Str("meeting-id"))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().POST(meetingChatAPIPath).
			Body(meetingChatRequest{MeetingID: runtime.Str("meeting-id")})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		data, err := runtime.CallAPITyped(http.MethodPost, meetingChatAPIPath, nil,
			meetingChatRequest{MeetingID: runtime.Str("meeting-id")})
		if err != nil {
			return err
		}
		result := meetingChatResponse{ChatID: common.GetString(data, "chat_id")}
		if result.ChatID == "" || result.ChatID == "0" {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "meeting chat response has no chat_id")
		}
		runtime.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Chat ID: %s\n", result.ChatID)
		})
		return nil
	},
}
