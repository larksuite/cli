// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	exttransport "github.com/larksuite/cli/extension/transport"
)

type cloneTestDecorator struct {
	base http.RoundTripper
}

func (d *cloneTestDecorator) RoundTrip(req *http.Request) (*http.Response, error) {
	return d.base.RoundTrip(req)
}

func (d *cloneTestDecorator) BaseRoundTripper() http.RoundTripper {
	return d.base
}

func (d *cloneTestDecorator) WithBaseRoundTripper(base http.RoundTripper) http.RoundTripper {
	return &cloneTestDecorator{base: base}
}

type headerCloneTestDecorator struct {
	base http.RoundTripper
}

func (d *headerCloneTestDecorator) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("X-Decorator", "applied")
	return d.base.RoundTrip(req)
}

func (d *headerCloneTestDecorator) BaseRoundTripper() http.RoundTripper {
	return d.base
}

func (d *headerCloneTestDecorator) WithBaseRoundTripper(base http.RoundTripper) http.RoundTripper {
	return &headerCloneTestDecorator{base: base}
}

func TestHTTPPolicyRouterClassifiesFromEndpointCatalog(t *testing.T) {
	exttransport.Register(nil)

	platformCalls := 0
	externalCalls := 0
	router := NewHTTPPolicyRouter(
		roundTripFunc(func(req *http.Request) (*http.Response, error) {
			platformCalls++
			return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
		}),
		roundTripFunc(func(req *http.Request) (*http.Response, error) {
			externalCalls++
			return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
		}),
	)

	for _, rawURL := range []string{
		"https://open.feishu.cn/open-apis/test",
		"https://example.com/file",
	} {
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := router.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	if platformCalls != 1 || externalCalls != 1 {
		t.Fatalf("platform calls = %d, external calls = %d; want 1 each", platformCalls, externalCalls)
	}
}

func TestHTTPPolicyRouterExplicitClassOverridesCatalog(t *testing.T) {
	exttransport.Register(nil)

	platformCalls := 0
	externalCalls := 0
	router := NewHTTPPolicyRouter(
		roundTripFunc(func(req *http.Request) (*http.Response, error) {
			platformCalls++
			return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
		}),
		roundTripFunc(func(req *http.Request) (*http.Response, error) {
			externalCalls++
			return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
		}),
	)

	req, err := http.NewRequest(http.MethodGet, "https://open.feishu.cn/open-apis/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req = WithRequestClass(req, exttransport.RequestClassExternal)
	resp, err := router.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if platformCalls != 0 || externalCalls != 1 {
		t.Fatalf("platform calls = %d, external calls = %d; want 0 and 1", platformCalls, externalCalls)
	}
}

func TestClientForRequestClassOutermostIntentWins(t *testing.T) {
	exttransport.Register(nil)
	platformCalls := 0
	externalCalls := 0
	router := NewHTTPPolicyRouter(
		roundTripFunc(func(req *http.Request) (*http.Response, error) {
			platformCalls++
			return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
		}),
		roundTripFunc(func(req *http.Request) (*http.Response, error) {
			externalCalls++
			return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
		}),
	)

	platform := ClientForRequestClass(&http.Client{Transport: router}, exttransport.RequestClassPlatform)
	external := ClientForRequestClass(platform, exttransport.RequestClassExternal)
	resp, err := external.Get("https://open.feishu.cn/open-apis/test")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if platformCalls != 0 || externalCalls != 1 {
		t.Fatalf("platform calls = %d, external calls = %d; want outer external intent to win", platformCalls, externalCalls)
	}
}

func TestHTTPPolicyRouterRejectsInvalidExplicitClass(t *testing.T) {
	exttransport.Register(nil)
	router := NewHTTPPolicyRouter(nil, nil)
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	req = WithRequestClass(req, exttransport.RequestClass("invalid"))
	if _, err := router.RoundTrip(req); err == nil || !strings.Contains(err.Error(), "unsupported HTTP request class") {
		t.Fatalf("RoundTrip() error = %v, want unsupported request class", err)
	} else if problem, ok := errs.ProblemOf(err); !ok ||
		problem.Category != errs.CategoryInternal ||
		problem.Subtype != errs.SubtypeUnknown {
		t.Fatalf("RoundTrip() problem = %#v, %v; want internal/unknown", problem, ok)
	}
}

func TestHTTPPolicyRouterRejectsNilRequest(t *testing.T) {
	router := NewHTTPPolicyRouter(nil, nil)
	_, err := router.RoundTrip(nil)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeUnknown {
		t.Fatalf("RoundTrip() problem = %#v, %v; want internal/unknown", problem, ok)
	}
}

func TestHTTPPolicyRouterReclassifiesRedirectTargets(t *testing.T) {
	interceptor := &testHeaderInterceptor{}
	exttransport.Register(scopedTestProvider{
		testProvider: testProvider{interceptor: interceptor},
		supported:    exttransport.RequestClassPlatform,
	})
	t.Cleanup(func() { exttransport.Register(nil) })

	receivedHeader := make(chan string, 1)
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		receivedHeader <- req.Header.Get("X-Test-Platform")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(external.Close)

	router := NewHTTPPolicyRouter(
		roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{external.URL}},
				Body:       http.NoBody,
				Request:    req,
			}, nil
		}),
		http.DefaultTransport,
	)
	client := &http.Client{Transport: router}
	req, err := http.NewRequest(http.MethodGet, "https://open.feishu.cn/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if got := <-receivedHeader; got != "" {
		t.Fatalf("redirect target received platform-scoped header %q", got)
	}
	if interceptor.calls != 1 {
		t.Fatalf("extension calls = %d, want only the initial platform request", interceptor.calls)
	}
}

