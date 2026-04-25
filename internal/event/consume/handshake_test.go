// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"net"
	"testing"
	"time"
)

// doHello must apply a read deadline on HelloAck so a wedged bus doesn't hang the consumer.
func TestDoHello_ReadDeadline(t *testing.T) {
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
	done := make(chan error, 1)
	go func() {
		_, _, err := doHello(client, "im.msg", []string{"im.msg"})
		done <- err
	}()

	select {
	case err := <-done:
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("doHello returned nil error when server never replied; must fail with deadline-driven error")
		}
		if elapsed > helloAckTimeout+2*time.Second {
			t.Errorf("doHello returned %v after %v; deadline should fire within ~%v", err, elapsed, helloAckTimeout)
		}
	case <-time.After(helloAckTimeout + 3*time.Second):
		t.Fatal("doHello hung past deadline + 3s slack: read deadline is missing or not being honoured")
	}
}
