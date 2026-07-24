// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package riskcontrol

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type countingSource struct {
	calls atomic.Int32
}

func (s *countingSource) Snapshot() Snapshot {
	s.calls.Add(1)
	return Snapshot{OSType: OSTypeMacOS, ProductModel: "Mac16,1"}
}

type staticSource Snapshot

func (s staticSource) Snapshot() Snapshot { return Snapshot(s) }

func TestTransportAuthorizesBeforeCollecting(t *testing.T) {
	tests := []struct {
		name          string
		requestURL    string
		authorization string
		wantSignals   bool
	}{
		{name: "authenticated official HTTPS", requestURL: "https://open.feishu.cn/open-apis/test", authorization: "Bearer token", wantSignals: true},
		{name: "Lark official HTTPS", requestURL: "https://open.larksuite.com/open-apis/test", authorization: "Bearer token", wantSignals: true},
		{name: "official explicit HTTPS port", requestURL: "https://OPEN.FEISHU.CN:443/open-apis/test", authorization: "Bearer token", wantSignals: true},
		{name: "unauthenticated", requestURL: "https://open.feishu.cn/open-apis/test", wantSignals: true},
		{name: "official non-OpenAPI origin", requestURL: "https://accounts.feishu.cn/open-apis/test", authorization: "Bearer token", wantSignals: true},
		{name: "off domain", requestURL: "https://example.com/test", authorization: "Bearer token", wantSignals: false},
		{name: "lookalike", requestURL: "https://open.feishu.cn.evil.example/test", authorization: "Bearer token", wantSignals: false},
		{name: "plain HTTP", requestURL: "http://open.feishu.cn/test", authorization: "Bearer token", wantSignals: false},
		{name: "non-default port", requestURL: "https://open.feishu.cn:8443/test", authorization: "Bearer token", wantSignals: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := &countingSource{}
			var received http.Header
			base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				received = req.Header.Clone()
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
			})
			req, err := http.NewRequest(http.MethodGet, test.requestURL, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", test.authorization)
			req.Header.Set(HeaderOSType, "caller-value")
			req.Header.Set(HeaderProductModel, "caller-value")
			req.Header["x-agent-device-type"] = []string{"non-canonical-caller-value"}

			resp, err := NewTransport(base, source).RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()

			gotSignals := received.Get(HeaderOSType) != ""
			if gotSignals != test.wantSignals {
				t.Fatalf("signals present = %t, want %t; headers=%v", gotSignals, test.wantSignals, received)
			}
			wantCalls := int32(0)
			if test.wantSignals {
				wantCalls = 1
			}
			if got := source.calls.Load(); got != wantCalls {
				t.Fatalf("Snapshot calls = %d, want %d", got, wantCalls)
			}
			if got := req.Header.Get(HeaderOSType); got != "caller-value" {
				t.Fatalf("caller request OS header = %q, want unchanged", got)
			}
			if got := req.Header.Get(HeaderProductModel); got != "caller-value" {
				t.Fatalf("caller request product-model header = %q, want unchanged", got)
			}
			if !test.wantSignals {
				for name := range received {
					if strings.EqualFold(name, HeaderProductModel) || strings.EqualFold(name, HeaderOSType) {
						t.Fatalf("restricted header leaked as %q", name)
					}
				}
			}
		})
	}
}

func TestTransportValidatesSourceSnapshot(t *testing.T) {
	var received http.Header
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		received = req.Header.Clone()
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	req, err := http.NewRequest(http.MethodGet, "https://open.feishu.cn/open-apis/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer token")

	resp, err := NewTransport(base, staticSource{
		OSType:       OSType("unsupported"),
		ProductModel: "unsafe\nvalue",
	}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if received.Get(HeaderOSType) == "" && received.Get(HeaderProductModel) == "" {
		t.Fatalf("no signals collected: %v", received)
	}
}
