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

// NewBaseTransport creates an *http.Transport cloned from http.DefaultTransport.
// If LARK_CLI_NO_PROXY is set, proxy support is disabled.
// Each call returns a new instance; use FallbackTransport for a shared singleton.
func NewBaseTransport() *http.Transport {
	def, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{}
	}
	t := def.Clone()
	if os.Getenv(EnvNoProxy) != "" {
		t.Proxy = nil
	}
	return t
}

// fallbackTransport is a lazily-initialized singleton used by transport
// decorators when their Base field is nil, preserving connection pooling.
var fallbackTransport = sync.OnceValue(func() *http.Transport {
	return NewBaseTransport()
})

// FallbackTransport returns a shared *http.Transport singleton suitable for
// use as a fallback when a transport decorator's Base is nil.
// Unlike NewBaseTransport (which clones per call), this reuses a single
// instance so that TCP connections and TLS sessions are pooled.
func FallbackTransport() *http.Transport {
	return fallbackTransport()
}
