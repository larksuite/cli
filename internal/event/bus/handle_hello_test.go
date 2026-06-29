// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bus

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/protocol"
)

// HelloAck write failure must unregister the conn from hub and bus before returning.
func TestHandleHello_HelloAckWriteFailureUnregisters(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	hub := NewHub()
	b := &Bus{
		hub:        hub,
		logger:     logger,
		conns:      make(map[*Conn]struct{}),
		idleTimer:  time.NewTimer(30 * time.Second),
		shutdownCh: make(chan struct{}, 1),
	}

	server, client := net.Pipe()
	client.Close()
	defer server.Close()

	hello := &protocol.Hello{
		PID:        9999,
		EventKey:   "im.msg",
		EventTypes: []string{"im.message.receive_v1"},
	}

	br := bufio.NewReader(server)

	done := make(chan struct{})
	go func() {
		b.handleHello(server, br, hello)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handleHello did not return within 3s: stuck on write or not handling the error path")
	}

	if got := hub.ConnCount(); got != 0 {
		t.Errorf("hub.ConnCount after failed HelloAck = %d, want 0 (connection must be unregistered)", got)
	}
	if got := hub.EventKeyCount("im.msg"); got != 0 {
		t.Errorf("hub.EventKeyCount(im.msg) after failed HelloAck = %d, want 0", got)
	}
	b.mu.Lock()
	remaining := len(b.conns)
	b.mu.Unlock()
	if remaining != 0 {
		t.Errorf("b.conns after failed HelloAck = %d entries, want 0", remaining)
	}
}

// TestHandleHello_LegacyClient_FallsBackToEventKey: a Hello with empty
// subscription_id registers under EventKey (today's behavior preserved).
func TestHandleHello_LegacyClient_FallsBackToEventKey(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	hub := NewHub()
	b := &Bus{
		hub:        hub,
		logger:     logger,
		conns:      make(map[*Conn]struct{}),
		idleTimer:  time.NewTimer(30 * time.Second),
		shutdownCh: make(chan struct{}, 1),
	}

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Legacy client: no subscription_id field (empty string).
	hello := &protocol.Hello{
		PID:            9999,
		EventKey:       "im.message",
		EventTypes:     []string{"im.message.receive_v1"},
		SubscriptionID: "", // legacy: empty, should fallback to EventKey
	}

	br := bufio.NewReader(server)

	done := make(chan struct{})
	go func() {
		b.handleHello(server, br, hello)
		close(done)
	}()

	// Read the HelloAck from client side to let handleHello complete.
	clientReader := bufio.NewReader(client)
	ackLine, err := clientReader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read HelloAck: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handleHello did not return within 3s")
	}

	// Assertions: registered under EventKey (not a qualified subscription ID).
	if got := hub.ConnCount(); got != 1 {
		t.Errorf("hub.ConnCount = %d, want 1", got)
	}
	if got := hub.EventKeyCount("im.message"); got != 1 {
		t.Errorf("hub.EventKeyCount(im.message) = %d, want 1", got)
	}
	if got := hub.SubCount("im.message"); got != 1 {
		t.Errorf("hub.SubCount(im.message) = %d, want 1 (legacy fallback to EventKey)", got)
	}
	if got := hub.SubCount("im.message:something"); got != 0 {
		t.Errorf("hub.SubCount(im.message:something) = %d, want 0 (should not exist)", got)
	}

	if ackLine == "" {
		t.Fatal("HelloAck was empty")
	}
}

// TestHandleHello_ModernClient_UsesSubscriptionID: a Hello with
// non-empty subscription_id registers under that ID, not EventKey.
func TestHandleHello_ModernClient_UsesSubscriptionID(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	hub := NewHub()
	b := &Bus{
		hub:        hub,
		logger:     logger,
		conns:      make(map[*Conn]struct{}),
		idleTimer:  time.NewTimer(30 * time.Second),
		shutdownCh: make(chan struct{}, 1),
	}

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Modern client: subscription_id explicitly set.
	subscriptionID := "mail.message:alice@example.com"
	hello := &protocol.Hello{
		PID:            8888,
		EventKey:       "mail.message",
		EventTypes:     []string{"mail.message.receive_v1"},
		SubscriptionID: subscriptionID, // modern: per-resource subscription
	}

	br := bufio.NewReader(server)

	done := make(chan struct{})
	go func() {
		b.handleHello(server, br, hello)
		close(done)
	}()

	// Read the HelloAck from client side to let handleHello complete.
	clientReader := bufio.NewReader(client)
	ackLine, err := clientReader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read HelloAck: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handleHello did not return within 3s")
	}

	// Assertions: registered under the subscription_id, not bare EventKey.
	if got := hub.ConnCount(); got != 1 {
		t.Errorf("hub.ConnCount = %d, want 1", got)
	}
	if got := hub.EventKeyCount("mail.message"); got != 1 {
		t.Errorf("hub.EventKeyCount(mail.message) = %d, want 1", got)
	}
	if got := hub.SubCount(subscriptionID); got != 1 {
		t.Errorf("hub.SubCount(%q) = %d, want 1 (modern: uses SubscriptionID)", subscriptionID, got)
	}
	if got := hub.SubCount("mail.message"); got != 0 {
		t.Errorf("hub.SubCount(mail.message) = %d, want 0 (modern: NOT registered under bare EventKey)", got)
	}

	if ackLine == "" {
		t.Fatal("HelloAck was empty")
	}
}

// TestHandleHello_SingleConsumerRejectsSecond: a SingleConsumer EventKey accepts
// the first consumer and rejects the second for the same SubscriptionID.
func TestHandleHello_SingleConsumerRejectsSecond(t *testing.T) {
	const key = "test.handlehello.exclusive"
	event.RegisterKey(event.KeyDefinition{
		Key:            key,
		EventType:      key,
		SingleConsumer: true,
		Schema:         event.SchemaDef{Native: &event.SchemaSpec{Raw: []byte(`{"type":"object"}`)}},
	})
	defer event.UnregisterKeyForTest(key)

	logger := log.New(io.Discard, "", 0)
	hub := NewHub()
	b := &Bus{
		hub:        hub,
		logger:     logger,
		conns:      make(map[*Conn]struct{}),
		idleTimer:  time.NewTimer(30 * time.Second),
		shutdownCh: make(chan struct{}, 1),
	}

	readAck := func(t *testing.T, pid int) *protocol.HelloAck {
		t.Helper()
		server, client := net.Pipe()
		t.Cleanup(func() { server.Close(); client.Close() })
		hello := &protocol.Hello{PID: pid, EventKey: key, EventTypes: []string{key}}
		go b.handleHello(server, bufio.NewReader(server), hello)
		line, err := protocol.ReadFrame(bufio.NewReader(client))
		if err != nil {
			t.Fatalf("read ack (pid %d): %v", pid, err)
		}
		msg, err := protocol.Decode(bytes.TrimRight(line, "\n"))
		if err != nil {
			t.Fatalf("decode ack (pid %d): %v", pid, err)
		}
		ack, ok := msg.(*protocol.HelloAck)
		if !ok {
			t.Fatalf("got %T, want *HelloAck", msg)
		}
		return ack
	}

	ack1 := readAck(t, 100)
	if ack1.Rejected {
		t.Fatalf("first consumer should be accepted, got rejected: %q", ack1.RejectReason)
	}

	ack2 := readAck(t, 200)
	if !ack2.Rejected {
		t.Fatal("second consumer should be rejected")
	}
	if !strings.Contains(ack2.RejectReason, "already running") {
		t.Errorf("reject reason = %q, want mention of 'already running'", ack2.RejectReason)
	}
}
