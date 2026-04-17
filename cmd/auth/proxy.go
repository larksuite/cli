// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar

package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/sidecar"
	"github.com/larksuite/cli/internal/vfs"
)

// ProxyOptions holds all inputs for auth proxy.
type ProxyOptions struct {
	Factory *cmdutil.Factory
	Listen  string
	KeyFile string
	LogFile string
}

// NewCmdAuthProxy creates the auth proxy subcommand.
func NewCmdAuthProxy(f *cmdutil.Factory) *cobra.Command {
	opts := &ProxyOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "Start auth sidecar proxy for sandbox environments",
		Long: `Start an auth sidecar proxy that holds real credentials and forwards
authenticated requests on behalf of CLI instances running in sandboxes.

The proxy listens on HTTP. Sandbox CLI instances connect via the
LARKSUITE_CLI_AUTH_PROXY environment variable. Requests are authenticated
using HMAC-SHA256 with a shared key.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProxy(cmd.Context(), opts)
		},
	}
	cmdutil.DisableAuthCheck(cmd)

	cmd.Flags().StringVar(&opts.Listen, "listen", sidecar.DefaultListenAddr, "listen address (host:port)")
	cmd.Flags().StringVar(&opts.KeyFile, "key-file", "/var/run/lark-sidecar/proxy.key", "path to write the HMAC key")
	cmd.Flags().StringVar(&opts.LogFile, "log-file", "", "audit log file (optional, stderr if empty)")

	return cmd
}

func runProxy(ctx context.Context, opts *ProxyOptions) error {
	// Reject self-proxy: if this process inherited AUTH_PROXY, the sidecar
	// credential provider would activate and return sentinel tokens instead
	// of real ones, breaking the "trusted side holds real credentials" premise.
	if v := os.Getenv(envvars.CliAuthProxy); v != "" {
		return output.ErrWithHint(output.ExitValidation, "config",
			fmt.Sprintf("%s is set in this environment (%s); the sidecar server must not run in sidecar client mode", envvars.CliAuthProxy, v),
			fmt.Sprintf("unset %s before starting the sidecar server", envvars.CliAuthProxy))
	}

	listenAddr := opts.Listen
	if listenAddr == "" {
		return output.ErrValidation("invalid --listen address: empty")
	}

	// Generate HMAC key (32 bytes = 256 bits)
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return output.Errorf(output.ExitInternal, "crypto", "failed to generate HMAC key: %v", err)
	}
	keyHex := hex.EncodeToString(keyBytes)

	// Write key to file
	keyDir := filepath.Dir(opts.KeyFile)
	if err := vfs.MkdirAll(keyDir, 0700); err != nil {
		return output.Errorf(output.ExitInternal, "filesystem", "failed to create key directory: %v", err)
	}
	if err := vfs.WriteFile(opts.KeyFile, []byte(keyHex), 0600); err != nil {
		return output.Errorf(output.ExitInternal, "filesystem", "failed to write key file: %v", err)
	}

	// Set up audit logger
	var auditLogger *log.Logger
	if opts.LogFile != "" {
		f, err := vfs.OpenFile(opts.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return output.Errorf(output.ExitInternal, "filesystem", "failed to open log file: %v", err)
		}
		defer f.Close()
		auditLogger = log.New(f, "", log.LstdFlags)
	} else {
		auditLogger = log.New(os.Stderr, "[audit] ", log.LstdFlags)
	}

	// Resolve credentials from this (trusted) environment
	cred := opts.Factory.Credential

	// Resolve config for appID
	cfg, err := opts.Factory.Config()
	if err != nil {
		return output.Errorf(output.ExitAuth, "config", "failed to load config: %v", err)
	}

	// Listen on TCP
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return output.ErrWithHint(output.ExitNetwork, "listen",
			fmt.Sprintf("failed to listen on %s: %v", listenAddr, err),
			"check if the port is already in use or try a different --listen address")
	}
	defer listener.Close()

	// Build target allowlist from both brand endpoints (feishu + lark)
	// so the sidecar can serve clients regardless of their brand setting.
	allowedHosts := buildAllowedHosts(
		core.ResolveEndpoints(core.BrandFeishu),
		core.ResolveEndpoints(core.BrandLark),
	)

	// Build allowed identities from strict mode
	allowedIDs := buildAllowedIdentities(cfg)

	// Use the hex-encoded key (not raw bytes) as the HMAC key.
	// The client reads the hex string from PROXY_KEY env var and uses
	// []byte(hexString) directly, so the server must do the same.
	handler := &proxyHandler{
		key:          []byte(keyHex),
		cred:         cred,
		appID:        cfg.AppID,
		brand:        cfg.Brand,
		logger:       auditLogger,
		forwardCl:    newForwardClient(),
		allowedHosts: allowedHosts,
		allowedIDs:   allowedIDs,
	}

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}

	// Graceful shutdown on signal or context cancellation
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigCh)
		select {
		case <-sigCh:
		case <-ctx.Done():
		}
		auditLogger.Println("shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			auditLogger.Printf("shutdown error: %v", err)
		}
	}()

	// Print startup info
	keyPrefix := keyHex
	if len(keyPrefix) > 8 {
		keyPrefix = keyPrefix[:8]
	}
	proxyURL := "http://" + listenAddr
	keyBasename := filepath.Base(opts.KeyFile)
	fmt.Fprintf(os.Stderr, "Auth sidecar listening on %s\n", proxyURL)
	fmt.Fprintf(os.Stderr, "HMAC key prefix: %s\n", keyPrefix)
	fmt.Fprintf(os.Stderr, "Full key written to .../%s (mode 0600)\n", keyBasename)
	fmt.Fprintf(os.Stderr, "\nSet in sandbox:\n")
	fmt.Fprintf(os.Stderr, "  export %s=%q\n", envvars.CliAuthProxy, proxyURL)
	fmt.Fprintf(os.Stderr, "  export %s=\"<read from key file>\"\n", envvars.CliProxyKey)
	fmt.Fprintf(os.Stderr, "  export %s=%q\n", envvars.CliAppID, cfg.AppID)
	fmt.Fprintf(os.Stderr, "  export %s=%q\n", envvars.CliBrand, string(cfg.Brand))

	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return output.Errorf(output.ExitNetwork, "serve", "sidecar server exited unexpectedly: %v", err)
	}
	return nil
}

// newForwardClient creates an HTTP client for forwarding requests to the
// Lark API. It strips Authorization on cross-host redirects and disables
// proxy to prevent real tokens from leaking through environment proxies.
func newForwardClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // never proxy the trusted hop
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			if len(via) > 0 && req.URL.Host != via[0].URL.Host {
				req.Header.Del("Authorization")
				req.Header.Del(sidecar.HeaderMCPUAT)
				req.Header.Del(sidecar.HeaderMCPTAT)
			}
			return nil
		},
	}
}

// buildAllowedHosts extracts the set of allowed target hostnames from
// multiple brand endpoints so the sidecar can serve both feishu and lark clients.
func buildAllowedHosts(endpoints ...core.Endpoints) map[string]bool {
	hosts := make(map[string]bool)
	for _, ep := range endpoints {
		for _, u := range []string{ep.Open, ep.Accounts, ep.MCP} {
			if idx := strings.Index(u, "://"); idx >= 0 {
				hosts[u[idx+3:]] = true
			}
		}
	}
	return hosts
}

// buildAllowedIdentities returns the set of identities the sidecar is allowed to serve,
// based on the trusted-side strict mode / SupportedIdentities configuration.
func buildAllowedIdentities(cfg *core.CliConfig) map[string]bool {
	ids := make(map[string]bool)
	switch {
	case cfg.SupportedIdentities == 0: // unknown/unset → allow both
		ids[sidecar.IdentityUser] = true
		ids[sidecar.IdentityBot] = true
	case cfg.SupportedIdentities&1 != 0: // SupportsUser bit
		ids[sidecar.IdentityUser] = true
	}
	if cfg.SupportedIdentities == 0 || cfg.SupportedIdentities&2 != 0 { // SupportsBot bit
		ids[sidecar.IdentityBot] = true
	}
	return ids
}

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

	// 7. Inject real token into the header specified by the client.
	// Standard OpenAPI uses "Authorization: Bearer <token>".
	// MCP uses "X-Lark-MCP-UAT: <token>" or "X-Lark-MCP-TAT: <token>".
	authHeader := r.Header.Get(sidecar.HeaderProxyAuthHeader)
	if authHeader == "" || authHeader == "Authorization" {
		forwardReq.Header.Set("Authorization", "Bearer "+tokenResult.Token)
	} else {
		forwardReq.Header.Set(authHeader, tokenResult.Token)
	}

	// 8. Forward request
	resp, err := h.forwardCl.Do(forwardReq)
	if err != nil {
		http.Error(w, "forward request failed: "+err.Error(), http.StatusBadGateway)
		h.logger.Printf("FORWARD_ERROR method=%s path=%s error=%q", r.Method, sanitizePath(pathAndQuery), sanitizeError(err))
		return
	}
	defer resp.Body.Close()

	// 9. Copy response back
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	// 10. Audit log
	h.logger.Printf("FORWARD method=%s path=%s identity=%s status=%d duration=%s",
		r.Method, sanitizePath(pathAndQuery), identity, resp.StatusCode, time.Since(start).Round(time.Millisecond))
}

// sanitizePath strips query parameters and replaces ID-like path segments
// with ":id" to prevent document tokens, chat IDs, etc. from leaking into logs.
// Example: /open-apis/docx/v1/documents/doxcnXXXX/blocks → /open-apis/docx/v1/documents/:id/blocks
func sanitizePath(pathAndQuery string) string {
	// Strip query
	path := pathAndQuery
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	// Replace ID-like segments (8+ chars, not a pure API keyword)
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if looksLikeID(p) {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

// looksLikeID returns true if a path segment appears to be a resource identifier
// rather than an API route keyword. Heuristic: 8+ chars and contains a digit.
func looksLikeID(seg string) bool {
	if len(seg) < 8 {
		return false
	}
	for _, c := range seg {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

// sanitizeError returns a safe error string for logging, capped at 200 bytes
// to avoid dumping upstream response bodies into audit logs.
func sanitizeError(err error) string {
	s := err.Error()
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

// isProxyHeader returns true for headers specific to the sidecar protocol.
func isProxyHeader(key string) bool {
	switch http.CanonicalHeaderKey(key) {
	case http.CanonicalHeaderKey(sidecar.HeaderProxyTarget),
		http.CanonicalHeaderKey(sidecar.HeaderProxyIdentity),
		http.CanonicalHeaderKey(sidecar.HeaderProxySignature),
		http.CanonicalHeaderKey(sidecar.HeaderProxyTimestamp),
		http.CanonicalHeaderKey(sidecar.HeaderBodySHA256),
		http.CanonicalHeaderKey(sidecar.HeaderProxyAuthHeader):
		return true
	}
	return false
}