func TestCloneHTTPTransportForRequestClassRebuildsDecorators(t *testing.T) {
	wantErr := errors.New("preserved proxy policy")
	base := &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			return nil, wantErr
		},
	}
	decorated := &cloneTestDecorator{base: base}
	router := NewHTTPPolicyRouter(decorated, decorated)

	rebuilt, concrete, ok := CloneHTTPTransportForRequestClass(router, exttransport.RequestClassExternal)
	if !ok {
		t.Fatal("CloneHTTPTransportForRequestClass() ok = false")
	}
	if concrete == base {
		t.Fatal("CloneHTTPTransportForRequestClass() reused the original *http.Transport")
	}
	if _, ok := rebuilt.(*cloneTestDecorator); !ok {
		t.Fatalf("rebuilt transport type = %T, want *cloneTestDecorator", rebuilt)
	}

	req, err := http.NewRequest(http.MethodGet, "https://external.example/file", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rebuilt.RoundTrip(req); !errors.Is(err, wantErr) {
		t.Fatalf("RoundTrip() error = %v, want %v", err, wantErr)
	}
}

func TestCloneHTTPTransportForRequestClassPreservesAutomaticHTTP2(t *testing.T) {
	previousProvider := exttransport.GetProvider()
	exttransport.Register(nil)
	t.Cleanup(func() { exttransport.Register(previousProvider) })
	source := &http.Transport{
		Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: "proxy.example:8080"}),
	}
	router := NewHTTPPolicyRouter(&http.Transport{}, source)

	_, cloned, ok := CloneHTTPTransportForRequestClass(router, exttransport.RequestClassExternal)
	if !ok {
		t.Fatal("CloneHTTPTransportForRequestClass() ok = false")
	}
	if !cloned.ForceAttemptHTTP2 {
		t.Fatal("ForceAttemptHTTP2 = false, want true")
	}
	if cloned.TLSNextProto != nil {
		t.Fatal("TLSNextProto is non-nil, want automatic HTTP/2")
	}
}

func TestCloneHTTPTransportForRequestClassKeepsOutermostIntent(t *testing.T) {
	platformErr := errors.New("platform transport")
	externalErr := errors.New("external transport")
	newBlocked := func(reason error) *http.Transport {
		return &http.Transport{Proxy: func(*http.Request) (*url.URL, error) { return nil, reason }}
	}
	router := NewHTTPPolicyRouter(newBlocked(platformErr), newBlocked(externalErr))
	platform := ClientForRequestClass(&http.Client{Transport: router}, exttransport.RequestClassPlatform)
	external := ClientForRequestClass(platform, exttransport.RequestClassExternal)

	source, ok := external.Transport.(interface {
		CloneHTTPTransport() (http.RoundTripper, *http.Transport, bool)
	})
	if !ok {
		t.Fatalf("transport type %T has no clone capability", external.Transport)
	}
	rebuilt, _, ok := source.CloneHTTPTransport()
	if !ok {
		t.Fatal("CloneHTTPTransport() ok = false")
	}
	req, err := http.NewRequest(http.MethodGet, "https://open.feishu.cn/open-apis/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rebuilt.RoundTrip(req); !errors.Is(err, externalErr) {
		t.Fatalf("RoundTrip() error = %v, want outer external transport error %v", err, externalErr)
	}
}

func TestClientForRequestClassOverridesCallerIntent(t *testing.T) {
	platformErr := errors.New("platform transport")
	externalErr := errors.New("external transport")
	newBlocked := func(reason error) *http.Transport {
		return &http.Transport{Proxy: func(*http.Request) (*url.URL, error) { return nil, reason }}
	}
	router := NewHTTPPolicyRouter(newBlocked(platformErr), newBlocked(externalErr))
	client := ClientForRequestClass(&http.Client{Transport: router}, exttransport.RequestClassExternal)
	req, err := http.NewRequest(http.MethodGet, "https://external.example/file", nil)
	if err != nil {
		t.Fatal(err)
	}
	req = WithRequestClass(req, exttransport.RequestClassPlatform)

	if _, err := client.Do(req); !errors.Is(err, externalErr) {
		t.Fatalf("Do() error = %v, want forced external transport error %v", err, externalErr)
	}
}

func TestTransformHTTPTransportReplacesLeafInsideDecorators(t *testing.T) {
	exttransport.Register(nil)
	decorated := &headerCloneTestDecorator{base: &http.Transport{}}
	router := NewHTTPPolicyRouter(decorated, decorated)
	client := ClientForRequestClass(&http.Client{Transport: router}, exttransport.RequestClassExternal)

	source, ok := client.Transport.(interface {
		TransformHTTPTransport(func(*http.Transport) (http.RoundTripper, bool)) (http.RoundTripper, bool)
	})
	if !ok {
		t.Fatalf("transport type %T has no transform capability", client.Transport)
	}
	rebuilt, ok := source.TransformHTTPTransport(func(*http.Transport) (http.RoundTripper, bool) {
		return roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if got := req.Header.Get("X-Decorator"); got != "applied" {
				t.Fatalf("leaf received X-Decorator = %q, want applied", got)
			}
			return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Request: req}, nil
		}), true
	})
	if !ok {
		t.Fatal("TransformHTTPTransport() ok = false")
	}

	req, err := http.NewRequest(http.MethodGet, "https://external.example/file", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := rebuilt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}
