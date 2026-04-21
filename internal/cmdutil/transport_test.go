// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	exttransport "github.com/larksuite/cli/extension/transport"
	internalauth "github.com/larksuite/cli/internal/auth"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// ---------------------------------------------------------------------------
// RetryTransport
// ---------------------------------------------------------------------------

func TestRetryTransport_NoRetry(t *testing.T) {
	calls := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	rt := &RetryTransport{Base: base, MaxRetries: 0}
	req, _ := http.NewRequest("GET", "http://example.com/test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestRetryTransport_RetryOn500(t *testing.T) {
	calls := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls < 3 {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("error"))}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	rt := &RetryTransport{Base: base, MaxRetries: 3, Delay: 1 * time.Millisecond}
	req, _ := http.NewRequest("GET", "http://example.com/test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 after retries, got %d", resp.StatusCode)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestRetryTransport_DefaultNoRetry(t *testing.T) {
	calls := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("error"))}, nil
	})
	rt := &RetryTransport{Base: base} // default MaxRetries=0
	req, _ := http.NewRequest("GET", "http://example.com/test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 500 {
		t.Errorf("expected 500 with no retries, got %d", resp.StatusCode)
	}
	if calls != 1 {
		t.Errorf("expected 1 call with default config, got %d", calls)
	}
}

// ---------------------------------------------------------------------------
// buildSDKTransport chain composition
// ---------------------------------------------------------------------------

func TestBuildSDKTransport_IncludesRetryTransport(t *testing.T) {
	transport := buildSDKTransport()

	sec, ok := transport.(*internalauth.SecurityPolicyTransport)
	if !ok {
		t.Fatalf("outer transport type = %T, want *auth.SecurityPolicyTransport", transport)
	}
	ua, ok := sec.Base.(*UserAgentTransport)
	if !ok {
		t.Fatalf("middle transport type = %T, want *UserAgentTransport", sec.Base)
	}
	if _, ok := ua.Base.(*RetryTransport); !ok {
		t.Fatalf("inner transport type = %T, want *RetryTransport", ua.Base)
	}
}

func TestBuildSDKTransport_WithExtension(t *testing.T) {
	exttransport.Register(&stubTransportProvider{})
	t.Cleanup(func() { exttransport.Register(nil) })

	transport := buildSDKTransport()

	// Chain: extensionMiddleware → SecurityPolicy → UserAgent → Retry → Base
	mid, ok := transport.(*extensionMiddleware)
	if !ok {
		t.Fatalf("outer transport type = %T, want *extensionMiddleware", transport)
	}
	sec, ok := mid.Base.(*internalauth.SecurityPolicyTransport)
	if !ok {
		t.Fatalf("transport type = %T, want *auth.SecurityPolicyTransport", mid.Base)
	}
	ua, ok := sec.Base.(*UserAgentTransport)
	if !ok {
		t.Fatalf("transport type = %T, want *UserAgentTransport", sec.Base)
	}
	if _, ok := ua.Base.(*RetryTransport); !ok {
		t.Fatalf("innermost transport type = %T, want *RetryTransport", ua.Base)
	}
}

func TestBuildSDKTransport_WithoutExtension(t *testing.T) {
	exttransport.Register(nil)

	transport := buildSDKTransport()

	sec, ok := transport.(*internalauth.SecurityPolicyTransport)
	if !ok {
		t.Fatalf("outer transport type = %T, want *auth.SecurityPolicyTransport", transport)
	}
	ua, ok := sec.Base.(*UserAgentTransport)
	if !ok {
		t.Fatalf("middle transport type = %T, want *UserAgentTransport", sec.Base)
	}
	if _, ok := ua.Base.(*RetryTransport); !ok {
		t.Fatalf("inner transport type = %T, want *RetryTransport", ua.Base)
	}
}

// ---------------------------------------------------------------------------
// extensionMiddleware — legacy Interceptor path
// ---------------------------------------------------------------------------

type stubTransportProvider struct {
	interceptor exttransport.Interceptor
}

