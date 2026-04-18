// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar_demo

package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"strings"
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

func (h *proxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

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
	target := r.Header.Get(sidecar.HeaderProxyTarget)
	if target == "" {
		http.Error(w, "missing "+sidecar.HeaderProxyTarget, http.StatusBadRequest)
		return
	}

	// Extract host from target (e.g. "https://open.feishu.cn" → "open.feishu.cn")
	targetHost := target
	if idx := strings.Index(target, "://"); idx >= 0 {
		targetHost = target[idx+3:]
	}

	signature := r.Header.Get(sidecar.HeaderProxySignature)
	pathAndQuery := r.URL.RequestURI()
	if err := sidecar.Verify(h.key, r.Method, targetHost, pathAndQuery, claimedSHA, ts, signature); err != nil {
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

	// 5. Determine and validate identity
	identity := r.Header.Get(sidecar.HeaderProxyIdentity)
	if identity == "" {
		identity = sidecar.IdentityBot
	}
	if !h.allowedIDs[identity] {
		http.Error(w, "identity not allowed: "+identity, http.StatusForbidden)
		h.logger.Printf("REJECT method=%s path=%s reason=\"identity %s not allowed by strict mode\"", r.Method, sanitizePath(pathAndQuery), identity)
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

	// 7. Build forwarding request
	forwardURL := target + pathAndQuery
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

	// 8. Inject real token into the header specified by the client.
	// Standard OpenAPI uses "Authorization: Bearer <token>".
	// MCP uses "X-Lark-MCP-UAT: <token>" or "X-Lark-MCP-TAT: <token>".
	authHeader := r.Header.Get(sidecar.HeaderProxyAuthHeader)
	if authHeader == "" || authHeader == "Authorization" {
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
