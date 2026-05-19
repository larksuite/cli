// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar_multi_tenant_demo

package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/vfs"
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
	authBridge   *authBridge

	// Per-client key isolation: keyHex → clientName.
	// Data-plane requests are signed with a client-specific key;
	// the matched key determines which client (and thus which user
	// token) to use. Protected by ckMu.
	ckMu       sync.RWMutex
	clientKeys map[string]clientKeyEntry
	keysDir    string // directory to scan for *.key files (excluding proxy.key)
}

type clientKeyEntry struct {
	key        []byte
	clientName string
}

// loadClientKeys scans keysDir for *.key files (excluding the shared
// proxy.key) and populates the clientKeys map. The filename stem (without
// .key) becomes the client identity. No naming convention is enforced.
// Safe to call multiple times (e.g. on cache miss).
func (h *proxyHandler) loadClientKeys() {
	if h.keysDir == "" {
		return
	}
	entries, err := vfs.ReadDir(h.keysDir)
	if err != nil {
		h.logger.Printf("KEYS_SCAN_ERROR dir=%s error=%q", h.keysDir, err.Error())
		return
	}

	sharedKeyHex := string(h.key)

	newKeys := make(map[string]clientKeyEntry)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".key") {
			continue
		}
		clientName := strings.TrimSuffix(name, ".key")
		if clientName == "" || clientName == "proxy" {
			continue
		}
		data, err := vfs.ReadFile(filepath.Join(h.keysDir, name))
		if err != nil {
			continue
		}
		keyHex := strings.TrimSpace(string(data))
		if len(keyHex) != 64 {
			h.logger.Printf("KEYS_SCAN_SKIP file=%s reason=\"key length %d, expected 64\"", name, len(keyHex))
			continue
		}
		if keyHex == sharedKeyHex {
			h.logger.Printf("KEYS_SCAN_SKIP file=%s reason=\"collides with shared proxy key\"", name)
			continue
		}
		if existing, ok := newKeys[keyHex]; ok {
			h.logger.Printf("KEYS_SCAN_SKIP file=%s reason=\"duplicate key, already loaded for client %s\"", name, existing.clientName)
			continue
		}
		newKeys[keyHex] = clientKeyEntry{key: []byte(keyHex), clientName: clientName}
	}

	h.ckMu.Lock()
	h.clientKeys = newKeys
	h.ckMu.Unlock()

	if len(newKeys) > 0 {
		names := make([]string, 0, len(newKeys))
		for _, e := range newKeys {
			names = append(names, e.clientName)
		}
		h.logger.Printf("KEYS_LOADED count=%d clients=%v", len(newKeys), names)
	}
}

// verifyWithClientKeys tries each client key to verify the HMAC.
// Returns the client name on success, or empty string + error if none match.
func (h *proxyHandler) verifyWithClientKeys(cr sidecar.CanonicalRequest, signature string) (string, error) {
	h.ckMu.RLock()
	keys := h.clientKeys
	h.ckMu.RUnlock()

	for _, entry := range keys {
		if err := sidecar.Verify(entry.key, cr, signature); err == nil {
			return entry.clientName, nil
		}
	}

	// Cache miss: rescan keys directory and retry once
	h.loadClientKeys()

	h.ckMu.RLock()
	keys = h.clientKeys
	h.ckMu.RUnlock()

	for _, entry := range keys {
		if err := sidecar.Verify(entry.key, cr, signature); err == nil {
			return entry.clientName, nil
		}
	}

	return "", fmt.Errorf("no client key matched")
}

// allowedAuthHeaders lists the only header names the sidecar will inject real
// tokens into.
var allowedAuthHeaders = map[string]bool{
	"Authorization":      true,
	sidecar.HeaderMCPUAT: true,
	sidecar.HeaderMCPTAT: true,
}

