// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"net/http"
	"time"

	"github.com/larksuite/cli/internal/transport"
)

var (
	_ transport.RoundTripperDecorator = (*RetryTransport)(nil)
	_ transport.RoundTripperDecorator = (*UserAgentTransport)(nil)
	_ transport.RoundTripperDecorator = (*BuildHeaderTransport)(nil)
	_ transport.RoundTripperDecorator = (*SecurityHeaderTransport)(nil)
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
	return transport.Fallback()
}

func (t *RetryTransport) BaseRoundTripper() http.RoundTripper {
	return t.base()
}

func (t *RetryTransport) WithBaseRoundTripper(base http.RoundTripper) http.RoundTripper {
	cloned := *t
	cloned.Base = base
	return &cloned
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

func (t *UserAgentTransport) BaseRoundTripper() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return transport.Fallback()
}

func (t *UserAgentTransport) WithBaseRoundTripper(base http.RoundTripper) http.RoundTripper {
	cloned := *t
	cloned.Base = base
	return &cloned
}

func (t *UserAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set(HeaderUserAgent, UserAgentValue())
	if t.Base != nil {
		return t.Base.RoundTrip(req)
	}
	return transport.Fallback().RoundTrip(req)
}

// BuildHeaderTransport is an http.RoundTripper that force-writes the
// X-Cli-Build header before every request. It remains in the SDK transport
// chain as a narrow defense-in-depth layer alongside SecurityHeaderTransport.
type BuildHeaderTransport struct {
	Base http.RoundTripper
}

func (t *BuildHeaderTransport) BaseRoundTripper() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return transport.Fallback()
}

func (t *BuildHeaderTransport) WithBaseRoundTripper(base http.RoundTripper) http.RoundTripper {
	cloned := *t
	cloned.Base = base
	return &cloned
}

func (t *BuildHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set(HeaderBuild, DetectBuildKind())
	if t.Base != nil {
		return t.Base.RoundTrip(req)
	}
	return transport.Fallback().RoundTrip(req)
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
	return transport.Fallback()
}

func (t *SecurityHeaderTransport) BaseRoundTripper() http.RoundTripper {
	return t.base()
}

func (t *SecurityHeaderTransport) WithBaseRoundTripper(base http.RoundTripper) http.RoundTripper {
	cloned := *t
	cloned.Base = base
	return &cloned
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
