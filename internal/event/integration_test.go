// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package event_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/bus"
	"github.com/larksuite/cli/internal/event/protocol"
	"github.com/larksuite/cli/internal/event/source"
	"github.com/larksuite/cli/internal/event/testutil"
	"github.com/larksuite/cli/internal/event/transport"
)

type integTestOut struct{ A string }

func integNativeSchema() event.SchemaDef {
	return event.SchemaDef{Native: &event.SchemaSpec{Type: reflect.TypeOf(integTestOut{})}}
}

func waitForBusReady(t *testing.T, tr transport.IPC, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if conn, err := tr.Dial(addr); err == nil {
			conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("bus at %s did not come up within 2s", addr)
}

func runBus(t *testing.T, b *bus.Bus, ctx context.Context) {
	t.Helper()
	errCh := make(chan error, 1)
	go func() { errCh <- b.Run(ctx) }()
	t.Cleanup(func() {
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("bus.Run returned unexpected error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Log("bus did not exit within 2s of test cleanup (non-fatal)")
		}
	})
}

type mockIntegSource struct {
	mu     sync.Mutex
	emitFn func(*event.RawEvent)
}

func (s *mockIntegSource) Name() string { return "mock-integration" }

func (s *mockIntegSource) Start(ctx context.Context, _ []string, emit func(*event.RawEvent), _ source.StatusNotifier) error {
	s.mu.Lock()
	s.emitFn = emit
	s.mu.Unlock()
	<-ctx.Done()
	return nil
}

func (s *mockIntegSource) emit(e *event.RawEvent) {
	s.mu.Lock()
	fn := s.emitFn
	s.mu.Unlock()
	if fn != nil {
		fn(e)
	}
}

func TestIntegration_BusToConsume(t *testing.T) {
	event.ResetRegistryForTest()
	source.ResetForTest()

	event.RegisterKey(event.KeyDefinition{
		Key:       "test.event.v1",
		EventType: "test.event.v1",
		Schema:    integNativeSchema(),
	})

	mockSrc := &mockIntegSource{}
	source.Register(mockSrc)

	dir := t.TempDir()
	addr := filepath.Join(dir, "t.sock")

	tr := transport.New()
	logger := log.New(os.Stderr, "[test-bus] ", log.LstdFlags)

	testTr := testutil.NewWrappedFake(tr, addr)
	b := bus.NewBus("test-app", "test-secret", "", testTr, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runBus(t, b, ctx)
	waitForBusReady(t, testTr, addr)

	conn, err := testTr.Dial(addr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	hello := &protocol.Hello{
		Type:       protocol.MsgTypeHello,
		PID:        os.Getpid(),
		EventKey:   "test.event.v1",
		EventTypes: []string{"test.event.v1"},
		Version:    "v1",
	}
	if err := protocol.Encode(conn, hello); err != nil {
		t.Fatalf("encode hello: %v", err)
	}

	scanner := bufio.NewScanner(conn)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if !scanner.Scan() {
		t.Fatal("no hello_ack received")
	}
	msg, err := protocol.Decode(scanner.Bytes())
	if err != nil {
		t.Fatalf("decode hello_ack: %v", err)
	}
	ack, ok := msg.(*protocol.HelloAck)
	if !ok {
		t.Fatalf("expected HelloAck, got %T", msg)
	}
	if !ack.FirstForKey {
		t.Error("expected first_for_key to be true")
	}

	mockSrc.emit(&event.RawEvent{
		EventID:   "evt-integration-1",
		EventType: "test.event.v1",
		Payload:   json.RawMessage(`{"test": true}`),
		Timestamp: time.Now(),
	})

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if !scanner.Scan() {
		t.Fatal("no event received")
	}
	evtMsg, err := protocol.Decode(scanner.Bytes())
	if err != nil {
		t.Fatalf("decode event: %v", err)
	}
	evt, ok := evtMsg.(*protocol.Event)
	if !ok {
		t.Fatalf("expected Event, got %T", evtMsg)
	}
	if evt.EventType != "test.event.v1" {
		t.Errorf("expected event_type %q, got %q", "test.event.v1", evt.EventType)
	}
	var payloadMap map[string]interface{}
	if err := json.Unmarshal(evt.Payload, &payloadMap); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if v, ok := payloadMap["test"]; !ok || v != true {
		t.Errorf("unexpected payload: %s", string(evt.Payload))
	}

	conn.Close()
	time.Sleep(100 * time.Millisecond)

	cancel()
}

func TestIntegration_MultipleConsumers(t *testing.T) {
	event.ResetRegistryForTest()
	source.ResetForTest()

	event.RegisterKey(event.KeyDefinition{
		Key:       "multi.event.v1",
		EventType: "multi.event.v1",
		Schema:    integNativeSchema(),
	})

	mockSrc := &mockIntegSource{}
	source.Register(mockSrc)

	dir := t.TempDir()
	addr := filepath.Join(dir, "m.sock")
	tr := transport.New()
	logger := log.New(os.Stderr, "[test-multi] ", log.LstdFlags)

	testTr := testutil.NewWrappedFake(tr, addr)
	b := bus.NewBus("test-multi", "test-secret", "", testTr, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runBus(t, b, ctx)
	waitForBusReady(t, testTr, addr)

	connectConsumer := func(name string) (net.Conn, *bufio.Scanner) {
		conn, err := testTr.Dial(addr)
		if err != nil {
			t.Fatalf("dial %s: %v", name, err)
		}
		hello := &protocol.Hello{
			Type:       protocol.MsgTypeHello,
			PID:        os.Getpid(),
			EventKey:   "multi.event.v1",
			EventTypes: []string{"multi.event.v1"},
			Version:    "v1",
		}
		protocol.Encode(conn, hello)
		sc := bufio.NewScanner(conn)
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		if !sc.Scan() {
			t.Fatalf("%s: no hello_ack", name)
		}
		msg, _ := protocol.Decode(sc.Bytes())
		if _, ok := msg.(*protocol.HelloAck); !ok {
			t.Fatalf("%s: expected HelloAck, got %T", name, msg)
		}
		return conn, sc
	}

	conn1, sc1 := connectConsumer("consumer-1")
	defer conn1.Close()
	conn2, sc2 := connectConsumer("consumer-2")
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	mockSrc.emit(&event.RawEvent{
		EventID:   "evt-multi-1",
		EventType: "multi.event.v1",
		Payload:   json.RawMessage(`{"fan":"out"}`),
		Timestamp: time.Now(),
	})

	for _, tc := range []struct {
		name string
		conn net.Conn
		sc   *bufio.Scanner
	}{
		{"consumer-1", conn1, sc1},
		{"consumer-2", conn2, sc2},
	} {
		tc.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		if !tc.sc.Scan() {
			t.Fatalf("%s: no event received", tc.name)
		}
		evtMsg, err := protocol.Decode(tc.sc.Bytes())
		if err != nil {
			t.Fatalf("%s: decode event: %v", tc.name, err)
		}
		evt, ok := evtMsg.(*protocol.Event)
		if !ok {
			t.Fatalf("%s: expected Event, got %T", tc.name, evtMsg)
		}
		if evt.EventType != "multi.event.v1" {
			t.Errorf("%s: expected event_type %q, got %q", tc.name, "multi.event.v1", evt.EventType)
		}
	}

	cancel()
}

func TestIntegration_DedupFilter(t *testing.T) {
	event.ResetRegistryForTest()
	source.ResetForTest()

	event.RegisterKey(event.KeyDefinition{
		Key:       "dedup.event.v1",
		EventType: "dedup.event.v1",
		Schema:    integNativeSchema(),
	})

	mockSrc := &mockIntegSource{}
	source.Register(mockSrc)

	dir := t.TempDir()
	addr := filepath.Join(dir, "d.sock")
	tr := transport.New()
	logger := log.New(os.Stderr, "[test-dedup] ", log.LstdFlags)

	testTr := testutil.NewWrappedFake(tr, addr)
	b := bus.NewBus("test-dedup", "test-secret", "", testTr, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runBus(t, b, ctx)
	waitForBusReady(t, testTr, addr)

	conn, err := testTr.Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	hello := &protocol.Hello{
		Type:       protocol.MsgTypeHello,
		PID:        os.Getpid(),
		EventKey:   "dedup.event.v1",
		EventTypes: []string{"dedup.event.v1"},
		Version:    "v1",
	}
	protocol.Encode(conn, hello)
	sc := bufio.NewScanner(conn)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if !sc.Scan() {
		t.Fatal("no hello_ack")
	}

	for i := 0; i < 2; i++ {
		mockSrc.emit(&event.RawEvent{
			EventID:   "evt-dedup-same",
			EventType: "dedup.event.v1",
			Payload:   json.RawMessage(`{"dup": true}`),
			Timestamp: time.Now(),
		})
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if !sc.Scan() {
		t.Fatal("expected at least one event")
	}
	evtMsg, _ := protocol.Decode(sc.Bytes())
	if _, ok := evtMsg.(*protocol.Event); !ok {
		t.Fatalf("expected Event, got %T", evtMsg)
	}

	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if sc.Scan() {
		t.Error("received duplicate event; dedup filter should have blocked it")
	}

	cancel()
}
