// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package validate

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/larksuite/cli/errs"
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
		if v4[0] == 0 { // RFC 1122 "this network"
			return true
		}
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
		if v4[0] >= 240 {
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
	_, err = resolveDownloadHost(ctx, u.Hostname(), net.DefaultResolver.LookupIP)
	return err
}

type downloadLookupIPFunc func(context.Context, string, string) ([]net.IP, error)

func resolveDownloadHost(ctx context.Context, rawHost string, lookupIP downloadLookupIPFunc) ([]net.IP, error) {
	host := strings.TrimSpace(strings.ToLower(rawHost))
	if host == "" {
		return nil, fmt.Errorf("URL host is required")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil, fmt.Errorf("local/internal host is not allowed")
	}
	if ip := net.ParseIP(host); ip != nil {
		if isRestrictedDownloadIP(ip) {
			return nil, fmt.Errorf("local/internal host is not allowed")
		}
		return []net.IP{ip}, nil
	}
	if lookupIP == nil {
		lookupIP = net.DefaultResolver.LookupIP
	}
	ips, err := lookupIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve host")
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("failed to resolve host")
	}
	for _, ip := range ips {
		if isRestrictedDownloadIP(ip) {
			return nil, fmt.Errorf("local/internal host is not allowed")
		}
	}
	return ips, nil
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
	cloned.Transport = &downloadSchemeTransport{
		base:      cloneDownloadTransport(base.Transport),
		allowHTTP: opts.AllowHTTP,
	}
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

type downloadSchemeTransport struct {
	base      http.RoundTripper
	allowHTTP bool
}

func (t *downloadSchemeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, errs.NewInternalError(
			errs.SubtypeUnknown,
			"download transport received a nil request",
		)
	}
	switch {
	case strings.EqualFold(req.URL.Scheme, "https"):
	case t.allowHTTP && strings.EqualFold(req.URL.Scheme, "http"):
	default:
		return nil, errs.NewSecurityPolicyError(
			errs.SubtypeAccessDenied,
			"only https URLs are supported",
		)
	}
	return t.base.RoundTrip(req)
}

type selectedDownloadProxyKey struct{}

type proxyAwareDownloadTransport struct {
	selectProxy func(*http.Request) (*url.URL, error)
	direct      http.RoundTripper
	proxied     *http.Transport
	lookupIP    downloadLookupIPFunc

	mu                 sync.Mutex
	proxiedByTLSServer map[string]*http.Transport
}

func (t *proxyAwareDownloadTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("download transport received a nil request")
	}
	proxyURL, err := t.selectProxy(req)
	if err != nil {
		return nil, err
	}
	if proxyURL == nil {
		return t.direct.RoundTrip(req)
	}

	targetIPs, err := resolveDownloadHost(req.Context(), req.URL.Hostname(), t.lookupIP)
	if err != nil {
		return nil, errs.NewSecurityPolicyError(
			errs.SubtypeAccessDenied,
			"blocked download target: %v",
			err,
		).WithCause(err)
	}
	if strings.EqualFold(req.URL.Scheme, "http") && net.ParseIP(req.URL.Hostname()) == nil {
		// HTTP proxies cannot pin the target IP separately from the Host header.
		return nil, errs.NewSecurityPolicyError(
			errs.SubtypeAccessDenied,
			"plain HTTP hostname downloads through a proxy are not allowed",
		).WithHint("use HTTPS or a literal public IP")
	}

	selected := *proxyURL
	proxied := t.proxied
	if strings.EqualFold(req.URL.Scheme, "https") {
		proxied = t.proxiedTransportForTLSServer(req.URL.Hostname())
	}

	var lastErr error
	for index, targetIP := range targetIPs {
		proxiedReq, pinErr := pinDownloadRequestTargetToIP(req, targetIP)
		if pinErr != nil {
			return nil, pinErr
		}
		ctx := context.WithValue(proxiedReq.Context(), selectedDownloadProxyKey{}, &selected)
		proxiedReq = proxiedReq.WithContext(ctx)

		resp, roundTripErr := proxied.RoundTrip(proxiedReq)
		if roundTripErr == nil {
			if resp != nil {
				// Hide the internal pinned URL from redirect handling.
				resp.Request = req
			}
			return resp, nil
		}
		lastErr = roundTripErr
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if req.Context().Err() != nil {
			break
		}
		if index+1 < len(targetIPs) && !canRetryDownloadTarget(req) {
			break
		}
	}
	return nil, lastErr
}

