// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package validate

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const (
	defaultDownloadMaxRedirects = 5
)

// DownloadHTTPClientOptions controls redirect and scheme behavior for
// untrusted-source downloads.
type DownloadHTTPClientOptions struct {
	// AllowHTTP controls whether plain HTTP URLs are permitted.
	// If false, any HTTP URL (initial or redirect target) is rejected.
	AllowHTTP bool
	// MaxRedirects limits follow-up redirects. Zero or negative uses default.
	MaxRedirects int
}

func isRestrictedDownloadIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 10 || v4[0] == 127 {
			return true
		}
		if v4[0] == 169 && v4[1] == 254 {
			return true
		}
		if v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31 {
			return true
		}
		if v4[0] == 192 && v4[1] == 168 {
			return true
		}
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 { // RFC6598 CGNAT
			return true
		}
		if v4[0] == 198 && (v4[1] == 18 || v4[1] == 19) { // RFC2544 benchmarking
			return true
		}
		return false
	}
	if ip.IsPrivate() {
		return true
	}
	ip16 := ip.To16()
	if ip16 == nil {
		return true
	}
	if ip16[0]&0xfe == 0xfc { // fc00::/7 unique local address
		return true
	}
	return false
}

// ValidateDownloadSourceURL validates a download URL and blocks local/internal targets.
func ValidateDownloadSourceURL(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http/https URLs are supported")
	}
	host := strings.TrimSpace(strings.ToLower(u.Hostname()))
	if host == "" {
		return fmt.Errorf("URL host is required")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("local/internal host is not allowed")
	}
	if ip := net.ParseIP(host); ip != nil {
		if isRestrictedDownloadIP(ip) {
			return fmt.Errorf("local/internal host is not allowed")
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("failed to resolve host")
	}
	if len(ips) == 0 {
		return fmt.Errorf("failed to resolve host")
	}
	for _, ip := range ips {
		if isRestrictedDownloadIP(ip) {
			return fmt.Errorf("local/internal host is not allowed")
		}
	}
	return nil
}

// NewDownloadHTTPClient clones base client and enforces download-safe redirect
// and connection rules for untrusted URLs.
func NewDownloadHTTPClient(base *http.Client, opts DownloadHTTPClientOptions) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	if opts.MaxRedirects <= 0 {
		opts.MaxRedirects = defaultDownloadMaxRedirects
	}

	cloned := *base
	cloned.Transport = cloneDownloadTransport(base.Transport)
	cloned.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= opts.MaxRedirects {
			return fmt.Errorf("too many redirects")
		}
		if len(via) > 0 {
			prev := via[len(via)-1]
			if strings.EqualFold(prev.URL.Scheme, "https") && strings.EqualFold(req.URL.Scheme, "http") {
				return fmt.Errorf("redirect from https to http is not allowed")
			}
		}
		if !opts.AllowHTTP && !strings.EqualFold(req.URL.Scheme, "https") {
			return fmt.Errorf("only https URLs are supported")
		}
		if err := ValidateDownloadSourceURL(req.Context(), req.URL.String()); err != nil {
			return fmt.Errorf("blocked redirect target: %w", err)
		}
		return nil
	}

	return &cloned
}

func cloneDownloadTransport(base http.RoundTripper) *http.Transport {
	var cloned *http.Transport
	if src, ok := base.(*http.Transport); ok && src != nil {
		cloned = src.Clone()
	} else {
		if def, ok := http.DefaultTransport.(*http.Transport); ok && def != nil {
			cloned = def.Clone()
		} else {
			cloned = &http.Transport{}
		}
	}

	origDial := cloned.DialContext
	cloned.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dialConn(ctx, origDial, network, addr)
		if err != nil {
			return nil, err
		}
		if err := validateConnRemoteIP(conn); err != nil {
			conn.Close()
			return nil, err
		}
		return conn, nil
	}

	if cloned.DialTLSContext != nil {
		origDialTLS := cloned.DialTLSContext
		cloned.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dialConn(ctx, origDialTLS, network, addr)
			if err != nil {
				return nil, err
			}
			if err := validateConnRemoteIP(conn); err != nil {
				conn.Close()
				return nil, err
			}
			return conn, nil
		}
	}

	return cloned
}

// DialContextFunc is the signature for DialContext / DialTLSContext.
type DialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// WrapDialContextWithIPCheck wraps a DialContext function to validate the
// remote IP after connection, rejecting local/internal addresses (SSRF protection).
func WrapDialContextWithIPCheck(origDial DialContextFunc) DialContextFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dialConn(ctx, origDial, network, addr)
		if err != nil {
			return nil, err
		}
		if err := validateConnRemoteIP(conn); err != nil {
			conn.Close()
			return nil, err
		}
		return conn, nil
	}
}

func dialConn(ctx context.Context, dialFn func(context.Context, string, string) (net.Conn, error), network, addr string) (net.Conn, error) {
	if dialFn != nil {
		return dialFn(ctx, network, addr)
	}
	var d net.Dialer
	return d.DialContext(ctx, network, addr)
}

func validateConnRemoteIP(conn net.Conn) error {
	if conn == nil {
		return fmt.Errorf("nil connection")
	}
	raddr := conn.RemoteAddr()
	if raddr == nil {
		return fmt.Errorf("missing remote address")
	}
	host, _, err := net.SplitHostPort(raddr.String())
	if err != nil {
		host = raddr.String()
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return fmt.Errorf("invalid remote IP")
	}
	if isRestrictedDownloadIP(ip) {
		return fmt.Errorf("local/internal host is not allowed")
	}
	return nil
}
