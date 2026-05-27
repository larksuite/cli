// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	minutesSpeakerReplaceSpeakerNotFoundCode = 2091001
	minutesSpeakerReplaceNoEditPermission    = 2091005
)

// MinutesSpeakerReplace replaces a speaker in a minute's transcript.
var MinutesSpeakerReplace = common.Shortcut{
	Service:     "minutes",
	Command:     "+speaker-replace",
	Description: "Replace a speaker in a minute's transcript (rebind from one user to another)",
	Risk:        "write",
	Scopes:      []string{"minutes:minutes:update"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "minute-token", Desc: "minute token", Required: true},
		{Name: "from-user-id", Desc: "speaker to replace, must be an open_id starting with 'ou_'", Required: true},
		{Name: "to-user-id", Desc: "new speaker, must be an open_id starting with 'ou_'", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))
		if minuteToken == "" {
			return output.ErrValidation("--minute-token is required")
		}
		if err := validate.ResourceName(minuteToken, "--minute-token"); err != nil {
			return output.ErrValidation("%s", err)
		}
		fromUserID := strings.TrimSpace(runtime.Str("from-user-id"))
		if fromUserID == "" {
			return output.ErrValidation("--from-user-id is required")
		}
		if _, err := common.ValidateUserID(fromUserID); err != nil {
			return output.ErrValidation("--from-user-id: %s", err)
		}
		toUserID := strings.TrimSpace(runtime.Str("to-user-id"))
		if toUserID == "" {
			return output.ErrValidation("--to-user-id is required")
		}
		if _, err := common.ValidateUserID(toUserID); err != nil {
			return output.ErrValidation("--to-user-id: %s", err)
		}
		if fromUserID == toUserID {
			return output.ErrValidation("--from-user-id and --to-user-id must be different")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))
		fromUserID := strings.TrimSpace(runtime.Str("from-user-id"))
		toUserID := strings.TrimSpace(runtime.Str("to-user-id"))
		return common.NewDryRunAPI().
			PUT(fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/transcript/speaker", validate.EncodePathSegment(minuteToken))).
			Body(map[string]interface{}{
				"minute_token": minuteToken,
				"from_user_id": fromUserID,
				"to_user_id":   toUserID,
			})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))
		fromUserID := strings.TrimSpace(runtime.Str("from-user-id"))
		toUserID := strings.TrimSpace(runtime.Str("to-user-id"))

		body := map[string]interface{}{
			"minute_token": minuteToken,
			"from_user_id": fromUserID,
			"to_user_id":   toUserID,
		}

		_, err := runtime.CallAPI(http.MethodPut,
			fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/transcript/speaker", validate.EncodePathSegment(minuteToken)),
			nil, body)
		if err != nil {
			return minutesSpeakerReplaceError(err, minuteToken, fromUserID)
		}

		outData := map[string]interface{}{
			"minute_token": minuteToken,
			"from_user_id": fromUserID,
			"to_user_id":   toUserID,
		}

		runtime.OutFormat(outData, nil, nil)
		return nil
	},
}

func minutesSpeakerReplaceError(err error, minuteToken, fromUserID string) error {
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		return err
	}

	switch exitErr.Detail.Code {
	case minutesSpeakerReplaceNoEditPermission:
		return &output.ExitError{
			Code: output.ExitAPI,
			Detail: &output.ErrDetail{
				Type:    "no_edit_permission",
				Code:    minutesSpeakerReplaceNoEditPermission,
				Message: fmt.Sprintf("No edit permission for minute %q: cannot replace the transcript speaker.", minuteToken),
				Hint:    "Ask the minute owner for minute edit permission",
				Detail:  exitErr.Detail.Detail,
			},
			Err: err,
		}
	case minutesSpeakerReplaceSpeakerNotFoundCode:
		return &output.ExitError{
			Code: output.ExitAPI,
			Detail: &output.ErrDetail{
				Type:    "speaker_not_found",
				Code:    minutesSpeakerReplaceSpeakerNotFoundCode,
				Message: fmt.Sprintf("Speaker not found in minute %q: --from-user-id %q does not match an existing speaker in the transcript.", minuteToken, fromUserID),
				Hint:    "Check --minute-token and --from-user-id. Use an open_id for a speaker that appears in the minute transcript, then retry.",
				Detail:  exitErr.Detail.Detail,
			},
			Err: err,
		}
	}

	return err
}
