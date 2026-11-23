// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package validate_test

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	exttransport "github.com/larksuite/cli/extension/transport"
	internaltransport "github.com/larksuite/cli/internal/transport"
	"github.com/larksuite/cli/internal/validate"
)

type opaqueRoundTripper struct {
	called bool
}

func (t *opaqueRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	t.called = true
	return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
}

type downloadTestProvider struct {
	interceptor exttransport.Interceptor
}

func (p downloadTestProvider) Name() string { return "download-test" }

func (p downloadTestProvider) ResolveInterceptor(context.Context) exttransport.Interceptor {
	return p.interceptor
}

type downloadHeaderInterceptor struct{}

func (downloadHeaderInterceptor) PreRoundTrip(req *http.Request) func(*http.Response, error) {
	req.Header.Set("X-Use-Proxy", "1")
	return nil
}

func TestNewDownloadHTTPClientPreservesPolicyRouterBaseTransport(t *testing.T) {
	wantErr := errors.New("proxy policy blocked request")
	base := &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			return nil, wantErr
		},
	}
	router := internaltransport.NewHTTPPolicyRouter(base, base)
	client := internaltransport.ClientForRequestClass(
		&http.Client{Transport: router},
		exttransport.RequestClassExternal,
	)

	download := validate.NewDownloadHTTPClient(client, validate.DownloadHTTPClientOptions{AllowHTTP: true})
	req, err := http.NewRequest(http.MethodGet, "https://external.example/file", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := download.Transport.RoundTrip(req)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("RoundTrip() error = %v, want preserved proxy error %v", err, wantErr)
	}
}

func TestNewDownloadHTTPClientRejectsInitialHTTPBeforeTransport(t *testing.T) {
	base := &opaqueRoundTripper{}
	download := validate.NewDownloadHTTPClient(
		&http.Client{Transport: base},
		validate.DownloadHTTPClientOptions{},
	)
	req, err := http.NewRequest(http.MethodGet, "http://203.0.113.10/file", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = download.Transport.RoundTrip(req)
	if err == nil {
		t.Fatal("RoundTrip() error = nil, want initial HTTP rejection")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryPolicy || problem.Subtype != errs.SubtypeAccessDenied {
		t.Fatalf("RoundTrip() problem = %#v, %v; want policy/access_denied", problem, ok)
	}
	if base.called {
		t.Fatal("base transport was called for a disallowed initial HTTP request")
	}
}

func TestNewDownloadHTTPClientAllowsSelectedLoopbackProxy(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Host != "203.0.113.10" {
			t.Errorf("proxy request target = %q, want 203.0.113.10", req.URL.Host)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(proxy.Close)
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	base := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	router := internaltransport.NewHTTPPolicyRouter(base, base)
	client := internaltransport.ClientForRequestClass(
		&http.Client{Transport: router},
		exttransport.RequestClassExternal,
	)
	download := validate.NewDownloadHTTPClient(client, validate.DownloadHTTPClientOptions{AllowHTTP: true})

	req, err := http.NewRequest(http.MethodGet, "http://203.0.113.10/file", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := download.Do(req)
	if err != nil {
		t.Fatalf("download through selected loopback proxy: %v", err)
	}
	resp.Body.Close()
}

func TestNewDownloadHTTPClientSelectsProxyAfterOuterDecorators(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := req.Header.Get("X-Use-Proxy"); got != "1" {
			t.Errorf("proxy received X-Use-Proxy = %q, want 1", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(proxy.Close)
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("proxy selector ran before decorators")
	base := &http.Transport{Proxy: func(req *http.Request) (*url.URL, error) {
		if req.Header.Get("X-Use-Proxy") != "1" {
			return nil, wantErr
		}
		return proxyURL, nil
	}}
	previousProvider := exttransport.GetProvider()
	exttransport.Register(downloadTestProvider{interceptor: downloadHeaderInterceptor{}})
	t.Cleanup(func() { exttransport.Register(previousProvider) })
	router := internaltransport.NewHTTPPolicyRouter(base, base)
	client := internaltransport.ClientForRequestClass(
		&http.Client{Transport: router},
		exttransport.RequestClassExternal,
	)
	download := validate.NewDownloadHTTPClient(client, validate.DownloadHTTPClientOptions{AllowHTTP: true})

	req, err := http.NewRequest(http.MethodGet, "http://203.0.113.10/file", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := download.Do(req)
	if err != nil {
		t.Fatalf("download through decorator-selected proxy: %v", err)
	}
	resp.Body.Close()
}

func TestNewDownloadHTTPClientGuardsLegacyDialTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	base := &http.Transport{DialTLS: func(_, _ string) (net.Conn, error) {
		return tls.Dial("tcp", server.Listener.Addr().String(), &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // local TLS server verifies the connection guard.
	}}
	download := validate.NewDownloadHTTPClient(&http.Client{Transport: base}, validate.DownloadHTTPClientOptions{AllowHTTP: true})
	req, err := http.NewRequest(http.MethodGet, "https://public.example/file", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = download.Transport.RoundTrip(req)
	if err == nil || !strings.Contains(err.Error(), "local/internal host is not allowed") {
		t.Fatalf("RoundTrip() error = %v, want legacy DialTLS IP guard", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryPolicy || problem.Subtype != errs.SubtypeAccessDenied {
		t.Fatalf("RoundTrip() problem = %#v, %v; want policy/access_denied", problem, ok)
	}
	var policyErr *errs.SecurityPolicyError
	if !errors.As(err, &policyErr) || policyErr.Cause == nil {
		t.Fatalf("RoundTrip() error = %T, want policy error with cause", err)
	}
}

func TestNewDownloadHTTPClientFailsClosedForOpaqueTransport(t *testing.T) {
	opaque := &opaqueRoundTripper{}
	client := internaltransport.ClientForRequestClass(
		&http.Client{Transport: opaque},
		exttransport.RequestClassExternal,
	)
	download := validate.NewDownloadHTTPClient(client, validate.DownloadHTTPClientOptions{AllowHTTP: true})
	req, err := http.NewRequest(http.MethodGet, "https://public.example/file", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = download.Transport.RoundTrip(req)
	if err == nil || !strings.Contains(err.Error(), "cannot safely clone download transport") {
		t.Fatalf("RoundTrip() error = %v, want fail-closed clone error", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeUnknown {
		t.Fatalf("RoundTrip() problem = %#v, %v; want internal/unknown", problem, ok)
	}
	if opaque.called {
		t.Fatal("opaque transport was called after safe cloning failed")
	}
}
