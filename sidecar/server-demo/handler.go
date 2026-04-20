// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar_demo

package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/sidecar"
)

// proxyHandler handles HTTP requests from sandbox CLI instances.
type proxyHandler struct {
	key          []byte
	cred         *credential.CredentialProvider
	appID        string
	brand        core.LarkBrand
	logger       *log.Logger
	forwardCl    *http.Client
	allowedHosts map[string]bool // target host allowlist derived from brand
	allowedIDs   map[string]bool // identity allowlist derived from strict mode
}

// allowedAuthHeaders lists the only header names the sidecar will inject real
// tokens into. Limiting this prevents a compromised sandbox from signing a
// request with X-Lark-Proxy-Auth-Header: Cookie (or User-Agent /
// X-Forwarded-For / any X-* header) and having the real token smuggled into
// an upstream header that Lark ignores for auth but intermediate logs may
// capture — an indirect exfiltration path.
//
// These three are the only values the CLI interceptor ever emits
// (Authorization for OpenAPI, MCP-UAT/TAT for the MCP protocol), so anything
// else is by definition a misuse.
var allowedAuthHeaders = map[string]bool{
	"Authorization":      true,
	sidecar.HeaderMCPUAT: true, // X-Lark-MCP-UAT
	sidecar.HeaderMCPTAT: true, // X-Lark-MCP-TAT
}

