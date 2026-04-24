// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package bus implements the event-bus daemon: it listens on an IPC
// socket, accepts consumer connections, starts event sources, and fans
// out events to matching subscribers via the Hub. The daemon is a
// single-user, per-AppID process; lifecycle is driven by consumer
// presence (idle timeout) and explicit shutdown commands.
package bus

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/protocol"
	"github.com/larksuite/cli/internal/event/source"
	"github.com/larksuite/cli/internal/event/transport"
)

const (
	idleTimeout = 30 * time.Second
)

// Bus is the central event bus daemon. It listens on an IPC socket,
// accepts consume client connections, starts event sources, and fans
// out events to matching subscribers via the Hub.
type Bus struct {
	appID     string
	appSecret string
	domain    string
	transport transport.IPC
	hub       *Hub
	dedup     *event.DedupFilter
	listener  net.Listener
	logger    *log.Logger
	startTime time.Time

	mu         sync.Mutex
	conns      map[*Conn]struct{}
	idleTimer  *time.Timer
	shutdownCh chan struct{}
}

// NewBus creates a new Bus daemon instance.
func NewBus(appID, appSecret, domain string, tr transport.IPC, logger *log.Logger) *Bus {
	return &Bus{
		appID:     appID,
		appSecret: appSecret,
		domain:    domain,
		transport: tr,
		hub:       NewHub(),
		dedup:     event.NewDedupFilter(),
		logger:    logger,
		startTime: time.Now(),
		conns:     make(map[*Conn]struct{}),
		// Buffered so handleShutdown and source-exit paths never drop the signal when
		// Run hasn't entered its select yet, or has already left it for another reason.
		// Run will observe the latched value whenever it next enters select.
		shutdownCh: make(chan struct{}, 1),
	}
}

// Run binds the IPC socket, starts event sources, and enters the accept
// loop. Blocks until ctx is cancelled, the idle timer fires (no active
// connections for idleTimeout), or a shutdown command is received.
func (b *Bus) Run(ctx context.Context) error {
	addr := b.transport.Address(b.appID)

	ln, err := b.transport.Listen(addr)
	if err != nil {
		// Probe: is this a live bus (bow out) or a stale socket (clean up)?
		// Dial uses the transport's baseline timeout (5s via DialTimeout
		// on Unix — see transport_unix.go dialTimeout; 5s via winio on
		// Windows) so a wedged peer can't hang startup here. There is a
		// tiny residual race where another bus cleans up and re-listens
		// between our probe and our Listen retry below — acceptable in
		// practice since listen-contending buses are a rare user error
		// (not an attacker), and both end up owning different socket
		// incarnations at worst (the loser just exits on its own Listen
		// failure on the next attempt).
		if probe, dialErr := b.transport.Dial(addr); dialErr == nil {
			probe.Close()
			b.logger.Printf("Another bus is already running for %s, exiting", b.appID)
			return nil
		}
		b.transport.Cleanup(addr)
		ln, err = b.transport.Listen(addr)
		if err != nil {
			return fmt.Errorf("bus listen: %w", err)
		}
	}
	b.listener = ln
	b.logger.Printf("Bus started for app=%s pid=%d addr=%s", b.appID, os.Getpid(), addr)

	b.idleTimer = time.NewTimer(idleTimeout)

	sourceCtx, sourceCancel := context.WithCancel(ctx)
	defer sourceCancel()
	b.startSources(sourceCtx)

	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		b.acceptLoop(ctx)
	}()

	// A bare `<-b.idleTimer.C` can observe a stale fire latched just before
	// a new Hello registered — Stop() doesn't drain .C, and the tick can
	// linger in the channel across the concurrent Stop+Reset in handleHello
	// / OnClose. Re-check the live conn count under lock before treating
	// the tick as "idle"; if a race happened, re-arm the timer and keep running.
	for {
		select {
		case <-ctx.Done():
			b.logger.Printf("Bus shutting down (context cancelled)")
		case <-b.idleTimer.C:
			b.mu.Lock()
			active := len(b.conns)
			if active > 0 {
				b.idleTimer.Reset(idleTimeout)
				b.mu.Unlock()
				continue
			}
			b.mu.Unlock()
			b.logger.Printf("Bus shutting down (idle %v, no active connections)", idleTimeout)
		case <-b.shutdownCh:
			b.logger.Printf("Bus shutting down (shutdown command received)")
		}
		break
	}

	b.listener.Close()
	// Don't delete the socket: Run() already handles stale sockets (probe
	// → cleanup → re-listen), and unconditional deletion races a new bus
	// that may have already recreated the socket at the same path.
	shutdownConns(b)
	<-acceptDone
	b.logger.Printf("Bus exited cleanly")
	return nil
}

