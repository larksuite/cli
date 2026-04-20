// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar_demo

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/sidecar"
)

// fakeExtProvider is a stub extcred.Provider for tests that returns a fixed token.
type fakeExtProvider struct {
	token string
}

func (f *fakeExtProvider) Name() string { return "fake" }
func (f *fakeExtProvider) ResolveAccount(ctx context.Context) (*extcred.Account, error) {
	return nil, nil
}
func (f *fakeExtProvider) ResolveToken(ctx context.Context, req extcred.TokenSpec) (*extcred.Token, error) {
	return &extcred.Token{Value: f.token, Source: "fake"}, nil
}

func discardLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

func newTestHandler(key []byte) *proxyHandler {
	return &proxyHandler{
		key:       key,
		logger:    discardLogger(),
		forwardCl: &http.Client{},
		allowedHosts: map[string]bool{
			"open.feishu.cn":     true,
			"accounts.feishu.cn": true,
			"mcp.feishu.cn":      true,
		},
		allowedIDs: map[string]bool{
			sidecar.IdentityUser: true,
			sidecar.IdentityBot:  true,
		},
	}
}

// signedReq creates a properly signed request for testing handler logic past
// HMAC verification. Identity defaults to bot and auth-header to
// "Authorization"; callers can override by mutating the returned request
// before calling ServeHTTP (and re-signing if they need the signature to
// remain valid after the mutation).
func signedReq(t *testing.T, key []byte, method, target, path string, body []byte) *http.Request {
	t.Helper()
	targetHost := target
	if idx := strings.Index(target, "://"); idx >= 0 {
		targetHost = target[idx+3:]
	}
	bodySHA := sidecar.BodySHA256(body)
	ts := sidecar.Timestamp()
	identity := sidecar.IdentityBot
	authHeader := "Authorization"
	sig := sidecar.Sign(key, sidecar.CanonicalRequest{
		Version:      sidecar.ProtocolV1,
		Method:       method,
		Host:         targetHost,
		PathAndQuery: path,
		BodySHA256:   bodySHA,
		Timestamp:    ts,
		Identity:     identity,
		AuthHeader:   authHeader,
	})

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set(sidecar.HeaderProxyVersion, sidecar.ProtocolV1)
	req.Header.Set(sidecar.HeaderProxyTarget, target)
	req.Header.Set(sidecar.HeaderProxyIdentity, identity)
	req.Header.Set(sidecar.HeaderProxyAuthHeader, authHeader)
	req.Header.Set(sidecar.HeaderBodySHA256, bodySHA)
	req.Header.Set(sidecar.HeaderProxyTimestamp, ts)
	req.Header.Set(sidecar.HeaderProxySignature, sig)
	return req
}

// resign recomputes the HMAC signature over the request's current proxy
// headers. Use this in tests that mutate a signed field (Identity,
// AuthHeader, Target host, etc.) after calling signedReq.
func resign(t *testing.T, key []byte, req *http.Request, body []byte) {
	t.Helper()
	target := req.Header.Get(sidecar.HeaderProxyTarget)
	targetHost := target
	if idx := strings.Index(target, "://"); idx >= 0 {
		targetHost = target[idx+3:]
	}
	sig := sidecar.Sign(key, sidecar.CanonicalRequest{
		Version:      req.Header.Get(sidecar.HeaderProxyVersion),
		Method:       req.Method,
		Host:         targetHost,
		PathAndQuery: req.URL.RequestURI(),
		BodySHA256:   sidecar.BodySHA256(body),
		Timestamp:    req.Header.Get(sidecar.HeaderProxyTimestamp),
		Identity:     req.Header.Get(sidecar.HeaderProxyIdentity),
		AuthHeader:   req.Header.Get(sidecar.HeaderProxyAuthHeader),
	})
	req.Header.Set(sidecar.HeaderProxySignature, sig)
}

