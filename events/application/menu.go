// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package application

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/internal/event"
)

// BotMenuOutput is the flattened shape for application.bot.menu_v6.
type BotMenuOutput struct {
	Type            string `json:"type"                         desc:"Event type; always application.bot.menu_v6"`
	EventID         string `json:"event_id,omitempty"           desc:"Globally unique event ID; safe for deduplication"`
	Timestamp       string `json:"timestamp,omitempty"          desc:"Event delivery time (ms timestamp string); prefers header.create_time" kind:"timestamp_ms"`
	AppID           string `json:"app_id,omitempty"             desc:"Application ID from the event header"`
	TenantKey       string `json:"tenant_key,omitempty"         desc:"Tenant key from the event header"`
	EventKey        string `json:"event_key,omitempty"          desc:"Developer-defined bot menu event key"`
	MenuTimestamp   string `json:"menu_timestamp,omitempty"     desc:"Menu click timestamp from the event body"                           kind:"timestamp_ms"`
	OperatorID      string `json:"operator_id,omitempty"        desc:"Operator open_id; kept as a short alias of operator_open_id"        kind:"open_id"`
	OperatorOpenID  string `json:"operator_open_id,omitempty"   desc:"Operator open_id"                                                   kind:"open_id"`
	OperatorUnionID string `json:"operator_union_id,omitempty"  desc:"Operator union_id"                                                  kind:"union_id"`
	OperatorUserID  string `json:"operator_user_id,omitempty"   desc:"Operator user_id"                                                   kind:"user_id"`
	OperatorName    string `json:"operator_name,omitempty"      desc:"Operator display name"`
}

func processBotMenu(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	var envelope struct {
		Header struct {
			EventID    string `json:"event_id"`
			EventType  string `json:"event_type"`
			CreateTime string `json:"create_time"`
			AppID      string `json:"app_id"`
			TenantKey  string `json:"tenant_key"`
		} `json:"header"`
		Event struct {
			EventKey  string          `json:"event_key"`
			Timestamp json.RawMessage `json:"timestamp"`
			Operator  struct {
				OperatorID struct {
					OpenID  string `json:"open_id"`
					UnionID string `json:"union_id"`
					UserID  string `json:"user_id"`
				} `json:"operator_id"`
				OperatorName string `json:"operator_name"`
			} `json:"operator"`
		} `json:"event"`
	}
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		return raw.Payload, nil //nolint:nilerr // passthrough on malformed payload so consumers still see the event
	}

	menuTimestamp := timestampMillisString(envelope.Event.Timestamp)
	timestamp := envelope.Header.CreateTime
	if timestamp == "" {
		timestamp = menuTimestamp
	}
	operatorID := envelope.Event.Operator.OperatorID.OpenID

	out := &BotMenuOutput{
		Type:            eventTypeBotMenuV6,
		EventID:         envelope.Header.EventID,
		Timestamp:       timestamp,
		AppID:           envelope.Header.AppID,
		TenantKey:       envelope.Header.TenantKey,
		EventKey:        envelope.Event.EventKey,
		MenuTimestamp:   menuTimestamp,
		OperatorID:      operatorID,
		OperatorOpenID:  operatorID,
		OperatorUnionID: envelope.Event.Operator.OperatorID.UnionID,
		OperatorUserID:  envelope.Event.Operator.OperatorID.UserID,
		OperatorName:    envelope.Event.Operator.OperatorName,
	}
	return json.Marshal(out)
}

func rawScalarString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return s
}

func timestampMillisString(raw json.RawMessage) string {
	s := rawScalarString(raw)
	if len(s) == 10 && allDigits(s) {
		return s + "000"
	}
	return s
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
