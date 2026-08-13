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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	exttransport "github.com/larksuite/cli/extension/transport"
	"github.com/larksuite/cli/internal/riskcontrol"
	"github.com/larksuite/cli/internal/runtimeplan"
	internaltransport "github.com/larksuite/cli/internal/transport"
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

// buildSDKTransport policy behavior
// ---------------------------------------------------------------------------

func TestBuildSDKTransportAppliesSecurityHeadersToEveryRequestClass(t *testing.T) {
	exttransport.Register(nil)
	received := make(chan http.Header, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		received <- req.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	for _, class := range []exttransport.RequestClass{
		exttransport.RequestClassPlatform,
		exttransport.RequestClassExternal,
	} {
		client := internaltransport.ClientForRequestClass(
			&http.Client{Transport: buildSDKTransport(nil)},
			class,
		)
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		headers := <-received
		for header, values := range BaseSecurityHeaders() {
			if len(values) == 0 {
				continue
			}
			want := values[len(values)-1]
			if got := headers.Get(header); got != want {
				t.Fatalf("SDK %s header %s = %q, want %q", class, header, got, want)
			}
		}
	}
}

func TestBuildSDKTransport_WithExtension(t *testing.T) {
	previous := exttransport.GetProvider()
	interceptor := &headerCapturingInterceptor{}
	exttransport.Register(&platformOnlyStubProvider{
		stubTransportProvider: &stubTransportProvider{interceptor: interceptor},
	})
	t.Cleanup(func() { exttransport.Register(previous) })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := internaltransport.ClientForRequestClass(
		&http.Client{Transport: buildSDKTransport(nil)},
		exttransport.RequestClassPlatform,
	)
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !interceptor.preCalled || !interceptor.postCalled {
		t.Fatal("SDK platform request did not execute extension pre/post hooks")
	}
}

func TestBuildSDKTransport_WithoutExtension(t *testing.T) {
	previous := exttransport.GetProvider()
	exttransport.Register(nil)
	t.Cleanup(func() { exttransport.Register(previous) })

	if _, ok := buildSDKTransport(nil).(*internaltransport.HTTPPolicyRouter); !ok {
		t.Fatalf(
			"buildSDKTransport() type = %T, want *transport.HTTPPolicyRouter",
			buildSDKTransport(nil),
		)
	}
}

func TestBuildSDKTransportSupportsPolicyLeafCloning(t *testing.T) {
	previous := exttransport.GetProvider()
	exttransport.Register(nil)
	t.Cleanup(func() { exttransport.Register(previous) })

	base := &http.Transport{}
	client := internaltransport.ClientForRequestClass(
		&http.Client{Transport: buildSDKTransportWithBase(base, nil)},
		exttransport.RequestClassExternal,
	)
	source, ok := client.Transport.(interface {
		CloneHTTPTransport() (http.RoundTripper, *http.Transport, bool)
	})
	if !ok {
		t.Fatalf("SDK request-class transport type = %T, want clone capability", client.Transport)
	}
	rebuilt, concrete, ok := source.CloneHTTPTransport()
	if !ok || rebuilt == nil || concrete == nil {
		t.Fatal("SDK policy graph could not clone its HTTP transport leaf")
	}
	if concrete == base {
		t.Fatal("SDK policy graph reused the original HTTP transport")
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
	transport := internaltransport.WrapWithExtension(base)
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

// buildTamperingInterceptor tries to delete and spoof security headers via
// PreRoundTrip. The SDK built-in chain must restore the real values before the
// request leaves the process.
type buildTamperingInterceptor struct{}

func (buildTamperingInterceptor) PreRoundTrip(req *http.Request) func(*http.Response, error) {
	req.Header.Del(HeaderBuild)
	req.Header.Set(HeaderBuild, "ext-tampered-build")
	req.Header.Del(HeaderSource)
	req.Header.Set(HeaderSource, "ext-tampered-source")
	return nil
}

type riskHeaderTamperingInterceptor struct{}

func (riskHeaderTamperingInterceptor) PreRoundTrip(req *http.Request) func(*http.Response, error) {
	req.Header.Set(riskcontrol.HeaderOSType, "extension-value")
	req.Header.Set(riskcontrol.HeaderProductModel, "extension-value")
	return nil
}

type bootstrapPolicyTamperingInterceptor struct{}

func (bootstrapPolicyTamperingInterceptor) PreRoundTrip(req *http.Request) func(*http.Response, error) {
	req.Header.Set(HeaderSource, "extension-value")
	req.Header.Set(riskcontrol.HeaderOSType, "extension-value")
	return nil
}

func TestNewDefaultInstallsSDKBootstrapSecurityPolicy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	oldTransport := http.DefaultClient.Transport
	oldCheckRedirect := http.DefaultClient.CheckRedirect
	t.Cleanup(func() {
		http.DefaultClient.Transport = oldTransport
		http.DefaultClient.CheckRedirect = oldCheckRedirect
	})

	previous := exttransport.GetProvider()
	exttransport.Register(&platformOnlyStubProvider{
		stubTransportProvider: &stubTransportProvider{
			interceptor: bootstrapPolicyTamperingInterceptor{},
		},
	})
	t.Cleanup(func() { exttransport.Register(previous) })

	var received http.Header
	network := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		received = req.Header.Clone()
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       http.NoBody,
			Request:    req,
		}, nil
	})
	http.DefaultClient.Transport = network
	http.DefaultClient.CheckRedirect = nil
	_ = NewDefault(nil, InvocationContext{})

	req, err := http.NewRequest(
		http.MethodPost,
		"https://open.feishu.cn/callback/ws/endpoint",
		strings.NewReader(`{"app_secret":"secret"}`),
	)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if got := received.Get(HeaderSource); got != SourceValue {
		t.Fatalf("%s = %q, want trusted value %q", HeaderSource, got, SourceValue)
	}
	if got := received.Get(riskcontrol.HeaderOSType); got != "" {
		t.Fatalf("%s = %q, want extension value stripped", riskcontrol.HeaderOSType, got)
	}
	if got := received.Get(HeaderBuild); got != DetectBuildKind() {
		t.Fatalf("%s = %q, want %q", HeaderBuild, got, DetectBuildKind())
	}
	if got := received.Get(HeaderUserAgent); got != UserAgentValue() {
		t.Fatalf("%s = %q, want %q", HeaderUserAgent, got, UserAgentValue())
	}
}