// TestProxyHandler_UnsupportedVersion verifies the handler rejects requests
// whose HeaderProxyVersion is absent or set to an unknown value. Kept in
// front so an old client paired with a newer server (or vice versa) surfaces
// a clear 400 instead of a misleading HMAC mismatch downstream.
func TestProxyHandler_UnsupportedVersion(t *testing.T) {
	h := newTestHandler([]byte("key"))
	for _, v := range []string{"", "v0", "v2"} {
		req := httptest.NewRequest("GET", "/path", nil)
		if v != "" {
			req.Header.Set(sidecar.HeaderProxyVersion, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("version=%q: expected 400, got %d", v, w.Code)
		}
	}
}

func TestProxyHandler_MissingTimestamp(t *testing.T) {
	h := newTestHandler([]byte("key"))
	req := httptest.NewRequest("GET", "/path", nil)
	req.Header.Set(sidecar.HeaderProxyVersion, sidecar.ProtocolV1)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestProxyHandler_MissingBodySHA(t *testing.T) {
	h := newTestHandler([]byte("key"))
	req := httptest.NewRequest("GET", "/path", nil)
	req.Header.Set(sidecar.HeaderProxyVersion, sidecar.ProtocolV1)
	req.Header.Set(sidecar.HeaderProxyTimestamp, sidecar.Timestamp())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestProxyHandler_BadHMAC(t *testing.T) {
	h := newTestHandler([]byte("real-key"))

	bodySHA := sidecar.BodySHA256(nil)
	ts := sidecar.Timestamp()

	req := httptest.NewRequest("GET", "/path", nil)
	req.Header.Set(sidecar.HeaderProxyVersion, sidecar.ProtocolV1)
	req.Header.Set(sidecar.HeaderProxyTarget, "https://open.feishu.cn")
	req.Header.Set(sidecar.HeaderProxyIdentity, sidecar.IdentityBot)
	req.Header.Set(sidecar.HeaderProxyAuthHeader, "Authorization")
	req.Header.Set(sidecar.HeaderProxyTimestamp, ts)
	req.Header.Set(sidecar.HeaderBodySHA256, bodySHA)
	req.Header.Set(sidecar.HeaderProxySignature, "bad-signature")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestProxyHandler_BodySHA256Mismatch(t *testing.T) {
	h := newTestHandler([]byte("key"))

	req := httptest.NewRequest("POST", "/path", bytes.NewReader([]byte("real body")))
	req.Header.Set(sidecar.HeaderProxyVersion, sidecar.ProtocolV1)
	req.Header.Set(sidecar.HeaderProxyTarget, "https://open.feishu.cn")
	req.Header.Set(sidecar.HeaderProxyTimestamp, sidecar.Timestamp())
	req.Header.Set(sidecar.HeaderBodySHA256, sidecar.BodySHA256([]byte("different body")))
	req.Header.Set(sidecar.HeaderProxySignature, "whatever")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestProxyHandler_TargetNotAllowed(t *testing.T) {
	key := []byte("test-key")
	h := newTestHandler(key)

	req := signedReq(t, key, "GET", "https://evil.com", "/steal", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for disallowed host, got %d", w.Code)
	}
}

func TestProxyHandler_IdentityNotAllowed(t *testing.T) {
	key := []byte("test-key")
	h := newTestHandler(key)
	// Restrict to bot only
	h.allowedIDs = map[string]bool{sidecar.IdentityBot: true}

	req := signedReq(t, key, "GET", "https://open.feishu.cn", "/open-apis/test", nil)
	req.Header.Set(sidecar.HeaderProxyIdentity, sidecar.IdentityUser)
	resign(t, key, req, nil) // identity is signed; must re-sign after mutation
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for disallowed identity, got %d", w.Code)
	}
}

// TestParseTarget covers the per-shape rejections directly, without the
// surrounding HTTP plumbing.
func TestParseTarget(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		wantErr bool
		wantSub string // expected fragment of the error message
	}{
		{name: "valid https", target: "https://open.feishu.cn", wantErr: false},
		{name: "valid https trailing slash", target: "https://open.feishu.cn/", wantErr: false},
		{name: "http downgrade", target: "http://open.feishu.cn", wantErr: true, wantSub: "scheme must be https"},
		{name: "missing scheme", target: "open.feishu.cn", wantErr: true, wantSub: "scheme must be https"},
		{name: "ftp scheme", target: "ftp://open.feishu.cn", wantErr: true, wantSub: "scheme must be https"},
		{name: "empty", target: "", wantErr: true, wantSub: "scheme must be https"},
		{name: "empty host", target: "https://", wantErr: true, wantSub: "missing host"},
		{name: "with path", target: "https://open.feishu.cn/open-apis", wantErr: true, wantSub: "path not allowed"},
		{name: "with query", target: "https://open.feishu.cn?a=1", wantErr: true, wantSub: "query not allowed"},
		{name: "with fragment", target: "https://open.feishu.cn#frag", wantErr: true, wantSub: "fragment not allowed"},
		{name: "with userinfo", target: "https://attacker:pw@open.feishu.cn", wantErr: true, wantSub: "userinfo not allowed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, err := parseTarget(tc.target)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got host=%q", host)
				}
				if tc.wantSub != "" && !strings.Contains(err.Error(), tc.wantSub) {
					t.Errorf("error %q should contain %q", err.Error(), tc.wantSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if host != "open.feishu.cn" {
				t.Errorf("host = %q, want %q", host, "open.feishu.cn")
			}
		})
	}
}

// TestProxyHandler_RejectsNonHTTPSTarget verifies end-to-end that a
// compromised sandbox holding a valid PROXY_KEY cannot coerce the sidecar
// into forwarding real tokens over cleartext HTTP or to an unexpected path.
// The check must fire before HMAC verification so that the request is
// rejected even when the signature is technically valid.
func TestProxyHandler_RejectsNonHTTPSTarget(t *testing.T) {
	key := []byte("test-key")
	h := newTestHandler(key)

	cases := []struct {
		name   string
		target string
	}{
		{"http downgrade", "http://open.feishu.cn"},
		{"bare hostname", "open.feishu.cn"},
		{"ftp scheme", "ftp://open.feishu.cn"},
		{"target with path", "https://open.feishu.cn/open-apis/evil"},
		{"target with query", "https://open.feishu.cn?steal=1"},
		{"target with userinfo", "https://attacker:pw@open.feishu.cn"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Sign with a valid key against the malicious target — proves the
			// scheme/shape check is not bypassed by signature legitimacy.
			req := signedReq(t, key, "GET", tc.target, "/open-apis/im/v1/chats", nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Errorf("expected 403 for target %q, got %d (body: %s)", tc.target, w.Code, w.Body.String())
			}
		})
	}
}

// TestProxyHandler_RejectsIdentityReplay locks in C1 end-to-end: a captured
// bot-signed request whose identity header is flipped to user (or vice versa)
// must be rejected at HMAC verification, not silently served with the wrong
// token type. Without identity in the canonical string this returns 200.
func TestProxyHandler_RejectsIdentityReplay(t *testing.T) {
	key := []byte("test-key")
	h := newTestHandler(key)

	req := signedReq(t, key, "GET", "https://open.feishu.cn", "/open-apis/test", nil)
	// Attacker flips identity without touching signature.
	req.Header.Set(sidecar.HeaderProxyIdentity, sidecar.IdentityUser)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("identity replay must fail signature verify (got %d, want 401): %s",
			w.Code, w.Body.String())
	}
}

