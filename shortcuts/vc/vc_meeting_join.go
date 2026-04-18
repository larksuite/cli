// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var meetingNumberRe = regexp.MustCompile(`^\d{9}$`)

// validMeetingNumber checks whether s is a valid 9-digit meeting number.
func validMeetingNumber(s string) bool {
	return meetingNumberRe.MatchString(s)
}

// VCMeetingJoin joins a meeting by meeting number via /vc/v1/bots/join.
var VCMeetingJoin = common.Shortcut{
	Service:     "vc",
	Command:     "+meeting-join",
	Description: "Join a meeting by meeting number (bot join)",
	Risk:        "write",
	Scopes:      []string{"vc:meeting.bot.join:write"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "meeting-number", Required: true, Desc: "meeting number to join"},
		{Name: "password", Desc: "meeting password (if required)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mn := strings.TrimSpace(runtime.Str("meeting-number"))
		if !validMeetingNumber(mn) {
			return common.FlagErrorf("--meeting-number must be exactly 9 digits, got %q", mn)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := buildMeetingJoinBody(runtime)
		return common.NewDryRunAPI().
			POST("/open-apis/vc/v1/bots/join").
			Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := buildMeetingJoinBody(runtime)
		data, err := runtime.DoAPIJSON("POST", "/open-apis/vc/v1/bots/join", nil, body)
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]interface{}{}
		}
		runtime.OutFormat(data, nil, func(w io.Writer) {
			meeting, _ := data["meeting"].(map[string]interface{})
			if meeting == nil {
				fmt.Fprintln(w, "Joined meeting (no meeting info returned).")
				return
			}
			fmt.Fprintf(w, "Joined meeting successfully.\n")
			if id := common.GetString(meeting, "id"); id != "" {
				fmt.Fprintf(w, "  Meeting ID:  %s\n", id)
			}
			if no := common.GetString(meeting, "meeting_no"); no != "" {
				fmt.Fprintf(w, "  Meeting No:  %s\n", no)
			}
			if topic := common.GetString(meeting, "topic"); topic != "" {
				fmt.Fprintf(w, "  Topic:       %s\n", topic)
			}
			if startTime := common.GetString(meeting, "start_time"); startTime != "" {
				fmt.Fprintf(w, "  Start Time:  %s\n", startTime)
			}
		})
		return nil
	},
}

func buildMeetingJoinBody(runtime *common.RuntimeContext) map[string]interface{} {
	meetingNo := strings.TrimSpace(runtime.Str("meeting-number"))
	body := map[string]interface{}{
		"join_type": 1,
		"join_identify": map[string]interface{}{
			"meeting_no": meetingNo,
		},
	}
	if pw := strings.TrimSpace(runtime.Str("password")); pw != "" {
		body["password"] = pw
	}
	return body
}
