// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package sidecar defines the wire protocol shared between the CLI client
// (running inside a sandbox) and an auth proxy. The proxy can be a local
// same-host sidecar over HTTP or a remote managed auth proxy over HTTPS.
package sidecar

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
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

	// HeaderProxySession authenticates requests to a remote managed auth
	// proxy. Local same-host sidecars use HeaderProxySignature with a local
	// HMAC key instead and must not require this header.
	HeaderProxySession = "X-Lark-Proxy-Session"
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

// ProxyMode classifies the auth proxy transport model.
type ProxyMode string

const (
	// ProxyModeLocal is a same-host sidecar reachable over HTTP.
	ProxyModeLocal ProxyMode = "local"
	// ProxyModeRemote is a managed auth proxy reachable over HTTPS.
	ProxyModeRemote ProxyMode = "remote"
)

// ProxyEndpoint is a validated proxy endpoint ready for request rewriting.
type ProxyEndpoint struct {
	Scheme string
	Host   string
	Mode   ProxyMode
}

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

var reservedRemoteProxyHosts = map[string]bool{
	"open.feishu.cn":         true,
	"open.larksuite.com":     true,
	"mcp.feishu.cn":          true,
	"mcp.larksuite.com":      true,
	"accounts.feishu.cn":     true,
	"accounts.larksuite.com": true,
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
//   - http://host:port  (local same-host sidecar)
//   - host:port         (local same-host sidecar, treated as http)
//   - https://host[:port] (remote managed auth proxy)
//
// Plain HTTP is allowed only for loopback or sameHostAliases. Remote managed
// proxies must use HTTPS because the auth proxy session and business request
// data traverse the network.
//
// Path, query, fragment, and userinfo are rejected to avoid ambiguous base
// paths, phishing-style URLs, and silently ignored policy hints.
//
// userinfo (user:pass@) is rejected unconditionally — the sidecar protocol
// does not use basic auth, and the syntactic slot exists only as a phishing
// vector (e.g. http://127.0.0.1@attacker.com).
//
// Returns an error if the value is not a valid proxy address.
func ValidateProxyAddr(addr string) error {
	_, err := ParseProxyEndpoint(addr)
	return err
}

// ParseProxyEndpoint validates and classifies an auth proxy address.
func ParseProxyEndpoint(addr string) (ProxyEndpoint, error) {
	if addr == "" {
		return ProxyEndpoint{}, fmt.Errorf("proxy address is empty")
	}

	// Bare host:port (no scheme) — validate as a net address.
	if !strings.Contains(addr, "://") {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return ProxyEndpoint{}, fmt.Errorf("invalid proxy address %q: expected host:port, http://host:port, or https://host[:port]", addr)
		}
		if host == "" || port == "" {
			return ProxyEndpoint{}, fmt.Errorf("invalid proxy address %q: host and port must not be empty", addr)
		}
		if err := validatePort(port); err != nil {
			return ProxyEndpoint{}, fmt.Errorf("invalid proxy address %q: %w", addr, err)
		}
		if !isSameHost(host) {
			return ProxyEndpoint{}, errNotSameHost(addr)
		}
		return ProxyEndpoint{Scheme: "http", Host: net.JoinHostPort(host, port), Mode: ProxyModeLocal}, nil
	}

	u, err := url.Parse(addr)
	if err != nil {
		return ProxyEndpoint{}, fmt.Errorf("invalid proxy address %q: %w", addr, err)
	}
	if u.User != nil {
		return ProxyEndpoint{}, fmt.Errorf("invalid proxy address %q: userinfo is not allowed", addr)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ProxyEndpoint{}, fmt.Errorf("invalid proxy address %q: scheme must be http or https", addr)
	}
	if u.Host == "" {
		return ProxyEndpoint{}, fmt.Errorf("invalid proxy address %q: missing host", addr)
	}
	host, err := validateURLAuthority(addr, u)
	if err != nil {
		return ProxyEndpoint{}, err
	}
	if u.Path != "" && u.Path != "/" {
		return ProxyEndpoint{}, fmt.Errorf("invalid proxy address %q: path is not allowed", addr)
	}
	if u.RawQuery != "" {
		return ProxyEndpoint{}, fmt.Errorf("invalid proxy address %q: query is not allowed", addr)
	}
	if u.Fragment != "" {
		return ProxyEndpoint{}, fmt.Errorf("invalid proxy address %q: fragment is not allowed", addr)
	}

	switch u.Scheme {
	case "http":
		// u.Hostname() strips the port and unwraps IPv6 brackets.
		if !isSameHost(host) {
			return ProxyEndpoint{}, errNotSameHost(addr)
		}
		return ProxyEndpoint{Scheme: "http", Host: u.Host, Mode: ProxyModeLocal}, nil
	case "https":
		if isReservedRemoteProxyHost(u.Host) {
			return ProxyEndpoint{}, fmt.Errorf("invalid proxy address %q: host %q is a Lark/Feishu upstream endpoint, not an auth proxy", addr, u.Host)
		}
		return ProxyEndpoint{Scheme: "https", Host: u.Host, Mode: ProxyModeRemote}, nil
	}
	panic("unreachable proxy scheme validation")
}

