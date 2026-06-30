// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
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
		_, _, err := doHello(client, "im.msg", []string{"im.msg"}, "")
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

func TestWrapHandshakeErrorPreservesTypedProblem(t *testing.T) {
	typed := errs.NewInternalError(errs.SubtypeInvalidResponse, "bad hello")

	got := wrapHandshakeError(typed)

	if got != typed {
		t.Fatalf("typed handshake error was rewrapped: %T %v", got, got)
	}
}

func TestWrapHandshakeErrorWrapsUntypedAsInternal(t *testing.T) {
	cause := errors.New("wire closed")

	got := wrapHandshakeError(cause)

	var internalErr *errs.InternalError
	if !errors.As(got, &internalErr) {
		t.Fatalf("error type = %T, want *errs.InternalError", got)
	}
	if internalErr.Subtype != errs.SubtypeSDKError {
		t.Fatalf("subtype = %q, want %q", internalErr.Subtype, errs.SubtypeSDKError)
	}
	if !errors.Is(got, cause) {
		t.Fatalf("cause not preserved: %v", got)
	}
}