// TestProxyHandler_RejectsAuthHeaderReplay is the companion: flipping
// X-Lark-Proxy-Auth-Header post-signature must invalidate the signature so
// an attacker cannot redirect the injected token into an unintended header.
func TestProxyHandler_RejectsAuthHeaderReplay(t *testing.T) {
	key := []byte("test-key")
	h := newTestHandler(key)

	req := signedReq(t, key, "GET", "https://open.feishu.cn", "/open-apis/test", nil)
	req.Header.Set(sidecar.HeaderProxyAuthHeader, "Cookie")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("auth-header replay must fail signature verify (got %d, want 401): %s",
			w.Code, w.Body.String())
	}
}

// TestProxyHandler_RejectsAuthHeaderNotInAllowlist pins the auth-header
// allowlist: even a correctly-signed request must be rejected if it asks
// the sidecar to inject the real token into an unintended header (e.g.
// Cookie / User-Agent / X-Forwarded-For). This closes the sidechannel
// where the real token ends up in headers that Lark ignores for auth but
// intermediate logs may capture.
func TestProxyHandler_RejectsAuthHeaderNotInAllowlist(t *testing.T) {
	key := []byte("test-key")
	h := newTestHandler(key)

	for _, bad := range []string{"Cookie", "User-Agent", "X-Forwarded-For", "X-Real-IP", "Set-Cookie"} {
		t.Run(bad, func(t *testing.T) {
			req := signedReq(t, key, "GET", "https://open.feishu.cn", "/open-apis/test", nil)
			req.Header.Set(sidecar.HeaderProxyAuthHeader, bad)
			resign(t, key, req, nil) // auth-header is signed; must re-sign after override
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Errorf("authHeader=%q: expected 403, got %d (body: %s)",
					bad, w.Code, w.Body.String())
			}
		})
	}
}

