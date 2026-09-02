// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	event "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/adapter/localbus/protocol"
	"github.com/larksuite/cli/internal/event/adapter/localbus/transport"
	"github.com/larksuite/cli/internal/event/testutil"
)

// legacyBusAcks replays, byte for byte, the hello_ack an older bus would send:
// the capability list either absent, empty, or missing the canonical-metadata
// entry.
var legacyBusAcks = map[string]string{
	"no capabilities field": `{"type":"hello_ack","bus_version":"v1","first_for_key":true}`,
	"empty capability list": `{"type":"hello_ack","bus_version":"v1","first_for_key":true,"capabilities":[]}`,
	"unrelated capability":  `{"type":"hello_ack","bus_version":"v1","first_for_key":true,"capabilities":["some_other_capability"]}`,
}

// resourceScopedDef is a key whose subscription identity carries a parameter,
// the shape that cannot fall back to a legacy bus: this consumer hashes the
// scope while an old consumer of the same resource uses the bare key, so each
// acts as first and last for its own scope and unsubscribes the other. The old
// bus can never grow a guard against that, so the only safe answer is refusal.
func resourceScopedDef(t *testing.T, key string, preConsume func()) *event.KeyDefinition {
	t.Helper()
	return compileDefForTest(t, event.KeyDefinition{
		Key:       key,
		EventType: key,
		Params: []event.ParamDef{
			{Name: "resource_id", Type: event.ParamString, Required: true, SubscriptionKey: true},
		},
		Schema: event.SchemaDef{Native: &event.SchemaSpec{Raw: json.RawMessage(`{"type":"object"}`)}},
		PreConsume: func(_ context.Context, _ event.APIClient, _ map[string]string) (func() error, error) {
			preConsume()
			return nil, nil
		},
	})
}

func TestCapabilityGate_RefusesLegacyBusForResourceScopedKeyBeforeAnySideEffect(t *testing.T) {
	for name, rawAck := range legacyBusAcks {
		t.Run(name, func(t *testing.T) {
			const key = "test.evt_capability_gate"
			var setupCalls atomic.Int64
			def := resourceScopedDef(t, key, func() { setupCalls.Add(1) })

			tr := startLegacyBusStub(t, rawAck)

			// Bounded on purpose: refusal returns before the consume loop
			// starts, so a regression that degrades instead would otherwise
			// block until the package timeout and report that rather than the
			// missing refusal.
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			err := Run(ctx, tr, "cap-gate-app", "", "", Options{
				EventKey: key,
				Def:      def,
				Params:   map[string]string{"resource_id": "res-1"},
				Runtime:  &fakeRT{},
				Out:      io.Discard,
				Quiet:    true,
			})
			if err == nil {
				t.Fatal("a resource-scoped key must refuse a bus without canonical metadata support")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Subtype != errs.SubtypeFailedPrecondition {
				t.Errorf("want a failed_precondition problem, got %v", err)
			}
			if !strings.Contains(err.Error(), protocol.CapabilityCanonicalMetadataV1) {
				t.Errorf("error must name the missing capability, got: %v", err)
			}
			if got := setupCalls.Load(); got != 0 {
				t.Errorf("pre-consume ran %d time(s) before the refusal; it must come first", got)
			}
		})
	}
}

// A key whose subscription identity is one-dimensional keeps working against an
// older bus: its scope string is byte-identical across versions, so nothing can
// desynchronize. The operator is told the mode in one line, which Run emits per
// connection rather than per event.
func TestCapabilityGate_LegacyBusDegradesForOneDimensionalKey(t *testing.T) {
	const key = "test.evt_capability_degrade"
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

	var errOut bytes.Buffer
	tr := startLegacyBusStub(t, legacyBusAcks["no capabilities field"])

	err := Run(context.Background(), tr, "cap-gate-app", "", "", Options{
		EventKey: key,
		Def:      def,
		Runtime:  &fakeRT{},
		ErrOut:   &errOut,
		Out:      io.Discard,
		Timeout:  200 * time.Millisecond,
	})
	if problem, ok := errs.ProblemOf(err); ok && problem.Subtype == errs.SubtypeFailedPrecondition {
		t.Fatalf("a one-dimensional key must not be refused for a legacy bus, got: %v", err)
	}
	if got := setupCalls.Load(); got != 1 {
		t.Errorf("pre-consume ran %d time(s); degrading must not skip preparation", got)
	}
	notices := strings.Count(errOut.String(), "legacy compatibility mode")
	if notices != 1 {
		t.Errorf("the compatibility notice must appear exactly once per connection, got %d", notices)
	}
}

// The current build's own ack must select the normal path — a negotiation that
// degrades everything would be just as wrong as one that refuses everything.
func TestCapabilityGate_CurrentAckSelectsNormalPath(t *testing.T) {
	ack := protocol.NewHelloAck("v1", true, protocol.CapabilityCanonicalMetadataV1)
	def := compileDefForTest(t, event.KeyDefinition{
		Key:       "any.key",
		EventType: "any.key",
		Schema:    event.SchemaDef{Native: &event.SchemaSpec{Raw: json.RawMessage(`{"type":"object"}`)}},
	})
	mode, err := negotiateMetadataMode(ack, def, "cap-gate-app")
	if err != nil {
		t.Fatalf("current bus ack must satisfy negotiation, got: %v", err)
	}
	if mode.enabled {
		t.Error("a capable bus must not put the connection in compatibility mode")
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
