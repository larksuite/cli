// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	meetingBotEndPath      = "/open-apis/vc/v1/bots/end"
	vcMeetingEndPathFormat = "/open-apis/vc/v1/meetings/%s/end"
)

type meetingEndRequest struct {
	MeetingID string `json:"meeting_id"`
}

// VCMeetingEnd ends an ongoing meeting as either a user or the host app bot.
var VCMeetingEnd = common.Shortcut{
	Service:                   "vc",
	Command:                   "+meeting-end",
	Description:               "End an ongoing meeting as a user or host app bot",
	Risk:                      "high-risk-write",
	Scopes:                    []string{},
	ConditionalUserScopes:     []string{"vc:meeting"},
	ConditionalBotScopes:      []string{"vc:meeting.bot.manage:write"},
	ConfirmationBeforeNetwork: true,
	AuthTypes:                 []string{"user", "bot"},
	HasFormat:                 true,
	Flags: []common.Flag{
		{Name: "meeting-id", Required: true, Desc: "meeting ID to end"},
	},
	Validate: validateMeetingEnd,
	DryRun:   buildMeetingEndDryRun,
	Execute:  executeMeetingEnd,
}

func validateMeetingEnd(_ context.Context, runtime *common.RuntimeContext) error {
	meetingID := runtime.Str("meeting-id")
	switch runtime.As() {
	case core.AsUser:
		return validateMeetingManagementID(meetingID)
	case core.AsBot:
		return validateMeetingIDFlag(meetingID)
	case core.AsAuto:
		if runtime.Bool("dry-run") {
			return explicitMeetingEndIdentityError()
		}
		// Offline confirmation preflight cannot observe the configured or
		// credential-derived identity. Apply only the validation shared by both
		// routes here; Execute validates again after full identity resolution.
		return validateMeetingManagementID(meetingID)
	default:
		return explicitMeetingEndIdentityError()
	}
}

func buildMeetingEndDryRun(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	switch runtime.As() {
	case core.AsUser:
		return common.NewDryRunAPI().PATCH(buildMeetingEndPath(runtime.Str("meeting-id")))
	case core.AsBot:
		return common.NewDryRunAPI().POST(meetingBotEndPath).Body(buildMeetingEndBody(runtime))
	default:
		// Validate rejects unresolved or unsupported identities before DryRun.
		return common.NewDryRunAPI()
	}
}

func executeMeetingEnd(_ context.Context, runtime *common.RuntimeContext) error {
	meetingID := strings.TrimSpace(runtime.Str("meeting-id"))
	switch runtime.As() {
	case core.AsUser:
		if err := validateMeetingManagementID(meetingID); err != nil {
			return err
		}
		return executeUserMeetingEnd(runtime, meetingID)
	case core.AsBot:
		if err := validateMeetingIDFlag(meetingID); err != nil {
			return err
		}
		return executeBotMeetingEnd(runtime, meetingID)
	default:
		return explicitMeetingEndIdentityError()
	}
}

func executeUserMeetingEnd(runtime *common.RuntimeContext, meetingID string) error {
	if err := runtime.EnsureScopes([]string{"vc:meeting"}); err != nil {
		return err
	}
	envelope, _, err := callMeetingManagementAPIEnvelope(runtime, http.MethodPatch, buildMeetingEndPath(meetingID), nil)
	if err != nil {
		return err
	}
	runtime.OutFormat(envelope, nil, nil)
	return nil
}

func executeBotMeetingEnd(runtime *common.RuntimeContext, meetingID string) error {
	if err := runtime.EnsureScopes([]string{"vc:meeting.bot.manage:write"}); err != nil {
		return err
	}
	data, err := runtime.CallAPITyped(http.MethodPost, meetingBotEndPath, nil, meetingEndRequest{MeetingID: meetingID})
	if err != nil {
		return err
	}
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
}

func explicitMeetingEndIdentityError() error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"--dry-run for +meeting-end requires explicit --as user or --as bot because offline preflight cannot resolve default or automatic identity").
		WithParam("--as")
}

func buildMeetingEndBody(runtime *common.RuntimeContext) meetingEndRequest {
	return meetingEndRequest{MeetingID: strings.TrimSpace(runtime.Str("meeting-id"))}
}

func validateMeetingIDFlag(value string) error {
	return validateMeetingEventsMeetingID(value)
}

func validateMeetingManagementID(meetingID string) error {
	meetingID = strings.TrimSpace(meetingID)
	value, err := strconv.ParseInt(meetingID, 10, 64)
	if err != nil || value <= 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--meeting-id must be a positive base-10 int64").WithParam("--meeting-id")
	}
	return nil
}

func buildMeetingEndPath(meetingID string) string {
	return fmt.Sprintf(vcMeetingEndPathFormat, validate.EncodePathSegment(strings.TrimSpace(meetingID)))
}