func (t *proxyAwareDownloadTransport) CloseIdleConnections() {
	if closer, ok := t.direct.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
	t.proxied.CloseIdleConnections()

	t.mu.Lock()
	defer t.mu.Unlock()
	for _, transport := range t.proxiedByTLSServer {
		transport.CloseIdleConnections()
	}
}

func (t *proxyAwareDownloadTransport) proxiedTransportForTLSServer(serverName string) *http.Transport {
	if configured := t.proxied.TLSClientConfig; configured != nil && configured.ServerName != "" {
		serverName = configured.ServerName
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if transport := t.proxiedByTLSServer[serverName]; transport != nil {
		return transport
	}

	transport := t.proxied.Clone()
	targetTLSConfig := cloneDownloadTLSConfig(transport.TLSClientConfig)
	targetTLSConfig.ServerName = serverName
	transport.TLSClientConfig = targetTLSConfig
	configureHTTPSProxyTLSDialer(transport, t.proxied)
	if t.proxiedByTLSServer == nil {
		t.proxiedByTLSServer = make(map[string]*http.Transport)
	}
	t.proxiedByTLSServer[serverName] = transport
	return transport
}

type blockedDownloadTransport struct {
	err error
}

func (t *blockedDownloadTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, t.err
}

func cloneDownloadTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if source, ok := base.(interface {
		TransformHTTPTransport(func(*http.Transport) (http.RoundTripper, bool)) (http.RoundTripper, bool)
	}); ok {
		rebuilt, transformed := source.TransformHTTPTransport(newDownloadTransportLeaf)
		if transformed && rebuilt != nil {
			return rebuilt
		}
	}
	if source, ok := base.(*http.Transport); ok && source != nil {
		rebuilt, transformed := newDownloadTransportLeaf(source)
		if transformed && rebuilt != nil {
			return rebuilt
		}
	}
	return &blockedDownloadTransport{err: errs.NewInternalError(
		errs.SubtypeUnknown,
		"cannot safely clone download transport %T",
		base,
	)}
}

func newDownloadTransportLeaf(source *http.Transport) (http.RoundTripper, bool) {
	return newDownloadTransportLeafWithResolver(source, net.DefaultResolver.LookupIP)
}

func newDownloadTransportLeafWithResolver(source *http.Transport, lookupIP downloadLookupIPFunc) (http.RoundTripper, bool) {
	if source == nil {
		return nil, false
	}

	selectProxy := source.Proxy
	direct := cloneDownloadHTTPTransport(source)
	direct.Proxy = nil
	configureDirectDownloadTransport(direct)
	if selectProxy == nil {
		return direct, true
	}

	// The proxied branch validates the requested URL before construction and
	// on every redirect. Its TCP peer is the selected proxy, so applying the
	// direct-origin IP guard there would incorrectly reject trusted loopback or
	// private-network proxies. Freeze the selected proxy in request context so
	// a stateful selector cannot switch the second lookup to direct egress.
	proxied := cloneDownloadHTTPTransport(source)
	proxied.Proxy = func(req *http.Request) (*url.URL, error) {
		selected, ok := req.Context().Value(selectedDownloadProxyKey{}).(*url.URL)
		if !ok || selected == nil {
			return nil, fmt.Errorf("download proxy selection is missing")
		}
		cloned := *selected
		return &cloned, nil
	}
	return &proxyAwareDownloadTransport{
		selectProxy:        selectProxy,
		direct:             direct,
		proxied:            proxied,
		lookupIP:           lookupIP,
		proxiedByTLSServer: make(map[string]*http.Transport),
	}, true
}

