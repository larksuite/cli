// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

// Every NewXxx helper must set the Type discriminator (Decode rejects messages without it).
func TestConstructors_PinTypeField(t *testing.T) {
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
		if gotT, wantT := fmt.Sprintf("%T", got), fmt.Sprintf("%T", want); gotT != wantT {
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

// EncodeWithDeadline must apply a write deadline so a wedged peer can't stall the writer forever.
func TestEncodeWithDeadline_AppliesDeadline(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
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

func TestReadFrame_RejectsOversized(t *testing.T) {
	big := bytes.Repeat([]byte("a"), MaxFrameBytes+1)
	big = append(big, '\n')
	br := bufio.NewReader(bytes.NewReader(big))
	_, err := ReadFrame(br)
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("ReadFrame on oversized input: err = %v, want ErrFrameTooLarge", err)
	}
}

func TestReadFrame_PropagatesEOF(t *testing.T) {
	br := bufio.NewReader(bytes.NewReader(nil))
	_, err := ReadFrame(br)
	if err != io.EOF {
		t.Errorf("err = %v, want io.EOF", err)
	}
}
