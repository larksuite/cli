// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	meetingBotJoinPath      = "/open-apis/vc/v1/bots/join"
	meetingJoinActionJoin   = "join"
	meetingJoinActionStart  = "start"
	meetingJoinStartAPIFlag = 2
)

var meetingNumberRe = regexp.MustCompile(`^\d{9}$`)

type meetingJoinIdentify struct {
	MeetingNo string `json:"meeting_no"`
}

type meetingJoinRequest struct {
	JoinType     int                 `json:"join_type"`
	JoinIdentify meetingJoinIdentify `json:"join_identify"`
	Password     string              `json:"password,omitempty"`
	CallID       string              `json:"call_id,omitempty"`
	Action       *int                `json:"action,omitempty"`
}

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
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "meeting-number", Required: true, Desc: "meeting number to join"},
		{Name: "password", Desc: "meeting password (if required)"},
		{Name: "call-id", Desc: "correlation id forwarded from invite event"},
		{Name: "action", Default: meetingJoinActionJoin, Desc: "meeting action (default: join; start initiates a Calendar meeting)", Enum: []string{meetingJoinActionJoin, meetingJoinActionStart}},
	},
	Normalize: func(_ context.Context, flags *common.FlagContext) error {
		return flags.SetCanonical("action", strings.ToLower(strings.TrimSpace(flags.Str("action"))))
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mn := strings.TrimSpace(runtime.Str("meeting-number"))
		if !validMeetingNumber(mn) {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--meeting-number must be exactly 9 digits, got %q", mn).WithParam("--meeting-number")
		}
		if meetingJoinAction(runtime) == meetingJoinActionStart && !runtime.As().IsBot() {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--action start requires --as bot").WithParam("--action")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST(meetingBotJoinPath).
			Body(buildMeetingJoinBody(runtime, meetingJoinAction(runtime)))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		action := meetingJoinAction(runtime)
		data, err := runtime.CallAPITyped("POST", meetingBotJoinPath, nil, buildMeetingJoinBody(runtime, action))
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]interface{}{}
		}
		runtime.OutFormat(data, nil, func(w io.Writer) {
			meeting, _ := data["meeting"].(map[string]interface{})
			if meeting == nil {
				if action == meetingJoinActionStart {
					fmt.Fprintln(w, "Started Calendar meeting (no meeting info returned).")
				} else {
					fmt.Fprintln(w, "Joined meeting (no meeting info returned).")
				}
				return
			}
			if action == meetingJoinActionStart {
				fmt.Fprintln(w, "Started Calendar meeting.")
			} else {
				fmt.Fprintln(w, "Joined meeting successfully.")
			}
			printMeetingSummary(w, data)
			if startTime := common.GetString(meeting, "start_time"); startTime != "" {
				fmt.Fprintf(w, "  Start Time:  %s\n", startTime)
			}
		})
		return nil
	},
}

func buildMeetingJoinBody(runtime *common.RuntimeContext, action string) meetingJoinRequest {
	body := meetingJoinRequest{
		JoinType:     1,
		JoinIdentify: meetingJoinIdentify{MeetingNo: strings.TrimSpace(runtime.Str("meeting-number"))},
	}
	if pw := strings.TrimSpace(runtime.Str("password")); pw != "" {
		body.Password = pw
	}
	if cid := strings.TrimSpace(runtime.Str("call-id")); cid != "" {
		body.CallID = cid
	}
	if action == meetingJoinActionStart {
		startAction := meetingJoinStartAPIFlag
		body.Action = &startAction
	}
	return body
}

func meetingJoinAction(runtime *common.RuntimeContext) string {
	if runtime.Str("action") == meetingJoinActionStart {
		return meetingJoinActionStart
	}
	return meetingJoinActionJoin
}

func printMeetingSummary(w io.Writer, data map[string]interface{}) {
	meeting, _ := data["meeting"].(map[string]interface{})
	if meeting == nil {
		return
	}
	if id := common.GetString(meeting, "id"); id != "" {
		fmt.Fprintf(w, "  Meeting ID:  %s\n", id)
	}
	if no := common.GetString(meeting, "meeting_no"); no != "" {
		fmt.Fprintf(w, "  Meeting No:  %s\n", no)
	}
	if topic := common.GetString(meeting, "topic"); topic != "" {
		fmt.Fprintf(w, "  Topic:       %s\n", topic)
	}
}
