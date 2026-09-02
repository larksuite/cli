// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package testutil

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"testing"

	"github.com/larksuite/cli/internal/event/adapter/localbus/protocol"
	"github.com/larksuite/cli/internal/event/adapter/localbus/transport"
	"github.com/larksuite/cli/internal/vfs"
)

// BusStub speaks just enough of the local bus protocol to drive a real
// consumer: it answers the status probe, replies to the hello with a
// caller-supplied ack, and then writes caller-supplied event frames.
//
// The ack and the frames are raw lines on purpose. Reproducing what an older
// bus put on the wire is the point — building them through the current
// protocol constructors would encode today's frame shape and prove nothing
// about compatibility.
type BusStub struct {
	rawAck    string
	rawFrames []string
}

// NewBusStub returns a stub that answers hello with rawAck and then writes
// rawFrames, in order, to every consumer that attaches.
func NewBusStub(rawAck string, rawFrames ...string) *BusStub {
	return &BusStub{rawAck: rawAck, rawFrames: rawFrames}
}

// Listen starts the stub and returns the transport a consumer should dial.
//
// The socket lives in a short directory of its own rather than t.TempDir():
// that path embeds the test name, and a unix socket path has a low length
// limit, so a descriptive subtest name would fail the bind.
func (s *BusStub) Listen(t *testing.T, appID string) *FakeTransport {
	t.Helper()
	dir, err := vfs.MkdirTemp("", "busstub-*")
	if err != nil {
		t.Fatalf("stub bus tempdir: %v", err)
	}
	t.Cleanup(func() { _ = vfs.RemoveAll(dir) })

	tr := NewWrappedFake(transport.New(), dir+"/bus.sock")
	ln, err := tr.Listen(appID)
	if err != nil {
		t.Fatalf("stub bus listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return tr
}

func (s *BusStub) serve(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	for {
		line, err := protocol.ReadFrame(br)
		if err != nil {
			return
		}
		msg, err := protocol.Decode(bytes.TrimRight(line, "\n"))
		if err != nil {
			continue
		}
		switch msg.(type) {
		case *protocol.StatusQuery:
			_ = protocol.Encode(conn, protocol.NewStatusResponse(1, 1, 0, nil))
			return
		case *protocol.Hello:
			if _, err := fmt.Fprintf(conn, "%s\n", s.rawAck); err != nil {
				return
			}
			for _, frame := range s.rawFrames {
				if _, err := fmt.Fprintf(conn, "%s\n", frame); err != nil {
					return
				}
			}
			// Hold the connection open like a real bus would, so the consumer
			// decides when to walk away.
			_, _ = protocol.ReadFrame(br)
			return
		}
	}
}

// LegacyAck is the hello_ack a bus predating canonical metadata sent: no
// capability list at all.
const LegacyAck = `{"type":"hello_ack","bus_version":"v1","first_for_key":true}`

// LegacyEventFrame renders the event frame such a bus wrote: the fields that
// version knew about and nothing else. app_id, tenant_key and observed_at are
// absent because they did not exist yet.
func LegacyEventFrame(eventType, eventID, sourceTime string, seq uint64, payload json.RawMessage) string {
	frame := map[string]any{
		"type":       "event",
		"event_type": eventType,
		"event_id":   eventID,
		"seq":        seq,
		"payload":    payload,
	}
	if sourceTime != "" {
		frame["source_time"] = sourceTime
	}
	raw, err := json.Marshal(frame)
	if err != nil {
		panic("testutil: marshal legacy frame: " + err.Error())
	}
	return string(raw)
}