func validateURLAuthority(addr string, u *url.URL) (string, error) {
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("invalid proxy address %q: missing host", addr)
	}
	if strings.HasSuffix(u.Host, ":") {
		return "", fmt.Errorf("invalid proxy address %q: port must not be empty", addr)
	}
	if port := u.Port(); port != "" {
		if err := validatePort(port); err != nil {
			return "", fmt.Errorf("invalid proxy address %q: %w", addr, err)
		}
	}
	return host, nil
}

func validatePort(port string) error {
	n, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid port %q", port)
	}
	if n < 1 || n > 65535 {
		return fmt.Errorf("port %q out of range", port)
	}
	return nil
}

func canonicalHTTPSAuthority(authority string) (string, error) {
	u, err := url.Parse("https://" + authority)
	if err != nil {
		return "", fmt.Errorf("invalid proxy host %q: %w", authority, err)
	}
	if u.Host == "" || u.Scheme != "https" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("invalid proxy host %q", authority)
	}
	host, err := validateURLAuthority(authority, u)
	if err != nil {
		return "", err
	}
	host = strings.ToLower(host)
	port := u.Port()
	if port == "" || port == "443" {
		if strings.Contains(host, ":") {
			return "[" + host + "]", nil
		}
		return host, nil
	}
	return net.JoinHostPort(host, port), nil
}

// NormalizeRemoteProxyTrustHost canonicalizes config entries for trusted
// remote HTTPS auth proxies. It accepts either "https://host[:port]" or
// "host[:port]" and strips the default HTTPS port.
func NormalizeRemoteProxyTrustHost(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("trusted auth proxy host is empty")
	}
	if strings.Contains(value, "://") {
		endpoint, err := ParseProxyEndpoint(value)
		if err != nil {
			return "", err
		}
		if endpoint.Mode != ProxyModeRemote {
			return "", fmt.Errorf("trusted auth proxy must use https")
		}
		return canonicalHTTPSAuthority(endpoint.Host)
	}
	return canonicalHTTPSAuthority(value)
}

// IsTrustedRemoteProxyHost reports whether endpointHost matches the trusted
// remote auth proxy host config. Entries without a port trust only the default
// HTTPS port; non-default ports must be listed explicitly.
func IsTrustedRemoteProxyHost(endpointHost string, trustedHosts []string) bool {
	want, err := canonicalHTTPSAuthority(endpointHost)
	if err != nil {
		return false
	}
	for _, trusted := range trustedHosts {
		got, err := NormalizeRemoteProxyTrustHost(trusted)
		if err != nil {
			continue
		}
		if got == want {
			return true
		}
	}
	return false
}

// ParseProxyTarget validates X-Lark-Proxy-Target and returns the authority
// used for HMAC input and upstream allowlist checks. Targets must be HTTPS
// origins only: no userinfo, path, query, or fragment.
func ParseProxyTarget(target string) (string, error) {
	u, err := url.Parse(target)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("scheme must be https, got %q", u.Scheme)
	}
	if u.User != nil {
		return "", fmt.Errorf("userinfo not allowed")
	}
	if u.Host == "" {
		return "", fmt.Errorf("missing host")
	}
	if _, err := validateURLAuthority(target, u); err != nil {
		return "", err
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

func isReservedRemoteProxyHost(authority string) bool {
	host, err := canonicalHTTPSAuthority(authority)
	if err != nil {
		return false
	}
	return reservedRemoteProxyHosts[host]
}

// ProxyHost extracts the host:port from an AUTH_PROXY URL.
// Input is expected to be an HTTP URL like "http://127.0.0.1:16384".
// Returns the host:port portion for URL rewriting.
func ProxyHost(authProxy string) string {
	if endpoint, err := ParseProxyEndpoint(authProxy); err == nil {
		return endpoint.Host
	}
	// Strip scheme
	host := authProxy
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	// Strip trailing slash
	host = strings.TrimRight(host, "/")
	return host
}