// TestProxyHandler_AcceptsAllowedAuthHeaders confirms the three protocol
// header names remain accepted after the allowlist is enforced. Uses
// newTestHandler which has no upstream forwarding set up, so reaching the
// forward step is proof the auth-header check passed.
func TestProxyHandler_AcceptsAllowedAuthHeaders(t *testing.T) {
	key := []byte("test-key")

	for _, good := range []string{"Authorization", sidecar.HeaderMCPUAT, sidecar.HeaderMCPTAT} {
		t.Run(good, func(t *testing.T) {
			// Use a handler with a real (fake) credential provider so we can
			// distinguish auth-header reject (403) from later failures.
			cred := credential.NewCredentialProvider(
				[]extcred.Provider{&fakeExtProvider{token: "real-token"}},
				nil, nil, nil,
			)
			h := &proxyHandler{
				key:          key,
				cred:         cred,
				appID:        "cli_test",
				logger:       discardLogger(),
				forwardCl:    &http.Client{},
				allowedHosts: map[string]bool{"open.feishu.cn": true},
				allowedIDs:   map[string]bool{sidecar.IdentityUser: true, sidecar.IdentityBot: true},
			}

			req := signedReq(t, key, "GET", "https://open.feishu.cn", "/open-apis/test", nil)
			req.Header.Set(sidecar.HeaderProxyAuthHeader, good)
			resign(t, key, req, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			// Expect NOT 403 "auth-header not allowed" — the request will fail
			// at forward (502 because open.feishu.cn isn't reachable without
			// an actual upstream in tests), but it must get past our check.
			if w.Code == http.StatusForbidden && strings.Contains(w.Body.String(), "auth-header not allowed") {
				t.Errorf("authHeader=%q was rejected by allowlist: %s", good, w.Body.String())
			}
		})
	}
}

func TestRun_RejectsSelfProxy(t *testing.T) {
	old, had := os.LookupEnv(envvars.CliAuthProxy)
	os.Setenv(envvars.CliAuthProxy, "http://127.0.0.1:16384")
	defer func() {
		if had {
			os.Setenv(envvars.CliAuthProxy, old)
		} else {
			os.Unsetenv(envvars.CliAuthProxy)
		}
	}()

	err := run(context.Background(), "127.0.0.1:0", "/tmp/should-not-be-created.key", "", "")
	if err == nil {
		t.Fatal("expected error when AUTH_PROXY is set")
	}
	if !strings.Contains(err.Error(), envvars.CliAuthProxy) {
		t.Errorf("error should mention %s, got: %v", envvars.CliAuthProxy, err)
	}
}

func TestForwardClient_RedirectStripsAuth(t *testing.T) {
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("Authorization leaked to redirect target: %s", auth)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer redirectTarget.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL+"/redirected", http.StatusFound)
	}))
	defer origin.Close()

	client := newForwardClient()
	req, _ := http.NewRequest("GET", origin.URL+"/start", nil)
	req.Header.Set("Authorization", "Bearer real-token")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
}