func (h *proxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// 0. Check protocol version. We reject rather than default so that an
	// old client paired with a newer server (or vice versa) fails loudly
	// instead of silently producing mismatched signatures.
	version := r.Header.Get(sidecar.HeaderProxyVersion)
	if version != sidecar.ProtocolV1 {
		http.Error(w, "unsupported "+sidecar.HeaderProxyVersion+": "+version, http.StatusBadRequest)
		return
	}

	// 1. Verify timestamp
	ts := r.Header.Get(sidecar.HeaderProxyTimestamp)
	if ts == "" {
		http.Error(w, "missing "+sidecar.HeaderProxyTimestamp, http.StatusBadRequest)
		return
	}

	// 2. Read body and verify SHA256
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	claimedSHA := r.Header.Get(sidecar.HeaderBodySHA256)
	if claimedSHA == "" {
		http.Error(w, "missing "+sidecar.HeaderBodySHA256, http.StatusBadRequest)
		return
	}
	actualSHA := sidecar.BodySHA256(body)
	if claimedSHA != actualSHA {
		http.Error(w, "body SHA256 mismatch", http.StatusBadRequest)
		return
	}

	// 3. Verify HMAC signature
	//Enforce scheme=https and reject any path/query embedded in the target.
	// The sandbox is untrusted: without this check it could send
	// X-Lark-Proxy-Target: http://open.feishu.cn to force the injected real
	// token out over cleartext HTTP, exposing it to any on-path attacker
	// between the sidecar and upstream.
	target := r.Header.Get(sidecar.HeaderProxyTarget)
	if target == "" {
		http.Error(w, "missing "+sidecar.HeaderProxyTarget, http.StatusBadRequest)
		return
	}

	pathAndQuery := r.URL.RequestURI()
	targetHost, err := parseTarget(target)
	if err != nil {
		http.Error(w, "invalid "+sidecar.HeaderProxyTarget+": "+err.Error(), http.StatusForbidden)
		h.logger.Printf("REJECT method=%s path=%s reason=%q", r.Method, sanitizePath(pathAndQuery), sanitizeError(err))
		return
	}

	// Identity and auth-header must be read before HMAC verification because
	// both are covered by the canonical signing string. Defaulting either one
	// server-side would let an attacker flip the injected token's identity or
	// target header within the replay window without invalidating the sig.
	identity := r.Header.Get(sidecar.HeaderProxyIdentity)
	if identity == "" {
		http.Error(w, "missing "+sidecar.HeaderProxyIdentity, http.StatusBadRequest)
		return
	}
	authHeader := r.Header.Get(sidecar.HeaderProxyAuthHeader)
	if authHeader == "" {
		http.Error(w, "missing "+sidecar.HeaderProxyAuthHeader, http.StatusBadRequest)
		return
	}

	signature := r.Header.Get(sidecar.HeaderProxySignature)
	if err := sidecar.Verify(h.key, sidecar.CanonicalRequest{
		Version:      version,
		Method:       r.Method,
		Host:         targetHost,
		PathAndQuery: pathAndQuery,
		BodySHA256:   claimedSHA,
		Timestamp:    ts,
		Identity:     identity,
		AuthHeader:   authHeader,
	}, signature); err != nil {
		http.Error(w, "HMAC verification failed: "+err.Error(), http.StatusUnauthorized)
		h.logger.Printf("REJECT method=%s path=%s reason=%q", r.Method, sanitizePath(pathAndQuery), sanitizeError(err))
		return
	}

	// 4. Validate target host against allowlist
	if !h.allowedHosts[targetHost] {
		http.Error(w, "target host not allowed: "+targetHost, http.StatusForbidden)
		h.logger.Printf("REJECT method=%s path=%s reason=\"target host %s not in allowlist\"", r.Method, sanitizePath(pathAndQuery), targetHost)
		return
	}

	// 5. Validate identity
	if !h.allowedIDs[identity] {
		http.Error(w, "identity not allowed: "+identity, http.StatusForbidden)
		h.logger.Printf("REJECT method=%s path=%s reason=\"identity %s not allowed by strict mode\"", r.Method, sanitizePath(pathAndQuery), identity)
		return
	}

	// 5.5 Validate auth-header (required — the client controls this value,
	// and without an allowlist a compromised sandbox could direct the real
	// token into arbitrary forwarded headers).
	if !allowedAuthHeaders[authHeader] {
		http.Error(w, "auth-header not allowed: "+authHeader, http.StatusForbidden)
		h.logger.Printf("REJECT method=%s path=%s reason=\"auth-header %s not in allowlist\"", r.Method, sanitizePath(pathAndQuery), authHeader)
		return
	}

	// 6. Resolve real token
	var tokenType credential.TokenType
	switch identity {
	case sidecar.IdentityUser:
		tokenType = credential.TokenTypeUAT
	default:
		tokenType = credential.TokenTypeTAT
	}

	tokenResult, err := h.cred.ResolveToken(r.Context(), credential.TokenSpec{
		Type:  tokenType,
		AppID: h.appID,
	})
	if err != nil {
		http.Error(w, "failed to resolve token: "+err.Error(), http.StatusInternalServerError)
		h.logger.Printf("TOKEN_ERROR method=%s path=%s identity=%s error=%q", r.Method, sanitizePath(pathAndQuery), identity, sanitizeError(err))
		return
	}

	// 7. Build forwarding request. Scheme is pinned to https here (not taken
	// from the client-supplied target) so any future change to parseTarget
	// cannot regress the cleartext-leak protection.
	forwardURL := "https://" + targetHost + pathAndQuery
	forwardReq, err := http.NewRequestWithContext(r.Context(), r.Method, forwardURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "failed to create forward request", http.StatusInternalServerError)
		return
	}

	// Copy non-proxy headers
	for k, vs := range r.Header {
		if isProxyHeader(k) {
			continue
		}
		for _, v := range vs {
			forwardReq.Header.Add(k, v)
		}
	}

	// Strip any client-supplied auth headers. The sidecar is the sole source
	// of authentication material on the forwarded request; a client could
	// otherwise smuggle an extra Authorization/MCP token alongside the one
	// the sidecar injects below.
	forwardReq.Header.Del("Authorization")
	forwardReq.Header.Del(sidecar.HeaderMCPUAT)
	forwardReq.Header.Del(sidecar.HeaderMCPTAT)

	// 8. Inject real token into the header the client committed to in the
	// signature. Standard OpenAPI uses "Authorization: Bearer <token>"; MCP
	// uses "X-Lark-MCP-UAT: <token>" or "X-Lark-MCP-TAT: <token>".
	if authHeader == "Authorization" {
		forwardReq.Header.Set("Authorization", "Bearer "+tokenResult.Token)
	} else {
		forwardReq.Header.Set(authHeader, tokenResult.Token)
	}

	// 9. Forward request
	resp, err := h.forwardCl.Do(forwardReq)
	if err != nil {
		http.Error(w, "forward request failed: "+err.Error(), http.StatusBadGateway)
		h.logger.Printf("FORWARD_ERROR method=%s path=%s error=%q", r.Method, sanitizePath(pathAndQuery), sanitizeError(err))
		return
	}
	defer resp.Body.Close()

	// 10. Copy response back
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	// 11. Audit log
	h.logger.Printf("FORWARD method=%s path=%s identity=%s status=%d duration=%s",
		r.Method, sanitizePath(pathAndQuery), identity, resp.StatusCode, time.Since(start).Round(time.Millisecond))
}

// parseTarget validates X-Lark-Proxy-Target and returns the host portion for
// HMAC input and allowlist lookup. The target must be "https://<host>" with no
// path, query, fragment, userinfo, or non-https scheme. Rejecting these shapes
// closes a token-leak channel: a compromised sandbox holding PROXY_KEY could
// otherwise request cleartext HTTP forwarding (or inject a path to a different
// endpoint than the allowlist entry implies).
func parseTarget(target string) (host string, err error) {
	u, perr := url.Parse(target)
	if perr != nil {
		return "", fmt.Errorf("parse: %w", perr)
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("scheme must be https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("missing host")
	}
	if u.User != nil {
		return "", fmt.Errorf("userinfo not allowed")
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("path not allowed (got %q)", u.Path)
	}
	if u.RawQuery != "" {
		return "", fmt.Errorf("query not allowed")
	}
	if u.Fragment != "" {
		return "", fmt.Errorf("fragment not allowed")
	}
	return u.Host, nil
}
