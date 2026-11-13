// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
)

// tokenEndpointServer serves the Token Endpoint over a real HTTP connection so
// the client's WroteRequest trace and transport errors are the production ones.
type tokenEndpointServer struct {
	mu            sync.Mutex
	refreshTokens []string
}

// record stores the refresh_token carried by one Token Endpoint request.
func (s *tokenEndpointServer) record(t *testing.T, r *http.Request) {
	t.Helper()
	var payload refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		t.Errorf("decode refresh request: %v", err)
	}
	s.mu.Lock()
	s.refreshTokens = append(s.refreshTokens, payload.RefreshToken)
	s.mu.Unlock()
}

// requests returns the refresh_token of each request received so far, in order.
func (s *tokenEndpointServer) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.refreshTokens...)
}

// clientFor routes requests for the documented Token Endpoint to srv.
func clientFor(srv *httptest.Server, timeout time.Duration) *http.Client {
	base := &http.Transport{Proxy: nil}
	return &http.Client{
		Timeout: timeout,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			routed := req.Clone(req.Context())
			routed.URL.Scheme = "http"
			routed.URL.Host = srv.Listener.Addr().String()
			routed.Host = ""
			return base.RoundTrip(routed)
		}),
	}
}

// requireOutcomeUnknownPreserved asserts a retryable outcome-unknown network
// error with recovery guidance, and that the stored token is still the original.
func requireOutcomeUnknownPreserved(t *testing.T, err error, stored *StoredUAToken, subtype errs.Subtype) {
	t.Helper()
	problem := requireRefreshProblem(t, err, errs.CategoryNetwork, subtype, true)
	if IsNeedUserAuthorizationError(err) {
		t.Fatalf("error = %v, want outcome-unknown error, not need-user-authorization", err)
	}
	if !strings.Contains(problem.Message, "token refresh outcome unknown") {
		t.Fatalf("message = %q, want outcome-unknown message", problem.Message)
	}
	if !strings.Contains(problem.Hint, "did not replay it and kept the stored token") {
		t.Fatalf("hint = %q, want outcome-unknown recovery", problem.Hint)
	}
	current := mustGetStoredToken(t, stored.AppId, stored.UserOpenId)
	if current == nil || current.RefreshToken != stored.RefreshToken {
		t.Fatalf("stored token = %#v, want original generation preserved", current)
	}
}

// A client timeout after the request was written must not replay the token.
func TestRefreshClientTimeoutAfterWriteDoesNotReplayRefreshToken(t *testing.T) {
	setupStoredTokenTest(t)
	stored := newRefreshTestToken()
	if err := SetStoredToken(stored); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}
	endpoint := &tokenEndpointServer{}
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpoint.record(t, r)
		// The server has the request but the answer never reaches the client,
		// as when the host is paused past the client timeout.
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	_, err := GetValidAccessToken(context.Background(), clientFor(srv, 200*time.Millisecond), newRefreshTestOptions(stored))

	requireOutcomeUnknownPreserved(t, err, stored, errs.SubtypeNetworkTimeout)
	if got := endpoint.requests(); len(got) != 1 {
		t.Fatalf("token endpoint requests = %d, want 1 (no replay of %q)", len(got), stored.RefreshToken)
	}
}

// Reproduces #2830: EOF after the write keeps the token without a replay, and
// only the next invocation's definitive 20064 answer clears it.
func TestRefreshEOFAfterWriteKeepsTokenUntilServerAnswers(t *testing.T) {
	setupStoredTokenTest(t)
	stored := newRefreshTestToken()
	if err := SetStoredToken(stored); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}
	endpoint := &tokenEndpointServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpoint.record(t, r)
		if len(endpoint.requests()) == 1 {
			// Rotate the token server-side, then drop the connection so the
			// client sees EOF after WroteRequest.
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			_ = conn.Close()
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":20064,"error":"invalid_grant","error_description":"The refresh token has been revoked. Please note that a refresh token can only be used once."}`))
	}))
	t.Cleanup(srv.Close)
	client := clientFor(srv, 5*time.Second)
	opts := newRefreshTestOptions(stored)

	_, err := GetValidAccessToken(context.Background(), client, opts)
	requireOutcomeUnknownPreserved(t, err, stored, errs.SubtypeNetworkTransport)
	if got := endpoint.requests(); len(got) != 1 {
		t.Fatalf("token endpoint requests after EOF = %d, want 1", len(got))
	}

	// The next invocation gets the definitive answer and clears the token there.
	_, err = GetValidAccessToken(context.Background(), client, opts)
	requireRefreshProblem(t, err, errs.CategoryAuthentication, errs.SubtypeRefreshTokenRevoked, false)
	if current := mustGetStoredToken(t, stored.AppId, stored.UserOpenId); current != nil {
		t.Fatalf("stored token = %#v, want cleared after a definitive revoked answer", current)
	}
}

// The refresh deadline firing after the write stops without clearing the token.
func TestRefreshDeadlineAfterWriteStopsAndPreserves(t *testing.T) {
	setupStoredTokenTest(t)
	stored := newRefreshTestToken()
	if err := SetStoredToken(stored); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}
	endpoint := &tokenEndpointServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpoint.record(t, r)
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// doRefreshToken receives the refresh transaction deadline from
	// refreshWithLock; drive it directly with a short one.
	refreshed, err := doRefreshToken(ctx, clientFor(srv, 0), newRefreshTestOptions(stored), stored)

	if refreshed != nil {
		t.Fatalf("refreshed = %#v, want nil", refreshed)
	}
	requireOutcomeUnknownPreserved(t, err, stored, errs.SubtypeNetworkTimeout)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded cause", err)
	}
	if got := endpoint.requests(); len(got) != 1 {
		t.Fatalf("token endpoint requests = %d, want 1", len(got))
	}
}

// A dial failure never reached the server, so the ordinary retry still applies.
func TestRefreshDialErrorBeforeWriteStillRetries(t *testing.T) {
	setupStoredTokenTest(t)
	stored := newRefreshTestToken()
	if err := SetStoredToken(stored); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}
	dialErr := errors.New("connect: connection refused")
	var dials atomic.Int32
	client := &http.Client{Transport: &http.Transport{
		Proxy: nil,
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			dials.Add(1)
			return nil, dialErr
		},
	}}

	_, err := GetValidAccessToken(context.Background(), client, newRefreshTestOptions(stored))

	problem := requireRefreshProblem(t, err, errs.CategoryNetwork, errs.SubtypeNetworkTransport, true)
	if strings.Contains(problem.Message, "outcome unknown") {
		t.Fatalf("message = %q, a dial error never reached the server", problem.Message)
	}
	if !errors.Is(err, dialErr) {
		t.Fatalf("error = %v, want dial error cause", err)
	}
	if got := dials.Load(); got != refreshMaxAttempts {
		t.Fatalf("dials = %d, want %d (pre-write failures keep retrying)", got, refreshMaxAttempts)
	}
	current := mustGetStoredToken(t, stored.AppId, stored.UserOpenId)
	if current == nil || current.RefreshToken != stored.RefreshToken {
		t.Fatalf("stored token = %#v, want original generation preserved", current)
	}
}
