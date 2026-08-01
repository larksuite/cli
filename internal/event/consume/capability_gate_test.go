// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/larksuite/cli/errs"
	event "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/adapter/localbus/protocol"
	"github.com/larksuite/cli/internal/event/adapter/localbus/transport"
	"github.com/larksuite/cli/internal/event/testutil"
)

// legacyBusAcks replays, byte for byte, the hello_ack an older bus would send:
// the capability list either absent, empty, or missing the canonical-metadata
// entry. Each one must be refused — and refused before any side effect runs.
var legacyBusAcks = map[string]string{
	"no capabilities field": `{"type":"hello_ack","bus_version":"v1","first_for_key":true}`,
	"empty capability list": `{"type":"hello_ack","bus_version":"v1","first_for_key":true,"capabilities":[]}`,
	"unrelated capability":  `{"type":"hello_ack","bus_version":"v1","first_for_key":true,"capabilities":["some_other_capability"]}`,
}

func TestCapabilityGate_RefusesLegacyBusBeforeAnySideEffect(t *testing.T) {
	for name, rawAck := range legacyBusAcks {
		t.Run(name, func(t *testing.T) {
			const key = "test.evt_capability_gate"
			var setupCalls atomic.Int64
			def := compileDefForTest(t, event.KeyDefinition{
				Key:       key,
				EventType: key,
				Schema:    event.SchemaDef{Native: &event.SchemaSpec{Raw: json.RawMessage(`{"type":"object"}`)}},
				PreConsume: func(_ context.Context, _ event.APIClient, _ map[string]string) (func() error, error) {
					setupCalls.Add(1)
					return nil, nil
				},
			})

			tr := startLegacyBusStub(t, rawAck)

			err := Run(context.Background(), tr, "cap-gate-app", "", "", Options{
				EventKey: key,
				Def:      def,
				Runtime:  &fakeRT{},
				Quiet:    true,
			})
			if err == nil {
				t.Fatal("consume must refuse a bus without canonical metadata support")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Subtype != errs.SubtypeFailedPrecondition {
				t.Errorf("want a failed_precondition problem, got %v", err)
			}
			if !strings.Contains(err.Error(), protocol.CapabilityCanonicalMetadataV1) {
				t.Errorf("error must name the missing capability, got: %v", err)
			}
			if got := setupCalls.Load(); got != 0 {
				t.Errorf("pre-consume ran %d time(s) before the capability check; the gate must come first", got)
			}
		})
	}
}

// The current build's own ack must pass — a gate that rejects everything
// would be just as wrong as one that accepts everything.
func TestCapabilityGate_CurrentAckPasses(t *testing.T) {
	ack := protocol.NewHelloAck("v1", true, protocol.CapabilityCanonicalMetadataV1)
	if err := capabilityError(ack, "any.key"); err != nil {
		t.Fatalf("current bus ack must satisfy the capability gate, got: %v", err)
	}
}

// startLegacyBusStub listens on a fake transport and speaks just enough of the
// wire protocol to let a consumer attach: it answers the status probe, then
// replies to the hello with the provided raw legacy ack line.
func startLegacyBusStub(t *testing.T, rawAck string) transport.IPC {
	t.Helper()
	dir, err := os.MkdirTemp("", "capgate-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	tr := testutil.NewWrappedFake(transport.New(), dir+"/bus.sock")
	ln, err := tr.Listen("cap-gate-app")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveLegacyConn(conn, rawAck)
		}
	}()
	return tr
}

func serveLegacyConn(conn net.Conn, rawAck string) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	line, err := protocol.ReadFrame(br)
	if err != nil {
		return
	}
	msg, err := protocol.Decode(bytes.TrimRight(line, "\n"))
	if err != nil {
		return
	}
	switch msg.(type) {
	case *protocol.StatusQuery:
		_ = protocol.Encode(conn, protocol.NewStatusResponse(1, 1, 0, nil))
	case *protocol.Hello:
		_, _ = fmt.Fprintf(conn, "%s\n", rawAck)
		// Hold the conn open like a real bus would; the consumer is expected
		// to walk away after inspecting the ack.
		_, _ = protocol.ReadFrame(br)
	}
}
