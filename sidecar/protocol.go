// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package sidecar defines the wire protocol shared between the CLI client
// (running inside a sandbox) and the auth sidecar proxy (running in a
// trusted environment). Communication uses HTTP for a same-host sidecar, or
// HTTPS (TLS) for a remote sidecar.
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

// errNotSameHost is the shared error returned when a plaintext (http) sidecar
// address does not resolve to the same physical host as the sandbox. Kept in
// one place so tests can look for a stable marker.
func errNotSameHost(addr string) error {
	return fmt.Errorf("invalid proxy address %q: a plaintext (http) sidecar must be "+
		"loopback (127.0.0.1 / ::1) or a recognized same-host alias "+
		"(localhost, host.docker.internal, host.containers.internal, "+
		"host.lima.internal, gateway.docker.internal). "+
		"For a remote sidecar on another machine, use an https:// address instead", addr)
}

// ValidateProxyAddr validates the LARKSUITE_CLI_AUTH_PROXY value.
// Accepted formats:
//   - https://host[:port]  (remote sidecar; cross-machine allowed)
//   - http://host:port     (plaintext; same-host only)
//   - host:port            (bare address, treated as plaintext http; same-host only)
//
// Scheme policy:
//   - https:// — any valid host is allowed, including a remote central sidecar
//     on another machine. TLS provides confidentiality over the untrusted
//     network; the per-request HMAC signature provides integrity/auth.
//   - http:// (or bare host:port) — plaintext, allowed only when the host is
//     loopback (127.0.0.1 / ::1) or a recognized same-host alias (a virtual
//     same-host bridge that stays on the physical machine). For a remote
//     sidecar, use an https:// address instead.
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

	// Bare host:port (no scheme) — treated as plaintext http, so same-host only.
	if !strings.Contains(addr, "://") {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return fmt.Errorf("invalid proxy address %q: expected host:port or http(s)://host[:port]", addr)
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
	// userinfo (user:pass@) is rejected unconditionally (phishing vector).
	if u.User != nil {
		return fmt.Errorf("invalid proxy address %q: userinfo is not allowed", addr)
	}
	if u.Host == "" {
		return fmt.Errorf("invalid proxy address %q: missing host", addr)
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("invalid proxy address %q: path is not allowed", addr)
	}
	if u.RawQuery != "" {
		return fmt.Errorf("invalid proxy address %q: query is not allowed", addr)
	}
	if u.Fragment != "" {
		return fmt.Errorf("invalid proxy address %q: fragment is not allowed", addr)
	}

	switch u.Scheme {
	case "https":
		// Remote sidecar over TLS. Cross-machine is allowed: https provides
		// confidentiality over the network and the per-request HMAC signature
		// provides integrity/authentication, so a remote central sidecar is
		// supported without exposing credentials or signing material in clear.
		return nil
	case "http":
		// Plaintext: only safe on the same physical host (loopback or a virtual
		// same-host bridge). For a remote sidecar use an https:// address.
		// u.Hostname() strips the port and unwraps IPv6 brackets.
		if !isSameHost(u.Hostname()) {
			return errNotSameHost(addr)
		}
		return nil
	default:
		return fmt.Errorf("invalid proxy address %q: scheme must be http or https", addr)
	}
}

// ProxyHost extracts the host:port from an AUTH_PROXY URL.
// Input is expected to be an http:// or https:// URL like
// "http://127.0.0.1:16384" or "https://sidecar.mycorp.com".
// Returns the host[:port] portion for URL rewriting.
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

// ProxyScheme returns the URL scheme the CLI must use when routing to the
// sidecar: "https" for a TLS (remote) sidecar, otherwise "http" (same-host
// plaintext). Input is a value already accepted by ValidateProxyAddr.
//
// It parses the address (rather than a case-sensitive prefix check) so the
// result stays consistent with ValidateProxyAddr, which relies on url.Parse
// normalizing the scheme. Otherwise "HTTPS://host" — accepted as https by
// ValidateProxyAddr — would silently downgrade to plaintext http here,
// breaking the "remote must use TLS" boundary.
func ProxyScheme(authProxy string) string {
	if u, err := url.Parse(authProxy); err == nil && strings.EqualFold(u.Scheme, "https") {
		return "https"
	}
	return "http"
}
