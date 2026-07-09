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
	Scopes:      []string{"minutes:minutes:readonly", "minutes:minutes:update"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "minute-token", Desc: "minute token", Required: true},
		{Name: "from-speaker-id", Desc: "speaker to replace: opaque speaker_id from transcript speakerlist API (do not pass display names)"},
		{Name: "from-user-id", Desc: "deprecated: open_id of the speaker to replace; prefer --from-speaker-id", Hidden: true},
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
		fromSpeakerID := strings.TrimSpace(runtime.Str("from-speaker-id"))
		fromUserID := strings.TrimSpace(runtime.Str("from-user-id"))
		if fromSpeakerID == "" && fromUserID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--from-speaker-id is required").WithParam("--from-speaker-id")
		}
		toUserID := strings.TrimSpace(runtime.Str("to-user-id"))
		if toUserID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--to-user-id is required").WithParam("--to-user-id")
		}
		if _, err := common.ValidateUserIDTyped("--to-user-id", toUserID); err != nil {
			return err
		}
		if fromSpeakerID == "" {
			if _, err := common.ValidateUserIDTyped("--from-user-id", fromUserID); err != nil {
				return err
			}
			if fromUserID == toUserID {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--from-user-id and --to-user-id must be different").WithParam("--to-user-id")
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))
		return common.NewDryRunAPI().
			PUT(fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/transcript/speaker", validate.EncodePathSegment(minuteToken))).
			Body(buildSpeakerReplaceRequestBody(runtime))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))
		fromSpeakerID := strings.TrimSpace(runtime.Str("from-speaker-id"))
		fromUserID := strings.TrimSpace(runtime.Str("from-user-id"))
		toUserID := strings.TrimSpace(runtime.Str("to-user-id"))

		_, err := runtime.CallAPITyped(http.MethodPut,
			fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/transcript/speaker", validate.EncodePathSegment(minuteToken)),
			map[string]interface{}{"user_id_type": "open_id"}, buildSpeakerReplaceRequestBodyResolved(fromSpeakerID, fromUserID, toUserID))
		if err != nil {
			return minutesSpeakerReplaceError(err, minuteToken, speakerReplaceSourceLabel(fromSpeakerID, fromUserID))
		}

		runtime.OutFormat(buildSpeakerReplaceOutputData(minuteToken, fromSpeakerID, fromUserID, toUserID), nil, nil)
		return nil
	},
}

func buildSpeakerReplaceRequestBody(runtime *common.RuntimeContext) map[string]interface{} {
	fromSpeakerID := strings.TrimSpace(runtime.Str("from-speaker-id"))
	fromUserID := strings.TrimSpace(runtime.Str("from-user-id"))
	toUserID := strings.TrimSpace(runtime.Str("to-user-id"))
	return buildSpeakerReplaceRequestBodyResolved(fromSpeakerID, fromUserID, toUserID)
}

func buildSpeakerReplaceRequestBodyResolved(fromSpeakerID, fromUserID, toUserID string) map[string]interface{} {
	body := map[string]interface{}{
		"to_user_id": toUserID,
	}
	if fromSpeakerID != "" {
		body["from_speaker_id"] = fromSpeakerID
	} else {
		body["from_user_id"] = fromUserID
	}
	return body
}

func buildSpeakerReplaceOutputData(minuteToken, fromSpeakerID, fromUserID, toUserID string) map[string]interface{} {
	out := map[string]interface{}{
		"minute_token": minuteToken,
		"to_user_id":   toUserID,
	}
	if fromSpeakerID != "" {
		out["from_speaker_id"] = fromSpeakerID
	} else {
		out["from_user_id"] = fromUserID
	}
	return out
}

func speakerReplaceSourceLabel(fromSpeakerID, fromUserID string) string {
	if fromSpeakerID != "" {
		return fromSpeakerID
	}
	return fromUserID
}

func minutesSpeakerReplaceError(err error, minuteToken, sourceSpeaker string) error {
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
		p.Message = fmt.Sprintf("Speaker not found in minute %q: source speaker %q does not match an existing speaker in the transcript.", minuteToken, sourceSpeaker)
		p.Hint = "Verify --from-speaker-id is a valid speaker_id or display name from the transcript; if multiple speakers share the same name, pass the exact speaker_id after reviewing their utterances."
	}
	return err
}
