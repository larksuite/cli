// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar

package sidecar

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/larksuite/cli/sidecar"
)

// failingBody is a ReadCloser that errors on Read and tracks Close calls.
type failingBody struct {
	err      error
	closed   bool
	readCall bool
}

func (b *failingBody) Read(p []byte) (int, error) {
	b.readCall = true
	return 0, b.err
}

func (b *failingBody) Close() error {
	b.closed = true
	return nil
}

func TestInterceptor_PreRoundTrip(t *testing.T) {
	key := []byte("test-key-for-hmac-signing-32byte!")
	interceptor := &Interceptor{key: key, sidecarHost: "127.0.0.1:16384"}

	body := []byte(`{"msg":"hello"}`)
	req, _ := http.NewRequest("POST", "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id", io.NopCloser(bytes.NewReader(body)))
	req.Header.Set("Authorization", "Bearer "+sidecar.SentinelUAT)
	req.Header.Set("X-Cli-Source", "lark-cli")

	post := interceptor.PreRoundTrip(req)

	if post != nil {
		t.Error("expected nil post hook")
	}

	// URL should be rewritten to sidecar
	if req.URL.Scheme != "http" {
		t.Errorf("scheme = %q, want %q", req.URL.Scheme, "http")
	}
	if req.URL.Host != "127.0.0.1:16384" {
		t.Errorf("host = %q, want %q", req.URL.Host, "127.0.0.1:16384")
	}

	// Original target should be preserved
	target := req.Header.Get(sidecar.HeaderProxyTarget)
	if target != "https://open.feishu.cn" {
		t.Errorf("target = %q, want %q", target, "https://open.feishu.cn")
	}

	// Identity should be user (from SentinelUAT)
	if identity := req.Header.Get(sidecar.HeaderProxyIdentity); identity != sidecar.IdentityUser {
		t.Errorf("identity = %q, want %q", identity, sidecar.IdentityUser)
	}

	// Authorization should be stripped
	if auth := req.Header.Get("Authorization"); auth != "" {
		t.Errorf("Authorization header should be stripped, got %q", auth)
	}

	// HMAC headers should be set
	if sig := req.Header.Get(sidecar.HeaderProxySignature); sig == "" {
		t.Error("signature header should be set")
	}
	if ts := req.Header.Get(sidecar.HeaderProxyTimestamp); ts == "" {
		t.Error("timestamp header should be set")
	}
	if sha := req.Header.Get(sidecar.HeaderBodySHA256); sha == "" {
		t.Error("body SHA256 header should be set")
	}
	if v := req.Header.Get(sidecar.HeaderProxyVersion); v != sidecar.ProtocolV1 {
		t.Errorf("version header = %q, want %q", v, sidecar.ProtocolV1)
	}

	// Non-proxy headers should be preserved
	if src := req.Header.Get("X-Cli-Source"); src != "lark-cli" {
		t.Errorf("X-Cli-Source should be preserved, got %q", src)
	}

	// Body should still be readable
	readBody, _ := io.ReadAll(req.Body)
	if !bytes.Equal(readBody, body) {
		t.Errorf("body should be preserved after PreRoundTrip")
	}
}

func TestInterceptor_BotIdentity(t *testing.T) {
	interceptor := &Interceptor{key: []byte("key"), sidecarHost: "127.0.0.1:16384"}

	req, _ := http.NewRequest("GET", "https://open.feishu.cn/open-apis/calendar/v4/events", nil)
	req.Header.Set("Authorization", "Bearer "+sidecar.SentinelTAT)

	interceptor.PreRoundTrip(req)

	if identity := req.Header.Get(sidecar.HeaderProxyIdentity); identity != sidecar.IdentityBot {
		t.Errorf("identity = %q, want %q", identity, sidecar.IdentityBot)
	}
}

func TestInterceptor_NonSentinelToken_PassThrough(t *testing.T) {
	interceptor := &Interceptor{key: []byte("key"), sidecarHost: "127.0.0.1:16384"}

	origURL := "https://some-cdn.example.com/presigned-download?token=abc"
	req, _ := http.NewRequest("GET", origURL, nil)
	req.Header.Set("Authorization", "Bearer some-real-token")

	post := interceptor.PreRoundTrip(req)

	// Should NOT be rewritten — no sentinel token
	if post != nil {
		t.Error("expected nil post hook for pass-through")
	}
	if req.URL.String() != origURL {
		t.Errorf("URL should be unchanged, got %q", req.URL.String())
	}
	if req.Header.Get(sidecar.HeaderProxyTarget) != "" {
		t.Error("proxy target header should not be set for pass-through")
	}
	if req.Header.Get("Authorization") != "Bearer some-real-token" {
		t.Error("Authorization should be preserved for pass-through")
	}
}

func TestInterceptor_NoAuth_PassThrough(t *testing.T) {
	interceptor := &Interceptor{key: []byte("key"), sidecarHost: "127.0.0.1:16384"}

	origURL := "https://cdn.feishu.cn/download/file"
	req, _ := http.NewRequest("GET", origURL, nil)

	interceptor.PreRoundTrip(req)

	// No Authorization header at all — should pass through
	if req.URL.String() != origURL {
		t.Errorf("URL should be unchanged for no-auth request, got %q", req.URL.String())
	}
}

