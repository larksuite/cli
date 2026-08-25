// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	vcMeetingParticipantKickoutPathFormat = "/open-apis/vc/v1/meetings/%s/kickout"
	minMeetingKickoutParticipants         = 1
	maxMeetingKickoutParticipants         = 10
	minMeetingKickoutUserType             = 1
	maxMeetingKickoutUserType             = 7
)

type meetingParticipantKickoutUser struct {
	ID       string `json:"id"`
	UserType int    `json:"user_type"`
}

type meetingParticipantKickoutBody struct {
	KickoutUsers []meetingParticipantKickoutUser `json:"kickout_users"`
}

// VCMeetingParticipantKickout removes one or more participants from an ongoing meeting.
var VCMeetingParticipantKickout = common.Shortcut{
	Service:                   "vc",
	Command:                   "+meeting-participant-kickout",
	Description:               "Remove one or more participants from an ongoing meeting",
	Risk:                      "high-risk-write",
	ConfirmationBeforeNetwork: true,
	Scopes:                    []string{},
	ConditionalUserScopes:     []string{"vc:meeting"},
	AuthTypes:                 []string{"user"},
	HasFormat:                 true,
	Flags: []common.Flag{
		{Name: "meeting-id", Required: true, Desc: "positive integer meeting ID"},
		{Name: "participant", Type: "string_array", Required: true, Desc: "participant tuple <id>=<user_type>; repeat 1 to 10 times"},
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		if err := validateMeetingManagementID(runtime.Str("meeting-id")); err != nil {
			return err
		}
		_, err := parseMeetingParticipantKickoutUsers(runtime.StrArray("participant"))
		return err
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, err := buildMeetingParticipantKickoutBody(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST(buildMeetingParticipantKickoutPath(runtime.Str("meeting-id"))).
			Body(body)
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		body, err := buildMeetingParticipantKickoutBody(runtime)
		if err != nil {
			return err
		}
		if err := runtime.EnsureScopes([]string{"vc:meeting"}); err != nil {
			return err
		}
		envelope, data, err := callMeetingManagementAPIEnvelope(
			runtime,
			http.MethodPost,
			buildMeetingParticipantKickoutPath(runtime.Str("meeting-id")),
			body,
		)
		if err != nil {
			return err
		}
		if err := validateMeetingParticipantKickoutResponse(data, body.KickoutUsers); err != nil {
			return err
		}
		// Keep the server's kickout_results fields, values, and order intact for
		// every output format; no client-side projection or conflict resolution.
		runtime.OutFormat(envelope, nil, nil)
		return nil
	},
}

func validateMeetingParticipantKickoutResponse(data map[string]interface{}, requested []meetingParticipantKickoutUser) error {
	if data == nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "meeting participant kickout response is missing data")
	}
	rawResults, ok := data["kickout_results"]
	if !ok {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "meeting participant kickout response is missing kickout_results")
	}
	encoded, err := json.Marshal(rawResults)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "meeting participant kickout response has invalid kickout_results").WithCause(err)
	}
	var results []map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &results); err != nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "meeting participant kickout response has invalid kickout_results").WithCause(err)
	}

	// OpenAPI preserves the original request body but normalizes identical
	// participant tuples to the first occurrence before executing HostManage.
	// Correlate against that normalized tuple set so a successful destructive
	// request is not reported as failed merely because the input had duplicates.
	want := make(map[string]struct{}, len(requested))
	for _, participant := range requested {
		key, ok := meetingParticipantTupleKey(participant.ID, participant.UserType)
		if !ok {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "meeting participant kickout request contains an invalid tuple")
		}
		want[key] = struct{}{}
	}
	if len(results) != len(want) {
		return errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"meeting participant kickout response returned %d result(s), want %d normalized requested tuple(s)",
			len(results),
			len(want),
		)
	}

	for index, result := range results {
		rawID, hasID := result["id"]
		rawUserType, hasUserType := result["user_type"]
		_, hasResult := result["result"]
		if !hasID || !hasUserType || !hasResult {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "meeting participant kickout result %d is missing id, user_type, or result", index)
		}
		var userType int
		if err := json.Unmarshal(rawUserType, &userType); err != nil || userType < minMeetingKickoutUserType || userType > maxMeetingKickoutUserType {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "meeting participant kickout result %d has invalid user_type", index)
		}
		key, ok := meetingParticipantResponseTupleKey(rawID, userType)
		if !ok {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "meeting participant kickout result %d has invalid id", index)
		}
		if _, requested := want[key]; !requested {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "meeting participant kickout response returned an unrequested or excess tuple")
		}
		delete(want, key)
	}
	if len(want) != 0 {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "meeting participant kickout response is missing a requested tuple")
	}
	return nil
}

