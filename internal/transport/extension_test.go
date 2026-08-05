// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/larksuite/cli/errs"
	exttransport "github.com/larksuite/cli/extension/transport"
)

type testProvider struct {
	interceptor  exttransport.Interceptor
	resolveCalls *int
}

func (p testProvider) Name() string { return "test-provider" }

func (p testProvider) ResolveInterceptor(context.Context) exttransport.Interceptor {
	if p.resolveCalls != nil {
		*p.resolveCalls++
	}
	return p.interceptor
}

type scopedTestProvider struct {
	testProvider
	supported exttransport.RequestClass
}

func (p scopedTestProvider) SupportsRequestClass(class exttransport.RequestClass) bool {
	return class == p.supported
}

type testHeaderInterceptor struct {
	calls int
}

func (i *testHeaderInterceptor) PreRoundTrip(req *http.Request) func(*http.Response, error) {
	i.calls++
	req.Header.Set("X-Test-Platform", "routed")
	return nil
}

type abortingTestInterceptor struct {
	reason error
	post   func(*http.Response, error)
}

func (i *abortingTestInterceptor) PreRoundTrip(*http.Request) func(*http.Response, error) {
	panic("PreRoundTrip called for abortable interceptor")
}

func (i *abortingTestInterceptor) PreRoundTripE(*http.Request) (func(*http.Response, error), error) {
	return i.post, i.reason
}

func TestLegacyProviderKeepsAllRequestBehavior(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()
	t.Setenv(EnvNoProxy, "")

	interceptor := &testHeaderInterceptor{}
	previousProvider := exttransport.GetProvider()
	exttransport.Register(testProvider{interceptor: interceptor})
	t.Cleanup(func() { exttransport.Register(previousProvider) })

	received := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		received <- req.Header.Get("X-Test-Platform")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	for _, client := range []*http.Client{
		ClientForRequestClass(NewHTTPClient(0), exttransport.RequestClassPlatform),
		NewExternalHTTPClient(0),
	} {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	if got := <-received; got != "routed" {
		t.Fatalf("platform request header = %q, want routed", got)
	}
	if got := <-received; got != "routed" {
		t.Fatalf("external request header = %q, want routed for legacy provider", got)
	}
	if interceptor.calls != 2 {
		t.Fatalf("extension calls = %d, want exactly 2", interceptor.calls)
	}
}

func TestScopedProviderOnlyRunsForSupportedRequestClass(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()
	t.Setenv(EnvNoProxy, "")

	interceptor := &testHeaderInterceptor{}
	previousProvider := exttransport.GetProvider()
	exttransport.Register(scopedTestProvider{
		testProvider: testProvider{interceptor: interceptor},
		supported:    exttransport.RequestClassPlatform,
	})
	t.Cleanup(func() { exttransport.Register(previousProvider) })

	received := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		received <- req.Header.Get("X-Test-Platform")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	clients := []*http.Client{
		ClientForRequestClass(NewHTTPClient(0), exttransport.RequestClassPlatform),
		NewExternalHTTPClient(0),
	}
	for _, client := range clients {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	if got := <-received; got != "routed" {
		t.Fatalf("platform request header = %q, want routed", got)
	}
	if got := <-received; got != "" {
		t.Fatalf("external request received scoped provider header %q", got)
	}
	if interceptor.calls != 1 {
		t.Fatalf("extension calls = %d, want exactly 1", interceptor.calls)
	}
}

func TestHTTPPolicyRouterResolvesProviderOnce(t *testing.T) {
	resolveCalls := 0
	previousProvider := exttransport.GetProvider()
	exttransport.Register(testProvider{
		interceptor:  &testHeaderInterceptor{},
		resolveCalls: &resolveCalls,
	})
	t.Cleanup(func() { exttransport.Register(previousProvider) })

	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
	})
	_ = NewHTTPPolicyRouter(base, base)

	if resolveCalls != 1 {
		t.Fatalf("ResolveInterceptor() calls = %d, want 1 per router", resolveCalls)
	}
}

