// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar_demo

package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/larksuite/cli/sidecar"
)

// newForwardClient creates an HTTP client for forwarding requests to the
// Lark API. It strips Authorization on cross-host redirects and disables
// proxy to prevent real tokens from leaking through environment proxies.
func newForwardClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // never proxy the trusted hop
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			if len(via) > 0 && req.URL.Host != via[0].URL.Host {
				req.Header.Del("Authorization")
				req.Header.Del(sidecar.HeaderMCPUAT)
				req.Header.Del(sidecar.HeaderMCPTAT)
			}
			return nil
		},
	}
}

// isProxyHeader returns true for headers specific to the sidecar protocol.
func isProxyHeader(key string) bool {
	switch http.CanonicalHeaderKey(key) {
	case http.CanonicalHeaderKey(sidecar.HeaderProxyTarget),
		http.CanonicalHeaderKey(sidecar.HeaderProxyIdentity),
		http.CanonicalHeaderKey(sidecar.HeaderProxySignature),
		http.CanonicalHeaderKey(sidecar.HeaderProxyTimestamp),
		http.CanonicalHeaderKey(sidecar.HeaderBodySHA256),
		http.CanonicalHeaderKey(sidecar.HeaderProxyAuthHeader):
		return true
	}
	return false
}
