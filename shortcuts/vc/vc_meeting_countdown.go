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
	meetingCountdownActionSet          = "set"
	meetingCountdownActionProlong      = "prolong"
	meetingCountdownActionEndInAdvance = "end_in_advance"
	meetingCountdownActionCloseWindow  = "close_window"
)

var meetingCountdownActions = []string{
	meetingCountdownActionSet,
	meetingCountdownActionProlong,
	meetingCountdownActionEndInAdvance,
	meetingCountdownActionCloseWindow,
}

// VCMeetingCountdown operates an in-meeting countdown.
var VCMeetingCountdown = common.Shortcut{
	Service:     "vc",
	Command:     "+meeting-countdown",
	Description: "Operate an in-meeting countdown",
	Risk:        "write",
	Scopes:      []string{"vc:meeting.interaction:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "meeting-id", Required: true, Desc: "meeting ID to operate"},
		{Name: "action", Required: true, Desc: "countdown action: set, prolong, end_in_advance, or close_window", Enum: meetingCountdownActions},
		{Name: "duration", Type: "int", Desc: "countdown duration in minutes; required for set and prolong"},
		{Name: "need-play-audio-at-end", Type: "bool", Desc: "play audio when a set countdown ends"},
		{Name: "reminder-before-end", Type: "int", Desc: "single reminder offset in minutes before countdown end"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateMeetingEventsMeetingID(runtime.Str("meeting-id")); err != nil {
			return err
		}
		_, err := buildMeetingCountdownBody(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, err := buildMeetingCountdownBody(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST(buildMeetingCountdownPath()).
			Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body, err := buildMeetingCountdownBody(runtime)
		if err != nil {
			return err
		}
		action, _ := body["action"].(string)
		data, err := runtime.CallAPITyped(http.MethodPost, buildMeetingCountdownPath(), nil, body)
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]interface{}{}
		}
		runtime.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintln(w, "Meeting countdown operated.")
			fmt.Fprintf(w, "  Action:  %s\n", action)
		})
		return nil
	},
}

func buildMeetingCountdownPath() string {
	return "/open-apis/vc/v1/bots/countdown"
}

func buildMeetingCountdownBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	meetingID := strings.TrimSpace(runtime.Str("meeting-id"))
	action := strings.ToLower(strings.TrimSpace(runtime.Str("action")))
	if err := validateMeetingCountdownAction(action); err != nil {
		return nil, err
	}

	duration := runtime.Int("duration")
	durationSet := runtime.Changed("duration")
	reminder := runtime.Int("reminder-before-end")
	reminderSet := runtime.Changed("reminder-before-end")
	audioAtEndSet := runtime.Changed("need-play-audio-at-end")
	if err := validateMeetingCountdownDuration(action, duration, durationSet); err != nil {
		return nil, err
	}
	if err := validateMeetingCountdownReminder(action, duration, reminder, reminderSet); err != nil {
		return nil, err
	}
	if action != meetingCountdownActionSet && audioAtEndSet {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--need-play-audio-at-end is only supported when --action set").WithParam("--need-play-audio-at-end")
	}

	body := map[string]interface{}{
		"meeting_id": meetingID,
		"action":     action,
	}
	if action == meetingCountdownActionSet || action == meetingCountdownActionProlong {
		body["duration"] = duration
	}
	if action == meetingCountdownActionSet {
		if audioAtEndSet {
			body["need_play_audio_at_end"] = runtime.Bool("need-play-audio-at-end")
		}
		if reminderSet {
			body["reminder_before_end"] = reminder
		}
	}
	return body, nil
}

func validateMeetingCountdownAction(action string) error {
	switch action {
	case meetingCountdownActionSet, meetingCountdownActionProlong, meetingCountdownActionEndInAdvance, meetingCountdownActionCloseWindow:
		return nil
	case "":
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--action is required").WithParam("--action")
	default:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--action must be one of set, prolong, end_in_advance, close_window").WithParam("--action")
	}
}

func validateMeetingCountdownDuration(action string, duration int, durationSet bool) error {
	switch action {
	case meetingCountdownActionSet, meetingCountdownActionProlong:
		if !durationSet || duration <= 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--duration must be a positive number of minutes when --action is set or prolong").WithParam("--duration")
		}
	default:
		if durationSet {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--duration is only supported when --action is set or prolong").WithParam("--duration")
		}
	}
	return nil
}

func validateMeetingCountdownReminder(action string, duration int, reminder int, reminderSet bool) error {
	if !reminderSet {
		return nil
	}
	if action != meetingCountdownActionSet {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--reminder-before-end is only supported when --action set").WithParam("--reminder-before-end")
	}
	if reminder <= 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--reminder-before-end must be one positive number of minutes").WithParam("--reminder-before-end")
	}
	if reminder >= duration {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--reminder-before-end must be less than --duration in minutes").WithParam("--reminder-before-end")
	}
	return nil
}
