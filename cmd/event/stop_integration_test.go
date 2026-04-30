// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"bufio"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event/protocol"
)

type mockTransport struct {
	mu      sync.Mutex
	addr    string
	cleaned bool
}

func (t *mockTransport) Listen(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}

func (t *mockTransport) Dial(addr string) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, 500*time.Millisecond)
}

func (t *mockTransport) Address(appID string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.addr
}

func (t *mockTransport) Cleanup(addr string) {
	t.mu.Lock()
	t.cleaned = true
	t.mu.Unlock()
}

func (t *mockTransport) didCleanup() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cleaned
}

type fakeBus struct {
	listener     net.Listener
	pid          int
	exitDelay    time.Duration
	unresponsive bool

	shutdownCount int32
	wg            sync.WaitGroup

	stopOnce sync.Once
	done     chan struct{}
}

func newFakeBus(t *testing.T, pid int, exitDelay time.Duration, unresponsive bool) *fakeBus {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	b := &fakeBus{
		listener:     ln,
		pid:          pid,
		exitDelay:    exitDelay,
		unresponsive: unresponsive,
		done:         make(chan struct{}),
	}
	b.wg.Add(1)
	go b.serve()
	return b
}

func (b *fakeBus) addr() string { return b.listener.Addr().String() }

func (b *fakeBus) serve() {
	defer b.wg.Done()
	for {
		conn, err := b.listener.Accept()
		if err != nil {
			return
		}
		b.wg.Add(1)
		go b.handle(conn)
	}
}

func (b *fakeBus) handle(conn net.Conn) {
	defer b.wg.Done()
	defer conn.Close()

	r := bufio.NewReader(conn)
	line, err := r.ReadBytes('\n')
	if err != nil {
		return
	}
	msg, err := protocol.Decode(line)
	if err != nil {
		return
	}

	switch msg.(type) {
	case *protocol.StatusQuery:
		_ = protocol.Encode(conn, &protocol.StatusResponse{
			Type:        protocol.MsgTypeStatusResponse,
			PID:         b.pid,
			UptimeSec:   1,
			ActiveConns: 0,
			Consumers:   nil,
		})
	case *protocol.Shutdown:
		atomic.AddInt32(&b.shutdownCount, 1)
		if b.unresponsive {
			return
		}
		if b.exitDelay > 0 {
			go func() {
				time.Sleep(b.exitDelay)
				b.stop()
			}()
		} else {
			go b.stop()
		}
	}
}

func (b *fakeBus) stop() {
	b.stopOnce.Do(func() {
		_ = b.listener.Close()
		close(b.done)
	})
}

func (b *fakeBus) wait(t *testing.T, budget time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(budget):
		t.Fatalf("fakeBus did not shut down within %v", budget)
	}
}

func TestStopReturnsStoppedOnlyAfterBusExits(t *testing.T) {
	const pid = 44441
	const exitDelay = 500 * time.Millisecond

	bus := newFakeBus(t, pid, exitDelay, false)
	defer bus.stop()
	tr := &mockTransport{addr: bus.addr()}

	start := time.Now()
	res := stopBusOne(tr, "test-app", false)
	elapsed := time.Since(start)

	if res.Status != "stopped" {
		t.Fatalf("status = %q (reason=%q); want stopped", res.Status, res.Reason)
	}
	if res.PID != pid {
		t.Fatalf("pid = %d; want %d", res.PID, pid)
	}
	if elapsed < 400*time.Millisecond {
		t.Fatalf("stopBusOne returned in %v; expected >= %v (waited for bus to exit)", elapsed, exitDelay)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("stopBusOne took %v; expected well under 3s", elapsed)
	}

	bus.wait(t, 2*time.Second)
	if got := atomic.LoadInt32(&bus.shutdownCount); got != 1 {
		t.Errorf("fakeBus received %d Shutdown messages; want 1", got)
	}
}

