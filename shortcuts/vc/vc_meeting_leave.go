// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// VCMeetingLeave leaves a meeting via /vc/v1/bots/leave.
var VCMeetingLeave = common.Shortcut{
	Service:     "vc",
	Command:     "+meeting-leave",
	Description: "Leave a meeting by meeting ID",
	Risk:        "write",
	Scopes:      []string{"vc:meeting.bot.join:write"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "meeting-id", Required: true, Desc: "meeting ID to leave"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("meeting-id")) == "" {
			return common.FlagErrorf("--meeting-id is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST("/open-apis/vc/v1/bots/leave").
			Body(map[string]interface{}{
				"meeting_id": strings.TrimSpace(runtime.Str("meeting-id")),
			})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		meetingID := strings.TrimSpace(runtime.Str("meeting-id"))
		body := map[string]interface{}{
			"meeting_id": meetingID,
		}
		data, err := runtime.DoAPIJSON("POST", "/open-apis/vc/v1/bots/leave", nil, body)
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]interface{}{}
		}
		runtime.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Left meeting %s successfully.\n", meetingID)
		})
		return nil
	},
}
