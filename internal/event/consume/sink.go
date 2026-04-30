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

type Sink interface {
	Write(data json.RawMessage) error
}

func newSink(opts Options) (Sink, error) {
	if opts.OutputDir != "" {
		if err := vfs.MkdirAll(opts.OutputDir, 0755); err != nil {
			return nil, fmt.Errorf("create output dir: %w", err)
		}
		// PID disambiguates filenames across processes sharing a Dir.
		return &DirSink{Dir: opts.OutputDir, pid: os.Getpid()}, nil
	}
	out := opts.Out
	if out == nil {
		out = os.Stdout //nolint:forbidigo // library-caller fallback; cmd path always sets Options.Out
	}
	return &WriterSink{W: out, ErrOut: opts.ErrOut}, nil
}

// WriterSink writes one JSON event per line; mu serialises concurrent worker writes.
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
		// non-JSON payload (e.g. --jq output): fall through to raw, log once
		if s.ErrOut != nil && s.prettyWarned.CompareAndSwap(false, true) {
			fmt.Fprintln(s.ErrOut, "WARN: --pretty: payload is not valid JSON; falling back to raw output (this and future malformed events)")
		}
	}
	_, err := fmt.Fprintln(s.W, string(data))
	return err
}

// DirSink writes one JSON file per event; nanos+pid+seq filename avoids cross-process collisions.
type DirSink struct {
	Dir string
	pid int
	seq atomic.Int64
}

func (s *DirSink) Write(data json.RawMessage) error {
	name := fmt.Sprintf("%d_%d_%d.json", time.Now().UnixNano(), s.pid, s.seq.Add(1))
	return vfs.WriteFile(filepath.Join(s.Dir, name), data, 0600) // 0600: payloads may carry PII
}
