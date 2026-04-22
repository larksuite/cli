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

// consumeLoop reads events from the bus socket and dispatches to workers.
// *lastForKey is set before conn closes so Run knows whether to run cleanup.
//
// An inner cancel lets a terminal sink error (stdout EPIPE) trigger the
// same shutdown path as Ctrl+C. Non-terminal sink errors (disk full on
// --output-dir) are logged and the loop keeps running — one bad event
// shouldn't tear down a long-running pipeline.
func consumeLoop(ctx context.Context, conn net.Conn, br *bufio.Reader, keyDef *event.KeyDefinition, opts Options, lastForKey *bool, emitted *atomic.Int64) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sink, err := newSink(opts)
	if err != nil {
		return err
	}

	// Compile jq once at startup; applyJQ reuses the *gojq.Code per event
	// so a bad expression fails immediately instead of burning on every
	// event. Must happen before any worker goroutine starts to avoid a
	// data race on jqCode. Run() also pre-flights the expression before
	// EnsureBus/PreConsume, but we re-compile here so callers that invoke
	// consumeLoop directly (tests, library use) don't rely on Run's guard.
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

	// stopReader lets shutdown preempt the reader goroutine so we can
	// reuse conn for the PreShutdownCheck round-trip — otherwise a second
	// scanner would race for the ack bytes.
	stopReader := make(chan struct{})
	readerDone := make(chan struct{})

	// Socket reader goroutine.
	//
	// bufio.Reader.ReadBytes('\n') (not Scanner) so short read deadlines
	// that fire mid-frame don't drop buffered bytes — on timeout we poll
	// stopReader and keep reading into the same buffer. The caller hands
	// in the reader carrying any bytes buffered past hello_ack.
	go func() {
		defer close(readerDone)
		defer close(socketCh)
		var buf []byte
		// lastSeq tracks the last seq we saw from the bus for this
		// connection. The bus assigns monotonically increasing per-conn
		// seqs in Hub.Publish (see bus/hub.go), so any gap here means
		// events were dropped by the bus's drop-oldest backpressure
		// path (sendCh overflow). This is consume-session-local state —
		// not a struct/package field — since each connection starts
		// fresh at seq=1 on the bus side.
		var lastSeq uint64
		for {
			select {
			case <-stopReader:
				return
			default:
			}
			conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			chunk, err := br.ReadBytes('\n')
			if len(chunk) > 0 {
				// Size cap on the partial-frame accumulator. Without this
				// check a bus that dribbles a multi-MB line at 200ms
				// boundaries would grow `buf` unbounded (the 200ms
				// deadline + continue path never breaks out). Reject the
				// frame and reset — protocol.MaxFrameBytes mirrors the
				// server-side cap so legitimate messages always fit.
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
					// Deadline hit mid-frame; buf retains partial bytes.
					// Loop back to poll stopReader and keep reading.
					continue
				}
				// EOF or other terminal error → reader is done.
				return
			}
			// A complete line (ending with '\n') arrived.
			line := buf
			// Trim trailing newline for the decoder.
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
				// Seq gap detection: the bus assigns monotonic per-conn seqs,
				// so lastSeq+1 is the expected next value. A skip means the
				// bus dropped events to cope with channel overflow before
				// they ever reached us — a WARN here is the consumer-visible
				// witness for that silent loss.
				if lastSeq > 0 && m.Seq > 0 && m.Seq > lastSeq+1 {
					gap := m.Seq - lastSeq - 1
					if !opts.Quiet {
						fmt.Fprintf(opts.ErrOut,
							"WARN: event seq gap %d->%d, missed %d events (dropped by bus backpressure)\n",
							lastSeq, m.Seq, gap)
					}
				}
				// Only advance forward. Concurrent Publishers can race on
				// sendMu so a later Seq may arrive before an earlier one;
				// clobbering lastSeq with the out-of-order lower value
				// would synthesise a false "gap" warning on the next event.
				if m.Seq > lastSeq {
					lastSeq = m.Seq
				}
				select {
				case socketCh <- m:
				default:
					// Back-pressure: drop the oldest event to make room (non-blocking).
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
				// Unknown message types are ignored to stay forward-compatible.
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
					// MaxEvents bound: cancel inner ctx so the select in the outer
					// switch observes ctx.Done() and goes through the normal shutdown
					// path (cleanup check, wg.Wait) rather than ripping connections.
					if checkMaxEvents(opts, emitted) {
						cancel()
						return
					}
				}
				if err != nil {
					if isTerminalSinkError(err) {
						// Downstream pipe closed (e.g. `| head -n 1` exited).
						// Trigger graceful shutdown so cleanup still runs.
						if !opts.Quiet {
							fmt.Fprintf(opts.ErrOut, "consume: output pipe closed (%v), shutting down\n", err)
						}
						cancel()
						return
					}
					// Non-terminal (e.g. disk full on --output-dir): log and continue.
					if !opts.Quiet {
						fmt.Fprintf(opts.ErrOut, "WARN: sink write failed, skipping event: %v\n", err)
					}
				}
			}
		}()
	}

	// Signal when ALL workers have drained.
	allDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(allDone)
	}()

	select {
	case <-ctx.Done():
		// Drain the reader and clear its deadline so the PreShutdownCheck
		// round-trip has exclusive use of conn.
		close(stopReader)
		<-readerDone
		conn.SetReadDeadline(time.Time{})
		*lastForKey = checkLastForKey(conn, opts.EventKey)
		conn.Close()
	case <-allDone:
		// Socket closed by bus side — we can't query, assume last.
		*lastForKey = true
	}

	// Wait for all workers to finish.
	// conn.Close() causes scanner EOF → close(socketCh) → workers exit range loop.
	wg.Wait()

	return nil
}

// processAndOutput runs Process, applies JQ, and writes the result.
// Returns (wrote, err): wrote=true only when sink.Write was called and
// succeeded; err is non-nil only for sink.Write failures. Process/JQ
// errors are logged and the event is dropped (wrote=false, err=nil).
// The caller decides whether a sink error is terminal (EPIPE) via
// isTerminalSinkError.
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

// isTerminalSinkError returns true when a sink.Write failure means the
// output channel is permanently broken and the consume should stop.
// EPIPE (broken pipe) and ErrClosed cover the "downstream consumer of
// stdout exited" case. Transient filesystem errors return false so the
// loop keeps running.
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
