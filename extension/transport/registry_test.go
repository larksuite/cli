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

type stubDistributionProvider struct {
	stubProvider
	manifestURL string
}

func (s *stubDistributionProvider) ResolveManifestURL(context.Context) string {
	return s.manifestURL
}

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

func TestDistributionProviderIsOptional(t *testing.T) {
	previous := GetProvider()
	t.Cleanup(func() { Register(previous) })
	p := &stubDistributionProvider{
		stubProvider: stubProvider{name: "distribution"},
		manifestURL:  "https://dist.example/manifest.json",
	}
	Register(p)
	configured, ok := GetProvider().(DistributionProvider)
	if !ok {
		t.Fatal("registered provider does not implement DistributionProvider")
	}
	if got := configured.ResolveManifestURL(context.Background()); got != p.manifestURL {
		t.Fatalf("ManifestURL = %q", got)
	}
}
