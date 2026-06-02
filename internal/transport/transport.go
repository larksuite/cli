// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"
)

// proxyPluginTransport is a fixed-proxy clone of http.DefaultTransport (with optional
// custom root CA), lazily built on first use when proxy plugin mode is enabled.
var proxyPluginTransport = sync.OnceValue(buildProxyPluginTransport)

// cachedBlockedTransport is a fail-closed transport cached on first use when
// the proxy plugin config exists but is invalid. This avoids cloning
// http.DefaultTransport on every pluginTransport call.
var cachedBlockedTransport = sync.OnceValue(buildBlockedTransport)

func buildBlockedTransport() http.RoundTripper {
	return failClosedTransport(fmt.Errorf("proxy plugin config is invalid: %w", loadErr))
}

func buildProxyPluginTransport() http.RoundTripper {
	def, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		// Cannot clone the stdlib transport. Fail closed with a concrete
		// *http.Transport (not a bare RoundTripper) so downcasting callers such
		// as Fallback cannot silently degrade this into a
		// direct-egress transport.
		return failClosedTransport(fmt.Errorf("proxy plugin transport unavailable: http.DefaultTransport is %T, want *http.Transport", http.DefaultTransport))
	}

	cfg, err := Load()
	if err != nil {
		// Fail closed: config file exists but is malformed/unreadable — do not
		// silently fall back to direct egress.
		return blockedTransport(def, fmt.Errorf("proxy plugin config is invalid: %w", err))
	}
	if cfg == nil || !cfg.Enabled() {
		return def
	}
	t, err := cfg.ApplyToTransport(def)
	if err != nil {
		// Fail closed: do not silently fall back to direct egress when the
		// operator explicitly enabled proxy plugin mode.
		return blockedTransport(def, fmt.Errorf("proxy plugin enabled but config is invalid: %w", err))
	}
	return t
}

// pluginTransport returns the proxy plugin transport when proxy plugin mode is
// configured. The bool return is false when the plugin is not configured or not enabled.
func pluginTransport() (http.RoundTripper, bool) {
	cfg, err := Load()
	if err != nil {
		return cachedBlockedTransport(), true
	}
	if cfg == nil || !cfg.Enabled() {
		return nil, false
	}
	return proxyPluginTransport(), true
}

// failClosedTransport returns a *http.Transport that always fails RoundTrip with
// err. It clones http.DefaultTransport when possible (preserving dial/timeout
// tuning); otherwise it builds a minimal transport. Returning a concrete
// *http.Transport (rather than a bare RoundTripper) is required so downcasting
// callers such as Fallback cannot silently degrade a fail-closed
// signal into a direct-egress transport.
func failClosedTransport(err error) *http.Transport {
	if def, ok := http.DefaultTransport.(*http.Transport); ok {
		return blockedTransport(def, err)
	}
	return &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			return nil, err
		},
	}
}

func blockedTransport(base *http.Transport, err error) *http.Transport {
	blocked := base.Clone()
	blocked.Proxy = func(*http.Request) (*url.URL, error) {
		return nil, err
	}
	return blocked
}
