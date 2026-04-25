// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/itchyny/gojq"
	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/protocol"
)

// consumeLoop reads events and dispatches to workers; cancels on terminal sink errors.
func consumeLoop(ctx context.Context, conn net.Conn, br *bufio.Reader, keyDef *event.KeyDefinition, opts Options, lastForKey *bool, emitted *atomic.Int64) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sink, err := newSink(opts)
	if err != nil {
		return err
	}

	// Compile before worker goroutines start to avoid a data race on jqCode.
	var jqCode *gojq.Code
	if opts.JQExpr != "" {
		jqCode, err = CompileJQ(opts.JQExpr)
		if err != nil {
			return err
		}
	}

	bufSize := keyDef.BufferSize
	if bufSize <= 0 {
		bufSize = event.DefaultBufferSize
	}
	socketCh := make(chan *protocol.Event, bufSize)

	// stopReader lets shutdown preempt the reader so PreShutdownCheck can reuse conn.
	stopReader := make(chan struct{})
	readerDone := make(chan struct{})

	// ReadBytes (not Scanner) so mid-frame read deadlines don't drop buffered bytes.
	go func() {
		defer close(readerDone)
		defer close(socketCh)
		var buf []byte
		var lastSeq uint64 // per-conn monotonic; gaps = bus drop-oldest backpressure
		for {
			select {
			case <-stopReader:
				return
			default:
			}
			conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			chunk, err := br.ReadBytes('\n')
			if len(chunk) > 0 {
				// Cap accumulator: dribbling multi-MB lines past 200ms deadlines could grow buf unbounded.
				if len(buf)+len(chunk) > protocol.MaxFrameBytes {
					if !opts.Quiet {
						fmt.Fprintf(opts.ErrOut,
							"WARN: dropping oversized frame (>%d bytes) from bus\n", protocol.MaxFrameBytes)
					}
					buf = nil
					continue
				}
				buf = append(buf, chunk...)
			}
			if err != nil {
				var ne net.Error
				if errors.As(err, &ne) && ne.Timeout() {
					continue
				}
				return
			}
			line := buf
			if n := len(line); n > 0 && line[n-1] == '\n' {
				line = line[:n-1]
			}
			buf = nil

			msg, decErr := protocol.Decode(line)
			if decErr != nil {
				continue
			}
			switch m := msg.(type) {
			case *protocol.Event:
				if lastSeq > 0 && m.Seq > 0 && m.Seq > lastSeq+1 {
					gap := m.Seq - lastSeq - 1
					if !opts.Quiet {
						fmt.Fprintf(opts.ErrOut,
							"WARN: event seq gap %d->%d, missed %d events (dropped by bus backpressure)\n",
							lastSeq, m.Seq, gap)
					}
				}
				// Only advance forward — concurrent publishers can deliver out-of-order.
				if m.Seq > lastSeq {
					lastSeq = m.Seq
				}
				select {
				case socketCh <- m:
				default:
					// drop-oldest back-pressure
					select {
					case <-socketCh:
					default:
					}
					select {
					case socketCh <- m:
					default:
					}
					if !opts.Quiet {
						fmt.Fprintf(opts.ErrOut, "WARN: consume backpressure, dropped oldest event\n")
					}
				}
			case *protocol.SourceStatus:
				if !opts.Quiet {
					if m.Detail != "" {
						fmt.Fprintf(opts.ErrOut, "[source] %s: %s (%s)\n", m.Source, m.State, m.Detail)
					} else {
						fmt.Fprintf(opts.ErrOut, "[source] %s: %s\n", m.Source, m.State)
					}
				}
			default:
				// forward-compatible: ignore unknown message types
			}
		}
	}()

	workers := keyDef.Workers
	if workers <= 0 {
		workers = 1
	}

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for evt := range socketCh {
				wrote, err := processAndOutput(ctx, keyDef, evt, opts, sink, jqCode)
				if wrote {
					emitted.Add(1)
					// cancel inner ctx so shutdown goes through normal cleanup, not conn rip.
					if checkMaxEvents(opts, emitted) {
						cancel()
						return
					}
				}
				if err != nil {
					if isTerminalSinkError(err) {
						if !opts.Quiet {
							fmt.Fprintf(opts.ErrOut, "consume: output pipe closed (%v), shutting down\n", err)
						}
						cancel()
						return
					}
					if !opts.Quiet {
						fmt.Fprintf(opts.ErrOut, "WARN: sink write failed, skipping event: %v\n", err)
					}
				}
			}
		}()
	}

	allDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(allDone)
	}()

	select {
	case <-ctx.Done():
		// Drain reader so PreShutdownCheck has exclusive conn.
		close(stopReader)
		<-readerDone
		conn.SetReadDeadline(time.Time{})
		*lastForKey = checkLastForKey(conn, opts.EventKey)
		conn.Close()
	case <-allDone:
		// bus-side close; can't query, assume last
		*lastForKey = true
	}

	wg.Wait()

	return nil
}

// processAndOutput returns (wrote, err); err non-nil only for sink.Write failures.
func processAndOutput(ctx context.Context, keyDef *event.KeyDefinition, evt *protocol.Event, opts Options, sink Sink, jqCode *gojq.Code) (bool, error) {
	var result json.RawMessage

	if keyDef.Process != nil {
		raw := &event.RawEvent{
			EventType: evt.EventType,
			Payload:   evt.Payload,
		}
		var err error
		result, err = keyDef.Process(ctx, opts.Runtime, raw, opts.Params)
		if err != nil {
			if !opts.Quiet {
				fmt.Fprintf(opts.ErrOut, "WARN: Process error: %v\n", err)
			}
			return false, nil
		}
		if result == nil {
			return false, nil
		}
	} else {
		result = evt.Payload
	}

	if jqCode != nil {
		filtered, err := applyJQ(jqCode, result)
		if err != nil {
			if !opts.Quiet {
				fmt.Fprintf(opts.ErrOut, "WARN: JQ error: %v\n", err)
			}
			return false, nil
		}
		if filtered == nil {
			return false, nil
		}
		result = filtered
	}

	if err := sink.Write(result); err != nil {
		return false, err
	}
	return true, nil
}

// isTerminalSinkError reports if the output channel is permanently broken (EPIPE/ErrClosed).
func isTerminalSinkError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EPIPE) {
		return true
	}
	if errors.Is(err, fs.ErrClosed) {
		return true
	}
	return false
}
