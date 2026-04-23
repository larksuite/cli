// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package util

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

const (
	// EnvNoProxy disables automatic proxy support when set to any non-empty value.
	EnvNoProxy = "LARK_CLI_NO_PROXY"
)

// proxyEnvKeys lists environment variables that Go's ProxyFromEnvironment reads.
var proxyEnvKeys = []string{
	"HTTPS_PROXY", "https_proxy",
	"HTTP_PROXY", "http_proxy",
	"ALL_PROXY", "all_proxy",
}

// DetectProxyEnv returns the first proxy-related environment variable that is set,
// or empty strings if none are configured.
func DetectProxyEnv() (key, value string) {
	for _, k := range proxyEnvKeys {
		if v := os.Getenv(k); v != "" {
			return k, v
		}
	}
	return "", ""
}

var proxyWarningOnce sync.Once

// redactProxyURL masks userinfo (username:password) in a proxy URL.
// Handles both scheme-prefixed ("http://user:pass@host") and bare ("user:pass@host") formats.
func redactProxyURL(raw string) string {
	// Try standard url.Parse first (works when scheme is present)
	u, err := url.Parse(raw)
	if err == nil && u.User != nil {
		return u.Scheme + "://***@" + u.Host + u.RequestURI()
	}

	// Fallback: handle bare URLs without scheme (e.g. "user:pass@proxy:8080")
	if at := strings.LastIndex(raw, "@"); at > 0 {
		return "***@" + raw[at+1:]
	}

	return raw
}

// WarnIfProxied prints a one-time warning to w when a proxy environment variable
// is detected and proxy is not disabled via LARK_CLI_NO_PROXY. Proxy credentials
// are redacted. Safe to call multiple times; only the first call prints.
func WarnIfProxied(w io.Writer) {
	proxyWarningOnce.Do(func() {
		if os.Getenv(EnvNoProxy) != "" {
			return
		}
		key, val := DetectProxyEnv()
		if key == "" {
			return
		}
		fmt.Fprintf(w, "[lark-cli] [WARN] proxy detected: %s=%s — requests (including credentials) will transit through this proxy. Set %s=1 to disable proxy.\n",
			key, redactProxyURL(val), EnvNoProxy)
	})
}

// noProxyTransport is a proxy-disabled clone of http.DefaultTransport,
// lazily built the first time LARK_CLI_NO_PROXY is observed set.
var noProxyTransport = sync.OnceValue(func() *http.Transport {
	def, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{}
	}
	t := def.Clone()
	t.Proxy = nil
	return t
})

// SharedTransport returns the base http.RoundTripper for CLI HTTP clients.
//
// By default it returns http.DefaultTransport — the stdlib-provided
// process-wide singleton — so every HTTP client in the process shares one
// TCP connection pool, TLS session cache, and HTTP/2 state. When
// LARK_CLI_NO_PROXY is set it returns a separate proxy-disabled singleton
// clone; LARK_CLI_NO_PROXY is checked on every call, but the clone is built
// at most once.
//
// The returned RoundTripper MUST NOT be mutated. Callers that need a
// customized transport should assert to *http.Transport and Clone() it.
// Using a shared base is required so persistConn readLoop/writeLoop
// goroutines are reused; cloning per call leaks them until IdleConnTimeout
// (~90s) fires.
func SharedTransport() http.RoundTripper {
	if os.Getenv(EnvNoProxy) != "" {
		return noProxyTransport()
	}
	return http.DefaultTransport
}

// FallbackTransport returns a shared *http.Transport singleton. It is a
// thin wrapper over SharedTransport retained so modules that were already
// on the leak-free singleton path (internal/auth, internal/cmdutil
// transport decorators) do not have to migrate. New code should prefer
// SharedTransport and treat the base as an http.RoundTripper.
func FallbackTransport() *http.Transport {
	if t, ok := SharedTransport().(*http.Transport); ok {
		return t
	}
	return noProxyTransport()
}
