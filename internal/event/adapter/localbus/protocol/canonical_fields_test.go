// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event/model"
)

// Every canonical fact the ingress parsed must survive the wire round trip
// verbatim — the consumer restores the event from this frame instead of
// re-deriving anything from the payload.
func TestEventFrame_CarriesCanonicalFactsVerbatim(t *testing.T) {
	observed := time.Date(2023, 11, 14, 22, 13, 20, 123456789, time.UTC)
	ev := &model.Event{
		EventID:    "evt-42",
		EventType:  "im.message.receive_v1",
		SourceTime: "1700000000000",
		AppID:      "cli_test_app",
		TenantKey:  "tenant_test",
		Payload:    json.RawMessage(`{"schema":"2.0"}`),
		Timestamp:  observed,
	}

	var buf bytes.Buffer
	if err := Encode(&buf, NewEvent(ev, 7)); err != nil {
		t.Fatalf("encode: %v", err)
	}
	line, err := ReadFrame(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	decoded, err := Decode(line)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	frame, ok := decoded.(*Event)
	if !ok {
		t.Fatalf("decoded %T, want *Event", decoded)
	}

	if frame.EventID != ev.EventID || frame.EventType != ev.EventType ||
		frame.SourceTime != ev.SourceTime || frame.AppID != ev.AppID ||
		frame.TenantKey != ev.TenantKey || frame.Seq != 7 {
		t.Errorf("canonical facts drifted across the wire: %+v", frame)
	}
	// observed_at is a fixed RFC3339Nano string contract, not an incidental
	// time.Time marshal shape.
	parsed, err := time.Parse(time.RFC3339Nano, frame.ObservedAt)
	if err != nil {
		t.Fatalf("observed_at %q is not RFC3339Nano: %v", frame.ObservedAt, err)
	}
	if !parsed.Equal(observed) {
		t.Errorf("observed_at: got %v, want %v", parsed, observed)
	}
}

// Facts the upstream omitted stay omitted on the wire: the frame never invents
// values, and absent facts must not even appear as empty strings.
func TestEventFrame_MissingFactsStayAbsent(t *testing.T) {
	ev := &model.Event{
		EventType: "im.message.receive_v1",
		EventID:   "evt-1",
		Payload:   json.RawMessage(`{}`),
	}

	raw, err := json.Marshal(NewEvent(ev, 1))
	if err != nil {
		t.Fatal(err)
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"source_time", "app_id", "tenant_key", "observed_at"} {
		if _, present := asMap[absent]; present {
			t.Errorf("field %q must be omitted when the fact is missing, frame: %s", absent, raw)
		}
	}
}
