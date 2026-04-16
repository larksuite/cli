// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar

package auth

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/sidecar"
)

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
			"mcp.feishu.cn":     true,
		},
		allowedIDs: map[string]bool{
			sidecar.IdentityUser: true,
			sidecar.IdentityBot:  true,
		},
	}
}

// signedReq creates a properly signed request for testing handler logic past HMAC verification.
func signedReq(t *testing.T, key []byte, method, target, path string, body []byte) *http.Request {
	t.Helper()
	targetHost := target
	if idx := len("https://"); len(target) > idx {
		targetHost = target[idx:]
	}
	bodySHA := sidecar.BodySHA256(body)
	ts := sidecar.Timestamp()
	sig := sidecar.Sign(key, method, targetHost, path, bodySHA, ts)

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set(sidecar.HeaderProxyTarget, target)
	req.Header.Set(sidecar.HeaderProxyIdentity, sidecar.IdentityBot)
	req.Header.Set(sidecar.HeaderBodySHA256, bodySHA)
	req.Header.Set(sidecar.HeaderProxyTimestamp, ts)
	req.Header.Set(sidecar.HeaderProxySignature, sig)
	return req
}

func TestProxyHandler_MissingTimestamp(t *testing.T) {
	h := newTestHandler([]byte("key"))
	req := httptest.NewRequest("GET", "/path", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestProxyHandler_MissingBodySHA(t *testing.T) {
	h := newTestHandler([]byte("key"))
	req := httptest.NewRequest("GET", "/path", nil)
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
	req.Header.Set(sidecar.HeaderProxyTarget, "https://open.feishu.cn")
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
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for disallowed identity, got %d", w.Code)
	}
}

func TestRunProxy_RejectsSelfProxy(t *testing.T) {
	old, had := os.LookupEnv(envvars.CliAuthProxy)
	os.Setenv(envvars.CliAuthProxy, "http://127.0.0.1:16384")
	defer func() {
		if had {
			os.Setenv(envvars.CliAuthProxy, old)
		} else {
			os.Unsetenv(envvars.CliAuthProxy)
		}
	}()

	err := runProxy(nil, &ProxyOptions{Listen: "127.0.0.1:0"})
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
		{"oc_abcdef12345678", true},  // chat ID
		{"v1", false},                // API version
		{"messages", false},          // route keyword
		{"open-apis", false},         // route prefix
		{"ab1", false},               // too short
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
