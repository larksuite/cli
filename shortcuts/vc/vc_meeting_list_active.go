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
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const vcMeetingListActiveAPIPath = "/open-apis/vc/v1/bots/user_active_meeting"

// VCMeetingListActive lists meetings the current or target user is actively in.
var VCMeetingListActive = common.Shortcut{
	Service:     "vc",
	Command:     "+meeting-list-active",
	Description: "List active meetings for the current identity or target user",
	Risk:        "read",
	Scopes:      []string{"vc:meeting.meetingevent:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "user-id", Desc: "target user ID when using bot identity"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateMeetingListActiveUserID(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		params, err := buildMeetingListActiveParams(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		dryRun := common.NewDryRunAPI().GET(vcMeetingListActiveAPIPath)
		if len(params) > 0 {
			dryRun.Params(params)
		}
		return dryRun
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		params, err := buildMeetingListActiveParams(runtime)
		if err != nil {
			return err
		}
		data, err := runtime.CallAPITyped(http.MethodGet, vcMeetingListActiveAPIPath, params, nil)
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]interface{}{}
		}
		meetings := common.GetSlice(data, "meetings")
		runtime.OutFormat(data, &output.Meta{Count: len(meetings)}, func(w io.Writer) {
			if len(meetings) == 0 {
				fmt.Fprintln(w, "No active meetings.")
				return
			}
			displayedMeetings := 0
			for _, raw := range meetings {
				meeting, _ := raw.(map[string]interface{})
				if meeting == nil {
					continue
				}
				if displayedMeetings > 0 {
					fmt.Fprintln(w)
				}
				displayedMeetings++
				title := common.GetString(meeting, "meeting_title")
				if title == "" {
					title = "Untitled meeting"
				}
				fmt.Fprintf(w, "%s\n", title)
				if id := common.GetString(meeting, "meeting_id"); id != "" {
					fmt.Fprintf(w, "  Meeting ID:  %s\n", id)
				}
				if no := common.GetString(meeting, "meeting_no"); no != "" {
					fmt.Fprintf(w, "  Meeting No:  %s\n", no)
				}
			}
			if displayedMeetings > 1 {
				fmt.Fprintln(w)
				fmt.Fprintln(w, "Multiple active meetings found. Ask the user to choose one meeting_id before calling +meeting-events.")
			}
		})
		return nil
	},
}

// validateMeetingListActiveUserID validates the target user only for bot identity.
func validateMeetingListActiveUserID(runtime *common.RuntimeContext) error {
	if !runtime.IsBot() {
		return nil
	}
	userID := strings.TrimSpace(runtime.Str("user-id"))
	if userID == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--user-id is required when --as bot").WithParam("--user-id")
	}
	if _, err := common.ValidateUserIDTyped("--user-id", userID); err != nil {
		return err
	}
	return nil
}

// buildMeetingListActiveParams builds the query params for active meeting lookup.
func buildMeetingListActiveParams(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	if err := validateMeetingListActiveUserID(runtime); err != nil {
		return nil, err
	}
	params := map[string]interface{}{}
	if runtime.IsBot() {
		userID := strings.TrimSpace(runtime.Str("user-id"))
		params["user_id"] = userID
	}
	return params, nil
}
