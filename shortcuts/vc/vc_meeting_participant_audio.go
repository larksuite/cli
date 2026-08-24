// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const meetingParticipantManageScope = "vc:meeting.bot.manage:write"

var meetingParticipantAudioFlags = []common.Flag{
	{Name: "meeting-id", Required: true, Desc: "meeting ID"},
	{Name: "target-user-id", Required: true, Desc: "target participant user ID"},
	{Name: "user-id-type", Default: "open_id", Desc: "target user ID type"},
}

// VCMeetingParticipantMute mutes a participant in a meeting.
var VCMeetingParticipantMute = common.Shortcut{
	Service:     "vc",
	Command:     "+meeting-participant-mute",
	Description: "Mute a participant in a meeting",
	Risk:        "write",
	Scopes:      []string{meetingParticipantManageScope},
	AuthTypes:   []string{"bot"},
	Flags:       meetingParticipantAudioFlags,
	Validate:    validateMeetingParticipantAudio,
	Execute: func(context.Context, *common.RuntimeContext) error {
		return meetingParticipantAudioSDKBlocker("BotMeetingParticipantMute")
	},
}

// VCMeetingParticipantUnmute asks a participant to unmute in a meeting.
var VCMeetingParticipantUnmute = common.Shortcut{
	Service:     "vc",
	Command:     "+meeting-participant-unmute",
	Description: "Request a participant to unmute in a meeting",
	Risk:        "write",
	Scopes:      []string{meetingParticipantManageScope},
	AuthTypes:   []string{"bot"},
	Flags:       meetingParticipantAudioFlags,
	Validate:    validateMeetingParticipantAudio,
	Execute: func(context.Context, *common.RuntimeContext) error {
		return meetingParticipantAudioSDKBlocker("BotMeetingParticipantUnmute")
	},
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

func meetingParticipantAudioSDKBlocker(method string) error {
	return errs.NewInternalError(errs.SubtypeSDKError, "%s is not available in the generated Lark SDK used by this build", method).
		WithHint("sync the generated VC SDK method, then wire this shortcut through it; no request was sent")
}
