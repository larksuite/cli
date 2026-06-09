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

func TestWatchStdinEOF_CancelsOnEOF(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watchStdinEOF(strings.NewReader(""), cancel, io.Discard)

	select {
	case <-ctx.Done():
	case <-time.After(1 * time.Second):
		t.Fatal("watchStdinEOF did not cancel within 1s of EOF")
	}
}

func TestWatchStdinEOF_StaysAliveWhileReaderBlocks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pr, _ := io.Pipe()
	defer pr.Close()

	watchStdinEOF(pr, cancel, io.Discard)

	select {
	case <-ctx.Done():
		t.Fatal("watchStdinEOF cancelled without EOF")
	case <-time.After(200 * time.Millisecond):
	}
}

// On EOF the watcher must emit a diagnostic naming stdin close + workarounds (daemon-style callers depend on it).
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

func TestShouldWatchStdinEOF(t *testing.T) {
	tests := []struct {
		name       string
		isTerminal bool
		maxEvents  int
		timeout    time.Duration
		want       bool
	}{
		{
			name:       "terminal",
			isTerminal: true,
			want:       false,
		},
		{
			name: "non terminal unbounded",
			want: true,
		},
		{
			name:      "non terminal negative max events is unbounded",
			maxEvents: -1,
			want:      true,
		},
		{
			name:    "non terminal negative timeout is unbounded",
			timeout: -1 * time.Second,
			want:    true,
		},
		{
			name:      "non terminal max events bounded",
			maxEvents: 1,
			want:      false,
		},
		{
			name:    "non terminal timeout bounded",
			timeout: 10 * time.Minute,
			want:    false,
		},
		{
			name:      "non terminal both bounds positive",
			maxEvents: 1,
			timeout:   10 * time.Minute,
			want:      false,
		},
		{
			name:      "non terminal bounded max events with negative timeout",
			maxEvents: 1,
			timeout:   -1 * time.Second,
			want:      false,
		},
		{
			name:      "non terminal bounded timeout with negative max events",
			maxEvents: -1,
			timeout:   10 * time.Minute,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldWatchStdinEOF(tt.isTerminal, tt.maxEvents, tt.timeout)
			if got != tt.want {
				t.Fatalf("shouldWatchStdinEOF() = %v, want %v", got, tt.want)
			}
		})
	}
}