// shutdownConns closes every connection managed by b. It snapshots b.conns
// under lock then releases before calling Close() — without this, c.Close()
// synchronously invokes onClose which re-acquires b.mu, causing a deterministic
// deadlock. Do not refactor this back into a persistent hold on b.mu.
func shutdownConns(b *Bus) {
	b.mu.Lock()
	conns := make([]*Conn, 0, len(b.conns))
	for c := range b.conns {
		conns = append(conns, c)
	}
	b.mu.Unlock()
	for _, c := range conns {
		c.Close()
	}
}

// startSources launches every registered source (or a default
// FeishuSource when none are registered). If any source exits before
// shutdown, the whole bus shuts down — a source-less bus is a zombie
// (consumers stay connected but never receive events, idle timer never
// fires).
func (b *Bus) startSources(ctx context.Context) {
	sources := source.All()
	if len(sources) == 0 {
		sources = []source.Source{&source.FeishuSource{
			AppID:     b.appID,
			AppSecret: b.appSecret,
			Domain:    b.domain,
			Logger:    b.logger,
		}}
	}
	eventTypes := subscribedEventTypes()
	b.hub.SetLogger(b.logger)
	for _, src := range sources {
		go func(s source.Source) {
			b.logger.Printf("Starting source: %s", s.Name())
			err := s.Start(ctx, eventTypes, func(raw *event.RawEvent) {
				b.logger.Printf("Event received: type=%s id=%s", raw.EventType, raw.EventID)
				if b.dedup.IsDuplicate(raw.EventID) {
					b.logger.Printf("Event deduplicated: id=%s", raw.EventID)
					return
				}
				b.hub.Publish(raw)
			}, func(state, detail string) {
				b.hub.BroadcastSourceStatus(s.Name(), state, detail)
			})
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				b.logger.Printf("Source %s exited with error: %v — shutting down bus", s.Name(), err)
			} else {
				b.logger.Printf("Source %s exited without error before shutdown — shutting down bus", s.Name())
			}
			select {
			case b.shutdownCh <- struct{}{}:
			default:
			}
		}(src)
	}
}

// subscribedEventTypes returns the deduplicated union of EventTypes from
// every registered EventKey. Sources that talk to a server-side
// dispatcher use this list to avoid subscribing to events no one wants.
func subscribedEventTypes() []string {
	seen := make(map[string]struct{})
	var types []string
	for _, def := range event.ListAll() {
		if _, ok := seen[def.EventType]; ok {
			continue
		}
		seen[def.EventType] = struct{}{}
		types = append(types, def.EventType)
	}
	return types
}

// acceptLoop accepts incoming IPC connections until the listener is closed.
func (b *Bus) acceptLoop(ctx context.Context) {
	for {
		conn, err := b.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// listener.Close() also causes Accept to return an error.
			// Check if we should exit by testing the listener.
			select {
			case <-ctx.Done():
				return
			default:
			}
			b.logger.Printf("Accept error: %v", err)
			return
		}
		go b.handleConn(conn)
	}
}

// handleConn reads the first protocol message and dispatches by type.
// The bufio.Reader is handed to Conn so bytes buffered past the first
// frame aren't lost to a second Scanner reading the same socket.
func (b *Bus) handleConn(conn net.Conn) {
	br := bufio.NewReader(conn)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := protocol.ReadFrame(br)
	if err != nil {
		conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{})

	msg, err := protocol.Decode(bytes.TrimRight(line, "\n"))
	if err != nil {
		conn.Close()
		return
	}

	switch m := msg.(type) {
	case *protocol.Hello:
		b.handleHello(conn, br, m)
	case *protocol.StatusQuery:
		b.handleStatusQuery(conn)
	case *protocol.Shutdown:
		b.handleShutdown(conn)
	default:
		conn.Close()
	}
}

