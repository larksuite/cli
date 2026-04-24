// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/larksuite/cli/internal/vfs"
)

// Sink is an output destination for processed events.
type Sink interface {
	Write(data json.RawMessage) error
}

// newSink picks a sink based on opts. DirSink's target directory is
// created once here so per-event writes skip the MkdirAll syscall.
func newSink(opts Options) (Sink, error) {
	if opts.OutputDir != "" {
		if err := vfs.MkdirAll(opts.OutputDir, 0755); err != nil {
			return nil, fmt.Errorf("create output dir: %w", err)
		}
		// Include PID so concurrent processes writing to the same directory
		// can't collide on filename even if their nanosecond clocks and
		// sequence resets happen to align (see DirSink.Write).
		return &DirSink{Dir: opts.OutputDir, pid: os.Getpid()}, nil
	}
	out := opts.Out
	if out == nil {
		// Defensive fallback for library callers that forget to wire
		// Options.Out; cmd/event/consume.go always injects IOStreams.Out.
		out = os.Stdout //nolint:forbidigo // library-caller fallback; cmd path always sets Options.Out
	}
	return &WriterSink{W: out, ErrOut: opts.ErrOut}, nil
}

// WriterSink writes one JSON event per line (or pretty-printed) to W.
// Concurrent writes are serialised so workers don't interleave bytes.
//
// When Pretty is true and a payload is not valid JSON (should be rare —
// the bus only forwards upstream payloads, which are JSON by protocol,
// but --jq output can technically be anything), the sink falls back to
// raw passthrough. If ErrOut is non-nil we emit a one-shot warning on
// the first such fallback so callers know the pretty formatting didn't
// apply and can investigate; subsequent fallbacks are silent to avoid
// log spam on pathological streams.
type WriterSink struct {
	W            io.Writer
	Pretty       bool
	ErrOut       io.Writer
	prettyWarned atomic.Bool
	mu           sync.Mutex
}

func (s *WriterSink) Write(data json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Pretty {
		var v interface{}
		if err := json.Unmarshal(data, &v); err == nil {
			pretty, _ := json.MarshalIndent(v, "", "  ")
			_, err := fmt.Fprintln(s.W, string(pretty))
			return err
		}
		// Non-JSON payload. Fall through to raw passthrough, but log once.
		if s.ErrOut != nil && s.prettyWarned.CompareAndSwap(false, true) {
			fmt.Fprintln(s.ErrOut, "WARN: --pretty: payload is not valid JSON; falling back to raw output (this and future malformed events)")
		}
	}
	_, err := fmt.Fprintln(s.W, string(data))
	return err
}

// DirSink writes each event as its own JSON file under Dir. Filenames
// combine a nanosecond timestamp, the writing process PID, and an atomic
// sequence so concurrent writers — whether goroutines in the same
// process or separate processes sharing a DirSink output — never
// collide on filename.
type DirSink struct {
	Dir string
	pid int
	seq atomic.Int64
}

func (s *DirSink) Write(data json.RawMessage) error {
	name := fmt.Sprintf("%d_%d_%d.json", time.Now().UnixNano(), s.pid, s.seq.Add(1))
	// Mode 0600: event payloads often carry message contents, user IDs,
	// and other PII — world-readable on a multi-user host leaks data.
	return vfs.WriteFile(filepath.Join(s.Dir, name), data, 0600)
}
