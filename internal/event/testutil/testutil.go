// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package testutil holds test-only helpers shared across event subsystem tests.
package testutil

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"

	"github.com/larksuite/cli/internal/event/transport"
)

// FakeTransport delegates to inner with a fixed addr, so tests can use t.TempDir paths.
type FakeTransport struct {
	addr     string
	inner    transport.IPC
	mu       sync.Mutex
	cleaned  bool
	cleanups int
}

func NewWrappedFake(inner transport.IPC, addr string) *FakeTransport {
	return &FakeTransport{addr: addr, inner: inner}
}

func (t *FakeTransport) Listen(_ string) (net.Listener, error) {
	return t.inner.Listen(t.addr)
}

func (t *FakeTransport) Dial(_ string) (net.Conn, error) {
	return t.inner.Dial(t.addr)
}

func (t *FakeTransport) Address(_ string) string { return t.addr }

func (t *FakeTransport) Cleanup(_ string) {
	t.mu.Lock()
	t.cleaned = true
	t.cleanups++
	t.mu.Unlock()
	t.inner.Cleanup(t.addr)
}

func (t *FakeTransport) DidCleanup() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cleaned
}

func (t *FakeTransport) CleanupCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cleanups
}

// StubAPIClient records the last call and returns Body or Err.
type StubAPIClient struct {
	Body string
	Err  error

	mu        sync.Mutex
	GotMethod string
	GotPath   string
	GotBody   interface{}
	Calls     int
}

func (s *StubAPIClient) CallAPI(_ context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	s.mu.Lock()
	s.GotMethod = method
	s.GotPath = path
	s.GotBody = body
	s.Calls++
	s.mu.Unlock()
	if s.Err != nil {
		return nil, s.Err
	}
	if s.Body == "" {
		return json.RawMessage("{}"), nil
	}
	return json.RawMessage(s.Body), nil
}

var ErrStubUnconfigured = errors.New("testutil.StubAPIClient: no body or err configured")
