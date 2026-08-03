// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/itchyny/gojq"
	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/adapter/localbus/protocol"
	"github.com/larksuite/cli/internal/event/processing"
)

// consumeLoop reads events and dispatches to workers; cancels on terminal sink errors.
func consumeLoop(ctx context.Context, conn net.Conn, br *bufio.Reader, keyDef *event.KeyDefinition, opts Options, subscriptionID string, lastForKey *bool, emitted *atomic.Int64) error {
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
		*lastForKey = checkLastForKey(conn, opts.EventKey, subscriptionID)
		conn.Close()
	case <-allDone:
		// bus-side close; can't query, assume last
		*lastForKey = true
	}

	wg.Wait()

	return nil
}

// diagnosticErrMaxLen caps how much of a Process error text reaches stderr.
// Error strings routinely embed input fragments (a parse error quoting the
// payload, an API response echo), so the diagnostic keeps only a bounded
// prefix of them.
const diagnosticErrMaxLen = 200

// truncateDiagnostic bounds s to diagnosticErrMaxLen bytes, backing off to
// the previous rune boundary so the cut never emits invalid UTF-8, and marks
// the cut explicitly.
func truncateDiagnostic(s string) string {
	if len(s) <= diagnosticErrMaxLen {
		return s
	}
	cut := diagnosticErrMaxLen
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "...(truncated)"
}

// processAndOutput returns (wrote, err); err non-nil only for sink.Write failures.
func processAndOutput(ctx context.Context, keyDef *event.KeyDefinition, evt *protocol.Event, opts Options, sink Sink, jqCode *gojq.Code) (bool, error) {
	raw := restoreCanonicalEvent(evt, opts.ErrOut, opts.Quiet)

	// Validate before any domain work: a payload header that contradicts the
	// canonical metadata means the two sources of truth diverged somewhere on
	// the delivery path — deliver neither. The diagnostic names identity
	// facts only; payload content never reaches stderr.
	if field := checkCanonicalConflict(raw); field != "" {
		if !opts.Quiet {
			fmt.Fprintf(opts.ErrOut, "WARN: event %s (%s) dropped: payload header conflicts with canonical metadata (field=%s)\n",
				raw.EventID, raw.EventType, field)
		}
		return false, nil
	}

	// Synchronous Match filter runs before any work (Process / sink write).
	if keyDef.Match != nil && !keyDef.Match(raw, opts.Params) {
		return false, nil
	}

	var result json.RawMessage

	if keyDef.Process != nil {
		var err error
		result, err = keyDef.Process(ctx, opts.Runtime, raw, opts.Params)
		if err != nil {
			if !opts.Quiet {
				if processing.IsDropMalformed(err) {
					fmt.Fprintf(opts.ErrOut, "WARN: event %s (%s) dropped: malformed payload\n",
						raw.EventID, raw.EventType)
				} else {
					fmt.Fprintf(opts.ErrOut, "WARN: Process error: %s\n", truncateDiagnostic(err.Error()))
				}
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

// restoreCanonicalEvent rebuilds the canonical event from the wire frame in
// full. Every fact the ingress parsed must survive into the domain hooks —
// restoring only a subset is how processors historically ended up re-parsing
// the payload header as a second source of truth.
func restoreCanonicalEvent(evt *protocol.Event, errOut io.Writer, quiet bool) *event.RawEvent {
	var observed time.Time
	if evt.ObservedAt != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, evt.ObservedAt); err == nil {
			observed = parsed
		} else if !quiet {
			// A non-empty observed_at that fails to parse is a delivery
			// defect of the same class as a canonical-metadata conflict:
			// surface it, keep the event (empty means "missing upstream
			// timestamp" and stays silent by design).
			fmt.Fprintf(errOut, "WARN: event %s (%s): malformed observed_at %q ignored: %v\n",
				evt.EventID, evt.EventType, evt.ObservedAt, err)
		}
	}
	return &event.RawEvent{
		EventID:    evt.EventID,
		EventType:  evt.EventType,
		SourceTime: evt.SourceTime,
		AppID:      evt.AppID,
		TenantKey:  evt.TenantKey,
		Payload:    evt.Payload,
		Timestamp:  observed,
	}
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
