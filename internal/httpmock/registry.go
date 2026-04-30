// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package httpmock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// Stub defines a preset HTTP response.
type Stub struct {
	Method      string      // empty = match any method
	URL         string      // substring match on URL
	Status      int         // default 200
	Body        interface{} // auto JSON-serialized
	RawBody     []byte      // raw bytes (takes precedence over Body when non-nil)
	ContentType string      // override Content-Type header (default: application/json)
	Headers     http.Header // optional full response headers (takes precedence over ContentType)
	matched     bool

	// BodyFilter (optional): match only when the captured request body satisfies
	// this predicate. Used to disambiguate multiple stubs that share a URL.
	BodyFilter func([]byte) bool

	// OnMatch (optional): runs synchronously after the stub matches but before
	// the response is composed. Used in tests to inject panics or count
	// in-flight goroutines.
	OnMatch func(req *http.Request)

	// Reusable (optional): when true, the stub stays available for further
	// matches after the first hit. Each match appends to CapturedBodies.
	Reusable bool

	// CapturedHeaders records the request headers of the matched request.
	// Populated after RoundTrip matches this stub.
	CapturedHeaders http.Header
	CapturedBody    []byte
	// CapturedBodies records every captured request body when Reusable is set.
	// (CapturedBody continues to record the most recent capture for back-compat.)
	CapturedBodies [][]byte
}

// Registry records stubs and implements http.RoundTripper.
type Registry struct {
	mu    sync.Mutex
	stubs []*Stub
}

// Register adds a stub to the registry.
func (r *Registry) Register(s *Stub) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.Status == 0 {
		s.Status = 200
	}
	r.stubs = append(r.stubs, s)
}

// RoundTrip implements http.RoundTripper.
func (r *Registry) RoundTrip(req *http.Request) (*http.Response, error) {
	urlStr := req.URL.String()

	// Read body once up-front so BodyFilter can inspect it without consuming
	// the original reader; restore for downstream consumers afterwards.
	// http.RoundTripper requires us to close the original body.
	var capturedBody []byte
	if req.Body != nil {
		var err error
		capturedBody, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("httpmock: read request body: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(capturedBody))
	}

	matched := r.match(req, urlStr, capturedBody)

	if matched != nil {
		// Restore body again in case OnMatch wants to read it.
		req.Body = io.NopCloser(bytes.NewReader(capturedBody))
		if matched.OnMatch != nil {
			matched.OnMatch(req)
		}
		resp, err := stubResponse(matched)
		if err != nil {
			return nil, fmt.Errorf("httpmock: stub %s %s: %w", matched.Method, matched.URL, err)
		}
		return resp, nil
	}
	return nil, fmt.Errorf("httpmock: no stub for %s %s", req.Method, req.URL)
}

// match selects the first stub whose Method/URL/BodyFilter all match the
// request, mutates its capture state, and returns it. defer-Unlock guarantees
// a panicking user-supplied BodyFilter cannot leak the mutex.
func (r *Registry) match(req *http.Request, urlStr string, capturedBody []byte) *Stub {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.stubs {
		if s.matched {
			continue
		}
		if s.Method != "" && s.Method != req.Method {
			continue
		}
		if s.URL != "" && !strings.Contains(urlStr, s.URL) {
			continue
		}
		if s.BodyFilter != nil && !s.BodyFilter(capturedBody) {
			continue
		}
		if !s.Reusable {
			s.matched = true
		}
		s.CapturedHeaders = req.Header.Clone()
		s.CapturedBody = capturedBody
		s.CapturedBodies = append(s.CapturedBodies, capturedBody)
		return s
	}
	return nil
}

// Verify asserts all stubs were matched.
func (r *Registry) Verify(t testing.TB) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.stubs {
		if s.matched {
			continue
		}
		// Reusable stubs never set s.matched; treat any captured hit as a match.
		if s.Reusable && len(s.CapturedBodies) > 0 {
			continue
		}
		t.Errorf("httpmock: unmatched stub: %s %s", s.Method, s.URL)
	}
}

func stubResponse(s *Stub) (*http.Response, error) {
	ct := s.ContentType
	if ct == "" {
		ct = "application/json"
	}

	var body io.ReadCloser
	if s.RawBody != nil {
		body = io.NopCloser(bytes.NewReader(s.RawBody))
	} else {
		switch v := s.Body.(type) {
		case string:
			body = io.NopCloser(strings.NewReader(v))
		case []byte:
			body = io.NopCloser(bytes.NewReader(v))
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("marshal body: %w", err)
			}
			body = io.NopCloser(bytes.NewReader(b))
		}
	}
	return &http.Response{
		StatusCode: s.Status,
		Header: func() http.Header {
			if s.Headers != nil {
				return s.Headers.Clone()
			}
			return http.Header{"Content-Type": []string{ct}}
		}(),
		Body: body,
	}, nil
}

// NewClient returns an http.Client that uses the Registry as its transport.
func NewClient(reg *Registry) *http.Client {
	return &http.Client{Transport: reg}
}