func cloneDownloadHTTPTransport(source *http.Transport) *http.Transport {
	cloned := source.Clone()
	if cloned.TLSNextProto == nil {
		if _, ok := source.TLSNextProto["h2"]; ok {
			cloned.ForceAttemptHTTP2 = true
		}
	}
	return cloned
}

func pinDownloadRequestTargetToIP(req *http.Request, targetIP net.IP) (*http.Request, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("download request URL is missing")
	}
	if targetIP == nil || isRestrictedDownloadIP(targetIP) {
		return nil, fmt.Errorf("blocked download target: local/internal host is not allowed")
	}

	originalHost := req.URL.Host
	pinnedHost := targetIP.String()
	if port := req.URL.Port(); port != "" {
		pinnedHost = net.JoinHostPort(pinnedHost, port)
	} else if strings.Contains(pinnedHost, ":") {
		pinnedHost = "[" + pinnedHost + "]"
	}

	pinned := req.Clone(req.Context())
	pinnedURL := *req.URL
	pinnedURL.Host = pinnedHost
	pinned.URL = &pinnedURL
	pinned.Host = originalHost
	return pinned, nil
}

func canRetryDownloadTarget(req *http.Request) bool {
	if req == nil || req.Body != nil {
		return false
	}
	return req.Method == http.MethodGet || req.Method == http.MethodHead
}

func cloneDownloadTLSConfig(config *tls.Config) *tls.Config {
	if config == nil {
		return &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return config.Clone()
}

func configureHTTPSProxyTLSDialer(transport, source *http.Transport) {
	if transport.DialTLSContext != nil || transport.DialTLS != nil {
		return
	}

	proxyTLSConfig := cloneDownloadTLSConfig(source.TLSClientConfig)
	transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		rawConn, err := dialDownloadProxy(ctx, source, network, addr)
		if err != nil {
			return nil, err
		}

		config := proxyTLSConfig.Clone()
		serverName, _, splitErr := net.SplitHostPort(addr)
		if splitErr != nil {
			rawConn.Close()
			return nil, fmt.Errorf("invalid HTTPS proxy address: %w", splitErr)
		}
		config.ServerName = serverName
		tlsConn := tls.Client(rawConn, config)
		handshakeCtx := ctx
		cancel := func() {}
		if source.TLSHandshakeTimeout > 0 {
			handshakeCtx, cancel = context.WithTimeout(ctx, source.TLSHandshakeTimeout)
		}
		defer cancel()
		if err := tlsConn.HandshakeContext(handshakeCtx); err != nil {
			rawConn.Close()
			return nil, err
		}
		return tlsConn, nil
	}
}

func dialDownloadProxy(ctx context.Context, source *http.Transport, network, addr string) (net.Conn, error) {
	if source.DialContext != nil {
		return source.DialContext(ctx, network, addr)
	}
	if source.Dial != nil {
		return source.Dial(network, addr)
	}
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, addr)
}

func configureDirectDownloadTransport(cloned *http.Transport) {
	origDial := cloned.DialContext
	cloned.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dialConn(ctx, origDial, network, addr)
		if err != nil {
			return nil, err
		}
		if err := validateConnRemoteIP(conn); err != nil {
			conn.Close()
			return nil, downloadTargetPolicyError(err)
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
				return nil, downloadTargetPolicyError(err)
			}
			return conn, nil
		}
	}
	if cloned.DialTLS != nil {
		origDialTLS := cloned.DialTLS
		cloned.DialTLS = func(network, addr string) (net.Conn, error) {
			conn, err := origDialTLS(network, addr)
			if err != nil {
				return nil, err
			}
			if err := validateConnRemoteIP(conn); err != nil {
				conn.Close()
				return nil, downloadTargetPolicyError(err)
			}
			return conn, nil
		}
	}

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
			return nil, downloadTargetPolicyError(err)
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

func downloadTargetPolicyError(err error) error {
	return errs.NewSecurityPolicyError(
		errs.SubtypeAccessDenied,
		"blocked download target: %v",
		err,
	).WithCause(err)
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