func TestForwardClient_RedirectStripsMCPHeaders(t *testing.T) {
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get(sidecar.HeaderMCPUAT); v != "" {
			t.Errorf("X-Lark-MCP-UAT leaked to redirect target: %s", v)
		}
		if v := r.Header.Get(sidecar.HeaderMCPTAT); v != "" {
			t.Errorf("X-Lark-MCP-TAT leaked to redirect target: %s", v)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer redirectTarget.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL+"/redirected", http.StatusFound)
	}))
	defer origin.Close()

	client := newForwardClient()
	req, _ := http.NewRequest("POST", origin.URL+"/mcp", nil)
	req.Header.Set(sidecar.HeaderMCPUAT, "real-uat-token")
	req.Header.Set(sidecar.HeaderMCPTAT, "real-tat-token")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
}

// TestProxyHandler_StripsClientSuppliedAuthHeaders verifies that the sidecar
// is the sole source of auth headers on the forwarded request. A malicious
// sandbox client must not be able to smuggle an Authorization/MCP header that
// rides along with the sidecar-injected real token.
func TestProxyHandler_StripsClientSuppliedAuthHeaders(t *testing.T) {
	const realToken = "real-tenant-access-token"

	// Capture what the upstream receives after sidecar forwarding.
	// TLS is required because parseTarget rejects non-https targets.
	var captured http.Header
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// Strip "https://" prefix to get host:port (matches what the handler sees).
	upstreamHost := strings.TrimPrefix(upstream.URL, "https://")

	cred := credential.NewCredentialProvider(
		[]extcred.Provider{&fakeExtProvider{token: realToken}},
		nil, nil, nil,
	)

	key := []byte("test-key")
	h := &proxyHandler{
		key:          key,
		cred:         cred,
		appID:        "cli_test",
		logger:       discardLogger(),
		forwardCl:    upstream.Client(), // trusts the httptest CA
		allowedHosts: map[string]bool{upstreamHost: true},
		allowedIDs:   map[string]bool{sidecar.IdentityUser: true, sidecar.IdentityBot: true},
	}

	cases := []struct {
		name                string
		proxyAuthHeader     string // which header sidecar should inject into
		wantInjectedHeader  string // the header the real token ends up in
		wantInjectedValue   string
		wantStrippedHeaders []string
	}{
		{
			name:                "inject Authorization, strip MCP attacker headers",
			proxyAuthHeader:     "Authorization",
			wantInjectedHeader:  "Authorization",
			wantInjectedValue:   "Bearer " + realToken,
			wantStrippedHeaders: []string{sidecar.HeaderMCPUAT, sidecar.HeaderMCPTAT},
		},
		{
			name:                "inject MCP UAT, strip Authorization attacker header",
			proxyAuthHeader:     sidecar.HeaderMCPUAT,
			wantInjectedHeader:  sidecar.HeaderMCPUAT,
			wantInjectedValue:   realToken,
			wantStrippedHeaders: []string{"Authorization", sidecar.HeaderMCPTAT},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			captured = nil

			req := signedReq(t, key, "GET", "https://"+upstreamHost, "/open-apis/test", nil)
			req.Header.Set(sidecar.HeaderProxyAuthHeader, tc.proxyAuthHeader)
			resign(t, key, req, nil) // auth-header is signed; re-sign after override

			// Attacker smuggles all three possible auth headers with bogus values.
			req.Header.Set("Authorization", "Bearer attacker-token")
			req.Header.Set(sidecar.HeaderMCPUAT, "attacker-uat")
			req.Header.Set(sidecar.HeaderMCPTAT, "attacker-tat")

			// Non-auth headers should still pass through.
			req.Header.Set("X-Custom-Header", "keep-me")

			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200 from upstream, got %d; body=%s", w.Code, w.Body.String())
			}
			if captured == nil {
				t.Fatal("upstream handler was not invoked")
			}

			// Injected header contains the real token (not the attacker value).
			if got := captured.Get(tc.wantInjectedHeader); got != tc.wantInjectedValue {
				t.Errorf("%s = %q, want %q", tc.wantInjectedHeader, got, tc.wantInjectedValue)
			}

			// All other auth headers must be stripped.
			for _, h := range tc.wantStrippedHeaders {
				if got := captured.Get(h); got != "" {
					t.Errorf("%s should be stripped, got %q", h, got)
				}
			}

			// Non-auth headers still forwarded.
			if got := captured.Get("X-Custom-Header"); got != "keep-me" {
				t.Errorf("X-Custom-Header = %q, want %q", got, "keep-me")
			}
		})
	}
}

