// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const meetingBotEndPath = "/open-apis/vc/v1/bots/end"

type meetingEndRequest struct {
	MeetingID string `json:"meeting_id"`
}

// VCMeetingEnd ends a meeting as the app bot when that bot is the current host.
var VCMeetingEnd = common.Shortcut{
	Service:     "vc",
	Command:     "+meeting-end",
	Description: "End a meeting as the host app bot",
	Risk:        "high-risk-write",
	Scopes:      []string{"vc:meeting.bot.manage:write"},
	AuthTypes:   []string{"bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "meeting-id", Required: true, Desc: "meeting ID to end"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateMeetingIDFlag(runtime.Str("meeting-id"))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().POST(meetingBotEndPath).Body(buildMeetingEndBody(runtime))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		data, err := runtime.CallAPITyped(http.MethodPost, meetingBotEndPath, nil, buildMeetingEndBody(runtime))
		if err != nil {
			return err
		}
		meetingID := strings.TrimSpace(runtime.Str("meeting-id"))
		if data == nil {
			data = map[string]interface{}{}
		}
		output := make(map[string]interface{}, len(data)+1)
		for key, value := range data {
			output[key] = value
		}
		output["meeting_id"] = meetingID
		runtime.OutFormat(output, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Ended meeting %s.\n", meetingID)
		})
		return nil
	},
}

func buildMeetingEndBody(runtime *common.RuntimeContext) meetingEndRequest {
	return meetingEndRequest{MeetingID: strings.TrimSpace(runtime.Str("meeting-id"))}
}

func validateMeetingIDFlag(value string) error {
	return validateMeetingEventsMeetingID(value)
}
