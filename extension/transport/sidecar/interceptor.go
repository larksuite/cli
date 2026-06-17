// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar

// Package sidecar provides a transport interceptor for auth proxy mode. When
// LARKSUITE_CLI_AUTH_PROXY is set, outgoing sentinel-authenticated requests
// are rewritten to either a local HTTP sidecar or a remote HTTPS managed auth
// proxy. The interceptor strips placeholder credentials, injects proxy
// headers, and signs each request with HMAC-SHA256.
package sidecar

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/larksuite/cli/extension/transport"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/sidecar"
)

// Provider implements transport.Provider for the sidecar mode.
type Provider struct{}

func (p *Provider) Name() string { return "sidecar" }

// ResolveInterceptor returns a SidecarInterceptor when sidecar mode is active.
// Returns nil when sidecar mode is disabled or the proxy address is invalid;
// in the latter case a warning is emitted to stderr and requests fall back to
// the non-sidecar transport path (where the credential layer will typically
// block them for lack of a valid account).
func (p *Provider) ResolveInterceptor(ctx context.Context) transport.Interceptor {
	proxyAddr := os.Getenv(envvars.CliAuthProxy)
	if proxyAddr == "" {
		return nil
	}
	endpoint, err := sidecar.ParseProxyEndpoint(proxyAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: invalid %s, sidecar interceptor disabled: %v\n", envvars.CliAuthProxy, err)
		return nil
	}
	if err := requireTrustedRemoteProxy(endpoint); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: %s is set but remote auth proxy is not trusted: %v\n", envvars.CliAuthProxy, err)
		return nil
	}

	signingKey, proxySession := proxyAuthMaterial(endpoint.Mode)
	if signingKey == "" {
		fmt.Fprintf(os.Stderr, "WARNING: %s is set but required auth material is missing for %s auth proxy mode\n", envvars.CliAuthProxy, endpoint.Mode)
		return nil
	}
	if endpoint.Mode == sidecar.ProxyModeRemote && proxySession == "" {
		fmt.Fprintf(os.Stderr, "WARNING: %s is set but %s is missing for remote auth proxy mode\n", envvars.CliAuthProxy, envvars.CliProxySession)
		return nil
	}

	return &Interceptor{
		key:          []byte(signingKey),
		proxyScheme:  endpoint.Scheme,
		sidecarHost:  endpoint.Host,
		proxyMode:    endpoint.Mode,
		proxySession: proxySession,
	}
}

// Interceptor rewrites requests for the sidecar proxy.
type Interceptor struct {
	key          []byte            // HMAC signing key
	proxyScheme  string            // proxy scheme for URL rewriting
	sidecarHost  string            // proxy host[:port] for URL rewriting
	proxyMode    sidecar.ProxyMode // local sidecar or remote managed proxy
	proxySession string            // remote auth proxy session credential
}

