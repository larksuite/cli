// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event/protocol"
)

// checkLastForKey must skip non-ack frames buffered before PreShutdownAck.
func TestCheckLastForKey_IgnoresNonAckFrames(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	errs := make(chan error, 2)
	go func() {
		buf := make([]byte, 4096)
		if _, err := server.Read(buf); err != nil && err != io.EOF {
			errs <- err
			return
		}
		evt := protocol.NewEvent("im.msg", "evt_1", "", 1, json.RawMessage(`{}`))
		if err := protocol.Encode(server, evt); err != nil {
			errs <- err
			return
		}
		ack := protocol.NewPreShutdownAck(false)
		if err := protocol.Encode(server, ack); err != nil {
			errs <- err
			return
		}
	}()

	got := checkLastForKey(client, "im.msg", "")
	if got != false {
		t.Errorf("checkLastForKey = %v, want false", got)
	}

	select {
	case err := <-errs:
		t.Fatalf("server goroutine error: %v", err)
	default:
	}
}

func TestCheckLastForKey_ReturnsAckValue(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 4096)
		_, _ = server.Read(buf)
		ack := protocol.NewPreShutdownAck(true)
		_ = protocol.Encode(server, ack)
	}()

	got := checkLastForKey(client, "im.msg", "")
	if got != true {
		t.Errorf("checkLastForKey = %v, want true", got)
	}
}

func TestCheckLastForKey_DefaultsToTrueOnTimeout(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()

	start := time.Now()
	got := checkLastForKey(client, "im.msg", "")
	elapsed := time.Since(start)

	if got != true {
		t.Errorf("checkLastForKey = %v, want true (default on timeout)", got)
	}
	if elapsed > preShutdownAckTimeout+2*time.Second {
		t.Errorf("elapsed = %v, expected ~%v (timeout-bounded)", elapsed, preShutdownAckTimeout)
	}
}

func TestCheckLastForKey_SendsSubscriptionID(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	done := make(chan string, 1)
	go func() {
		br := bufio.NewReader(b)
		line, err := protocol.ReadFrame(br)
		if err != nil {
			done <- "READ_ERR"
			return
		}
		msg, err := protocol.Decode(bytes.TrimRight(line, "\n"))
		if err != nil {
			done <- "DECODE_ERR"
			return
		}
		check, ok := msg.(*protocol.PreShutdownCheck)
		if !ok {
			done <- "WRONG_TYPE"
			return
		}
		done <- check.SubscriptionID
		// Reply with ack so client returns
		ack := protocol.NewPreShutdownAck(true)
		_ = protocol.EncodeWithDeadline(b, ack, protocol.WriteTimeout)
	}()

	_ = checkLastForKey(a, "mail.x", "mail.x:alice")
	got := <-done
	if got != "mail.x:alice" {
		t.Errorf("PreShutdownCheck.SubscriptionID on wire = %q, want %q", got, "mail.x:alice")
	}
}
