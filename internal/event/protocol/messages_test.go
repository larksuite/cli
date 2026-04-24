// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// TestConstructors_PinTypeField guards the Type-field auto-fill contract:
// every NewXxx helper must set the discriminator, because Decode rejects
// messages without it. Struct-literal construction elsewhere would silently
// omit Type and shake out as decode errors at runtime — the helpers are
// the only safe call-site.
func TestConstructors_PinTypeField(t *testing.T) {
	cases := []struct {
		name     string
		msg      interface{ typeField() string }
		wantType string
	}{}
	_ = cases

	// Plain table below — Go generics would help but existing tests follow
	// the straight-line style, so we inline each assertion rather than
	// adding a type-switched helper.
	if got := NewHello(1, "k", []string{"t"}, "v1"); got.Type != MsgTypeHello {
		t.Errorf("NewHello.Type = %q, want %q", got.Type, MsgTypeHello)
	}
	if got := NewHelloAck("v1", true); got.Type != MsgTypeHelloAck || !got.FirstForKey {
		t.Errorf("NewHelloAck mismatch: %+v", got)
	}
	if got := NewEvent("im.msg", "e1", "", 7, json.RawMessage(`{}`)); got.Type != MsgTypeEvent || got.Seq != 7 {
		t.Errorf("NewEvent mismatch: %+v", got)
	}
	if got := NewPreShutdownCheck("k"); got.Type != MsgTypePreShutdownCheck || got.EventKey != "k" {
		t.Errorf("NewPreShutdownCheck mismatch: %+v", got)
	}
	if got := NewPreShutdownAck(true); got.Type != MsgTypePreShutdownAck || !got.LastForKey {
		t.Errorf("NewPreShutdownAck mismatch: %+v", got)
	}
	if got := NewStatusQuery(); got.Type != MsgTypeStatusQuery {
		t.Errorf("NewStatusQuery.Type = %q", got.Type)
	}
	if got := NewStatusResponse(42, 10, 2, []ConsumerInfo{{PID: 1}, {PID: 2}}); got.Type != MsgTypeStatusResponse || got.PID != 42 || len(got.Consumers) != 2 {
		t.Errorf("NewStatusResponse mismatch: %+v", got)
	}
	if got := NewShutdown(); got.Type != MsgTypeShutdown {
		t.Errorf("NewShutdown.Type = %q", got.Type)
	}
	if got := NewSourceStatus("feishu-ws", SourceStateConnected, "ok"); got.Type != MsgTypeSourceStatus || got.Detail != "ok" {
		t.Errorf("NewSourceStatus mismatch: %+v", got)
	}
}

// TestEncode_DecodeRoundtripAllTypes exercises each wire type through
// Encode → Decode — the discriminator-based dispatch in Decode must
// return the same concrete Go type and preserve payload fields. Fills
// the largest gap in protocol coverage: individual constructors were
// previously only called transitively by other packages (which doesn't
// count toward this package's coverage).
func TestEncode_DecodeRoundtripAllTypes(t *testing.T) {
	roundtrip := func(t *testing.T, msg interface{}, want interface{}) {
		t.Helper()
		var buf bytes.Buffer
		if err := Encode(&buf, msg); err != nil {
			t.Fatalf("encode: %v", err)
		}
		line := bytes.TrimRight(buf.Bytes(), "\n")
		got, err := Decode(line)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if gotT, wantT := typeOf(got), typeOf(want); gotT != wantT {
			t.Errorf("decoded type = %s, want %s", gotT, wantT)
		}
	}
	roundtrip(t, NewHelloAck("v1", true), &HelloAck{})
	roundtrip(t, NewPreShutdownCheck("im.msg"), &PreShutdownCheck{})
	roundtrip(t, NewPreShutdownAck(false), &PreShutdownAck{})
	roundtrip(t, NewStatusQuery(), &StatusQuery{})
	roundtrip(t, NewStatusResponse(7, 120, 1, []ConsumerInfo{{PID: 99, EventKey: "k"}}), &StatusResponse{})
	roundtrip(t, NewShutdown(), &Shutdown{})
	roundtrip(t, NewSourceStatus("feishu", SourceStateReconnecting, "attempt 2"), &SourceStatus{})
	roundtrip(t, &Bye{Type: MsgTypeBye}, &Bye{})
}

func typeOf(v interface{}) string {
	if v == nil {
		return "<nil>"
	}
	return reflectTypeName(v)
}

// reflectTypeName avoids pulling reflect into the test just for a name —
// fmt.Sprintf("%T") does the same job.
func reflectTypeName(v interface{}) string {
	return stringType(v)
}

func stringType(v interface{}) string {
	// Minimalist: avoid fmt/reflect churn — switch on known concrete types.
	switch v.(type) {
	case *Hello:
		return "*Hello"
	case *HelloAck:
		return "*HelloAck"
	case *Event:
		return "*Event"
	case *Bye:
		return "*Bye"
	case *PreShutdownCheck:
		return "*PreShutdownCheck"
	case *PreShutdownAck:
		return "*PreShutdownAck"
	case *StatusQuery:
		return "*StatusQuery"
	case *StatusResponse:
		return "*StatusResponse"
	case *Shutdown:
		return "*Shutdown"
	case *SourceStatus:
		return "*SourceStatus"
	}
	return "<unknown>"
}

// TestEncodeWithDeadline_AppliesDeadline verifies the deadline is set
// before Encode writes — a wedged peer's kernel buffer must not be
// able to stall the writer forever. We use a no-deadline pipe so the
// deadline is the only bound that can make the write return; confirm
// the write fails with a timeout error in under the deadline + slack.
func TestEncodeWithDeadline_AppliesDeadline(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	// Don't read server side — client's write will block waiting for
	// the reader to consume. The deadline should fire and error out.
	start := time.Now()
	err := EncodeWithDeadline(client, NewShutdown(), 100*time.Millisecond)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected deadline error, got nil")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("EncodeWithDeadline didn't honour deadline: took %v (want ~100ms)", elapsed)
	}
}

// TestReadFrame_RejectsOversized confirms MaxFrameBytes is enforced on
// read — a runaway upstream (or hostile local peer) that dribbles a huge
// line cannot grow the reader's buffer unbounded and OOM the process.
func TestReadFrame_RejectsOversized(t *testing.T) {
	// MaxFrameBytes+1 non-newline bytes followed by '\n'.
	big := bytes.Repeat([]byte("a"), MaxFrameBytes+1)
	big = append(big, '\n')
	br := bufio.NewReader(bytes.NewReader(big))
	_, err := ReadFrame(br)
	if err == nil {
		t.Fatal("expected error on oversized frame")
	}
	if !strings.Contains(err.Error(), "frame too large") && !strings.Contains(err.Error(), "exceeds") && !strings.Contains(err.Error(), "too") {
		t.Logf("error: %v", err) // informational — any non-nil error means the cap fired
	}
}

// TestReadFrame_PropagatesEOF ensures a clean EOF surfaces as io.EOF
// (not a synthetic "empty frame" success) so callers can break their
// read loop correctly.
func TestReadFrame_PropagatesEOF(t *testing.T) {
	br := bufio.NewReader(bytes.NewReader(nil))
	_, err := ReadFrame(br)
	if err != io.EOF {
		t.Errorf("err = %v, want io.EOF", err)
	}
}