func TestInterceptor_MCP_UAT(t *testing.T) {
	interceptor := &Interceptor{key: []byte("key"), sidecarHost: "127.0.0.1:16384"}

	req, _ := http.NewRequest("POST", "https://mcp.feishu.cn/mcp/v1/tools/call", bytes.NewReader([]byte(`{"jsonrpc":"2.0"}`)))
	req.Header.Set(sidecar.HeaderMCPUAT, sidecar.SentinelUAT)

	interceptor.PreRoundTrip(req)

	// Should be intercepted and rewritten
	if req.URL.Host != "127.0.0.1:16384" {
		t.Errorf("host = %q, want sidecar host", req.URL.Host)
	}
	if identity := req.Header.Get(sidecar.HeaderProxyIdentity); identity != sidecar.IdentityUser {
		t.Errorf("identity = %q, want %q", identity, sidecar.IdentityUser)
	}
	if ah := req.Header.Get(sidecar.HeaderProxyAuthHeader); ah != sidecar.HeaderMCPUAT {
		t.Errorf("auth header = %q, want %q", ah, sidecar.HeaderMCPUAT)
	}
	// MCP sentinel should be stripped
	if v := req.Header.Get(sidecar.HeaderMCPUAT); v != "" {
		t.Errorf("MCP-UAT should be stripped, got %q", v)
	}
}

func TestInterceptor_MCP_TAT(t *testing.T) {
	interceptor := &Interceptor{key: []byte("key"), sidecarHost: "127.0.0.1:16384"}

	req, _ := http.NewRequest("POST", "https://mcp.feishu.cn/mcp/v1/tools/call", bytes.NewReader([]byte(`{}`)))
	req.Header.Set(sidecar.HeaderMCPTAT, sidecar.SentinelTAT)

	interceptor.PreRoundTrip(req)

	if identity := req.Header.Get(sidecar.HeaderProxyIdentity); identity != sidecar.IdentityBot {
		t.Errorf("identity = %q, want %q", identity, sidecar.IdentityBot)
	}
	if ah := req.Header.Get(sidecar.HeaderProxyAuthHeader); ah != sidecar.HeaderMCPTAT {
		t.Errorf("auth header = %q, want %q", ah, sidecar.HeaderMCPTAT)
	}
}

func TestInterceptor_StandardAuth_SetsAuthorizationHeader(t *testing.T) {
	interceptor := &Interceptor{key: []byte("key"), sidecarHost: "127.0.0.1:16384"}

	req, _ := http.NewRequest("GET", "https://open.feishu.cn/open-apis/test", nil)
	req.Header.Set("Authorization", "Bearer "+sidecar.SentinelUAT)

	interceptor.PreRoundTrip(req)

	if ah := req.Header.Get(sidecar.HeaderProxyAuthHeader); ah != "Authorization" {
		t.Errorf("auth header = %q, want %q", ah, "Authorization")
	}
}

// TestInterceptor_BodyReadError verifies that when io.ReadAll on the request
// body fails partway, PreRoundTrip skips the rewrite entirely rather than
// signing a truncated body (which would produce a misleading HMAC mismatch on
// the sidecar side) and releases the original body.
func TestInterceptor_BodyReadError(t *testing.T) {
	interceptor := &Interceptor{key: []byte("key"), sidecarHost: "127.0.0.1:16384"}

	const origURL = "https://open.feishu.cn/open-apis/im/v1/messages"
	body := &failingBody{err: errors.New("disk gremlin")}

	req, _ := http.NewRequest("POST", origURL, body)
	req.Header.Set("Authorization", "Bearer "+sidecar.SentinelUAT)

	post := interceptor.PreRoundTrip(req)

	if post != nil {
		t.Error("expected nil post hook on body read failure")
	}

	// Original body must be closed to avoid leaking fd/pipe-like resources.
	if !body.readCall {
		t.Error("expected ReadAll to have attempted reading from the body")
	}
	if !body.closed {
		t.Error("expected original body to be Close()'d after read failure")
	}

	// URL must NOT be rewritten — request should fall through to the next
	// layer (credential) which can surface a meaningful error.
	if req.URL.String() != origURL {
		t.Errorf("URL should be unchanged on read failure, got %q", req.URL.String())
	}

	// No proxy/HMAC headers should leak onto the request.
	for _, h := range []string{
		sidecar.HeaderProxyVersion,
		sidecar.HeaderProxyTarget,
		sidecar.HeaderProxySignature,
		sidecar.HeaderProxyTimestamp,
		sidecar.HeaderBodySHA256,
		sidecar.HeaderProxyIdentity,
		sidecar.HeaderProxyAuthHeader,
	} {
		if v := req.Header.Get(h); v != "" {
			t.Errorf("%s should not be set on read failure, got %q", h, v)
		}
	}
}

func TestInterceptor_EmptyBody(t *testing.T) {
	interceptor := &Interceptor{key: []byte("key"), sidecarHost: "127.0.0.1:16384"}

	req, _ := http.NewRequest("GET", "https://open.feishu.cn/path", nil)
	req.Header.Set("Authorization", "Bearer "+sidecar.SentinelTAT)
	interceptor.PreRoundTrip(req)

	sha := req.Header.Get(sidecar.HeaderBodySHA256)
	expectedEmpty := sidecar.BodySHA256(nil)
	if sha != expectedEmpty {
		t.Errorf("body SHA256 = %q, want empty-string SHA256 %q", sha, expectedEmpty)
	}
}
