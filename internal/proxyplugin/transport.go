// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package proxyplugin

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
// http.DefaultTransport on every SharedTransport call.
var cachedBlockedTransport = sync.OnceValue(buildBlockedTransport)

func buildBlockedTransport() http.RoundTripper {
	def, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return blockedRoundTripper{err: fmt.Errorf("proxy plugin config is invalid and http.DefaultTransport is %T, want *http.Transport: %w", http.DefaultTransport, loadErr)}
	}
	return blockedTransport(def, fmt.Errorf("proxy plugin config is invalid: %w", loadErr))
}

func buildProxyPluginTransport() http.RoundTripper {
	def, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return blockedRoundTripper{err: fmt.Errorf("proxy plugin transport unavailable: http.DefaultTransport is %T, want *http.Transport", http.DefaultTransport)}
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

// SharedTransport returns the proxy plugin transport when proxy plugin mode is
// configured. The bool return is false when the plugin is not configured or not enabled.
func SharedTransport() (http.RoundTripper, bool) {
	cfg, err := Load()
	if err != nil {
		return cachedBlockedTransport(), true
	}
	if cfg == nil || !cfg.Enabled() {
		return nil, false
	}
	return proxyPluginTransport(), true
}

type blockedRoundTripper struct {
	err error
}

func (b blockedRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, b.err
}

func blockedTransport(base *http.Transport, err error) *http.Transport {
	blocked := base.Clone()
	blocked.Proxy = func(*http.Request) (*url.URL, error) {
		return nil, err
	}
	return blocked
}
