// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// TestWatchStdinEOF_CancelsOnEOF — feeding an already-closed reader
// causes the watcher to cancel the context quickly (< 1s).
func TestWatchStdinEOF_CancelsOnEOF(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watchStdinEOF(strings.NewReader(""), cancel, io.Discard)

	select {
	case <-ctx.Done():
		// Expected — cancel fired on EOF.
	case <-time.After(1 * time.Second):
		t.Fatal("watchStdinEOF did not cancel within 1s of EOF")
	}
}

// TestWatchStdinEOF_StaysAliveWhileReaderBlocks — if the reader never
// delivers EOF, the watcher must not fire the cancel.
func TestWatchStdinEOF_StaysAliveWhileReaderBlocks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// io.Pipe gives us a reader that will block forever (no writer).
	pr, _ := io.Pipe()
	defer pr.Close()

	watchStdinEOF(pr, cancel, io.Discard)

	select {
	case <-ctx.Done():
		t.Fatal("watchStdinEOF cancelled without EOF")
	case <-time.After(200 * time.Millisecond):
		// Expected — still alive.
	}
}

// TestWatchStdinEOF_DiagnosticMessage — on EOF the watcher writes a
// self-explanatory diagnostic to errOut that names the cause and points
// at the workarounds. This is the single biggest footgun for daemon-
// style callers (`< /dev/null`, `nohup`, systemd), so the message must
// stay descriptive enough to unstick a new user in seconds.
func TestWatchStdinEOF_DiagnosticMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var buf bytes.Buffer
	watchStdinEOF(strings.NewReader(""), cancel, &buf)

	select {
	case <-ctx.Done():
		got := buf.String()
		for _, want := range []string{"stdin closed", "--max-events", "--timeout", "SIGTERM"} {
			if !strings.Contains(got, want) {
				t.Errorf("diagnostic missing %q; got:\n%s", want, got)
			}
		}
	case <-time.After(1 * time.Second):
		t.Fatal("watchStdinEOF did not cancel within 1s of EOF")
	}
}
