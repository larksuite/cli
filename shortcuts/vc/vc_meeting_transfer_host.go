// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	vcMeetingTransferHostPathFormat      = "/open-apis/vc/v1/meetings/%s/set_host"
	defaultMeetingTransferHostUserIDType = "open_id"
)

type meetingTransferHostUser struct {
	ID       string `json:"id"`
	UserType int    `json:"user_type"`
}

type meetingTransferHostBody struct {
	HostUser meetingTransferHostUser `json:"host_user"`
}

// VCMeetingTransferHost transfers the host role of an ongoing meeting to one participant.
var VCMeetingTransferHost = common.Shortcut{
	Service:               "vc",
	Command:               "+meeting-transfer-host",
	Description:           "Transfer the host role of an ongoing meeting to a participant",
	Risk:                  "high-risk-write",
	Scopes:                []string{},
	ConditionalUserScopes: []string{"vc:meeting"},
	AuthTypes:             []string{"user"},
	HasFormat:             true,
	Flags: []common.Flag{
		{Name: "meeting-id", Required: true, Desc: "positive integer meeting ID"},
		{Name: "user-id-type", Default: defaultMeetingTransferHostUserIDType, Desc: "user ID type for host_user.id", Enum: []string{"open_id", "union_id", "user_id"}},
		{Name: "participant", Required: true, Desc: "new host tuple <user_id>=<user_type>"},
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		if err := validateMeetingManagementID(runtime.Str("meeting-id")); err != nil {
			return err
		}
		_, err := buildMeetingTransferHostBody(runtime)
		return err
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, err := buildMeetingTransferHostBody(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			PATCH(buildMeetingTransferHostPath(runtime.Str("meeting-id"))).
			Params(buildMeetingTransferHostParams(runtime)).
			Body(body)
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		body, err := buildMeetingTransferHostBody(runtime)
		if err != nil {
			return err
		}
		if err := runtime.EnsureScopes([]string{"vc:meeting"}); err != nil {
			return err
		}
		envelope, _, err := callMeetingManagementAPIEnvelope(
			runtime,
			http.MethodPatch,
			buildMeetingTransferHostPath(runtime.Str("meeting-id")),
			buildMeetingTransferHostParams(runtime),
			body,
		)
		if err != nil {
			return err
		}
		runtime.OutFormat(envelope, nil, nil)
		return nil
	},
}

func buildMeetingTransferHostPath(meetingID string) string {
	return fmt.Sprintf(vcMeetingTransferHostPathFormat, validate.EncodePathSegment(strings.TrimSpace(meetingID)))
}

func buildMeetingTransferHostBody(runtime *common.RuntimeContext) (meetingTransferHostBody, error) {
	value := runtime.Str("participant")
	if strings.Count(value, "=") != 1 {
		return meetingTransferHostBody{}, invalidMeetingTransferHostParticipant("must contain exactly one '='")
	}
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return meetingTransferHostBody{}, invalidMeetingTransferHostParticipant("must contain exactly one '='")
	}
	if parts[0] == "" {
		return meetingTransferHostBody{}, invalidMeetingTransferHostParticipant("id must not be empty")
	}
	if strings.TrimSpace(parts[0]) != parts[0] {
		return meetingTransferHostBody{}, invalidMeetingTransferHostParticipant("id must not have surrounding whitespace")
	}
	userType, err := strconv.Atoi(parts[1])
	if err != nil || userType < 1 || userType > 7 {
		return meetingTransferHostBody{}, invalidMeetingTransferHostParticipant("user_type must be an integer from 1 to 7")
	}
	return meetingTransferHostBody{HostUser: meetingTransferHostUser{ID: parts[0], UserType: userType}}, nil
}

func buildMeetingTransferHostParams(runtime *common.RuntimeContext) map[string]interface{} {
	userIDType := strings.TrimSpace(runtime.Str("user-id-type"))
	if userIDType == "" {
		userIDType = defaultMeetingTransferHostUserIDType
	}
	return map[string]interface{}{"user_id_type": userIDType}
}

func invalidMeetingTransferHostParticipant(reason string) error {
	return errs.NewValidationError(
		errs.SubtypeInvalidArgument,
		"--participant value 1 %s; expected <id>=<user_type>",
		reason,
	).WithParam("--participant")
}