// meetingParticipantResponseTupleKey accepts the response's canonical i64 JSON
// number and the historical string representation. It normalizes only the
// private correlation key; the original server envelope remains untouched.
func meetingParticipantResponseTupleKey(rawID json.RawMessage, userType int) (string, bool) {
	if len(rawID) == 0 || string(rawID) == "null" {
		return "", false
	}
	id := string(rawID)
	if rawID[0] == '"' {
		if err := json.Unmarshal(rawID, &id); err != nil {
			return "", false
		}
	}
	return meetingParticipantTupleKey(id, userType)
}

func meetingParticipantTupleKey(id string, userType int) (string, bool) {
	idValue, err := strconv.ParseInt(id, 10, 64)
	if err != nil || idValue <= 0 {
		return "", false
	}
	return fmt.Sprintf("%d/%d", idValue, userType), true
}

func buildMeetingParticipantKickoutPath(meetingID string) string {
	return fmt.Sprintf(vcMeetingParticipantKickoutPathFormat, validate.EncodePathSegment(strings.TrimSpace(meetingID)))
}

func buildMeetingParticipantKickoutBody(runtime *common.RuntimeContext) (meetingParticipantKickoutBody, error) {
	users, err := parseMeetingParticipantKickoutUsers(runtime.StrArray("participant"))
	if err != nil {
		return meetingParticipantKickoutBody{}, err
	}
	return meetingParticipantKickoutBody{KickoutUsers: users}, nil
}

func parseMeetingParticipantKickoutUsers(values []string) ([]meetingParticipantKickoutUser, error) {
	if len(values) < minMeetingKickoutParticipants || len(values) > maxMeetingKickoutParticipants {
		return nil, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--participant must be repeated between %d and %d times",
			minMeetingKickoutParticipants,
			maxMeetingKickoutParticipants,
		).WithParam("--participant")
	}

	users := make([]meetingParticipantKickoutUser, 0, len(values))
	for index, value := range values {
		if strings.Count(value, "=") != 1 {
			return nil, invalidMeetingParticipantTuple(index, "must contain exactly one '='")
		}
		parts := strings.SplitN(value, "=", 2)
		if parts[0] == "" {
			return nil, invalidMeetingParticipantTuple(index, "id must not be empty")
		}
		if strings.TrimSpace(parts[0]) != parts[0] {
			return nil, invalidMeetingParticipantTuple(index, "id must not have surrounding whitespace")
		}
		idValue, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || idValue <= 0 {
			return nil, invalidMeetingParticipantTuple(index, "id must be a positive base-10 int64")
		}
		userType, err := strconv.Atoi(parts[1])
		if err != nil || userType < minMeetingKickoutUserType || userType > maxMeetingKickoutUserType {
			return nil, invalidMeetingParticipantTuple(index, "user_type must be an integer from 1 to 7")
		}
		users = append(users, meetingParticipantKickoutUser{
			ID:       parts[0],
			UserType: userType,
		})
	}
	return users, nil
}

func invalidMeetingParticipantTuple(index int, reason string) error {
	return errs.NewValidationError(
		errs.SubtypeInvalidArgument,
		"--participant value %d %s; expected <id>=<user_type>",
		index+1,
		reason,
	).WithParam("--participant")
}
