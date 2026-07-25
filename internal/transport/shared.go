// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"net/http"
	"os"
	"sync"
	"time"

	exttransport "github.com/larksuite/cli/extension/transport"
)

// Shared returns the base http.RoundTripper for all CLI HTTP clients.
//
// Precedence (highest first):
//  1. proxy-plugin mode — force traffic through a fixed loopback proxy;
//     FAIL-CLOSED when the plugin config exists but is invalid.
//  2. LARK_CLI_NO_PROXY — direct egress, proxy disabled.
//  3. http.DefaultTransport — the stdlib process-wide singleton (honors
//     HTTP(S)_PROXY), so every client shares one connection pool / TLS cache.
//
// The returned RoundTripper MUST NOT be mutated. Callers that need a customized
// transport should assert to *http.Transport and Clone() it. A shared base is
// required so persistConn read/write goroutines are reused; cloning per call
// leaks them until IdleConnTimeout (~90s) fires.
func Shared() http.RoundTripper {
	// Proxy-plugin mode overrides everything, INCLUDING LARK_CLI_NO_PROXY. When
	// the plugin config exists but is invalid, pluginTransport returns a
	// fail-closed transport with ok=true and we return it here — we MUST NOT
	// fall through to the NO_PROXY / DefaultTransport direct-egress paths below.
	if t, ok := pluginTransport(); ok {
		return t
	}
	if os.Getenv(EnvNoProxy) != "" {
		return noProxyTransport()
	}
	return http.DefaultTransport
}

// Fallback returns a shared *http.Transport. It is a thin wrapper over Shared
// retained so modules already on the leak-free singleton path (internal/auth,
// internal/cmdutil transport decorators) do not have to migrate. New code
// should prefer Shared and treat the base as an http.RoundTripper.
//
// Fail-closed invariant: pluginTransport always expresses its blocked transport
// as a concrete *http.Transport (see failClosedTransport), so the assertion
// below preserves the block. The noProxyTransport() fallback is therefore only
// reached when no proxy plugin is configured and some external code replaced
// http.DefaultTransport with a non-*http.Transport — a case with no fail-closed
// intent, where a proxy-disabled transport is acceptable.
func Fallback() *http.Transport {
	if t, ok := Shared().(*http.Transport); ok {
		return t
	}
	return noProxyTransport()
}

// NewHTTPClient returns a policy-routed client over the shared proxy-aware
// transport. Known platform endpoints use the platform request class; all
// other URLs use the external request class. Existing unscoped transport
// providers continue to apply to both classes.
//
// A zero timeout means no client-level timeout (callers relying on context
// deadlines pass 0).
func NewHTTPClient(timeout time.Duration) *http.Client {
	base := Shared()
	return &http.Client{
		Transport: NewHTTPPolicyRouter(base, base),
		Timeout:   timeout,
	}
}

// NewExternalHTTPClient returns a client for user-provided, pre-signed, CDN,
// package-registry, and other non-platform URLs. It forces the external policy
// while preserving the shared proxy configuration and the historical behavior
// of unscoped transport providers. A zero timeout means no client-level timeout.
func NewExternalHTTPClient(timeout time.Duration) *http.Client {
	return ClientForRequestClass(NewHTTPClient(timeout), exttransport.RequestClassExternal)
}

// noProxyTransport is a proxy-disabled clone of http.DefaultTransport, lazily
// built the first time LARK_CLI_NO_PROXY is observed set.
var noProxyTransport = sync.OnceValue(func() *http.Transport {
	def, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{}
	}
	t := def.Clone()
	t.Proxy = nil
	return t
})
