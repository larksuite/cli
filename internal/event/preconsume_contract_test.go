// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

// PreConsume first/last lifecycle contract tests.
//
// These tests pin the setup/cleanup ownership semantics of the consume
// pipeline against a real in-process bus (fake transport on a temp socket):
//
//   - PreConsume (setup) runs exactly once per key while at least one consumer
//     stays connected — a duplicate setup would register a duplicate
//     server-side subscription record.
//   - cleanup runs at most once, and only when the exiting consumer is the
//     LAST one for its key — an early cleanup would tear down a server-side
//     subscription that other still-live consumers depend on.
//
// They are a frozen baseline: any refactor of the bus/consume lifecycle must
// keep these green (or consciously revisit the contract — see the note on
// TestPreConsumeContract_OwnerExitsFirst).
package event_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/bus"
	"github.com/larksuite/cli/internal/event/catalog"
	"github.com/larksuite/cli/internal/event/consume"
	"github.com/larksuite/cli/internal/event/adapter/localbus/protocol"
	"github.com/larksuite/cli/internal/event/testutil"
	"github.com/larksuite/cli/internal/event/adapter/localbus/transport"
)

// contractAppSeq makes every test invocation use a distinct app id. The bus
// pins its per-app-id alive lock fd until process exit, so reusing an app id
// within one test process (e.g. under -count=2) would make the second bus
// conclude another bus is already running and refuse to start.
var contractAppSeq atomic.Int64

type preConsumeCounters struct {
	setup   atomic.Int64
	cleanup atomic.Int64
}

// contractKey declares a synthetic EventKey whose PreConsume counts setups.
// With withCleanup, PreConsume returns a closure that counts cleanups;
// without, it returns (nil, nil) — modeling keys whose server-side
// subscription is a durable relationship with deliberately no unsubscribe.
// Callers compile the declaration into the snapshot the bus and consumers use.
func contractKey(key string, withCleanup bool) (event.KeyDefinition, *preConsumeCounters) {
	c := &preConsumeCounters{}
	def := event.KeyDefinition{
		Key:       key,
		EventType: key,
		Schema:    integNativeSchema(),
		PreConsume: func(context.Context, event.APIClient, map[string]string) (func() error, error) {
			c.setup.Add(1)
			if !withCleanup {
				return nil, nil
			}
			return func() error {
				c.cleanup.Add(1)
				return nil
			}, nil
		},
	}
	return def, c
}

// resolveDef pulls the canonical definition for a key out of a compiled
// snapshot — what the CLI entry point hands consume.Run as Options.Def.
func resolveDef(t *testing.T, snap *catalog.Snapshot, key string) *event.KeyDefinition {
	t.Helper()
	entry, ok := snap.Resolve(key)
	if !ok {
		t.Fatalf("compiled snapshot has no entry for %s", key)
	}
	return entry.Definition()
}