func TestBuildAllowedHosts(t *testing.T) {
	feishu := struct{ Open, Accounts, MCP string }{
		"https://open.feishu.cn", "https://accounts.feishu.cn", "https://mcp.feishu.cn",
	}
	lark := struct{ Open, Accounts, MCP string }{
		"https://open.larksuite.com", "https://accounts.larksuite.com", "https://mcp.larksuite.com",
	}
	hosts := buildAllowedHosts(feishu, lark)
	// feishu hosts
	if !hosts["open.feishu.cn"] {
		t.Error("expected open.feishu.cn in allowlist")
	}
	if !hosts["mcp.feishu.cn"] {
		t.Error("expected mcp.feishu.cn in allowlist")
	}
	// lark hosts
	if !hosts["open.larksuite.com"] {
		t.Error("expected open.larksuite.com in allowlist")
	}
	if !hosts["mcp.larksuite.com"] {
		t.Error("expected mcp.larksuite.com in allowlist")
	}
	// evil host
	if hosts["evil.com"] {
		t.Error("evil.com should not be in allowlist")
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/open-apis/im/v1/messages?receive_id_type=chat_id", "/open-apis/im/v1/messages"},
		{"/open-apis/calendar/v4/events", "/open-apis/calendar/v4/events"},
		{"/open-apis/docx/v1/documents/doxcnABCD1234/blocks", "/open-apis/docx/v1/documents/:id/blocks"},
		{"/open-apis/im/v1/chats/oc_abcdef12345678/members", "/open-apis/im/v1/chats/:id/members"},
		{"/path?secret=abc", "/path"},
	}
	for _, tt := range tests {
		if got := sanitizePath(tt.input); got != tt.want {
			t.Errorf("sanitizePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLooksLikeID(t *testing.T) {
	tests := []struct {
		seg  string
		want bool
	}{
		{"doxcnABCD1234", true},     // doc token
		{"oc_abcdef12345678", true}, // chat ID
		{"v1", false},               // API version
		{"messages", false},         // route keyword
		{"open-apis", false},        // route prefix
		{"ab1", false},              // too short
	}
	for _, tt := range tests {
		if got := looksLikeID(tt.seg); got != tt.want {
			t.Errorf("looksLikeID(%q) = %v, want %v", tt.seg, got, tt.want)
		}
	}
}

func TestSanitizeError(t *testing.T) {
	short := fmt.Errorf("short error")
	if got := sanitizeError(short); got != "short error" {
		t.Errorf("got %q", got)
	}

	longMsg := make([]byte, 300)
	for i := range longMsg {
		longMsg[i] = 'x'
	}
	long := fmt.Errorf("%s", string(longMsg))
	got := sanitizeError(long)
	if len(got) > 210 {
		t.Errorf("expected truncation, got %d chars", len(got))
	}
	if !bytes.HasSuffix([]byte(got), []byte("...")) {
		t.Errorf("expected '...' suffix, got %q", got[len(got)-10:])
	}
}
