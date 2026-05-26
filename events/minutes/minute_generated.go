// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/validate"
)

const (
	minutesDetailRetryDelay = 500 * time.Millisecond
	minutesDetailMaxRetries = 2
)

// MinutesMinuteSourceOutput is the flattened minute source payload.
type MinutesMinuteSourceOutput struct {
	SourceType     string `json:"source_type,omitempty"      desc:"Minute source type"`
	SourceEntityID string `json:"source_entity_id,omitempty" desc:"Source entity ID"`
}

// MinutesMinuteGeneratedOutput is the flattened shape for minutes.minute.generated_v1.
type MinutesMinuteGeneratedOutput struct {
	Type         string                     `json:"type"                     desc:"Event type; always minutes.minute.generated_v1"`
	EventID      string                     `json:"event_id,omitempty"       desc:"Globally unique event ID; safe for deduplication"`
	Timestamp    string                     `json:"timestamp,omitempty"      desc:"Event delivery time (ms timestamp string); taken from header.create_time when present" kind:"timestamp_ms"`
	MinuteToken  string                     `json:"minute_token,omitempty"   desc:"Minute token"`
	Title        string                     `json:"title,omitempty"          desc:"Minute title"`
	MinuteSource *MinutesMinuteSourceOutput `json:"minute_source,omitempty"  desc:"Minute source metadata"`
}

func processMinutesMinuteGenerated(ctx context.Context, rt event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	var envelope struct {
		Header struct {
			EventID    string `json:"event_id"`
			EventType  string `json:"event_type"`
			CreateTime string `json:"create_time"`
		} `json:"header"`
		Event struct {
			MinuteToken  string `json:"minute_token"`
			MinuteSource struct {
				SourceType     string `json:"source_type"`
				SourceEntityID string `json:"source_entity_id"`
			} `json:"minute_source"`
		} `json:"event"`
	}
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		return raw.Payload, nil //nolint:nilerr // passthrough on malformed payload so consumers still see the event
	}

	out := &MinutesMinuteGeneratedOutput{
		Type:        envelope.Header.EventType,
		EventID:     envelope.Header.EventID,
		Timestamp:   envelope.Header.CreateTime,
		MinuteToken: envelope.Event.MinuteToken,
	}
	if out.Type == "" {
		out.Type = raw.EventType
	}
	if src := envelope.Event.MinuteSource; src.SourceType != "" || src.SourceEntityID != "" {
		out.MinuteSource = &MinutesMinuteSourceOutput{
			SourceType:     src.SourceType,
			SourceEntityID: src.SourceEntityID,
		}
	}

	if rt != nil && out.MinuteToken != "" {
		fillMinutesMinuteGeneratedDetails(ctx, rt, out)
	}

	return json.Marshal(out)
}

func fillMinutesMinuteGeneratedDetails(ctx context.Context, rt event.APIClient, out *MinutesMinuteGeneratedOutput) {
	if rt == nil || out == nil || out.MinuteToken == "" {
		return
	}

	path := fmt.Sprintf(pathMinuteDetailFmt, validate.EncodePathSegment(out.MinuteToken))

	type minuteDetailResp struct {
		Data struct {
			Minute struct {
				Title string `json:"title"`
			} `json:"minute"`
		} `json:"data"`
	}

	for attempt := 0; attempt <= minutesDetailMaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(minutesDetailRetryDelay)
		}

		raw, err := rt.CallAPI(ctx, "GET", path, nil)
		if err != nil {
			continue
		}

		var resp minuteDetailResp
		if err := json.Unmarshal(raw, &resp); err != nil {
			continue
		}

		if resp.Data.Minute.Title == "" {
			continue
		}

		out.Title = resp.Data.Minute.Title
		return
	}
}