func TestSDKBootstrapBridgeBlocksCrossOriginRedirectAfterSameOriginHop(t *testing.T) {
	var externalCalls atomic.Int32
	var relayBody string
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "external.example" {
			externalCalls.Add(1)
			return noContentResponse(req), nil
		}
		switch req.URL.Path {
		case "/bootstrap":
			return redirectResponse(req, http.StatusTemporaryRedirect, "/relay"), nil
		case "/relay":
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			relayBody = string(body)
			return redirectResponse(
				req,
				http.StatusPermanentRedirect,
				"https://external.example/target",
			), nil
		default:
			return noContentResponse(req), nil
		}
	})

	client := &http.Client{Transport: base}
	installSDKTransportBridge(client, func(req *http.Request) bool {
		return req.URL != nil && req.URL.Path == "/bootstrap"
	}, identityTransportPolicy)

	const secret = "app_secret=secret"
	req, err := http.NewRequest(
		http.MethodPost,
		"https://platform.example/bootstrap",
		strings.NewReader(secret),
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "cross-origin redirect") {
		t.Fatalf("Do() error = %v, want cross-origin redirect rejection", err)
	}
	if problem, ok := errs.ProblemOf(err); !ok ||
		problem.Category != errs.CategoryPolicy ||
		problem.Subtype != errs.SubtypeAccessDenied {
		t.Fatalf("Do() problem = %#v, %v; want policy/access_denied", problem, ok)
	}
	if relayBody != secret {
		t.Fatalf("same-origin relay body = %q, want %q", relayBody, secret)
	}
	if got := externalCalls.Load(); got != 0 {
		t.Fatalf("cross-origin target calls = %d, want 0", got)
	}
}

func TestSDKBootstrapRedirectGuardClassifiesInvalidLocation(t *testing.T) {
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return redirectResponse(req, http.StatusFound, "%"), nil
	})
	client := &http.Client{Transport: &sameOriginRedirectTransport{base: base}}
	resp, err := client.Get("https://platform.example/bootstrap")
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "invalid redirect location") {
		t.Fatalf("Do() error = %v, want invalid redirect rejection", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("Do() problem = %#v, %v; want internal/invalid_response", problem, ok)
	}
}

type redirectPolicyInterceptor struct {
	calls int
}

func (i *redirectPolicyInterceptor) PreRoundTrip(req *http.Request) func(*http.Response, error) {
	i.calls++
	req.Header.Set("X-Extension-Hop", strconv.Itoa(i.calls))
	req.Header.Set("X-Reserved", "extension")
	return nil
}

func TestSDKBootstrapBridgeRetainsPoliciesAcrossSameOriginRedirect(t *testing.T) {
	previousProvider := exttransport.GetProvider()
	interceptor := &redirectPolicyInterceptor{}
	exttransport.Register(scopedTestProvider{
		testProvider: testProvider{interceptor: interceptor},
		supported:    exttransport.RequestClassPlatform,
	})
	t.Cleanup(func() { exttransport.Register(previousProvider) })

	var finalHeaders http.Header
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/bootstrap":
			return redirectResponse(req, http.StatusTemporaryRedirect, "/next"), nil
		case "/next":
			finalHeaders = req.Header.Clone()
			return noContentResponse(req), nil
		default:
			return noContentResponse(req), nil
		}
	})

	builtInCalls := 0
	client := &http.Client{Transport: base}
	installSDKTransportBridge(
		client,
		func(req *http.Request) bool {
			return req.URL != nil && req.URL.Path == "/bootstrap"
		},
		func(base http.RoundTripper) http.RoundTripper {
			return roundTripFunc(func(req *http.Request) (*http.Response, error) {
				builtInCalls++
				req = req.Clone(req.Context())
				req.Header.Set("X-Builtin-Hop", strconv.Itoa(builtInCalls))
				req.Header.Set("X-Reserved", "trusted")
				return base.RoundTrip(req)
			})
		},
	)

	req, err := http.NewRequest(
		http.MethodPost,
		"https://platform.example/bootstrap",
		strings.NewReader("body"),
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if finalHeaders == nil {
		t.Fatal("same-origin redirect target was not called")
	}
	if interceptor.calls != 2 {
		t.Fatalf("extension calls = %d, want 2", interceptor.calls)
	}
	if builtInCalls != 2 {
		t.Fatalf("built-in policy calls = %d, want 2", builtInCalls)
	}
	if got := finalHeaders.Get("X-Extension-Hop"); got != "2" {
		t.Fatalf("final X-Extension-Hop = %q, want 2", got)
	}
	if got := finalHeaders.Get("X-Builtin-Hop"); got != "2" {
		t.Fatalf("final X-Builtin-Hop = %q, want 2", got)
	}
	if got := finalHeaders.Get("X-Reserved"); got != "trusted" {
		t.Fatalf("final X-Reserved = %q, want trusted built-in value", got)
	}
}