// startContractBus runs an in-process bus on a temp-dir socket with a mock
// source (so no real upstream connection is attempted) and returns the fake
// transport plus the unique app id consumers must use. snap is the compiled
// catalog the bus serves.
func startContractBus(t *testing.T, snap *catalog.Snapshot) (*testutil.FakeTransport, string) {
	t.Helper()

	appID := fmt.Sprintf("preconsume-contract-%d-%d", os.Getpid(), contractAppSeq.Add(1))
	// Short-named temp dir instead of t.TempDir(): the long test names would
	// push the unix socket path past the OS sun_path limit (~104 bytes on
	// macOS), making bind fail with EINVAL.
	sockDir, err := os.MkdirTemp("", "pcc-*")
	if err != nil {
		t.Fatalf("create socket dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	addr := filepath.Join(sockDir, "s")
	tr := testutil.NewWrappedFake(transport.New(), addr)
	logger := log.New(os.Stderr, "[contract-bus] ", log.LstdFlags)
	b := bus.NewBus(appID, "test-secret", "", tr, logger, snap, &mockIntegSource{})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	runBus(t, b, ctx)
	// Registered after runBus so it fires before runBus's wait-for-exit cleanup.
	t.Cleanup(cancel)
	waitForBusReady(t, tr, addr)
	return tr, appID
}

// contractConsumer drives one consume.Run in a goroutine with its own
// cancellable context, so tests control exit order deterministically.
type contractConsumer struct {
	name   string
	cancel context.CancelFunc
	done   chan error
}

func startContractConsumer(t *testing.T, tr transport.IPC, appID string, def *event.KeyDefinition, name string) *contractConsumer {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h := &contractConsumer{name: name, cancel: cancel, done: make(chan error, 1)}
	go func() {
		h.done <- consume.Run(ctx, tr, appID, "", "", consume.Options{
			EventKey: def.Key,
			Def:      def,
			Params:   map[string]string{},
			Quiet:    true,
			Out:      io.Discard,
			ErrOut:   io.Discard,
		})
	}()
	return h
}

// stop cancels the consumer's context and waits for consume.Run to return,
// i.e. its shutdown path (last-for-key check + possible cleanup) completed.
func (h *contractConsumer) stop(t *testing.T) {
	t.Helper()
	h.cancel()
	select {
	case err := <-h.done:
		if err != nil {
			t.Fatalf("consumer %s exited with error: %v", h.name, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("consumer %s did not exit within 10s after cancel", h.name)
	}
}

func waitForState(t *testing.T, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", desc)
}

// busSubscriberCount asks the bus (status query round-trip) how many consumer
// connections its hub currently tracks. The count drops only after the bus
// fully processed a disconnect, so polling it makes exit ordering
// deterministic instead of sleep-based.
func busSubscriberCount(tr transport.IPC, addr string) (int, error) {
	conn, err := tr.Dial(addr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return 0, err
	}
	if err := protocol.Encode(conn, protocol.NewStatusQuery()); err != nil {
		return 0, err
	}
	line, err := protocol.ReadFrame(bufio.NewReader(conn))
	if err != nil {
		return 0, err
	}
	msg, err := protocol.Decode(bytes.TrimRight(line, "\n"))
	if err != nil {
		return 0, err
	}
	resp, ok := msg.(*protocol.StatusResponse)
	if !ok {
		return 0, fmt.Errorf("expected StatusResponse, got %T", msg)
	}
	return resp.ActiveConns, nil
}

func waitForSubscriberCount(t *testing.T, tr transport.IPC, appID string, want int) {
	t.Helper()
	addr := tr.Address(appID)
	waitForState(t, fmt.Sprintf("bus subscriber count == %d", want), func() bool {
		n, err := busSubscriberCount(tr, addr)
		return err == nil && n == want
	})
}

// TestPreConsumeContract_NonOwnerExitsFirst: A connects first (runs setup,
// holds the cleanup closure), B joins, then B exits BEFORE A. B is not the
// last consumer for the key, so nothing may be cleaned up while A still
// depends on the server-side subscription. A then exits last and runs cleanup
// exactly once.
func TestPreConsumeContract_NonOwnerExitsFirst(t *testing.T) {
	const key = "contract.nonowner.v1"
	keyDef, counters := contractKey(key, true)
	snap := compileTestSnapshot(t, keyDef)
	def := resolveDef(t, snap, key)
	tr, appID := startContractBus(t, snap)

	a := startContractConsumer(t, tr, appID, def, "A")
	waitForSubscriberCount(t, tr, appID, 1)
	waitForState(t, "consumer A pre-consume setup", func() bool { return counters.setup.Load() == 1 })

	// A is registered and set up, so B deterministically joins as non-first
	// and never runs PreConsume (no duplicate server-side subscription).
	b := startContractConsumer(t, tr, appID, def, "B")
	waitForSubscriberCount(t, tr, appID, 2)

	// B exits while A is still connected: not last for the key, no cleanup.
	b.stop(t)
	// Wait until the bus fully processed B's disconnect before exiting A,
	// so A's last-for-key check cannot race B's teardown.
	waitForSubscriberCount(t, tr, appID, 1)

	// A exits last: it holds the cleanup closure and is last, cleanup runs once.
	a.stop(t)
	waitForSubscriberCount(t, tr, appID, 0)

	if got := counters.setup.Load(); got != 1 {
		t.Errorf("setup ran %d times, want 1 (duplicate setup would register duplicate server-side subscriptions)", got)
	}
	if got := counters.cleanup.Load(); got != 1 {
		t.Errorf("cleanup ran %d times, want 1 (last consumer must tear down the subscription exactly once)", got)
	}
}

// TestPreConsumeContract_OwnerExitsFirst: A connects first (runs setup, holds
// the cleanup closure), B joins, then A — the closure OWNER — exits first.
//
// This pins the CURRENT, KNOWN-LEAK semantics: cleanup ownership is not
// transferable. A skips cleanup because B is still connected (correct — B
// depends on the subscription), but when B later exits as the last consumer
// it has no closure to run, so the server-side subscription is never cleaned
// up. Total cleanups: 0.
//
// If the cleanup==0 assertion here turns red, the cleanup ownership semantics
// have been changed (e.g. handing the closure off to survivors). That may
// well be an improvement, but it alters observable lifecycle behavior for
// every EventKey — take it through design review first, then update this
// baseline deliberately.
func TestPreConsumeContract_OwnerExitsFirst(t *testing.T) {
	const key = "contract.owner.v1"
	keyDef, counters := contractKey(key, true)
	snap := compileTestSnapshot(t, keyDef)
	def := resolveDef(t, snap, key)
	tr, appID := startContractBus(t, snap)

	a := startContractConsumer(t, tr, appID, def, "A")
	waitForSubscriberCount(t, tr, appID, 1)
	waitForState(t, "consumer A pre-consume setup", func() bool { return counters.setup.Load() == 1 })

	b := startContractConsumer(t, tr, appID, def, "B")
	waitForSubscriberCount(t, tr, appID, 2)

	// Owner exits first: B is still connected, so A must NOT run cleanup —
	// doing so would unsubscribe the server-side state B depends on.
	a.stop(t)
	waitForSubscriberCount(t, tr, appID, 1)

	// B exits last: it is last for the key, but it never ran PreConsume and
	// holds no cleanup closure — nothing runs. This is the leak being pinned.
	b.stop(t)
	waitForSubscriberCount(t, tr, appID, 0)

	if got := counters.setup.Load(); got != 1 {
		t.Errorf("setup ran %d times, want 1 (B joined as non-first and must not re-run setup)", got)
	}
	if got := counters.cleanup.Load(); got != 0 {
		t.Errorf("cleanup ran %d times, want 0 — current contract leaks the server-side subscription when the setup owner exits before the last consumer; see the test comment before changing this", got)
	}
}

// TestPreConsumeContract_NoCleanupKey: the key's PreConsume returns (nil, nil)
// — setup establishes a durable server-side relationship that is deliberately
// never unsubscribed. Regardless of exit order, cleanup count stays 0, and
// setup re-runs each time a consumer becomes first for the key again.
func TestPreConsumeContract_NoCleanupKey(t *testing.T) {
	const key = "contract.nocleanup.v1"
	keyDef, counters := contractKey(key, false)
	snap := compileTestSnapshot(t, keyDef)
	def := resolveDef(t, snap, key)
	tr, appID := startContractBus(t, snap)

	// Round 1: non-owner exits first.
	a := startContractConsumer(t, tr, appID, def, "A")
	waitForSubscriberCount(t, tr, appID, 1)
	waitForState(t, "consumer A pre-consume setup", func() bool { return counters.setup.Load() == 1 })
	b := startContractConsumer(t, tr, appID, def, "B")
	waitForSubscriberCount(t, tr, appID, 2)
	b.stop(t)
	waitForSubscriberCount(t, tr, appID, 1)
	a.stop(t)
	waitForSubscriberCount(t, tr, appID, 0)

	// Round 2: owner exits first. The key's consumer count dropped to zero
	// above, so C is first for the key again and setup runs a second time.
	c := startContractConsumer(t, tr, appID, def, "C")
	waitForSubscriberCount(t, tr, appID, 1)
	waitForState(t, "consumer C pre-consume setup", func() bool { return counters.setup.Load() == 2 })
	d := startContractConsumer(t, tr, appID, def, "D")
	waitForSubscriberCount(t, tr, appID, 2)
	c.stop(t)
	waitForSubscriberCount(t, tr, appID, 1)
	d.stop(t)
	waitForSubscriberCount(t, tr, appID, 0)

	if got := counters.setup.Load(); got != 2 {
		t.Errorf("setup ran %d times, want 2 (once per first-for-key consumer)", got)
	}
	if got := counters.cleanup.Load(); got != 0 {
		t.Errorf("cleanup ran %d times, want 0 (key declares no cleanup: the server-side subscription is a durable relationship)", got)
	}
}
