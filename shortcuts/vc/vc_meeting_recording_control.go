// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const recordingControlScope = "vc:record"

// VCMeetingRecordingStart starts recording an active meeting.
var VCMeetingRecordingStart = newRecordingControlShortcut(
	"+meeting-recording-start",
	"Start recording an active meeting",
	"start",
	"started",
)

// VCMeetingRecordingStop stops recording an active meeting.
var VCMeetingRecordingStop = newRecordingControlShortcut(
	"+meeting-recording-stop",
	"Stop recording an active meeting",
	"stop",
	"stopped",
)

func newRecordingControlShortcut(command, description, action, successVerb string) common.Shortcut {
	return common.Shortcut{
		Service:     "vc",
		Command:     command,
		Description: description,
		Risk:        "write",
		Scopes:      []string{recordingControlScope},
		AuthTypes:   []string{"user"},
		HasFormat:   true,
		Flags: []common.Flag{
			{Name: "meeting-id", Required: true, Desc: "meeting ID to control recording for"},
		},
		Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
			return validateMeetingEventsMeetingID(runtime.Str("meeting-id"))
		},
		DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			meetingID := strings.TrimSpace(runtime.Str("meeting-id"))
			return common.NewDryRunAPI().
				PATCH(recordingControlPath(meetingID, action))
		},
		Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
			meetingID := strings.TrimSpace(runtime.Str("meeting-id"))
			_, err := runtime.CallAPITyped(http.MethodPatch, recordingControlPath(meetingID, action), nil, nil)
			if err != nil {
				return err
			}
			result := map[string]interface{}{
				"action":     action,
				"meeting_id": meetingID,
			}
			runtime.OutFormat(result, nil, func(w io.Writer) {
				fmt.Fprintf(w, "Meeting recording %s for %s.\n", successVerb, meetingID)
			})
			return nil
		},
	}
}

func recordingControlPath(meetingID, action string) string {
	return fmt.Sprintf(
		"/open-apis/vc/v1/meetings/%s/recording/%s",
		validate.EncodePathSegment(meetingID),
		action,
	)
}