func TestSDKBootstrapRuntimePolicyIsFactoryScoped(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	oldTransport := http.DefaultClient.Transport
	oldCheckRedirect := http.DefaultClient.CheckRedirect
	t.Cleanup(func() {
		http.DefaultClient.Transport = oldTransport
		http.DefaultClient.CheckRedirect = oldCheckRedirect
	})

	var networkCalls atomic.Int32
	http.DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		networkCalls.Add(1)
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       http.NoBody,
			Request:    req,
		}, nil
	})
	http.DefaultClient.CheckRedirect = nil

	earlier := NewDefaultWithRuntimePlan(
		nil,
		InvocationContext{},
		nil,
		runtimeplan.Default(),
		nil,
	)
	laterFailure := errors.New("later managed runtime is unavailable")
	later := NewDefaultWithRuntimePlan(
		nil,
		InvocationContext{},
		nil,
		runtimeplan.Failed(laterFailure, runtimeplan.MetadataEmbeddedOnly),
		nil,
	)

	request := func(ctx context.Context) (*http.Response, error) {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"https://open.feishu.cn/callback/ws/endpoint",
			strings.NewReader(`{"app_secret":"secret"}`),
		)
		if err != nil {
			return nil, err
		}
		return http.DefaultClient.Do(req)
	}

	resp, err := request(earlier.SDKBootstrapContext(context.Background()))
	if err != nil {
		t.Fatalf("earlier Factory bootstrap inherited later plan: %v", err)
	}
	resp.Body.Close()

	if resp, err = request(later.SDKBootstrapContext(context.Background())); !errors.Is(err, laterFailure) {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		t.Fatalf("later Factory bootstrap error = %v, want %v", err, laterFailure)
	}

	// Unscoped dependency traffic uses only the process-wide common policy; a
	// Factory plan must never become its implicit fallback.
	resp, err = request(context.Background())
	if err != nil {
		t.Fatalf("unscoped bootstrap inherited a Factory plan: %v", err)
	}
	resp.Body.Close()
	const concurrentRequests = 16
	var wg sync.WaitGroup
	failures := make(chan error, concurrentRequests*2)
	for i := 0; i < concurrentRequests; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			resp, err := request(earlier.SDKBootstrapContext(context.Background()))
			if err != nil {
				failures <- err
				return
			}
			resp.Body.Close()
		}()
		go func() {
			defer wg.Done()
			resp, err := request(later.SDKBootstrapContext(context.Background()))
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
			if !errors.Is(err, laterFailure) {
				failures <- err
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Errorf("concurrent bootstrap policy mismatch: %v", err)
	}
	if got, want := networkCalls.Load(), int32(2+concurrentRequests); got != want {
		t.Fatalf("network calls = %d, want %d (default-plan requests only)", got, want)
	}
}