type redirectRewriteInterceptor struct {
	target       *url.URL
	postLocation string
	calls        int
}

func (i *redirectRewriteInterceptor) PreRoundTrip(req *http.Request) func(*http.Response, error) {
	i.calls++
	req.URL.Scheme = i.target.Scheme
	req.URL.Host = i.target.Host
	if i.postLocation == "" {
		return nil
	}
	return func(resp *http.Response, err error) {
		if err == nil && resp != nil && isFollowedRedirect(resp.StatusCode) {
			resp.Header.Set("Location", i.postLocation)
		}
	}
}

func TestSDKBootstrapRedirectGuardUsesLogicalURLAfterExtensionRewrite(t *testing.T) {
	sidecarURL, err := url.Parse("https://sidecar.example")
	if err != nil {
		t.Fatal(err)
	}
	sidecarCalls := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != sidecarURL.Host {
			t.Fatalf("network host = %q, want extension target %q", req.URL.Host, sidecarURL.Host)
		}
		sidecarCalls++
		switch req.URL.Path {
		case "/bootstrap":
			return redirectResponse(
				req,
				http.StatusTemporaryRedirect,
				"https://platform.example/next",
			), nil
		case "/next":
			return noContentResponse(req), nil
		default:
			return noContentResponse(req), nil
		}
	})

	previousProvider := exttransport.GetProvider()
	interceptor := &redirectRewriteInterceptor{target: sidecarURL}
	exttransport.Register(scopedTestProvider{
		testProvider: testProvider{interceptor: interceptor},
		supported:    exttransport.RequestClassPlatform,
	})
	t.Cleanup(func() { exttransport.Register(previousProvider) })

	client := &http.Client{Transport: base}
	installSDKTransportBridge(client, func(req *http.Request) bool {
		return req.URL != nil && req.URL.Path == "/bootstrap"
	}, identityTransportPolicy)

	req, err := http.NewRequest(
		http.MethodPost,
		"https://platform.example/bootstrap",
		strings.NewReader("body"),
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if sidecarCalls != 2 {
		t.Fatalf("sidecar calls = %d, want 2", sidecarCalls)
	}
	if interceptor.calls != 2 {
		t.Fatalf("extension calls = %d, want 2", interceptor.calls)
	}
}

func TestSDKBootstrapRedirectGuardChecksLocationAfterExtensionPostHook(t *testing.T) {
	var externalCalls atomic.Int32
	sidecarURL, err := url.Parse("https://sidecar.example")
	if err != nil {
		t.Fatal(err)
	}
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "external.example" {
			externalCalls.Add(1)
			return noContentResponse(req), nil
		}
		return redirectResponse(
			req,
			http.StatusTemporaryRedirect,
			"https://platform.example/next",
		), nil
	})

	previousProvider := exttransport.GetProvider()
	exttransport.Register(scopedTestProvider{
		testProvider: testProvider{interceptor: &redirectRewriteInterceptor{
			target:       sidecarURL,
			postLocation: "https://external.example/target",
		}},
		supported: exttransport.RequestClassPlatform,
	})
	t.Cleanup(func() { exttransport.Register(previousProvider) })

	client := &http.Client{Transport: base}
	installSDKTransportBridge(client, func(req *http.Request) bool {
		return req.URL != nil && req.URL.Path == "/bootstrap"
	}, identityTransportPolicy)
	req, err := http.NewRequest(
		http.MethodPost,
		"https://platform.example/bootstrap",
		strings.NewReader("secret"),
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "cross-origin redirect") {
		t.Fatalf("Do() error = %v, want post-hook Location rejection", err)
	}
	if problem, ok := errs.ProblemOf(err); !ok ||
		problem.Category != errs.CategoryPolicy ||
		problem.Subtype != errs.SubtypeAccessDenied {
		t.Fatalf("Do() problem = %#v, %v; want policy/access_denied", problem, ok)
	}
	if got := externalCalls.Load(); got != 0 {
		t.Fatalf("post-hook redirect target calls = %d, want 0", got)
	}
}

