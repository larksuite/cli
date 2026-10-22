// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// agendaOutput wraps the agenda event list so the citation builder can read
// each event's app_link while keeping it off the wire. app_link stays on the
// in-memory map as builder raw material; MarshalJSON strips it so the
// serialized payload is exactly the bare event array as before.
type agendaOutput struct {
	events []map[string]interface{}
}

func (a agendaOutput) MarshalJSON() ([]byte, error) {
	cleaned := make([]map[string]interface{}, len(a.events))
	for i, e := range a.events {
		c := make(map[string]interface{}, len(e))
		for k, v := range e {
			if k == "app_link" {
				continue
			}
			c[k] = v
		}
		cleaned[i] = c
	}
	return json.Marshal(cleaned)
}

// calendarAgendaCitations builds one citation per event that carries an
// app_link, drawing title and url straight from the payload the command
// already has in hand. It runs only when the citation gate is on.
func calendarAgendaCitations(_ *common.RuntimeContext, data any) []citation.Citation {
	out, ok := data.(agendaOutput)
	if !ok {
		return nil
	}
	citations := make([]citation.Citation, 0, len(out.events))
	for _, e := range out.events {
		appLink, _ := e["app_link"].(string)
		if appLink == "" {
			continue
		}
		summary, _ := e["summary"].(string)
		citations = append(citations, citation.Citation{
			SourceType: citation.SourceCalendar,
			URL:        appLink,
			Title:      summary,
		})
	}
	return citations
}

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
