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
	meetingParticipantManageScope = "vc:meeting.bot.manage:write"
	meetingParticipantMutePath    = "/open-apis/v1/bots/mute"
	meetingParticipantUnmutePath  = "/open-apis/v1/bots/unmute"
)

var meetingParticipantAudioFlags = []common.Flag{
	{Name: "meeting-id", Required: true, Desc: "meeting ID"},
	{Name: "target-user-id", Required: true, Desc: "target participant user ID"},
	{Name: "user-id-type", Default: "open_id", Desc: "target user ID type", Enum: []string{"open_id", "union_id", "user_id"}},
}

// VCMeetingParticipantMute mutes a participant in a meeting.
var VCMeetingParticipantMute = newMeetingParticipantAudioShortcut(
	"+meeting-participant-mute",
	"Mute a participant in a meeting",
	meetingParticipantMutePath,
	"mute",
	"completed",
	"Participant muted.",
)

// VCMeetingParticipantUnmute asks a participant to unmute in a meeting.
var VCMeetingParticipantUnmute = newMeetingParticipantAudioShortcut(
	"+meeting-participant-unmute",
	"Request a participant to unmute in a meeting",
	meetingParticipantUnmutePath,
	"request_unmute",
	"request_sent",
	"Unmute request sent.",
)

func newMeetingParticipantAudioShortcut(command, description, path, action, status, successMessage string) common.Shortcut {
	return common.Shortcut{
		Service:     "vc",
		Command:     command,
		Description: description,
		Risk:        "write",
		Scopes:      []string{meetingParticipantManageScope},
		AuthTypes:   []string{"bot"},
		HasFormat:   true,
		Flags:       meetingParticipantAudioFlags,
		Validate:    validateMeetingParticipantAudio,
		DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			return common.NewDryRunAPI().
				POST(path).
				Params(buildMeetingParticipantAudioParams(runtime)).
				Body(buildMeetingParticipantAudioBody(runtime))
		},
		Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
			_, err := runtime.CallAPITyped(
				http.MethodPost,
				path,
				buildMeetingParticipantAudioParams(runtime),
				buildMeetingParticipantAudioBody(runtime),
			)
			if err != nil {
				return err
			}
			result := map[string]interface{}{
				"action":         action,
				"meeting_id":     strings.TrimSpace(runtime.Str("meeting-id")),
				"status":         status,
				"target_user_id": strings.TrimSpace(runtime.Str("target-user-id")),
				"user_id_type":   strings.TrimSpace(runtime.Str("user-id-type")),
			}
			runtime.OutFormat(result, nil, func(w io.Writer) {
				fmt.Fprintln(w, successMessage)
				fmt.Fprintf(w, "  Meeting ID:     %s\n", result["meeting_id"])
				fmt.Fprintf(w, "  Target User ID: %s\n", result["target_user_id"])
			})
			return nil
		},
	}
}

func validateMeetingParticipantAudio(_ context.Context, runtime *common.RuntimeContext) error {
	if err := validateMeetingEventsMeetingID(runtime.Str("meeting-id")); err != nil {
		return err
	}
	if strings.TrimSpace(runtime.Str("target-user-id")) == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--target-user-id is required").WithParam("--target-user-id")
	}
	return nil
}

func buildMeetingParticipantAudioParams(runtime *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{
		"user_id_type": strings.TrimSpace(runtime.Str("user-id-type")),
	}
}

func buildMeetingParticipantAudioBody(runtime *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{
		"meeting_id":     strings.TrimSpace(runtime.Str("meeting-id")),
		"target_user_id": strings.TrimSpace(runtime.Str("target-user-id")),
	}
}