func TestSameOriginNormalizesDefaultPort(t *testing.T) {
	left, err := url.Parse("https://platform.example/bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	right, err := url.Parse("https://platform.example:443/next")
	if err != nil {
		t.Fatal(err)
	}
	if !sameOrigin(left, right) {
		t.Fatal("sameOrigin() = false for equivalent default HTTPS ports")
	}
}

func TestDefaultClientBridgeCoversWebSocketSDKBootstrap(t *testing.T) {
	preserveHTTPClientState(t, sdkBootstrapHTTPClient)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()
	t.Setenv(EnvNoProxy, "1")

	previousProvider := exttransport.GetProvider()
	interceptor := &testHeaderInterceptor{}
	exttransport.Register(scopedTestProvider{
		testProvider: testProvider{interceptor: interceptor},
		supported:    exttransport.RequestClassPlatform,
	})
	t.Cleanup(func() { exttransport.Register(previousProvider) })

	seenHeader := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		seenHeader <- req.Header.Get("X-Test-Platform")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":400,"msg":"stop after bootstrap"}`)
	}))
	t.Cleanup(server.Close)

	installSDKTransportBridge(sdkBootstrapHTTPClient, func(req *http.Request) bool {
		return req.URL != nil && req.URL.Host == strings.TrimPrefix(server.URL, "http://")
	}, identityTransportPolicy)

	client := larkws.NewClient(
		"test-app",
		"test-secret",
		larkws.WithDomain(server.URL),
		larkws.WithAutoReconnect(false),
	)
	if err := client.Start(context.Background()); err == nil {
		t.Fatal("WebSocket SDK Start() error = nil, want bootstrap failure")
	}
	if got := <-seenHeader; got != "routed" {
		t.Fatalf("WebSocket bootstrap header = %q, want routed", got)
	}
	if interceptor.calls != 1 {
		t.Fatalf("extension calls = %d, want exactly 1 bootstrap call", interceptor.calls)
	}
}

