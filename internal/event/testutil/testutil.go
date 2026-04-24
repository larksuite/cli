// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package testutil holds shared helpers used across the event subsystem's
// test files. Types here are test-only but live outside _test.go so they
// can be imported by tests in sibling packages.
package testutil

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"

	"github.com/larksuite/cli/internal/event/transport"
)

// FakeTransport is a test-only transport.IPC that delegates every call
// to an underlying real transport, rewriting `addr` to the constructor-
// supplied value. Lets tests drive real Unix-socket behaviour on a short
// per-test path (e.g. t.TempDir()) without threading the address through
// production interfaces.
type FakeTransport struct {
	addr     string
	inner    transport.IPC
	mu       sync.Mutex
	cleaned  bool
	cleanups int
}

// NewWrappedFake returns a FakeTransport that delegates to inner but always
// overrides the address with addr. Caller owns inner's lifecycle.
func NewWrappedFake(inner transport.IPC, addr string) *FakeTransport {
	return &FakeTransport{addr: addr, inner: inner}
}

// Listen implements transport.IPC. The addr parameter is ignored — the
// constructor-supplied address wins so tests don't have to thread it.
func (t *FakeTransport) Listen(_ string) (net.Listener, error) {
	return t.inner.Listen(t.addr)
}

// Dial implements transport.IPC. See Listen comment for addr handling.
func (t *FakeTransport) Dial(_ string) (net.Conn, error) {
	return t.inner.Dial(t.addr)
}

// Address implements transport.IPC. Returns the constructor address
// regardless of the appID argument.
func (t *FakeTransport) Address(_ string) string { return t.addr }

// Cleanup implements transport.IPC. Records the call so tests can assert
// that Cleanup did or did not fire, then delegates to inner.
func (t *FakeTransport) Cleanup(_ string) {
	t.mu.Lock()
	t.cleaned = true
	t.cleanups++
	t.mu.Unlock()
	t.inner.Cleanup(t.addr)
}

// DidCleanup reports whether Cleanup has been called at least once.
func (t *FakeTransport) DidCleanup() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cleaned
}

// CleanupCount returns the number of times Cleanup has been invoked.
func (t *FakeTransport) CleanupCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cleanups
}

// StubAPIClient is a minimal event.APIClient / appmeta.APIClient /
// consume.APIClient stub. It records the last call and returns a
// pre-configured body or error.
type StubAPIClient struct {
	Body string // body to return on success; empty ⇒ "{}"
	Err  error  // if non-nil, returned instead of Body

	mu        sync.Mutex
	GotMethod string
	GotPath   string
	GotBody   interface{}
	Calls     int
}

// CallAPI records the call and returns the configured response.
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

// ErrStubUnconfigured is returned as a sentinel so tests can assert via
// errors.Is when they expect a stub to have been skipped.
var ErrStubUnconfigured = errors.New("testutil.StubAPIClient: no body or err configured")
