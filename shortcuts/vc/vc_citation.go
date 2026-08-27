// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

func vcDetailCitations(_ *common.RuntimeContext, data any) []citation.Citation {
	out, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	meetings, ok := out["meetings"].([]*meetingDetailItem)
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

type vcSearchPayload struct {
	Data   any               `json:"-"`
	Topics map[string]string `json:"-"`
}

func (p vcSearchPayload) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Data)
}

func vcSearchPayloadParts(data any) (any, map[string]string) {
	switch payload := data.(type) {
	case vcSearchPayload:
		return payload.Data, payload.Topics
	case *vcSearchPayload:
		if payload != nil {
			return payload.Data, payload.Topics
		}
	}
	return data, nil
}

func vcSearchCitationEnvelopeRequested(runtime *common.RuntimeContext) bool {
	if runtime == nil || !citation.Enabled() {
		return false
	}
	if runtime.JqExpr != "" {
		return true
	}
	switch strings.ToLower(runtime.Format) {
	case "pretty", "table", "csv", "ndjson":
		return false
	default:
		return true
	}
}

func vcSearchCitations(_ *common.RuntimeContext, data any) []citation.Citation {
	payload, topics := vcSearchPayloadParts(data)
	out, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	items, ok := out["items"].([]interface{})
	if !ok {
		return nil
	}
	citations := make([]citation.Citation, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		appLink := meetingSearchAppLink(item)
		if appLink == "" {
			continue
		}
		citations = append(citations, citation.Citation{
			SourceType: citation.SourceMeeting,
			URL:        appLink,
			Title:      topics[common.GetString(item, "id")],
		})
	}
	return citations
}

func meetingSearchAppLink(item map[string]interface{}) string {
	meta, ok := item["meta_data"].(map[string]interface{})
	if !ok {
		return ""
	}
	return common.GetString(meta, "app_link")
}

func fetchMeetingTopic(ctx context.Context, runtime *common.RuntimeContext, meetingID string) string {
	if err := ctx.Err(); err != nil {
		return ""
	}
	data, err := runtime.CallAPITyped(http.MethodGet,
		fmt.Sprintf("/open-apis/vc/v1/meetings/%s", validate.EncodePathSegment(meetingID)),
		map[string]interface{}{"with_participants": "false", "query_mode": "0"}, nil)
	if err != nil {
		fmt.Fprintf(runtime.IO().ErrOut, "%s topic lookup failed for meeting_id=%s: %v\n",
			searchLogPrefix, sanitizeLogValue(meetingID), err)
		return ""
	}
	meeting, _ := data["meeting"].(map[string]any)
	if meeting == nil {
		return ""
	}
	topic, _ := meeting["topic"].(string)
	return topic
}

func searchCitationTopics(ctx context.Context, runtime *common.RuntimeContext, items []interface{}) map[string]string {
	topics := make(map[string]string, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		meetingID := common.GetString(item, "id")
		if meetingID == "" || meetingSearchAppLink(item) == "" {
			continue
		}
		if _, done := topics[meetingID]; done {
			continue
		}
		if topic := fetchMeetingTopic(ctx, runtime, meetingID); topic != "" {
			topics[meetingID] = topic
		}
	}
	return topics
}
