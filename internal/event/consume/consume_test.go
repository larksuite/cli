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
	"testing"

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
