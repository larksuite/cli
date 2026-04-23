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

// ProtocolV1 is the wire-protocol version string embedded in every signed
// request. Servers must reject requests whose HeaderProxyVersion is not a
// version they understand. Bump this constant (and update the canonical
// string) for any breaking change to signing inputs.
const ProtocolV1 = "v1"

// Proxy request headers set by the CLI transport interceptor.
const (
	// HeaderProxyVersion carries the wire-protocol version (e.g. ProtocolV1).
	// Servers must reject requests whose version they do not understand. The
	// value is also included in the canonical signing string so that a request
	// signed for one version cannot be replayed as another.
	HeaderProxyVersion = "X-Lark-Proxy-Version"

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

// sameHostAliases names DNS aliases commonly used to reach the host running
// the sandbox across a container / VM boundary. Traffic to these names stays
// on the physical machine (via a virtual bridge), so a plaintext sidecar
// channel still satisfies the sidecar pattern's same-host confidentiality
// requirement. Adding to this list has real security implications — only add
// names that are universally same-host by the runtime's design.
var sameHostAliases = map[string]bool{
	"localhost":                true, // universal
	"host.docker.internal":     true, // Docker Desktop (macOS / Windows)
	"host.containers.internal": true, // Podman Desktop
	"host.lima.internal":       true, // Lima / colima / rancher-desktop
	"gateway.docker.internal":  true, // Docker Desktop alt name
}

// isSameHost returns true when host is either a loopback IP or a recognized
// same-host DNS alias. Does not perform DNS resolution — a tampered /etc/hosts
// that points an alias elsewhere is out of scope (attacker with that access
// already has ambient control of the machine).
func isSameHost(host string) bool {
	if sameHostAliases[host] {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// errNotSameHost is the shared error returned when the sidecar address does
// not resolve to the same physical host as the sandbox. Kept in one place so
// tests can look for a stable marker.
func errNotSameHost(addr string) error {
	return fmt.Errorf("invalid proxy address %q: host must be loopback "+
		"(127.0.0.1 / ::1) or a recognized same-host alias "+
		"(localhost, host.docker.internal, host.containers.internal, "+
		"host.lima.internal, gateway.docker.internal). "+
		"The sidecar must run on the same physical machine as the sandbox — "+
		"cross-machine deployment is not a sidecar and is not supported", addr)
}

// ValidateProxyAddr validates the LARKSUITE_CLI_AUTH_PROXY value.
// Accepted formats:
//   - http://host:port
//   - host:port         (bare address, treated as http)
//
// Host must be loopback or in sameHostAliases. The sidecar pattern is
// inherently same-machine; cross-machine deployment is a different product
// and is not supported by this feature.
//
// https:// is rejected because sidecar is a same-host pattern: loopback
// and virtual same-host bridges don't traverse any untrusted medium, so
// TLS adds no security. Cross-machine deployment is out of scope (see the
// host constraint above), so there is no scenario today where https
// provides a real benefit over http on loopback.
//
// userinfo (user:pass@) is rejected unconditionally — the sidecar protocol
// does not use basic auth, and the syntactic slot exists only as a phishing
// vector (e.g. http://127.0.0.1@attacker.com).
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
			return fmt.Errorf("invalid proxy address %q: expected host:port or http://host:port", addr)
		}
		if host == "" || port == "" {
			return fmt.Errorf("invalid proxy address %q: host and port must not be empty", addr)
		}
		if !isSameHost(host) {
			return errNotSameHost(addr)
		}
		return nil
	}

	u, err := url.Parse(addr)
	if err != nil {
		return fmt.Errorf("invalid proxy address %q: %w", addr, err)
	}
	if u.User != nil {
		return fmt.Errorf("invalid proxy address %q: userinfo is not allowed", addr)
	}
	if u.Scheme == "https" {
		return fmt.Errorf("invalid proxy address %q: use http:// — sidecar is "+
			"same-host only (loopback or virtual same-host bridge), so TLS adds "+
			"no security; cross-machine deployment is out of scope", addr)
	}
	if u.Scheme != "http" {
		return fmt.Errorf("invalid proxy address %q: scheme must be http", addr)
	}
	if u.Host == "" {
		return fmt.Errorf("invalid proxy address %q: missing host", addr)
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("invalid proxy address %q: path is not allowed", addr)
	}
	// u.Hostname() strips the port and unwraps IPv6 brackets.
	if !isSameHost(u.Hostname()) {
		return errNotSameHost(addr)
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