func (s *stubTransportProvider) Name() string { return "stub" }
func (s *stubTransportProvider) ResolveInterceptor(context.Context) exttransport.Interceptor {
	if s.interceptor != nil {
		return s.interceptor
	}
	return &stubTransportImpl{}
}

type stubTransportImpl struct{}

func (s *stubTransportImpl) PreRoundTrip(req *http.Request) func(*http.Response, error) {
	return nil
}

// headerCapturingInterceptor sets custom headers in PreRoundTrip and records
// whether PostRoundTrip was called, to verify execution order.
type headerCapturingInterceptor struct {
	preCalled  bool
	postCalled bool
}

func (h *headerCapturingInterceptor) PreRoundTrip(req *http.Request) func(*http.Response, error) {
	h.preCalled = true
	// Set a custom header that should survive (no built-in override)
	req.Header.Set("X-Custom-Trace", "ext-trace-123")
	// Try to override a security header — should be overwritten by SecurityHeaderTransport
	req.Header.Set(HeaderSource, "ext-tampered")
	return func(resp *http.Response, err error) {
		h.postCalled = true
	}
}

func TestExtensionInterceptor_ExecutionOrder(t *testing.T) {
	var receivedHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ic := &headerCapturingInterceptor{}
	exttransport.Register(&stubTransportProvider{interceptor: ic})
	t.Cleanup(func() { exttransport.Register(nil) })

	// Use HTTP transport chain (has SecurityHeaderTransport)
	var base http.RoundTripper = http.DefaultTransport
	base = &RetryTransport{Base: base}
	base = &SecurityHeaderTransport{Base: base}
	transport := wrapWithExtension(base)
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// PreRoundTrip was called
	if !ic.preCalled {
		t.Fatal("PreRoundTrip was not called")
	}
	// PostRoundTrip (closure) was called
	if !ic.postCalled {
		t.Fatal("PostRoundTrip closure was not called")
	}
	// Custom header set by extension survives (no built-in override)
	if got := receivedHeaders.Get("X-Custom-Trace"); got != "ext-trace-123" {
		t.Fatalf("X-Custom-Trace = %q, want %q", got, "ext-trace-123")
	}
	// Security header overridden by extension is restored by SecurityHeaderTransport
	if got := receivedHeaders.Get(HeaderSource); got != SourceValue {
		t.Fatalf("%s = %q, want %q (built-in should override extension)", HeaderSource, got, SourceValue)
	}
}

// interceptorFunc adapts a function to exttransport.Interceptor.
type interceptorFunc func(*http.Request) func(*http.Response, error)

func (f interceptorFunc) PreRoundTrip(req *http.Request) func(*http.Response, error) { return f(req) }

func TestExtensionInterceptor_ContextTamperPrevented(t *testing.T) {
	type ctxKeyType string
	const testKey ctxKeyType = "original"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var ctxValue any

	// Use a custom transport that captures the context value seen by the built-in chain
	capturer := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		ctxValue = req.Context().Value(testKey)
		return http.DefaultTransport.RoundTrip(req)
	})

	// Interceptor that tries to tamper with context
	tamperIC := interceptorFunc(func(req *http.Request) func(*http.Response, error) {
		// Try to replace context with a new one
		*req = *req.WithContext(context.WithValue(req.Context(), testKey, "tampered"))
		return nil
	})

	mid := &extensionMiddleware{Base: capturer, Ext: tamperIC}

	origCtx := context.WithValue(context.Background(), testKey, "original")
	req, _ := http.NewRequestWithContext(origCtx, "GET", srv.URL, nil)
	resp, err := mid.RoundTrip(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// Built-in chain should see original context, not tampered
	if ctxValue != "original" {
		t.Fatalf("built-in chain saw context value %q, want %q", ctxValue, "original")
	}
}

// ---------------------------------------------------------------------------
// extensionMiddleware — PreRoundTripE abort path
// ---------------------------------------------------------------------------

// abortingInterceptor implements exttransport.AbortableInterceptor and
// records invocation of the pre and post hooks. These middleware tests only
// assert middleware-level integration; pure *AbortError behavior
// (Error/Unwrap/Is/As) is covered in extension/transport/errors_test.go.
type abortingInterceptor struct {
	reason     error // if non-nil, PreRoundTripE returns this to abort
	nilPost    bool  // if true, PreRoundTripE returns a nil post func
	preECalled bool
	postCalled bool
	postResp   *http.Response
	postErr    error
}

