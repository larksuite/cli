// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"context"
	"net/http"
	"testing"
)

type stubInterceptor struct{}

func (s *stubInterceptor) PreRoundTrip(req *http.Request) func(*http.Response, error) {
	return nil
}

type stubProvider struct {
	name string
}

func (s *stubProvider) Name() string                                   { return s.name }
func (s *stubProvider) ResolveInterceptor(context.Context) Interceptor { return &stubInterceptor{} }

func TestGetProvider_NilByDefault(t *testing.T) {
	mu.Lock()
	provider = nil
	mu.Unlock()

	if got := GetProvider(); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestRegisterAndGet(t *testing.T) {
	mu.Lock()
	provider = nil
	mu.Unlock()

	p := &stubProvider{name: "a"}
	Register(p)

	got := GetProvider()
	if got != p {
		t.Fatalf("expected registered provider, got %v", got)
	}
}

func TestLastRegistrationWins(t *testing.T) {
	mu.Lock()
	provider = nil
	mu.Unlock()

	a := &stubProvider{name: "a"}
	b := &stubProvider{name: "b"}
	Register(a)
	Register(b)

	got := GetProvider()
	if got != b {
		t.Fatalf("expected provider b, got %v", got)
	}
}

func TestResolveInterceptor_ReturnsNonNil(t *testing.T) {
	mu.Lock()
	provider = nil
	mu.Unlock()

	p := &stubProvider{name: "test"}
	Register(p)

	ic := GetProvider().ResolveInterceptor(context.Background())
	if ic == nil {
		t.Fatal("expected non-nil Interceptor")
	}
}
