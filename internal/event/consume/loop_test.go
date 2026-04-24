// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/protocol"
)

// echoKeyDef returns a KeyDefinition with a trivial Process that echoes the
// raw payload unchanged — enough to exercise consumeLoop's Process → sink
// path without depending on a real EventKey registration.
func echoKeyDef(key string) *event.KeyDefinition {
	return &event.KeyDefinition{
		Key:        key,
		EventType:  key,
		BufferSize: 32,
		Workers:    1,
		Process: func(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
			return raw.Payload, nil
		},
	}
}

// busSide simulates the bus end of the consume pipe: writes a burst of
// Events then, on receiving PreShutdownCheck, replies with a
// PreShutdownAck{LastForKey: lastForKey}. Returns when either the writer
// closes the pipe or we've replied with the ack.
func busSide(t *testing.T, server net.Conn, events []*protocol.Event, ackLast bool) {
	t.Helper()
	for _, evt := range events {
		if err := protocol.Encode(server, evt); err != nil {
			return
		}
	}
	br := bufio.NewReader(server)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_ = server.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		line, err := protocol.ReadFrame(br)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return // EOF / conn closed
		}
		msg, decErr := protocol.Decode(bytes.TrimRight(line, "\n"))
		if decErr != nil {
			continue
		}
		if _, ok := msg.(*protocol.PreShutdownCheck); ok {
			_ = protocol.Encode(server, protocol.NewPreShutdownAck(ackLast))
			return
		}
	}
}

// TestConsumeLoop_DeliversEventsAndExitsOnMaxEvents exercises the primary
// happy path of consumeLoop: events arriving over conn are decoded,
// Process runs, the sink receives NDJSON, and MaxEvents triggers the
// ctx.Done shutdown branch (which sends PreShutdownCheck and reads the
// ack for lastForKey). Covers the reader goroutine + worker loop +
// max-events cancel + shutdown handshake interaction that earlier tests
// miss entirely (loop.go was 0% covered at patch time).
func TestConsumeLoop_DeliversEventsAndExitsOnMaxEvents(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	events := []*protocol.Event{
		protocol.NewEvent("test.evt", "e1", "", 1, json.RawMessage(`{"n":1}`)),
		protocol.NewEvent("test.evt", "e2", "", 2, json.RawMessage(`{"n":2}`)),
	}
	go busSide(t, server, events, true)

	var stdout bytes.Buffer
	opts := Options{
		EventKey:  "test.key",
		Out:       &stdout,
		ErrOut:    io.Discard,
		Quiet:     true,
		MaxEvents: 2,
	}

	var lastForKey bool
	var emitted atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := consumeLoop(ctx, client, bufio.NewReader(client), echoKeyDef("test.key"), opts, &lastForKey, &emitted)
	if err != nil {
		t.Fatalf("consumeLoop: %v", err)
	}
	if got := emitted.Load(); got != 2 {
		t.Errorf("emitted = %d, want 2", got)
	}
	if !lastForKey {
		t.Error("lastForKey = false, want true (bus acked LastForKey=true)")
	}
	out := stdout.String()
	for _, want := range []string{`{"n":1}`, `{"n":2}`} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q; full:\n%s", want, out)
		}
	}
}

