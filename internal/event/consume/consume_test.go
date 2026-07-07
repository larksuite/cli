// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/protocol"
	"github.com/larksuite/cli/internal/event/transport"
)

// fakeRT is a minimal event.APIClient mock.
type fakeRT struct {
	err error
}

func (f *fakeRT) CallAPI(_ context.Context, _, _ string, _ interface{}) (json.RawMessage, error) {
	return nil, f.err
}

type scriptedTransport struct {
	dials          atomic.Int32
	consumeDone    chan error
	sawPreShutdown atomic.Bool
}

func (s *scriptedTransport) Listen(string) (net.Listener, error) {
	return nil, errors.New("not implemented")
}
func (s *scriptedTransport) Address(string) string { return "scripted" }
func (s *scriptedTransport) Cleanup(string)        {}

func (s *scriptedTransport) Dial(string) (net.Conn, error) {
	client, server := net.Pipe()
	switch s.dials.Add(1) {
	case 1:
		go func() {
			defer server.Close()
			br := bufio.NewReader(server)
			line, err := protocol.ReadFrame(br)
			if err != nil {
				return
			}
			msg, err := protocol.Decode(bytes.TrimRight(line, "\n"))
			if err != nil {
				return
			}
			if _, ok := msg.(*protocol.StatusQuery); ok {
				_ = protocol.Encode(server, protocol.NewStatusResponse(123, 1, 0, nil))
			}
		}()
	case 2:
		go s.serveConsume(server)
	default:
		server.Close()
		client.Close()
		return nil, errors.New("unexpected extra dial")
	}
	return client, nil
}

func (s *scriptedTransport) serveConsume(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)

	line, err := protocol.ReadFrame(br)
	if err != nil {
		s.consumeDone <- err
		return
	}
	msg, err := protocol.Decode(bytes.TrimRight(line, "\n"))
	if err != nil {
		s.consumeDone <- err
		return
	}
	if _, ok := msg.(*protocol.Hello); !ok {
		s.consumeDone <- errors.New("expected hello")
		return
	}
	if err := protocol.Encode(conn, protocol.NewHelloAck("v1", true)); err != nil {
		s.consumeDone <- err
		return
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		line, err := protocol.ReadFrame(br)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			s.consumeDone <- err
			return
		}
		msg, err := protocol.Decode(bytes.TrimRight(line, "\n"))
		if err != nil {
			continue
		}
		if _, ok := msg.(*protocol.PreShutdownCheck); ok {
			s.sawPreShutdown.Store(true)
			_ = protocol.Encode(conn, protocol.NewPreShutdownAck(true))
			s.consumeDone <- nil
			return
		}
	}
	s.consumeDone <- errors.New("timed out waiting for pre-shutdown check")
}

func TestNormalizeParams_ErrorIsWrappedWithEventKey(t *testing.T) {
	// Drives the real Run() path: NormalizeParams fails before EnsureBus, so no
	// bus is contacted, yet the production error-wrapping is exercised — if Run()
	// ever stops wrapping, this test fails.
	const key = "test.evt_normalize_fail"
	event.RegisterKey(event.KeyDefinition{
		Key:       key,
		EventType: key,
		Schema:    event.SchemaDef{Custom: &event.SchemaSpec{Raw: json.RawMessage(`{"type":"object"}`)}},
		NormalizeParams: func(_ context.Context, _ event.APIClient, _ map[string]string) error {
			return errors.New("simulated normalize failure")
		},
	})
	defer event.UnregisterKeyForTest(key)

	err := Run(context.Background(), transport.New(), "app", "", "", Options{
		EventKey: key,
		Runtime:  &fakeRT{},
		Quiet:    true,
	})
	if err == nil {
		t.Fatal("expected Run to fail when NormalizeParams errors")
	}
	if !strings.Contains(err.Error(), "normalize params for "+key+":") {
		t.Errorf("error not wrapped with EventKey prefix: %v", err)
	}
	if !strings.Contains(err.Error(), "simulated normalize failure") {
		t.Errorf("underlying error not propagated: %v", err)
	}
}

func TestRun_StdinClosedWaitsUntilAfterPreConsumeThenCleansUp(t *testing.T) {
	const key = "test.evt_stdin_closed_after_preconsume"
	var preConsumeCalled atomic.Bool
	var cleanupCalled atomic.Bool
	event.RegisterKey(event.KeyDefinition{
		Key:       key,
		EventType: key,
		Schema:    event.SchemaDef{Custom: &event.SchemaSpec{Raw: json.RawMessage(`{"type":"object"}`)}},
		PreConsume: func(ctx context.Context, _ event.APIClient, _ map[string]string) (func() error, error) {
			if err := ctx.Err(); err != nil {
				t.Fatalf("PreConsume ctx already canceled: %v", err)
			}
			preConsumeCalled.Store(true)
			return func() error {
				cleanupCalled.Store(true)
				return nil
			}, nil
		},
	})
	defer event.UnregisterKeyForTest(key)

	stdinClosed := make(chan struct{})
	close(stdinClosed)
	tr := &scriptedTransport{consumeDone: make(chan error, 1)}

	err := Run(context.Background(), tr, "app", "", "", Options{
		EventKey:    key,
		Runtime:     &fakeRT{},
		Quiet:       true,
		StdinClosed: stdinClosed,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !preConsumeCalled.Load() {
		t.Fatal("PreConsume was not called")
	}
	if !tr.sawPreShutdown.Load() {
		t.Fatal("PreShutdownCheck was not sent")
	}
	if !cleanupCalled.Load() {
		t.Fatal("cleanup was not called")
	}
	select {
	case err := <-tr.consumeDone:
		if err != nil {
			t.Fatalf("scripted server error: %v", err)
		}
	default:
	}
}

func TestDoHello_PassesSubscriptionIDToWire(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	// Server-side: read Hello, decode, assert SubscriptionID, send ack
	done := make(chan string, 1)
	go func() {
		br := bufio.NewReader(b)
		line, err := protocol.ReadFrame(br)
		if err != nil {
			done <- "READ_ERR:" + err.Error()
			return
		}
		msg, err := protocol.Decode(bytes.TrimRight(line, "\n"))
		if err != nil {
			done <- "DECODE_ERR:" + err.Error()
			return
		}
		if hello, ok := msg.(*protocol.Hello); ok {
			done <- hello.SubscriptionID
			// send ack so client can return
			ack := protocol.NewHelloAck("v1", true)
			_ = protocol.EncodeWithDeadline(b, ack, protocol.WriteTimeout)
		} else {
			done <- "WRONG_TYPE"
		}
	}()

	ack, _, err := doHello(a, "mail.x", []string{"mail.x"}, "mail.x:alice")
	if err != nil {
		t.Fatalf("doHello error: %v", err)
	}
	if ack == nil {
		t.Fatal("got nil ack")
	}
	got := <-done
	if got != "mail.x:alice" {
		t.Errorf("Hello.SubscriptionID on wire = %q, want %q", got, "mail.x:alice")
	}
}
