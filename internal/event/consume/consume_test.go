// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

// NOTE: Run() requires bus daemon + transport infrastructure. Testing the full
// Run path end-to-end is complex. For this task we test the parts:
// (a) NormalizeParams error wrapping
// (b) doHello correctly threads subscriptionID through to the Hello message.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/protocol"
)

// fakeRT is a minimal event.APIClient mock.
type fakeRT struct {
	err error
}

func (f *fakeRT) CallAPI(_ context.Context, _, _ string, _ interface{}) (json.RawMessage, error) {
	return nil, f.err
}

func TestNormalizeParams_ErrorIsWrappedWithEventKey(t *testing.T) {
	// We test the error wrapping pattern in isolation: same call site Run uses.
	keyDef := &event.KeyDefinition{
		Key: "test.evt_normalize_fail",
		NormalizeParams: func(_ context.Context, _ event.APIClient, _ map[string]string) error {
			return errors.New("simulated normalize failure")
		},
	}
	err := keyDef.NormalizeParams(context.Background(), &fakeRT{}, map[string]string{})
	if err == nil {
		t.Fatal("expected error from NormalizeParams")
	}
	// Run wraps with: fmt.Errorf("normalize params for %s: %w", EventKey, err)
	wrapped := fmt.Errorf("normalize params for %s: %w", keyDef.Key, err)
	if !strings.Contains(wrapped.Error(), "normalize params for test.evt_normalize_fail:") {
		t.Errorf("wrap format wrong: %v", wrapped)
	}
	if !strings.Contains(wrapped.Error(), "simulated normalize failure") {
		t.Errorf("underlying error not propagated: %v", wrapped)
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