func (h *proxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Route management endpoints to authBridge (different HMAC scheme)
	if len(r.URL.Path) > 10 && r.URL.Path[:10] == "/_sidecar/" {
		if h.authBridge != nil {
			h.authBridge.ServeHTTP(w, r)
		} else {
			http.Error(w, "auth bridge not configured", http.StatusNotImplemented)
		}
		return
	}

	start := time.Now()

	// 0. Check protocol version
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
	cr := sidecar.CanonicalRequest{
		Version:      version,
		Method:       r.Method,
		Host:         targetHost,
		PathAndQuery: pathAndQuery,
		BodySHA256:   claimedSHA,
		Timestamp:    ts,
		Identity:     identity,
		AuthHeader:   authHeader,
	}

	// Try the primary (shared) key first, then per-client keys.
	// matchedClient is empty when using the shared key.
	var matchedClient string
	if err := sidecar.Verify(h.key, cr, signature); err != nil {
		client, clientErr := h.verifyWithClientKeys(cr, signature)
		if clientErr != nil {
			http.Error(w, "HMAC verification failed: "+err.Error(), http.StatusUnauthorized)
			h.logger.Printf("REJECT method=%s path=%s reason=%q", r.Method, sanitizePath(pathAndQuery), "no key matched")
			return
		}
		matchedClient = client
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

	// 5.5 Validate auth-header
	if !allowedAuthHeaders[authHeader] {
		http.Error(w, "auth-header not allowed: "+authHeader, http.StatusForbidden)
		h.logger.Printf("REJECT method=%s path=%s reason=\"auth-header %s not in allowlist\"", r.Method, sanitizePath(pathAndQuery), authHeader)
		return
	}

	// 6. Resolve real token
	// UAT (user identity): per-client isolation via matched PROXY_KEY.
	// TAT (bot identity): shared credential provider (app-level).
	var resolvedToken string
	if identity == sidecar.IdentityUser && h.authBridge != nil {
		token, err := h.authBridge.resolveUserTokenByClient(matchedClient)
		if err != nil {
			http.Error(w, "failed to resolve user token: "+err.Error(), http.StatusInternalServerError)
			h.logger.Printf("TOKEN_ERROR method=%s path=%s identity=%s client=%s error=%q",
				r.Method, sanitizePath(pathAndQuery), identity, matchedClient, sanitizeError(err))
			return
		}
		resolvedToken = token
	} else {
		tokenResult, err := h.cred.ResolveToken(r.Context(), credential.TokenSpec{
			Type:  credential.TokenTypeTAT,
			AppID: h.appID,
		})
		if err != nil {
			http.Error(w, "failed to resolve token: "+err.Error(), http.StatusInternalServerError)
			h.logger.Printf("TOKEN_ERROR method=%s path=%s identity=%s error=%q", r.Method, sanitizePath(pathAndQuery), identity, sanitizeError(err))
			return
		}
		resolvedToken = tokenResult.Token
	}

	// 7. Build forwarding request
	forwardURL := "https://" + targetHost + pathAndQuery
	forwardReq, err := http.NewRequestWithContext(r.Context(), r.Method, forwardURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "failed to create forward request", http.StatusInternalServerError)
		return
	}

	for k, vs := range r.Header {
		if isProxyHeader(k) {
			continue
		}
		for _, v := range vs {
			forwardReq.Header.Add(k, v)
		}
	}

	forwardReq.Header.Del("Authorization")
	forwardReq.Header.Del(sidecar.HeaderMCPUAT)
	forwardReq.Header.Del(sidecar.HeaderMCPTAT)

	// 8. Inject real token
	if authHeader == "Authorization" {
		forwardReq.Header.Set("Authorization", "Bearer "+resolvedToken)
	} else {
		forwardReq.Header.Set(authHeader, resolvedToken)
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
	clientTag := ""
	if matchedClient != "" {
		clientTag = " client=" + matchedClient
	}
	h.logger.Printf("FORWARD method=%s path=%s identity=%s status=%d duration=%s%s",
		r.Method, sanitizePath(pathAndQuery), identity, resp.StatusCode, time.Since(start).Round(time.Millisecond), clientTag)
}

// parseTarget validates X-Lark-Proxy-Target and returns the host portion.
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
