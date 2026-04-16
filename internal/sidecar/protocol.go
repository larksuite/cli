// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package sidecar defines the wire protocol shared between the CLI client
// (running inside a sandbox) and the auth sidecar proxy (running in a
// trusted environment). Communication uses plain HTTP.
package sidecar

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Proxy request headers set by the CLI transport interceptor.
const (
	// HeaderProxyTarget carries the original request host (e.g. "open.feishu.cn").
	HeaderProxyTarget = "X-Lark-Proxy-Target"

	// HeaderProxyIdentity carries the resolved identity type ("user" or "bot").
	HeaderProxyIdentity = "X-Lark-Proxy-Identity"

	// HeaderProxySignature carries the HMAC-SHA256 hex signature.
	HeaderProxySignature = "X-Lark-Proxy-Signature"

	// HeaderProxyTimestamp carries the Unix epoch seconds string used in signing.
	HeaderProxyTimestamp = "X-Lark-Proxy-Timestamp"

	// HeaderBodySHA256 carries the hex-encoded SHA-256 digest of the request body.
	HeaderBodySHA256 = "X-Lark-Body-SHA256"

	// HeaderProxyAuthHeader tells the sidecar which header to inject the real
	// token into. Defaults to "Authorization" for standard OpenAPI requests.
	// MCP requests use "X-Lark-MCP-UAT" or "X-Lark-MCP-TAT".
	HeaderProxyAuthHeader = "X-Lark-Proxy-Auth-Header"
)

// MCP auth headers used by the Lark MCP protocol.
const (
	HeaderMCPUAT = "X-Lark-MCP-UAT"
	HeaderMCPTAT = "X-Lark-MCP-TAT"
)

// Sentinel token values returned by the noop credential provider.
// These are placeholder strings that flow through the SDK auth pipeline
// but are stripped by the transport interceptor before reaching the sidecar.
const (
	SentinelUAT = "sidecar-managed-uat" // User Access Token placeholder
	SentinelTAT = "sidecar-managed-tat" // Tenant Access Token placeholder
)

// IdentityUser and IdentityBot are the wire values for HeaderProxyIdentity.
const (
	IdentityUser = "user"
	IdentityBot  = "bot"
)

// MaxTimestampDrift is the maximum allowed difference (in seconds) between
// the request timestamp and the server's current time.
const MaxTimestampDrift = 60

// DefaultListenAddr is the default sidecar listen address (localhost only).
const DefaultListenAddr = "127.0.0.1:16384"

// ValidateProxyAddr validates the LARKSUITE_CLI_AUTH_PROXY value.
// Accepted formats:
//   - http://host:port  or  https://host:port
//   - host:port         (bare address, treated as http)
//
// Returns an error if the value is not a valid proxy address.
func ValidateProxyAddr(addr string) error {
	if addr == "" {
		return fmt.Errorf("proxy address is empty")
	}

	// Bare host:port (no scheme) — validate as a net address.
	if !strings.Contains(addr, "://") {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return fmt.Errorf("invalid proxy address %q: expected host:port or http(s)://host:port", addr)
		}
		if host == "" || port == "" {
			return fmt.Errorf("invalid proxy address %q: host and port must not be empty", addr)
		}
		return nil
	}

	u, err := url.Parse(addr)
	if err != nil {
		return fmt.Errorf("invalid proxy address %q: %w", addr, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid proxy address %q: scheme must be http or https", addr)
	}
	if u.Host == "" {
		return fmt.Errorf("invalid proxy address %q: missing host", addr)
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("invalid proxy address %q: path is not allowed", addr)
	}
	return nil
}

// ProxyHost extracts the host:port from an AUTH_PROXY URL.
// Input is expected to be an HTTP URL like "http://127.0.0.1:16384".
// Returns the host:port portion for URL rewriting.
func ProxyHost(authProxy string) string {
	// Strip scheme
	host := authProxy
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	// Strip trailing slash
	host = strings.TrimRight(host, "/")
	return host
}
