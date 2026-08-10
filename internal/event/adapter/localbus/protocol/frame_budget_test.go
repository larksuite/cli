// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event/model"
)

// A payload the ingress accepts must survive framing. The two limits used to be
// the same number, which left the top of the accepted payload range framing
// into something the consumer refused to read: the bus wrote it, the consumer
// dropped it, and the event was lost with only a warning to show for it.
func TestFrameBudget_LargestAcceptedPayloadStillFits(t *testing.T) {
	if MaxFrameBytes <= MaxEventPayloadBytes {
		t.Fatalf("MaxFrameBytes (%d) must exceed MaxEventPayloadBytes (%d), otherwise a full-size payload cannot be framed",
			MaxFrameBytes, MaxEventPayloadBytes)
	}

	// A payload of exactly the accepted maximum, carrying long but realistic
	// canonical metadata: a long event type, a long event id, a tenant key and
	// the widest possible seq.
	body := `{"text":"` + strings.Repeat("x", MaxEventPayloadBytes-len(`{"text":""}`)) + `"}`
	if len(body) != MaxEventPayloadBytes {
		t.Fatalf("test payload is %d bytes, want exactly %d", len(body), MaxEventPayloadBytes)
	}

	ev := &model.Event{
		EventID:    "c2f8a1e0-3b4d-4f6a-9c8e-7d5b1a2f3e4c-0123456789",
		EventType:  "vc.recording.recording_transcript_generated_v1",
		SourceTime: "1700000000000",
		AppID:      "cli_FAKEFAKEFAKEFAKE",
		TenantKey:  "TENANT_FAKE_FOR_FRAME_SIZE_TESTS",
		Payload:    json.RawMessage(body),
		Timestamp:  time.Date(2026, 8, 4, 12, 34, 56, 123456789, time.UTC),
	}

	var buf bytes.Buffer
	if err := Encode(&buf, NewEvent(ev, 1<<64-1)); err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Encode appends the delimiter, so the buffer is the whole frame as it goes
	// on the wire — which is what the reader measures.
	if buf.Len() > MaxFrameBytes {
		t.Errorf("a full-size payload framed to %d bytes, over the %d limit; raise maxFrameOverheadBytes",
			buf.Len(), MaxFrameBytes)
	}

	// The frame must also come back out, not just fit: this is the read path the
	// consumer uses.
	line, err := ReadFrame(bufio.NewReaderSize(bytes.NewReader(buf.Bytes()), MaxFrameBytes+1))
	if err != nil {
		t.Fatalf("a full-size frame must be readable, got: %v", err)
	}
	decoded, err := Decode(bytes.TrimRight(line, "\n"))
	if err != nil {
		t.Fatalf("decode a full-size frame: %v", err)
	}
	frame, ok := decoded.(*Event)
	if !ok {
		t.Fatalf("decoded %T, want *Event", decoded)
	}
	if !bytes.Equal(frame.Payload, json.RawMessage(body)) {
		t.Error("a full-size payload must round trip unchanged")
	}
}

// The overhead allowance has to be more than decoration: it must cover the
// metadata a frame actually carries, with the payload budget left over.
func TestFrameBudget_OverheadCoversCanonicalMetadata(t *testing.T) {
	ev := &model.Event{
		EventID:    "c2f8a1e0-3b4d-4f6a-9c8e-7d5b1a2f3e4c-0123456789",
		EventType:  "vc.recording.recording_transcript_generated_v1",
		SourceTime: "1700000000000",
		AppID:      "cli_FAKEFAKEFAKEFAKE",
		TenantKey:  "TENANT_FAKE_FOR_FRAME_SIZE_TESTS",
		Payload:    json.RawMessage(`{}`),
		Timestamp:  time.Date(2026, 8, 4, 12, 34, 56, 123456789, time.UTC),
	}
	var buf bytes.Buffer
	if err := Encode(&buf, NewEvent(ev, 1<<64-1)); err != nil {
		t.Fatalf("encode: %v", err)
	}
	overhead := buf.Len() - len(`{}`)
	if overhead > maxFrameOverheadBytes {
		t.Errorf("frame metadata measures %d bytes, over the %d allowance", overhead, maxFrameOverheadBytes)
	}
}