func TestBuildSDKTransport_StripsExtensionRiskHeaders(t *testing.T) {
	previous := exttransport.GetProvider()
	exttransport.Register(&stubTransportProvider{interceptor: riskHeaderTamperingInterceptor{}})
	t.Cleanup(func() { exttransport.Register(previous) })

	var received http.Header
	network := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		received = req.Header.Clone()
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	req, err := http.NewRequest(http.MethodGet, "https://open.feishu.cn/open-apis/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer token")

	client := internaltransport.ClientForRequestClass(
		&http.Client{Transport: buildSDKTransportWithBase(network, nil)},
		exttransport.RequestClassPlatform,
	)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if received.Get(riskcontrol.HeaderOSType) != "" || received.Get(riskcontrol.HeaderProductModel) != "" {
		t.Fatalf("extension risk headers reached network: %v", received)
	}
}

// TestBuildHeaderTransport_SDKChain_OverridesTamperedHeader verifies that the
// SDK chain restores both the build classification and the full security
// header set after an extension runs.
func TestBuildHeaderTransport_SDKChain_OverridesTamperedHeader(t *testing.T) {
	var receivedBuild, receivedSource string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBuild = r.Header.Get(HeaderBuild)
		receivedSource = r.Header.Get(HeaderSource)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exttransport.Register(&stubTransportProvider{interceptor: buildTamperingInterceptor{}})
	t.Cleanup(func() { exttransport.Register(nil) })

	// Replicate the SDK built-in chain inside buildSDKTransport.
	var base http.RoundTripper = http.DefaultTransport
	base = &RetryTransport{Base: base}
	base = &UserAgentTransport{Base: base}
	base = &BuildHeaderTransport{Base: base}
	base = &SecurityHeaderTransport{Base: base}
	transport := internaltransport.WrapWithExtension(base)
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if receivedBuild == "ext-tampered-build" {
		t.Fatalf("%s = %q, extension tampering leaked to network", HeaderBuild, receivedBuild)
	}
	want := DetectBuildKind()
	if receivedBuild != want {
		t.Fatalf("%s = %q, want %q", HeaderBuild, receivedBuild, want)
	}
	if receivedSource != SourceValue {
		t.Fatalf("%s = %q, want %q", HeaderSource, receivedSource, SourceValue)
	}
}

// TestBuildHeaderTransport_OverridesEvenWithoutTamper verifies that even if
// no extension is registered, BuildHeaderTransport writes X-Cli-Build.
func TestBuildHeaderTransport_OverridesEvenWithoutTamper(t *testing.T) {
	var receivedBuild string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBuild = r.Header.Get(HeaderBuild)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	transport := &BuildHeaderTransport{Base: http.DefaultTransport}
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if receivedBuild == "" {
		t.Fatalf("%s header missing, BuildHeaderTransport did not inject", HeaderBuild)
	}
	want := DetectBuildKind()
	if receivedBuild != want {
		t.Fatalf("%s = %q, want %q", HeaderBuild, receivedBuild, want)
	}
}

// TestBuildHeaderTransport_NilBase_UsesFallback verifies that when Base is nil,
// the transport still sets X-Cli-Build and routes the request through
// transport.Fallback rather than panicking. This covers the fallback
// branch in RoundTrip that is otherwise unreachable with a non-nil Base.
func TestBuildHeaderTransport_NilBase_UsesFallback(t *testing.T) {
	var receivedBuild string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBuild = r.Header.Get(HeaderBuild)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	transport := &BuildHeaderTransport{Base: nil}
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request via nil-Base transport failed: %v", err)
	}
	resp.Body.Close()

	want := DetectBuildKind()
	if receivedBuild != want {
		t.Fatalf("%s = %q, want %q (header must be set even on nil-Base path)",
			HeaderBuild, receivedBuild, want)
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

	mid := &internaltransport.ExtensionMiddleware{Base: capturer, Ext: tamperIC}

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

		mid := &internaltransport.ExtensionMiddleware{Base: base, Ext: ic, ExtName: "stub"}
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

		mid := &internaltransport.ExtensionMiddleware{Base: base, Ext: ic, ExtName: "stub"}
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

	mid := &internaltransport.ExtensionMiddleware{Base: base, Ext: ic, ExtName: "stub"}
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
