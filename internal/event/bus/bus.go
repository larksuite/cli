// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package bus implements the per-AppID event-bus daemon; lifecycle is driven by consumer presence (idle timeout) and explicit shutdown.
package bus

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/busdiscover"
	"github.com/larksuite/cli/internal/event/protocol"
	"github.com/larksuite/cli/internal/event/source"
	"github.com/larksuite/cli/internal/event/transport"
	"github.com/larksuite/cli/internal/lockfile"
)

const (
	idleTimeout = 30 * time.Second
)

// Bus is the central event bus daemon.
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

	// pidHandle pins the alive.lock fd to the bus lifetime; OS releases on exit.
	pidHandle *busdiscover.Handle
}

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
		// Buffered so shutdown and source-exit paths never drop the signal.
		shutdownCh: make(chan struct{}, 1),
	}
}

// Run binds the IPC socket, starts event sources, and blocks in the accept loop until shutdown.
func (b *Bus) Run(ctx context.Context) error {
	addr := b.transport.Address(b.appID)

	// alive.lock before bind: closes the cleanup-TOCTOU race where two newly forked
	// buses each unlink and rebind the socket. Brief retry covers stop-then-restart.
	eventsDir := filepath.Join(core.GetConfigDir(), "events", event.SanitizeAppID(b.appID))
	pidHandle, pidErr := acquireAliveLock(eventsDir)
	if pidErr != nil {
		if errors.Is(pidErr, lockfile.ErrHeld) {
			b.logger.Printf("Another bus already holds %s/bus.alive.lock, exiting", eventsDir)
			return nil
		}
		b.logger.Printf("[bus] pid file write failed: %v (status discovery may miss this bus)", pidErr)
	} else {
		b.pidHandle = pidHandle
	}

	ln, err := b.transport.Listen(addr)
	if err != nil {
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

	// Re-check live conn count under lock: a stale idle tick can linger past a concurrent Stop+Reset.
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
	// Don't delete the socket: Run() handles stale sockets on startup, and deletion races a new bus.
	shutdownConns(b)
	<-acceptDone
	b.logger.Printf("Bus exited cleanly")
	return nil
}

// shutdownConns snapshots b.conns under lock then releases before Close() — Close→onClose reacquires b.mu.
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

// startSources launches registered sources (or a default FeishuSource); any source exit triggers full bus shutdown.
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

// subscribedEventTypes returns the deduplicated union of EventTypes from every registered EventKey.
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

// acceptLoop accepts IPC connections until the listener is closed.
func (b *Bus) acceptLoop(ctx context.Context) {
	for {
		conn, err := b.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
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

// handleConn reads the first protocol message and dispatches; the bufio.Reader is handed to Conn so buffered bytes carry over.
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

// handleHello registers a consume connection with the hub; reader carries bytes already pulled off conn.
func (b *Bus) handleHello(conn net.Conn, reader *bufio.Reader, hello *protocol.Hello) {
	bc := NewConn(conn, reader, hello.EventKey, hello.EventTypes, hello.PID)
	bc.SetLogger(b.logger)

	// Register + isFirst under one lock; blocks on any in-progress cleanup lock for the same EventKey.
	firstForKey := b.hub.RegisterAndIsFirst(bc)

	bc.SetCheckLastForKey(func(eventKey string) bool {
		return b.hub.AcquireCleanupLock(eventKey)
	})
	bc.SetOnClose(func(c *Conn) {
		b.hub.UnregisterAndIsLast(c)
		// Release is idempotent and must fire on every disconnect path so waiters don't block forever.
		b.hub.ReleaseCleanupLock(c.EventKey())
		b.mu.Lock()
		delete(b.conns, c)
		remaining := len(b.conns)
		b.mu.Unlock()
		b.logger.Printf("Consumer disconnected: pid=%d key=%s (remaining=%d)", c.PID(), c.EventKey(), remaining)
		if remaining == 0 {
			// Stop+drain before Reset (Go docs) to avoid a stale fire in .C.
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
	// Stop+drain under mu so a fire can't slip past a fresh registration.
	if !b.idleTimer.Stop() {
		select {
		case <-b.idleTimer.C:
		default:
		}
	}
	b.mu.Unlock()

	ack := protocol.NewHelloAck("v1", firstForKey)
	// writeFrame shares writeMu with every other write; bc.Close on failure unwinds hub+bus registration via onClose.
	if err := bc.writeFrame(ack); err != nil {
		b.logger.Printf("WARN: hello_ack write to pid=%d key=%q failed: %v (rejecting connection)",
			hello.PID, hello.EventKey, err)
		bc.Close()
		return
	}

	// Quote untrusted fields to prevent log forging via embedded newlines.
	b.logger.Printf("Consumer connected: pid=%d key=%q event_types=%q first=%v",
		hello.PID, hello.EventKey, hello.EventTypes, firstForKey)

	bc.Start()
}

// handleStatusQuery replies with status and closes.
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

// handleShutdown signals Run() to exit.
func (b *Bus) handleShutdown(conn net.Conn) {
	defer conn.Close()
	b.logger.Printf("Received shutdown command")
	select {
	case b.shutdownCh <- struct{}{}:
	default:
	}
}

const (
	aliveLockMaxWait      = 2 * time.Second
	aliveLockPollInterval = 50 * time.Millisecond
)

// acquireAliveLock retries on ErrHeld so a stop-then-immediate-restart finds the lock free.
func acquireAliveLock(eventsDir string) (*busdiscover.Handle, error) {
	deadline := time.Now().Add(aliveLockMaxWait)
	for {
		h, err := busdiscover.WritePIDFile(eventsDir, os.Getpid())
		if err == nil {
			return h, nil
		}
		if !errors.Is(err, lockfile.ErrHeld) || time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(aliveLockPollInterval)
	}
}
