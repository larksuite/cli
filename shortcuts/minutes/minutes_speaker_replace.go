// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
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
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--minute-token is required").WithParam("--minute-token")
		}
		if err := validate.ResourceName(minuteToken, "--minute-token"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--minute-token")
		}
		fromUserID := strings.TrimSpace(runtime.Str("from-user-id"))
		if fromUserID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--from-user-id is required").WithParam("--from-user-id")
		}
		if _, err := common.ValidateUserIDTyped("--from-user-id", fromUserID); err != nil {
			return err
		}
		toUserID := strings.TrimSpace(runtime.Str("to-user-id"))
		if toUserID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--to-user-id is required").WithParam("--to-user-id")
		}
		if _, err := common.ValidateUserIDTyped("--to-user-id", toUserID); err != nil {
			return err
		}
		if fromUserID == toUserID {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--from-user-id and --to-user-id must be different").WithParam("--to-user-id")
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

		_, err := runtime.CallAPITyped(http.MethodPut,
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
	p, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	switch p.Code {
	case minutesSpeakerReplaceNoEditPermission:
		p.Message = fmt.Sprintf("No edit permission for minute %q: cannot replace the transcript speaker.", minuteToken)
		p.Hint = "Ask the minute owner for minute edit permission"
	case minutesSpeakerReplaceSpeakerNotFoundCode:
		p.Subtype = errs.SubtypeNotFound
		p.Message = fmt.Sprintf("Speaker not found in minute %q: --from-user-id %q does not match an existing speaker in the transcript.", minuteToken, fromUserID)
		p.Hint = "Check --minute-token and --from-user-id. Use an open_id for a speaker that appears in the minute transcript, then retry."
	}
	return err
}
