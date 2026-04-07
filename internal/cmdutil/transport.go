// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"net/http"
	"time"

	exttransport "github.com/larksuite/cli/extension/transport"
	"github.com/larksuite/cli/internal/util"
)

// RetryTransport is an http.RoundTripper that retries on 5xx responses
// and network errors. MaxRetries defaults to 0 (no retries).
type RetryTransport struct {
	Base       http.RoundTripper
	MaxRetries int
	Delay      time.Duration // base delay for exponential backoff; defaults to 500ms
}

func (t *RetryTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return util.FallbackTransport()
}

func (t *RetryTransport) delay() time.Duration {
	if t.Delay > 0 {
		return t.Delay
	}
	return 500 * time.Millisecond
}

// RoundTrip implements http.RoundTripper.
func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base().RoundTrip(req)
	if t.MaxRetries <= 0 {
		return resp, err
	}

	for attempt := 0; attempt < t.MaxRetries; attempt++ {
		if err == nil && resp.StatusCode < 500 {
			return resp, nil
		}
		// Clone request for retry
		cloned := req.Clone(req.Context())
		if req.Body != nil && req.GetBody != nil {
			cloned.Body, _ = req.GetBody()
		}
		delay := t.delay() * (1 << uint(attempt))
		time.Sleep(delay)
		resp, err = t.base().RoundTrip(cloned)
	}
	return resp, err
}

// UserAgentTransport is an http.RoundTripper that sets the User-Agent header.
// Used in the SDK transport chain to override the SDK's default User-Agent.
type UserAgentTransport struct {
	Base http.RoundTripper
}

func (t *UserAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set(HeaderUserAgent, UserAgentValue())
	if t.Base != nil {
		return t.Base.RoundTrip(req)
	}
	return util.FallbackTransport().RoundTrip(req)
}

// SecurityHeaderTransport is an http.RoundTripper that injects CLI security
// headers into every request. Shortcut headers are read from the request context.
type SecurityHeaderTransport struct {
	Base http.RoundTripper
}

func (t *SecurityHeaderTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return util.FallbackTransport()
}

// RoundTrip implements http.RoundTripper.
func (t *SecurityHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	for k, vs := range BaseSecurityHeaders() {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	// Shortcut headers are propagated via context (see section 5.6 of the design doc).
	if name, ok := ShortcutNameFromContext(req.Context()); ok {
		req.Header.Set(HeaderShortcut, name)
	}
	if eid, ok := ExecutionIdFromContext(req.Context()); ok {
		req.Header.Set(HeaderExecutionId, eid)
	}
	return t.base().RoundTrip(req)
}

// extensionMiddleware wraps the built-in transport chain with pre/post hooks.
// The built-in chain always executes and cannot be skipped or overridden.
// The original request context is restored after PreRoundTrip to prevent
// extensions from tampering with cancellation, deadlines, or built-in values.
type extensionMiddleware struct {
	Base http.RoundTripper
	Ext  exttransport.Interceptor
}

// RoundTrip calls PreRoundTrip, restores the original context, executes
// the built-in chain, then calls the post hook if non-nil.
func (m *extensionMiddleware) RoundTrip(req *http.Request) (*http.Response, error) {
	origCtx := req.Context()
	req = req.Clone(origCtx) // isolate caller's request before extension mutations
	post := m.Ext.PreRoundTrip(req)
	req = req.WithContext(origCtx) // restore original context
	resp, err := m.Base.RoundTrip(req)
	if post != nil {
		post(resp, err)
	}
	return resp, err
}

// wrapWithExtension wraps transport with the registered extension middleware.
// If no extension is registered, returns transport unchanged.
func wrapWithExtension(transport http.RoundTripper) http.RoundTripper {
	p := exttransport.GetProvider()
	if p == nil {
		return transport
	}
	tr := p.ResolveInterceptor(context.Background())
	if tr == nil {
		return transport
	}
	return &extensionMiddleware{Base: transport, Ext: tr}
}
