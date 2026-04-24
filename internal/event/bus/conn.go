// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bus

import (
	"bufio"
	"bytes"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/larksuite/cli/internal/event/protocol"
)

const (
	sendChCap    = 100
	writeTimeout = 5 * time.Second
)

// Conn represents a single consume client connection in the Bus.
type Conn struct {
	conn    net.Conn
	reader  *bufio.Reader
	sendCh  chan interface{}
	sendMu sync.Mutex // serialises PushDropOldest to keep drop+push atomic
	// writeMu serialises all net.Conn writes. protocol.Encode plus
	// SetWriteDeadline is a 2-call sequence shared between SenderLoop
	// (event frames), handleControlMessage (PreShutdownAck), and the
	// HelloAck write in bus.handleHello — all of which can race if the
	// mutex is bypassed. Without it the shared write deadline corrupts
	// and large frames interleave bytes on the wire.
	writeMu         sync.Mutex
	eventKey        string
	eventTypes      []string
	pid             int
	onClose         func(*Conn)
	checkLastForKey func(eventKey string) bool
	logger          *log.Logger
	closed          chan struct{}
	closeOnce       sync.Once
	received        atomic.Int64  // events Hub has fanned out to us (post-filter)
	seqCounter      atomic.Uint64 // per-conn monotonic seq assigned by Hub.Publish
	dropped         atomic.Int64  // events evicted via drop-oldest backpressure
}

// NewConn creates a Conn. reader may be a *bufio.Reader already
// attached to conn (handed over from Bus.handleConn so any buffered
// bytes aren't lost during handoff). Passing nil is acceptable — the
// conn-bound test helpers do this — and we construct a fresh Reader.
func NewConn(conn net.Conn, reader *bufio.Reader, eventKey string, eventTypes []string, pid int) *Conn {
	if reader == nil {
		reader = bufio.NewReader(conn)
	}
	return &Conn{
		conn:       conn,
		reader:     reader,
		sendCh:     make(chan interface{}, sendChCap),
		eventKey:   eventKey,
		eventTypes: eventTypes,
		pid:        pid,
		closed:     make(chan struct{}),
	}
}

// SetOnClose installs a callback invoked once when the connection
// shuts down (socket EOF, peer Bye, or explicit Close).
func (c *Conn) SetOnClose(fn func(*Conn)) { c.onClose = fn }

// SetCheckLastForKey installs the callback used to answer a
// PreShutdownCheck from the consumer. Returning true means "you are the
// last subscriber for this EventKey, run cleanup."
func (c *Conn) SetCheckLastForKey(fn func(string) bool) { c.checkLastForKey = fn }

// SetLogger attaches a logger for write-failure / control-message
// diagnostics. nil is tolerated (logging becomes a no-op).
func (c *Conn) SetLogger(l *log.Logger) { c.logger = l }

// EventKey returns the EventKey this connection subscribed to during Hello.
func (c *Conn) EventKey() string { return c.eventKey }

// EventTypes returns the upstream event types this connection wants to receive.
func (c *Conn) EventTypes() []string { return c.eventTypes }

// SendCh returns the buffered outbound channel the Hub pushes events into.
func (c *Conn) SendCh() chan interface{} { return c.sendCh }

// PID returns the consumer-reported PID (for observability only; not trusted).
func (c *Conn) PID() int { return c.pid }

// IncrementReceived bumps the per-conn events-fanned-out counter.
func (c *Conn) IncrementReceived() { c.received.Add(1) }

// Received returns the current events-fanned-out counter value.
func (c *Conn) Received() int64 { return c.received.Load() }

// NextSeq returns a monotonically increasing seq number for this connection.
// Used by Hub.Publish to assign sequence numbers to events destined for this
// consumer, so the consumer can detect gaps caused by backpressure drops.
// First call returns 1 (atomic.Uint64 starts at 0, Add returns the new value).
func (c *Conn) NextSeq() uint64 { return c.seqCounter.Add(1) }

// DroppedCount returns the number of events dropped due to backpressure on
// this connection.
func (c *Conn) DroppedCount() int64 { return c.dropped.Load() }

// IncrementDropped records that one event was dropped due to backpressure.
func (c *Conn) IncrementDropped() { c.dropped.Add(1) }

// Start launches the sender and reader goroutines. Must be called exactly once.
func (c *Conn) Start() {
	go c.SenderLoop()
	go c.ReaderLoop()
}

