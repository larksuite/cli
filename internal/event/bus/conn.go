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
	conn            net.Conn
	reader          *bufio.Reader
	sendCh          chan interface{}
	sendMu          sync.Mutex // serialises drop+push atomically
	writeMu         sync.Mutex // serialises all net.Conn writes (Encode+SetWriteDeadline is a 2-call sequence)
	eventKey        string
	eventTypes      []string
	pid             int
	onClose         func(*Conn)
	checkLastForKey func(eventKey string) bool
	logger          *log.Logger
	closed          chan struct{}
	closeOnce       sync.Once
	received        atomic.Int64  // events fanned out to us (post-filter)
	seqCounter      atomic.Uint64 // per-conn monotonic seq assigned by Hub.Publish
	dropped         atomic.Int64  // events evicted via drop-oldest backpressure
}

// NewConn creates a Conn; pass a reader with pre-buffered bytes (handoff from Bus.handleConn) or nil for a fresh one.
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

func (c *Conn) SetOnClose(fn func(*Conn)) { c.onClose = fn }

// SetCheckLastForKey: returning true means "you are the last subscriber, run cleanup".
func (c *Conn) SetCheckLastForKey(fn func(string) bool) { c.checkLastForKey = fn }

// SetLogger attaches a logger (nil tolerated).
func (c *Conn) SetLogger(l *log.Logger) { c.logger = l }

func (c *Conn) EventKey() string         { return c.eventKey }
func (c *Conn) EventTypes() []string     { return c.eventTypes }
func (c *Conn) SendCh() chan interface{} { return c.sendCh }
func (c *Conn) PID() int                 { return c.pid }
func (c *Conn) IncrementReceived()       { c.received.Add(1) }
func (c *Conn) Received() int64          { return c.received.Load() }

// NextSeq returns the next monotonic seq for this conn (first call returns 1).
func (c *Conn) NextSeq() uint64 { return c.seqCounter.Add(1) }

func (c *Conn) DroppedCount() int64 { return c.dropped.Load() }
func (c *Conn) IncrementDropped()   { c.dropped.Add(1) }

// Start launches the sender and reader goroutines; call exactly once.
func (c *Conn) Start() {
	go c.SenderLoop()
	go c.ReaderLoop()
}

// writeFrame is the sole write path, serialised via writeMu.
func (c *Conn) writeFrame(msg interface{}) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	return protocol.Encode(c.conn, msg)
}

// SenderLoop exits on closed (not sendCh close) so Hub.Publish can send without panic risk.
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

// ReaderLoop reads control messages (Bye, PreShutdownCheck) until EOF.
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
		// sendCh is NOT closed: would race with Hub.Publish holding SendCh() after RUnlock.
		if c.onClose != nil {
			c.onClose(c)
		}
	})
}

// TrySend enqueues non-evictively under sendMu so it respects PushDropOldest's atomicity contract.
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

// PushDropOldest enqueues msg; on full channel evicts one oldest and retries, atomically under sendMu.
// Returns (enqueued, dropped). A rare concurrent drain may make drop unnecessary — still succeeds with dropped=false.
func (c *Conn) PushDropOldest(msg interface{}) (enqueued, dropped bool) {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	select {
	case c.sendCh <- msg:
		return true, false
	default:
	}
	select {
	case <-c.sendCh:
		dropped = true
	default:
	}
	select {
	case c.sendCh <- msg:
		return true, dropped
	default:
		return false, dropped
	}
}

// Close is idempotent.
func (c *Conn) Close() {
	c.shutdown()
}