// TestConsumeLoop_SeqGapEmitsWarning guards the seq-gap detection inside
// the reader goroutine: the bus assigns monotonic per-conn seqs, so a
// skip at the consumer side means the bus dropped events via backpressure.
// The WARN line is the only observable that silent loss is detected.
func TestConsumeLoop_SeqGapEmitsWarning(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	events := []*protocol.Event{
		protocol.NewEvent("test.evt", "e1", "", 1, json.RawMessage(`{"n":1}`)),
		protocol.NewEvent("test.evt", "e5", "", 5, json.RawMessage(`{"n":5}`)), // gap of 3
	}
	go busSide(t, server, events, true)

	var stdout, stderr bytes.Buffer
	opts := Options{
		EventKey:  "test.key",
		Out:       &stdout,
		ErrOut:    &stderr,
		Quiet:     false, // must be false for seq-gap WARN to print
		MaxEvents: 2,
	}

	var lastForKey bool
	var emitted atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := consumeLoop(ctx, client, bufio.NewReader(client), echoKeyDef("test.key"), opts, &lastForKey, &emitted); err != nil {
		t.Fatalf("consumeLoop: %v", err)
	}
	if got := emitted.Load(); got != 2 {
		t.Errorf("emitted = %d, want 2", got)
	}
	if !strings.Contains(stderr.String(), "WARN: event seq gap 1->5") {
		t.Errorf("stderr missing seq-gap warning; got:\n%s", stderr.String())
	}
}

// TestConsumeLoop_JQFilterAppliedPerEvent verifies the JQ pre-compile and
// per-event apply path. CompileJQ errors fail fast in Run(); once we're
// in consumeLoop a valid expression must filter every event's processed
// payload before it reaches the sink.
func TestConsumeLoop_JQFilterAppliedPerEvent(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// Payloads: keep={"keep":true,"n":1} and drop={"keep":false,"n":2}.
	events := []*protocol.Event{
		protocol.NewEvent("test.evt", "e1", "", 1, json.RawMessage(`{"keep":true,"n":1}`)),
		protocol.NewEvent("test.evt", "e2", "", 2, json.RawMessage(`{"keep":false,"n":2}`)),
	}
	go busSide(t, server, events, true)

	var stdout bytes.Buffer
	opts := Options{
		EventKey:  "test.key",
		Out:       &stdout,
		ErrOut:    io.Discard,
		Quiet:     true,
		JQExpr:    "select(.keep) | .n",
		MaxEvents: 1, // keep event emits once; drop event produces no sink write so we bound on the keep
	}

	var lastForKey bool
	var emitted atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := consumeLoop(ctx, client, bufio.NewReader(client), echoKeyDef("test.key"), opts, &lastForKey, &emitted); err != nil {
		t.Fatalf("consumeLoop: %v", err)
	}
	if got := emitted.Load(); got != 1 {
		t.Errorf("emitted = %d, want 1 (only the selected event should count)", got)
	}
	out := strings.TrimSpace(stdout.String())
	if out != "1" {
		t.Errorf("stdout = %q, want %q (only .n of the kept event)", out, "1")
	}
}

// TestConsumeLoop_CompileJQFailsEarly guards the jq pre-compile gate
// inside consumeLoop: library callers that skip Run's own pre-compile
// (tests, direct use) still get an immediate error instead of a
// long-running consume that never emits.
func TestConsumeLoop_CompileJQFailsEarly(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	opts := Options{
		EventKey: "test.key",
		Out:      io.Discard,
		ErrOut:   io.Discard,
		Quiet:    true,
		JQExpr:   "not a valid jq expression (((",
	}

	var lastForKey bool
	var emitted atomic.Int64
	err := consumeLoop(context.Background(), client, bufio.NewReader(client), echoKeyDef("test.key"), opts, &lastForKey, &emitted)
	if err == nil {
		t.Fatal("consumeLoop should fail immediately on bad jq expression")
	}
}

// TestIsTerminalSinkError covers the classifier. EPIPE / fs.ErrClosed
// are terminal (downstream pipe permanently gone → must shut down),
// anything else is transient.
func TestIsTerminalSinkError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"EPIPE raw", syscall.EPIPE, true},
		{"EPIPE wrapped", fmt.Errorf("write: %w", syscall.EPIPE), true},
		{"ErrClosed", io.ErrClosedPipe, false}, // not fs.ErrClosed
		{"transient disk full", errors.New("no space left on device"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTerminalSinkError(tc.err); got != tc.want {
				t.Errorf("isTerminalSinkError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
