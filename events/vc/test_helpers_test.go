// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/event"
)

// fillCanonicalFromHeader copies the payload envelope header metadata onto
// the RawEvent canonical fields. Process handlers read event_id and
// create_time from the RawEvent, which the consume pipeline fills from the
// envelope header before dispatch; tests that hand-build a RawEvent must
// mirror that so both views agree.
func fillCanonicalFromHeader(t *testing.T, raw *event.RawEvent) {
	t.Helper()
	var envelope struct {
		Header struct {
			EventID    string `json:"event_id"`
			EventType  string `json:"event_type"`
			CreateTime string `json:"create_time"`
		} `json:"header"`
	}
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		t.Fatalf("parse envelope header: %v", err)
	}
	raw.EventID = envelope.Header.EventID
	if envelope.Header.EventType != "" {
		raw.EventType = envelope.Header.EventType
	}
	raw.SourceTime = envelope.Header.CreateTime
}

type stubAPIClient struct {
	callFn func(ctx context.Context, method, path string, body any) (json.RawMessage, error)
}

func (s *stubAPIClient) CallAPI(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	if s.callFn == nil {
		return nil, nil
	}
	return s.callFn(ctx, method, path, body)
}

func assertSubscriptionRequest(t *testing.T, gotBody any, wantEventType string) {
	t.Helper()
	want := map[string]string{"event_type": wantEventType}
	if !reflect.DeepEqual(gotBody, want) {
		t.Fatalf("request body = %#v, want %#v", gotBody, want)
	}
}