// PreRoundTrip is a no-op that satisfies the legacy Interceptor method; the
// middleware never calls it when PreRoundTripE is present.
func (*abortingInterceptor) PreRoundTrip(*http.Request) func(*http.Response, error) {
	return nil
}

func (a *abortingInterceptor) PreRoundTripE(req *http.Request) (func(*http.Response, error), error) {
	a.preECalled = true
	if a.nilPost {
		return nil, a.reason
	}
	return func(resp *http.Response, err error) {
		a.postCalled = true
		a.postResp = resp
		a.postErr = err
	}, a.reason
}

func TestExtensionMiddleware_PreRoundTripEAbort(t *testing.T) {
	innerErr := errors.New("denied by policy")

	t.Run("skips base and wires AbortError fields", func(t *testing.T) {
		ic := &abortingInterceptor{reason: innerErr}
		baseCalls := 0
		base := roundTripFunc(func(*http.Request) (*http.Response, error) {
			baseCalls++
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		})

		mid := &extensionMiddleware{Base: base, Ext: ic, ExtName: "stub"}
		req, _ := http.NewRequest("GET", "http://example.invalid/", nil)
		resp, err := mid.RoundTrip(req)

		if resp != nil {
			t.Fatalf("resp = %v, want nil on abort", resp)
		}
		if baseCalls != 0 {
			t.Fatalf("base RoundTrip called %d times on abort, want 0", baseCalls)
		}
		if !ic.preECalled {
			t.Fatal("PreRoundTripE was not called")
		}

		var aErr *exttransport.AbortError
		if !errors.As(err, &aErr) {
			t.Fatalf("errors.As(*AbortError) = false, err = %v (%T)", err, err)
		}
		if aErr.Extension != "stub" || aErr.Reason != innerErr {
			t.Fatalf("AbortError = %+v, want {Extension:stub Reason:%v}", aErr, innerErr)
		}

		// Post must see the original inner err, not the *AbortError wrapper.
		if !ic.postCalled {
			t.Fatal("post hook was not called on abort")
		}
		if ic.postResp != nil {
			t.Fatalf("post resp = %v, want nil", ic.postResp)
		}
		if ic.postErr != innerErr {
			t.Fatalf("post err = %v, want original inner err %v", ic.postErr, innerErr)
		}
	})

	t.Run("nil post still returns AbortError", func(t *testing.T) {
		ic := &abortingInterceptor{reason: innerErr, nilPost: true}
		base := roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("base must not be called on abort")
			return nil, nil
		})

		mid := &extensionMiddleware{Base: base, Ext: ic, ExtName: "stub"}
		req, _ := http.NewRequest("GET", "http://example.invalid/", nil)
		_, err := mid.RoundTrip(req)

		var aErr *exttransport.AbortError
		if !errors.As(err, &aErr) {
			t.Fatalf("errors.As(*AbortError) = false, err = %v", err)
		}
	})
}

func TestExtensionMiddleware_PreRoundTripEHappyPath(t *testing.T) {
	ic := &abortingInterceptor{} // reason == nil → no abort
	baseCalls := 0
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		baseCalls++
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})

	mid := &extensionMiddleware{Base: base, Ext: ic, ExtName: "stub"}
	req, _ := http.NewRequest("GET", "http://example.invalid/", nil)
	resp, err := mid.RoundTrip(req)
	if err != nil {
		t.Fatalf("happy path returned err: %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("resp = %v, want 200", resp)
	}
	if baseCalls != 1 {
		t.Fatalf("base RoundTrip called %d times, want 1", baseCalls)
	}
	if !ic.preECalled {
		t.Fatal("PreRoundTripE was not called")
	}
	if !ic.postCalled || ic.postErr != nil {
		t.Fatalf("post hook not called or err != nil: called=%v err=%v", ic.postCalled, ic.postErr)
	}
}