func TestSDKTransportBridgeUsesPinnedClientAfterGlobalReplacement(t *testing.T) {
	preserveHTTPClientState(t, sdkBootstrapHTTPClient)
	oldDefaultClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = oldDefaultClient })

	var pinnedCalls atomic.Int32
	pinnedHeader := make(chan string, 1)
	sdkBootstrapHTTPClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		pinnedCalls.Add(1)
		pinnedHeader <- req.Header.Get("X-Pinned-Bridge")
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"code":400,"msg":"stop"}`)),
			Request:    req,
		}, nil
	})
	sdkBootstrapHTTPClient.CheckRedirect = nil

	var replacementCalls atomic.Int32
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		replacementCalls.Add(1)
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       http.NoBody,
			Request:    req,
		}, nil
	})}

	InstallSDKTransportBridge(func(base http.RoundTripper) http.RoundTripper {
		return roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req = req.Clone(req.Context())
			req.Header.Set("X-Pinned-Bridge", "routed")
			return base.RoundTrip(req)
		})
	})

	client := larkws.NewClient(
		"test-app",
		"test-secret",
		larkws.WithAutoReconnect(false),
	)
	if err := client.Start(context.Background()); err == nil {
		t.Fatal("WebSocket SDK Start() error = nil, want bootstrap failure")
	}
	if got := pinnedCalls.Load(); got != 1 {
		t.Fatalf("SDK-pinned client calls = %d, want 1", got)
	}
	if got := <-pinnedHeader; got != "routed" {
		t.Fatalf("SDK-pinned bridge header = %q, want routed", got)
	}
	if got := replacementCalls.Load(); got != 0 {
		t.Fatalf("replacement DefaultClient calls = %d, want 0", got)
	}
}

func TestSDKWebSocketBootstrapMatcherIsNarrow(t *testing.T) {
	tests := []struct {
		name   string
		method string
		url    string
		want   bool
	}{
		{
			name:   "platform bootstrap",
			method: http.MethodPost,
			url:    "https://open.feishu.cn/callback/ws/endpoint",
			want:   true,
		},
		{
			name:   "other platform path",
			method: http.MethodPost,
			url:    "https://open.feishu.cn/open-apis/test",
		},
		{
			name:   "wrong bootstrap method",
			method: http.MethodGet,
			url:    "https://open.feishu.cn/callback/ws/endpoint",
		},
		{
			name:   "external lookalike",
			method: http.MethodPost,
			url:    "https://external.example/callback/ws/endpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.url, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := isSDKWebSocketBootstrapRequest(req); got != tt.want {
				t.Fatalf("isSDKWebSocketBootstrapRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSDKTransportBridgeLeavesOtherPlatformPathsUntouched(t *testing.T) {
	previousProvider := exttransport.GetProvider()
	interceptor := &testHeaderInterceptor{}
	exttransport.Register(scopedTestProvider{
		testProvider: testProvider{interceptor: interceptor},
		supported:    exttransport.RequestClassPlatform,
	})
	t.Cleanup(func() { exttransport.Register(previousProvider) })

	baseCalls := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		baseCalls++
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
	})}
	installSDKTransportBridge(client, isSDKWebSocketBootstrapRequest, nil)

	req, err := http.NewRequest(http.MethodGet, "https://open.feishu.cn/open-apis/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if baseCalls != 1 {
		t.Fatalf("base calls = %d, want 1", baseCalls)
	}
	if interceptor.calls != 0 {
		t.Fatalf("extension calls = %d, want 0 for unmatched DefaultClient traffic", interceptor.calls)
	}
}

func TestSDKTransportBridgeNilBasePreservesDefaultTransportForUnmatchedRequest(t *testing.T) {
	oldDefaultTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = oldDefaultTransport })

	unsetProxyPluginEnv(t)
	resetProxyPluginState()
	t.Setenv(EnvNoProxy, "1")

	var firstCalls atomic.Int32
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		firstCalls.Add(1)
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
	})
	client := &http.Client{}
	installSDKTransportBridge(client, func(*http.Request) bool { return false }, nil)

	var currentCalls atomic.Int32
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		currentCalls.Add(1)
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
	})

	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:1/unmatched", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if got := firstCalls.Load(); got != 0 {
		t.Fatalf("install-time DefaultTransport calls = %d, want 0", got)
	}
	if got := currentCalls.Load(); got != 1 {
		t.Fatalf("request-time DefaultTransport calls = %d, want 1", got)
	}
}

func TestSDKTransportBridgeUpdatesPlatformPolicy(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return noContentResponse(req), nil
	})}
	var firstCalls, secondCalls int
	build := func(calls *int) transportPolicyBuilder {
		return func(base http.RoundTripper) http.RoundTripper {
			*calls++
			return base
		}
	}
	match := func(*http.Request) bool { return true }
	installSDKTransportBridge(client, match, build(&firstCalls))
	installSDKTransportBridge(client, match, build(&secondCalls))
	req, err := http.NewRequest(http.MethodPost, "https://platform.example/bootstrap", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if firstCalls != 0 || secondCalls != 1 {
		t.Fatalf("policy calls = (%d, %d), want (0, 1)", firstCalls, secondCalls)
	}
}

func TestSDKBootstrapTransportFailsClosedWithoutPlatformPolicy(t *testing.T) {
	var baseCalls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		baseCalls.Add(1)
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       http.NoBody,
			Request:    req,
		}, nil
	})}
	installSDKTransportBridge(client, func(*http.Request) bool { return true }, nil)

	req, err := http.NewRequest(http.MethodPost, "https://platform.example/bootstrap", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "policy is not configured") {
		t.Fatalf("Do() error = %v, want missing policy rejection", err)
	}
	if problem, ok := errs.ProblemOf(err); !ok ||
		problem.Category != errs.CategoryInternal ||
		problem.Subtype != errs.SubtypeUnknown {
		t.Fatalf("Do() problem = %#v, %v; want internal/unknown", problem, ok)
	}
	if got := baseCalls.Load(); got != 0 {
		t.Fatalf("base transport calls = %d, want 0", got)
	}
}

func TestSDKBootstrapTransportFailsClosedForNilPlatformTransport(t *testing.T) {
	var baseCalls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		baseCalls.Add(1)
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       http.NoBody,
			Request:    req,
		}, nil
	})}
	installSDKTransportBridge(client, func(*http.Request) bool { return true }, func(http.RoundTripper) http.RoundTripper {
		return nil
	})

	req, err := http.NewRequest(http.MethodPost, "https://platform.example/bootstrap", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "nil transport") {
		t.Fatalf("Do() error = %v, want nil policy transport rejection", err)
	}
	if problem, ok := errs.ProblemOf(err); !ok ||
		problem.Category != errs.CategoryInternal ||
		problem.Subtype != errs.SubtypeUnknown {
		t.Fatalf("Do() problem = %#v, %v; want internal/unknown", problem, ok)
	}
	if got := baseCalls.Load(); got != 0 {
		t.Fatalf("base transport calls = %d, want 0", got)
	}
}

func TestSDKBootstrapRedirectPolicyRetainsDefaultLimit(t *testing.T) {
	policy := sdkBootstrapRedirectPolicy(nil, nil)
	via := make([]*http.Request, 10)
	err := policy(&http.Request{}, via)
	if err == nil {
		t.Fatal("redirect policy error = nil after 10 redirects")
	}
	if problem, ok := errs.ProblemOf(err); !ok ||
		problem.Category != errs.CategoryNetwork ||
		problem.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("redirect problem = %#v, %v; want network/transport", problem, ok)
	}
}

func TestExtensionMiddlewareUsesFallbackWhenBaseIsNil(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()
	t.Setenv(EnvNoProxy, "")

	previous := http.DefaultTransport
	var calls atomic.Int32
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       http.NoBody,
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previous })

	req, err := http.NewRequest(http.MethodGet, "https://external.example/file", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&ExtensionMiddleware{Ext: &testHeaderInterceptor{}}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := calls.Load(); got != 1 {
		t.Fatalf("fallback transport calls = %d, want 1", got)
	}
}

func TestExtensionMiddlewareAbortsBeforeBase(t *testing.T) {
	reason := errors.New("blocked")
	baseCalled := false
	postCalled := false
	interceptor := &abortingTestInterceptor{
		reason: reason,
		post: func(resp *http.Response, err error) {
			postCalled = true
			if resp != nil || err != reason {
				t.Errorf("post arguments = (%v, %v), want (nil, reason)", resp, err)
			}
		},
	}
	middleware := &ExtensionMiddleware{
		Base: roundTripFunc(func(*http.Request) (*http.Response, error) {
			baseCalled = true
			return nil, nil
		}),
		Ext:     interceptor,
		ExtName: "test-provider",
	}

	resp, err := middleware.RoundTrip(httptest.NewRequest(http.MethodGet, "https://example.com", nil))
	if resp != nil {
		t.Fatalf("response = %v, want nil", resp)
	}
	var abortErr *exttransport.AbortError
	if !errors.As(err, &abortErr) {
		t.Fatalf("error = %T, want *transport.AbortError", err)
	}
	if abortErr.Extension != "test-provider" || abortErr.Reason != reason {
		t.Fatalf("abort error = %#v, want provider and reason", abortErr)
	}
	if baseCalled {
		t.Fatal("base transport was called")
	}
	if !postCalled {
		t.Fatal("post hook was not called")
	}
}

func preserveHTTPClientState(t *testing.T, client *http.Client) {
	t.Helper()
	oldTransport := client.Transport
	oldCheckRedirect := client.CheckRedirect
	t.Cleanup(func() {
		client.Transport = oldTransport
		client.CheckRedirect = oldCheckRedirect
	})
}

func identityTransportPolicy(base http.RoundTripper) http.RoundTripper {
	return base
}

func redirectResponse(req *http.Request, status int, location string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Location": []string{location}},
		Body:       http.NoBody,
		Request:    req,
	}
}

func noContentResponse(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       http.NoBody,
		Request:    req,
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
