// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"

	"github.com/larksuite/cli/internal/event"
)

// VCBotEventOutput is the raw-preserving shape for bot-observed VC events.
type VCBotEventOutput struct {
	Type              string          `json:"type"                         desc:"Event type; one of the supported vc.bot.* keys"`
	EventID           string          `json:"event_id,omitempty"           desc:"Globally unique event ID; safe for deduplication"`
	Timestamp         string          `json:"timestamp,omitempty"          desc:"Event delivery time (ms timestamp string); taken from header.create_time when present" kind:"timestamp_ms"`
	CallID            string          `json:"call_id,omitempty"            desc:"Bot invitation call ID; pass through to vc agent join when present"`
	MeetingNo         string          `json:"meeting_no,omitempty"         desc:"Meeting number when present in the bot event payload"`
	ActivityEventType string          `json:"activity_event_type,omitempty" desc:"Meeting activity event subtype when present"`
	ChatEmojiTypes    []string        `json:"chat_emoji_types,omitempty"   desc:"Feishu post emotion emoji_type values extracted from vc.bot.meeting_event_v1 payloads"`
	RawEvent          json.RawMessage `json:"raw_event,omitempty"          desc:"Original VC bot event payload; authoritative for fields not normalized by lark-cli"`
}

func processVCBotMeetingInvited(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	return processVCBotEvent(raw, false)
}

func processVCBotMeetingEvent(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	return processVCBotEvent(raw, true)
}

func processVCBotMeetingEnded(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	return processVCBotEvent(raw, false)
}

func processVCBotEvent(raw *event.RawEvent, includeEmojiTypes bool) (json.RawMessage, error) {
	var payload any
	decoder := json.NewDecoder(bytes.NewReader(raw.Payload))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return raw.Payload, nil //nolint:nilerr // passthrough on malformed payload so consumers still see the event
	}

	out := &VCBotEventOutput{
		Type:              firstString(payload, "event_type"),
		EventID:           firstString(payload, "event_id"),
		Timestamp:         firstString(payload, "create_time"),
		CallID:            firstString(payload, "call_id"),
		MeetingNo:         firstString(payload, "meeting_no"),
		ActivityEventType: firstString(payload, "activity_event_type"),
		RawEvent:          append(json.RawMessage(nil), raw.Payload...),
	}
	if out.Type == "" {
		out.Type = raw.EventType
	}
	if includeEmojiTypes {
		out.ChatEmojiTypes = botEmojiTypes(payload)
	}
	return json.Marshal(out)
}

func firstString(value any, key string) string {
	switch v := value.(type) {
	case map[string]any:
		if raw, ok := v[key]; ok {
			if s := jsonString(raw); s != "" {
				return s
			}
		}
		for _, child := range orderedChildren(v) {
			if s := firstString(child, key); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range v {
			if s := firstString(child, key); s != "" {
				return s
			}
		}
	}
	return ""
}

func orderedChildren(v map[string]any) []any {
	priority := []string{"header", "event", "meeting", "meeting_info", "activity", "message", "reaction_type"}
	out := make([]any, 0, len(v))
	used := make(map[string]bool, len(priority))
	for _, key := range priority {
		if child, ok := v[key]; ok {
			out = append(out, child)
			used[key] = true
		}
	}
	rest := make([]string, 0, len(v))
	for key := range v {
		if !used[key] {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	for _, key := range rest {
		out = append(out, v[key])
	}
	return out
}

func botEmojiTypes(value any) []string {
	seen := map[string]bool{}
	var out []string
	collectEmojiTypes(value, seen, &out)
	return out
}

func collectEmojiTypes(value any, seen map[string]bool, out *[]string) {
	switch v := value.(type) {
	case map[string]any:
		for _, key := range []string{"emoji_type", "chat_emoji_type"} {
			if s := jsonString(v[key]); s != "" && !seen[s] {
				seen[s] = true
				*out = append(*out, s)
			}
		}
		if raw, ok := v["chat_emoji_types"]; ok {
			for _, s := range jsonStringSlice(raw) {
				if !seen[s] {
					seen[s] = true
					*out = append(*out, s)
				}
			}
		}
		for _, child := range v {
			collectEmojiTypes(child, seen, out)
		}
	case []any:
		for _, child := range v {
			collectEmojiTypes(child, seen, out)
		}
	}
}

func jsonString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	}
	return ""
}

func jsonStringSlice(value any) []string {
	switch v := value.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := jsonString(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), v...)
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	}
	return nil
}