// PreRoundTrip rewrites the request for sidecar routing when it carries a
// sentinel token. Requests without a sentinel token (e.g. pre-signed download
// URLs) are passed through unmodified.
//
// Supports two auth patterns:
//   - Standard OpenAPI: Authorization: Bearer <sentinel>
//   - MCP protocol:     X-Lark-MCP-UAT/TAT: <sentinel>
func (i *Interceptor) PreRoundTrip(req *http.Request) func(resp *http.Response, err error) {
	identity, authHeader := detectSentinel(req)
	if identity == "" {
		return nil // not a sidecar-managed request, pass through
	}

	// 1. Buffer the body first, before mutating any request state. A partial
	// read would sign a truncated body and cause a misleading HMAC mismatch
	// on the sidecar side; bail out early and let the request fall through
	// unmodified so the credential layer can surface an actionable error.
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		_ = req.Body.Close() // release original body (fd/pipe/etc.) after buffering
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: sidecar interceptor failed to read request body: %v\n", err)
			return nil
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		if req.GetBody != nil {
			req.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(bodyBytes)), nil
			}
		}
	}

	// 2. Save original target (scheme://host)
	originalScheme := "https"
	if req.URL.Scheme != "" {
		originalScheme = req.URL.Scheme
	}
	originalHost := req.URL.Host
	req.Header.Set(sidecar.HeaderProxyTarget, originalScheme+"://"+originalHost)

	// 3. Set identity and tell sidecar which header to inject real token into
	req.Header.Set(sidecar.HeaderProxyIdentity, identity)
	req.Header.Set(sidecar.HeaderProxyAuthHeader, authHeader)

	// 4. Strip placeholder auth header(s)
	req.Header.Del("Authorization")
	req.Header.Del(sidecar.HeaderMCPUAT)
	req.Header.Del(sidecar.HeaderMCPTAT)
	req.Header.Del(sidecar.HeaderProxySession)

	bodySHA := sidecar.BodySHA256(bodyBytes)
	req.Header.Set(sidecar.HeaderBodySHA256, bodySHA)

	pathAndQuery := req.URL.RequestURI()
	ts := sidecar.Timestamp()
	// Cover identity and authHeader in the signature so an on-path attacker
	// within the replay window cannot flip the injected token's identity or
	// redirect the token into a different header.
	sig := sidecar.Sign(i.key, sidecar.CanonicalRequest{
		Version:      sidecar.ProtocolV1,
		Method:       req.Method,
		Host:         originalHost,
		PathAndQuery: pathAndQuery,
		BodySHA256:   bodySHA,
		Timestamp:    ts,
		Identity:     identity,
		AuthHeader:   authHeader,
	})
	req.Header.Set(sidecar.HeaderProxyVersion, sidecar.ProtocolV1)
	req.Header.Set(sidecar.HeaderProxyTimestamp, ts)
	req.Header.Set(sidecar.HeaderProxySignature, sig)
	if i.proxyMode == sidecar.ProxyModeRemote && i.proxySession != "" {
		req.Header.Set(sidecar.HeaderProxySession, i.proxySession)
	}

	// 5. Rewrite URL to route through the configured auth proxy.
	proxyScheme := i.proxyScheme
	if proxyScheme == "" {
		proxyScheme = "http"
	}
	req.URL.Scheme = proxyScheme
	req.URL.Host = i.sidecarHost

	return nil // no post-hook needed
}

// detectSentinel checks both standard Authorization and MCP auth headers for
// sentinel tokens. Returns the identity ("user"/"bot") and the header name
// that carried the sentinel.
//
// Returns ("", "") when the request carries no sentinel token — typically
// requests that require no auth (e.g. pre-signed download URLs where the
// token is embedded in the URL query parameters).
func detectSentinel(req *http.Request) (identity, authHeader string) {
	// Check standard Authorization: Bearer <sentinel>
	if auth := req.Header.Get("Authorization"); auth != "" {
		token := strings.TrimPrefix(auth, "Bearer ")
		switch token {
		case sidecar.SentinelUAT:
			return sidecar.IdentityUser, "Authorization"
		case sidecar.SentinelTAT:
			return sidecar.IdentityBot, "Authorization"
		}
	}
	// Check MCP headers: X-Lark-MCP-UAT/TAT: <sentinel>
	if v := req.Header.Get(sidecar.HeaderMCPUAT); v == sidecar.SentinelUAT {
		return sidecar.IdentityUser, sidecar.HeaderMCPUAT
	}
	if v := req.Header.Get(sidecar.HeaderMCPTAT); v == sidecar.SentinelTAT {
		return sidecar.IdentityBot, sidecar.HeaderMCPTAT
	}
	return "", ""
}

func init() {
	proxyAddr := os.Getenv(envvars.CliAuthProxy)
	if proxyAddr == "" {
		return
	}
	if _, err := sidecar.ParseProxyEndpoint(proxyAddr); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: ignoring invalid %s: %v\n", envvars.CliAuthProxy, err)
		return
	}
	transport.Register(&Provider{})
}

func proxyAuthMaterial(mode sidecar.ProxyMode) (signingKey, proxySession string) {
	switch mode {
	case sidecar.ProxyModeRemote:
		return os.Getenv(envvars.CliProxyKey), os.Getenv(envvars.CliProxySession)
	case sidecar.ProxyModeLocal, "":
		return os.Getenv(envvars.CliProxyKey), ""
	default:
		return "", ""
	}
}

func requireTrustedRemoteProxy(endpoint sidecar.ProxyEndpoint) error {
	if endpoint.Mode != sidecar.ProxyModeRemote {
		return nil
	}
	cfg, err := core.LoadAuthProxyConfig()
	if err != nil {
		return fmt.Errorf("failed to load auth proxy trust config: %w", err)
	}
	if sidecar.IsTrustedRemoteProxyHost(endpoint.Host, cfg.TrustedHosts) {
		return nil
	}
	return fmt.Errorf("remote auth proxy host %q is not trusted; run `lark-cli config auth-proxy trust https://%s` outside the agent environment", endpoint.Host, endpoint.Host)
}