// writeFrame is the single write path to c.conn. Serialised via writeMu
// so SenderLoop (event frames), handleControlMessage (PreShutdownAck),
// and bus.handleHello (HelloAck) never interleave bytes or race the
// shared SetWriteDeadline call.
func (c *Conn) writeFrame(msg interface{}) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	return protocol.Encode(c.conn, msg)
}

// SenderLoop writes messages from sendCh to the connection.
// It exits when the closed channel is signaled, not when sendCh is closed,
// so that Hub.Publish can safely send to sendCh without risk of panic.
func (c *Conn) SenderLoop() {
	for {
		select {
		case <-c.closed:
			return
		case msg := <-c.sendCh:
			if err := c.writeFrame(msg); err != nil {
				if c.logger != nil {
					c.logger.Printf("WARN: write to pid=%d failed: %v", c.pid, err)
				}
				c.shutdown()
				return
			}
		}
	}
}

// ReaderLoop monitors the connection for EOF/close and processes any
// control messages (Bye, PreShutdownCheck) the consume client sends.
// Uses the bufio.Reader handed in from handleConn so buffered bytes
// carried over from the Hello read aren't dropped.
func (c *Conn) ReaderLoop() {
	for {
		line, err := protocol.ReadFrame(c.reader)
		if err != nil {
			break
		}
		line = bytes.TrimRight(line, "\n")
		if len(line) == 0 {
			continue
		}
		msg, err := protocol.Decode(line)
		if err != nil {
			continue
		}
		c.handleControlMessage(msg)
	}
	c.shutdown()
}

func (c *Conn) handleControlMessage(msg interface{}) {
	switch m := msg.(type) {
	case *protocol.Bye:
		c.shutdown()
	case *protocol.PreShutdownCheck:
		// Query the Hub (via callback) and reply with whether this is the
		// last consumer for the given EventKey. The consume client uses this
		// to decide whether to run cleanup (e.g. unsubscribe mailbox).
		lastForKey := true
		if c.checkLastForKey != nil {
			lastForKey = c.checkLastForKey(m.EventKey)
		}
		ack := protocol.NewPreShutdownAck(lastForKey)
		if err := c.writeFrame(ack); err != nil && c.logger != nil {
			c.logger.Printf("WARN: pre_shutdown_ack to pid=%d failed: %v", c.pid, err)
		}
	}
}

func (c *Conn) shutdown() {
	c.closeOnce.Do(func() {
		close(c.closed)
		c.conn.Close()
		// NOTE: sendCh is intentionally NOT closed here. SenderLoop exits via
		// the closed channel. Closing sendCh would race with Hub.Publish which
		// may still hold a reference to this conn's SendCh() after RUnlock.
		if c.onClose != nil {
			c.onClose(c)
		}
	})
}

// TrySend enqueues msg onto sendCh without evicting, under sendMu so it
// respects PushDropOldest's atomicity contract. Returns true iff the
// channel had room. Used by Hub.BroadcastSourceStatus for best-effort
// fan-out of source-level status messages — a source-status that can't
// fit the queue is dropped silently (the event itself isn't event data
// worth applying back-pressure for) but the send still synchronises
// with concurrent PushDropOldest so a broadcaster cannot steal a slot
// in the window between another goroutine's drop and retry push.
func (c *Conn) TrySend(msg interface{}) bool {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	select {
	case c.sendCh <- msg:
		return true
	default:
		return false
	}
}

// PushDropOldest enqueues msg onto sendCh. If sendCh is full, it drops
// exactly one oldest event and retries the push atomically under sendMu —
// preventing concurrent Publish callers from racing each other into a state
// where the channel refills between drop and push.
//
// Returns (enqueued, dropped) where enqueued=true means msg is now in the
// channel, and dropped=true means an older event was evicted to make room.
//
// If the push would require evicting but the channel drained on its own
// between operations (SenderLoop raced us), the eviction is skipped and the
// push still succeeds — in that case returns (true, false).
func (c *Conn) PushDropOldest(msg interface{}) (enqueued, dropped bool) {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	// Fast path: channel has room.
	select {
	case c.sendCh <- msg:
		return true, false
	default:
	}
	// Slow path: drop one oldest if we can, then push.
	select {
	case <-c.sendCh:
		dropped = true
	default:
		// SenderLoop drained it between our check and here — harmless.
	}
	select {
	case c.sendCh <- msg:
		return true, dropped
	default:
		// Should essentially never happen under sendMu: we either dropped
		// one (making room) or the channel was empty. But if something
		// external drained and refilled at exactly the wrong instant,
		// fail gracefully rather than block.
		return false, dropped
	}
}

// Close shuts the connection down idempotently (safe to call multiple times).
func (c *Conn) Close() {
	c.shutdown()
}
