// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"fmt"
	"net/http"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

func calendarMeetingCitations(_ *common.RuntimeContext, data any) []citation.Citation {
	out, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	meetings, ok := out["meetings"].([]*meetingInfoItem)
	if !ok {
		return nil
	}
	citations := make([]citation.Citation, 0, len(meetings))
	for _, meeting := range meetings {
		if meeting == nil || meeting.URL == "" {
			continue
		}
		citations = append(citations, citation.Citation{
			SourceType: citation.SourceMeeting,
			URL:        meeting.URL,
			Title:      meeting.Topic,
		})
	}
	return citations
}

func annotateMeetingTopics(ctx context.Context, runtime *common.RuntimeContext, results []*meetingInfoItem) {
	if !citation.Enabled() {
		return
	}
	for _, result := range results {
		if result == nil || result.MeetingID == "" || result.URL == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return
		}
		result.Topic = fetchMeetingTopic(ctx, runtime, result.MeetingID)
	}
}

func fetchMeetingTopic(_ context.Context, runtime *common.RuntimeContext, meetingID string) string {
	data, err := runtime.CallAPITyped(http.MethodGet,
		fmt.Sprintf("/open-apis/vc/v1/meetings/%s", validate.EncodePathSegment(meetingID)),
		map[string]interface{}{"with_participants": "false", "query_mode": "0"}, nil)
	if err != nil {
		fmt.Fprintf(runtime.IO().ErrOut, "%s topic lookup failed for meeting_id=%s: %v\n",
			meetingLogPrefix, meetingID, err)
		return ""
	}
	meeting, _ := data["meeting"].(map[string]any)
	if meeting == nil {
		return ""
	}
	topic, _ := meeting["topic"].(string)
	return topic
}