func TestStopTimesOutOnUnresponsiveBusWithoutForce(t *testing.T) {
	const pid = 44442

	origKill := killProcess
	t.Cleanup(func() { killProcess = origKill })
	var killCalls []int
	var killMu sync.Mutex
	killProcess = func(p int) error {
		killMu.Lock()
		killCalls = append(killCalls, p)
		killMu.Unlock()
		return nil
	}

	bus := newFakeBus(t, pid, 0, true)
	defer bus.stop()
	tr := &mockTransport{addr: bus.addr()}

	origBudget := shutdownBudget
	t.Cleanup(func() { shutdownBudget = origBudget })
	shutdownBudget = 500 * time.Millisecond

	start := time.Now()
	res := stopBusOne(tr, "test-app", false)
	elapsed := time.Since(start)

	if res.Status != "error" {
		t.Fatalf("status = %q (reason=%q); want error", res.Status, res.Reason)
	}
	if res.PID != pid {
		t.Errorf("pid = %d; want %d", res.PID, pid)
	}
	if elapsed < shutdownBudget || elapsed > shutdownBudget+2*time.Second {
		t.Fatalf("elapsed = %v; want >= %v and < %v", elapsed, shutdownBudget, shutdownBudget+2*time.Second)
	}
	if !strings.Contains(res.Reason, "did not exit within") {
		t.Errorf("reason %q should mention 'did not exit within'", res.Reason)
	}
	killMu.Lock()
	defer killMu.Unlock()
	if len(killCalls) != 0 {
		t.Errorf("killProcess called %v; want 0 calls without --force", killCalls)
	}
	if tr.didCleanup() {
		t.Errorf("Cleanup should not be called when --force is false")
	}
}

func TestStopForceKillsUnresponsiveBus(t *testing.T) {
	const pid = 44443

	origKill := killProcess
	t.Cleanup(func() { killProcess = origKill })
	var killCalls []int
	var killMu sync.Mutex
	killProcess = func(p int) error {
		killMu.Lock()
		killCalls = append(killCalls, p)
		killMu.Unlock()
		return nil
	}

	bus := newFakeBus(t, pid, 0, true)
	defer bus.stop()
	tr := &mockTransport{addr: bus.addr()}

	origBudget := shutdownBudget
	t.Cleanup(func() { shutdownBudget = origBudget })
	shutdownBudget = 500 * time.Millisecond

	start := time.Now()
	res := stopBusOne(tr, "test-app", true)
	elapsed := time.Since(start)

	if res.Status != "stopped" {
		t.Fatalf("status = %q (reason=%q); want stopped", res.Status, res.Reason)
	}
	if res.PID != pid {
		t.Errorf("pid = %d; want %d", res.PID, pid)
	}
	if elapsed < shutdownBudget || elapsed > shutdownBudget+2*time.Second {
		t.Fatalf("elapsed = %v; want >= %v and < %v", elapsed, shutdownBudget, shutdownBudget+2*time.Second)
	}
	if !strings.Contains(res.Reason, "killed") {
		t.Errorf("reason %q should mention 'killed'", res.Reason)
	}

	killMu.Lock()
	defer killMu.Unlock()
	if len(killCalls) != 1 || killCalls[0] != pid {
		t.Errorf("killProcess calls = %v; want [%d]", killCalls, pid)
	}
	if !tr.didCleanup() {
		t.Errorf("Cleanup was not invoked after force-kill")
	}
}

func TestStopReturnsStoppedFastWhenBusExitsImmediately(t *testing.T) {
	const pid = 12345

	bus := newFakeBus(t, pid, 0, false)
	defer bus.stop()
	tr := &mockTransport{addr: bus.addr()}

	start := time.Now()
	res := stopBusOne(tr, "test-app", false)
	elapsed := time.Since(start)

	if res.Status != "stopped" {
		t.Fatalf("expected stopped, got %q (reason: %s)", res.Status, res.Reason)
	}
	if res.PID != pid {
		t.Errorf("expected PID=%d, got %d", pid, res.PID)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("expected fast return (<500ms), got %v — possibly waiting the full budget", elapsed)
	}
}

func TestStopForceHandlesProcessAlreadyDeadRace(t *testing.T) {
	const pid = 99999

	origKill := killProcess
	t.Cleanup(func() { killProcess = origKill })
	var killCalls []int
	var killMu sync.Mutex
	killProcess = func(p int) error {
		killMu.Lock()
		killCalls = append(killCalls, p)
		killMu.Unlock()
		return os.ErrProcessDone
	}

	bus := newFakeBus(t, pid, 0, true)
	defer bus.stop()
	tr := &mockTransport{addr: bus.addr()}

	res := stopBusOne(tr, "test-app", true)

	if res.Status != "stopped" {
		t.Errorf("expected stopped (race treated as success), got %q (reason: %s)", res.Status, res.Reason)
	}
	killMu.Lock()
	if len(killCalls) != 1 || killCalls[0] != pid {
		t.Errorf("expected killProcess called once with pid=%d, got %v", pid, killCalls)
	}
	killMu.Unlock()
	if !tr.didCleanup() {
		t.Error("expected Cleanup to be called even when kill reported already-dead")
	}
	if !strings.Contains(res.Reason, "exited during kill attempt") {
		t.Errorf("expected reason about race, got %q", res.Reason)
	}
}
