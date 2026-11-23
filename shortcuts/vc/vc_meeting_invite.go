// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	meetingBotInvitePath           = "/open-apis/vc/v1/bots/invite"
	meetingInviteTypeAllSuggested  = "ALL_SUGGESTED"
	meetingInviteTypeSelected      = "SELECTED"
	meetingInviteeLimit            = 200
	meetingInviteTypeAllValue      = 1
	meetingInviteTypeSelectedValue = 2
	meetingInviteeUserType         = 1
	meetingInviteStatusSucceeded   = 1
	meetingInviteStatusFailed      = 2

	meetingInviteCandidateLimitNotice = "Some eligible candidates were not invited because the service limit is 200."
)

type meetingInvitee struct {
	ID       string `json:"id"`
	UserType int    `json:"user_type"`
}

type meetingInviteRequest struct {
	MeetingID  string           `json:"meeting_id"`
	InviteType int              `json:"invite_type"`
	Invitees   []meetingInvitee `json:"invitees,omitempty"`
}

type meetingInviteResult struct {
	ID     string
	Status int
}

// VCMeetingInvite invites users through the Agent bot invite path.
var VCMeetingInvite = common.Shortcut{
	Service:     "vc",
	Command:     "+meeting-invite",
	Description: "Invite selected or all eligible users as the app bot",
	Risk:        "write",
	Scopes:      []string{"vc:meeting.bot.join:write"},
	AuthTypes:   []string{"bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "meeting-id", Required: true, Desc: "meeting ID"},
		{Name: "type", Required: true, Desc: "invite type", Enum: []string{meetingInviteTypeAllSuggested, meetingInviteTypeSelected}},
		{Name: "open-ids", Type: "string_slice", Desc: "user open_ids for SELECTED (maximum 200)"},
	},
	Normalize: func(_ context.Context, flags *common.FlagContext) error {
		inviteType := strings.ToUpper(strings.TrimSpace(flags.Str("type")))
		if inviteType == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--type is required").WithParam("--type")
		}
		return flags.SetCanonical("type", inviteType)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateMeetingIDFlag(runtime.Str("meeting-id")); err != nil {
			return err
		}
		return validateMeetingInviteFlags(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().POST(meetingBotInvitePath).
			Params(buildMeetingInviteParams()).
			Body(buildMeetingInviteBody(runtime))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		data, err := runtime.CallAPITyped(http.MethodPost, meetingBotInvitePath, buildMeetingInviteParams(), buildMeetingInviteBody(runtime))
		if err != nil {
			return err
		}
		meetingID := strings.TrimSpace(runtime.Str("meeting-id"))
		if data == nil {
			data = map[string]interface{}{}
		}
		output := buildMeetingInviteOutput(data)
		output["meeting_id"] = meetingID
		runtime.OutFormat(output, nil, func(w io.Writer) {
			printMeetingInviteResult(w, output)
		})
		return nil
	},
}

func validateMeetingInviteFlags(runtime *common.RuntimeContext) error {
	openIDs := normalizeMeetingInviteOpenIDs(runtime.StrSlice("open-ids"))
	switch runtime.Str("type") {
	case meetingInviteTypeSelected:
		if len(openIDs) == 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--open-ids is required when --type is SELECTED").WithParam("--open-ids")
		}
		if len(openIDs) > meetingInviteeLimit {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--open-ids accepts at most %d users, got %d", meetingInviteeLimit, len(openIDs)).WithParam("--open-ids")
		}
		for _, openID := range openIDs {
			if !strings.HasPrefix(openID, "ou_") {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--open-ids only accepts user open_id values (ou_xxx)").WithParam("--open-ids")
			}
		}
	case meetingInviteTypeAllSuggested:
		if len(openIDs) != 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--open-ids must not be set when --type is ALL_SUGGESTED").WithParam("--open-ids")
		}
	}
	return nil
}

func buildMeetingInviteBody(runtime *common.RuntimeContext) meetingInviteRequest {
	body := meetingInviteRequest{
		MeetingID: strings.TrimSpace(runtime.Str("meeting-id")),
	}
	if runtime.Str("type") == meetingInviteTypeSelected {
		body.InviteType = meetingInviteTypeSelectedValue
		body.Invitees = buildMeetingInviteUsers(normalizeMeetingInviteOpenIDs(runtime.StrSlice("open-ids")))
	} else {
		body.InviteType = meetingInviteTypeAllValue
	}
	return body
}

func buildMeetingInviteParams() map[string]interface{} {
	return map[string]interface{}{"user_id_type": "open_id"}
}

func buildMeetingInviteOutput(data map[string]interface{}) map[string]interface{} {
	output := make(map[string]interface{}, len(data)+1)
	for key, value := range data {
		output[key] = value
	}
	if common.GetBool(data, "has_more") {
		output["notice"] = meetingInviteCandidateLimitNotice
	}
	delete(output, "has_more")
	return output
}

func printMeetingInviteResult(w io.Writer, data map[string]interface{}) {
	fmt.Fprintln(w, "Invite request sent.")
	if failedCount, ok := common.GetFloatOK(data, "failed_count"); ok {
		fmt.Fprintf(w, "  Failed:   %d\n", int(failedCount))
	}
	if invitedCount, ok := common.GetFloatOK(data, "invited_count"); ok {
		fmt.Fprintf(w, "  Invited:  %d\n", int(invitedCount))
	}
	if notice := common.GetString(data, "notice"); notice != "" {
		fmt.Fprintf(w, "  Note: %s\n", notice)
	}
	results := meetingInviteResults(data)
	if len(results) > 0 {
		fmt.Fprintln(w, "  Invite results:")
		for _, result := range results {
			fmt.Fprintf(w, "    %s: %s\n", result.ID, meetingInviteStatusLabel(result.Status))
		}
	}
}

func meetingInviteResults(data map[string]interface{}) []meetingInviteResult {
	items, _ := data["invite_results"].([]interface{})
	results := make([]meetingInviteResult, 0, len(items))
	common.EachMap(items, func(item map[string]interface{}) {
		status, ok := common.GetFloatOK(item, "status")
		if id := common.GetStringLoose(item, "id"); id != "" && ok {
			results = append(results, meetingInviteResult{ID: id, Status: int(status)})
		}
	})
	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})
	return results
}

func meetingInviteStatusLabel(status int) string {
	switch status {
	case meetingInviteStatusSucceeded:
		return "invited"
	case meetingInviteStatusFailed:
		return "failed"
	default:
		return fmt.Sprintf("unknown (%d)", status)
	}
}

func buildMeetingInviteUsers(openIDs []string) []meetingInvitee {
	invitees := make([]meetingInvitee, 0, len(openIDs))
	for _, openID := range openIDs {
		invitees = append(invitees, meetingInvitee{
			ID:       openID,
			UserType: meetingInviteeUserType,
		})
	}
	return invitees
}

func normalizeMeetingInviteOpenIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	openIDs := make([]string, 0, len(values))
	for _, value := range values {
		openID := strings.TrimSpace(value)
		if openID == "" {
			continue
		}
		if _, ok := seen[openID]; ok {
			continue
		}
		seen[openID] = struct{}{}
		openIDs = append(openIDs, openID)
	}
	return openIDs
}