// handleHello registers a consume connection with the hub. reader
// carries any bytes handleConn already pulled off conn.
func (b *Bus) handleHello(conn net.Conn, reader *bufio.Reader, hello *protocol.Hello) {
	bc := NewConn(conn, reader, hello.EventKey, hello.EventTypes, hello.PID)
	bc.SetLogger(b.logger)

	// Register + isFirst decision under one lock so concurrent Hellos
	// can't both trigger PreConsume. Waits if another conn is currently
	// holding a cleanup lock for the same EventKey, closing the
	// PreShutdownCheck × Hello TOCTOU race.
	firstForKey := b.hub.RegisterAndIsFirst(bc)

	bc.SetCheckLastForKey(func(eventKey string) bool {
		// Atomically reserve cleanup rights. If someone else is already
		// cleaning up, or another subscriber exists, this returns false so
		// consume doesn't run cleanup (and won't tear down upstream
		// subscription while a peer is still using it). On true return,
		// the lock is released in OnClose.
		return b.hub.AcquireCleanupLock(eventKey)
	})
	bc.SetOnClose(func(c *Conn) {
		b.hub.UnregisterAndIsLast(c)
		// Release cleanup lock if this connection had acquired one. Release
		// is idempotent — safe even when AcquireCleanupLock was never
		// called or returned false. Must fire on every disconnect path
		// (PreShutdownCheck, socket EOF, forced shutdown) so waiters in
		// RegisterAndIsFirst don't block forever.
		b.hub.ReleaseCleanupLock(c.EventKey())
		b.mu.Lock()
		delete(b.conns, c)
		remaining := len(b.conns)
		b.mu.Unlock()
		b.logger.Printf("Consumer disconnected: pid=%d key=%s (remaining=%d)", c.PID(), c.EventKey(), remaining)
		if remaining == 0 {
			// Stop+drain before Reset — Go docs require this to avoid
			// resetting a timer with a stale fire already sitting in .C.
			if !b.idleTimer.Stop() {
				select {
				case <-b.idleTimer.C:
				default:
				}
			}
			b.idleTimer.Reset(idleTimeout)
		}
	})

	b.mu.Lock()
	b.conns[bc] = struct{}{}
	// Stop+drain the idle timer while holding mu so a fire can't slip
	// past a fresh registration — Run re-checks count under the same lock.
	if !b.idleTimer.Stop() {
		select {
		case <-b.idleTimer.C:
		default:
		}
	}
	b.mu.Unlock()

	ack := protocol.NewHelloAck("v1", firstForKey)
	// Route through bc.writeFrame so all writes to this connection go
	// through the same mutex (see writeMu docs). On failure we must
	// undo the hub + bus-level registration — otherwise the consumer
	// lives on in b.conns / hub.subscribers without ever receiving an
	// ack, skewing first/last bookkeeping and keeping the bus non-idle.
	// bc.Close() triggers the onClose callback which handles both,
	// including the firstForKey==true case (keyCounts incremented to 1
	// by RegisterAndIsFirst is decremented back to 0 by
	// UnregisterAndIsLast; no PreConsume was triggered because the
	// client never received the ack).
	if err := bc.writeFrame(ack); err != nil {
		b.logger.Printf("WARN: hello_ack write to pid=%d key=%q failed: %v (rejecting connection)",
			hello.PID, hello.EventKey, err)
		bc.Close()
		return
	}

	// Quote untrusted fields — EventKey / EventTypes come straight off
	// the wire from an unprivileged local process, and a value with
	// embedded newlines would forge bus.log entries.
	b.logger.Printf("Consumer connected: pid=%d key=%q event_types=%q first=%v",
		hello.PID, hello.EventKey, hello.EventTypes, firstForKey)

	bc.Start()
}

// handleStatusQuery responds with Bus status information and closes the connection.
func (b *Bus) handleStatusQuery(conn net.Conn) {
	defer conn.Close()
	resp := protocol.NewStatusResponse(
		os.Getpid(),
		int(time.Since(b.startTime).Seconds()),
		b.hub.ConnCount(),
		b.hub.Consumers(),
	)
	_ = protocol.EncodeWithDeadline(conn, resp, protocol.WriteTimeout)
}

// handleShutdown signals Run() to exit gracefully.
func (b *Bus) handleShutdown(conn net.Conn) {
	defer conn.Close()
	b.logger.Printf("Received shutdown command")
	select {
	case b.shutdownCh <- struct{}{}:
	default:
	}
}
