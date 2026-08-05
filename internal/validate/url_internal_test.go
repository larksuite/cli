// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package validate

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
)

func TestProxiedHTTPSDownloadPinsValidatedTargetIP(t *testing.T) {
	const (
		targetHost = "rebind.example"
		targetIP   = "203.0.113.10"
	)

	proxyCalled := make(chan struct{}, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		proxyCalled <- struct{}{}
		if req.Method != http.MethodConnect {
			t.Errorf("proxy request method = %q, want CONNECT", req.Method)
		}
		if got := req.Host; got != targetIP+":443" {
			t.Errorf("proxy CONNECT target = %q, want validated IP %q", got, targetIP+":443")
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(proxy.Close)
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}

	lookupIP := func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP(targetIP)}, nil
	}
	transport, ok := newDownloadTransportLeafWithResolver(
		&http.Transport{Proxy: http.ProxyURL(proxyURL)},
		lookupIP,
	)
	if !ok {
		t.Fatal("newDownloadTransportLeafWithResolver() did not rebuild transport")
	}

	req, err := http.NewRequest(http.MethodGet, "https://"+targetHost+"/file", nil)
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := pinDownloadRequestTargetToIP(req, net.ParseIP(targetIP))
	if err != nil {
		t.Fatal(err)
	}
	if pinned.Host != targetHost {
		t.Fatalf("pinned request Host = %q, want %q", pinned.Host, targetHost)
	}
	if _, err := transport.RoundTrip(req); err == nil {
		t.Fatal("RoundTrip() error = nil, want proxy rejection after CONNECT")
	}
	select {
	case <-proxyCalled:
	default:
		t.Fatal("proxy was not called")
	}
}

func TestRestrictedDownloadIPBlocksReservedIPv4(t *testing.T) {
	for _, rawIP := range []string{"0.1.2.3", "240.0.0.1"} {
		if !isRestrictedDownloadIP(net.ParseIP(rawIP)) {
			t.Fatalf("%s was classified as safe", rawIP)
		}
	}
	if isRestrictedDownloadIP(net.ParseIP("1.1.1.1")) {
		t.Fatal("1.1.1.1 was classified as restricted")
	}
}

func TestCloneDownloadTLSConfigSetsMinimumVersion(t *testing.T) {
	if got := cloneDownloadTLSConfig(nil).MinVersion; got != tls.VersionTLS12 {
		t.Fatalf("MinVersion = %d, want TLS 1.2", got)
	}
	configured := &tls.Config{MinVersion: tls.VersionTLS13}
	if got := cloneDownloadTLSConfig(configured).MinVersion; got != tls.VersionTLS13 {
		t.Fatalf("cloned MinVersion = %d, want TLS 1.3", got)
	}
}

func TestCloneDownloadHTTPTransportPreservesHTTP2Policy(t *testing.T) {
	tests := []struct {
		name            string
		source          *http.Transport
		wantForce       bool
		wantH2Handler   bool
		wantProtocolMap bool
	}{
		{
			name: "automatic",
			source: &http.Transport{
				Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: "proxy.example:8080"}),
			},
			wantForce: true,
		},
		{
			name: "custom TLS without opt-in",
			source: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		},
		{
			name: "custom dial without opt-in",
			source: &http.Transport{
				DialContext: func(context.Context, string, string) (net.Conn, error) {
					return nil, errors.New("unused")
				},
			},
		},
		{
			name:      "explicit opt-in",
			source:    &http.Transport{ForceAttemptHTTP2: true},
			wantForce: true,
		},
		{
			name: "explicit h2 handler",
			source: &http.Transport{
				TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{
					"h2": func(string, *tls.Conn) http.RoundTripper { return nil },
				},
			},
			wantH2Handler:   true,
			wantProtocolMap: true,
		},
		{
			name: "explicit opt-out",
			source: &http.Transport{
				TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{},
			},
			wantProtocolMap: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cloned := cloneDownloadHTTPTransport(test.source)
			if cloned.ForceAttemptHTTP2 != test.wantForce {
				t.Fatalf("ForceAttemptHTTP2 = %v, want %v", cloned.ForceAttemptHTTP2, test.wantForce)
			}
			_, hasH2Handler := cloned.TLSNextProto["h2"]
			if hasH2Handler != test.wantH2Handler {
				t.Fatalf("h2 handler = %v, want %v", hasH2Handler, test.wantH2Handler)
			}
			if hasProtocolMap := cloned.TLSNextProto != nil; hasProtocolMap != test.wantProtocolMap {
				t.Fatalf("TLSNextProto is non-nil = %v, want %v", hasProtocolMap, test.wantProtocolMap)
			}
		})
	}
}

func TestCloneDownloadTransportPreservesAutomaticHTTP2(t *testing.T) {
	source := &http.Transport{
		Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: "proxy.example:8080"}),
	}
	rebuilt := cloneDownloadTransport(source)
	proxyAware, ok := rebuilt.(*proxyAwareDownloadTransport)
	if !ok {
		t.Fatalf("transport type = %T, want *proxyAwareDownloadTransport", rebuilt)
	}
	direct, ok := proxyAware.direct.(*http.Transport)
	if !ok {
		t.Fatalf("direct transport type = %T, want *http.Transport", proxyAware.direct)
	}
	for name, transport := range map[string]*http.Transport{
		"direct":  direct,
		"proxied": proxyAware.proxied,
	} {
		if !transport.ForceAttemptHTTP2 {
			t.Fatalf("%s ForceAttemptHTTP2 = false, want true", name)
		}
	}
}

func TestProxiedDownloadRejectsRestrictedResolvedTarget(t *testing.T) {
	var proxyCalled atomic.Bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyCalled.Store(true)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(proxy.Close)
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}

	lookupIP := func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	transport, ok := newDownloadTransportLeafWithResolver(
		&http.Transport{Proxy: http.ProxyURL(proxyURL)},
		lookupIP,
	)
	if !ok {
		t.Fatal("newDownloadTransportLeafWithResolver() did not rebuild transport")
	}

	req, err := http.NewRequest(http.MethodGet, "http://rebind.example/file", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = transport.RoundTrip(req)
	if err == nil {
		t.Fatal("RoundTrip() error = nil, want restricted target rejection")
	}
	if problem, ok := errs.ProblemOf(err); !ok ||
		problem.Category != errs.CategoryPolicy ||
		problem.Subtype != errs.SubtypeAccessDenied {
		t.Fatalf("RoundTrip() problem = %#v, %v; want policy/access_denied", problem, ok)
	}
	if proxyCalled.Load() {
		t.Fatal("proxy was called for a restricted resolved target")
	}
}

func TestProxiedPlainHTTPHostnameRejectsLocalProxy(t *testing.T) {
	var proxyCalled atomic.Bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyCalled.Store(true)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(proxy.Close)
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}

	lookupIP := func(ctx context.Context, _, host string) ([]net.IP, error) {
		if host == "public.example" {
			return []net.IP{net.ParseIP("203.0.113.10")}, nil
		}
		return net.DefaultResolver.LookupIP(ctx, "ip", host)
	}
	transport, ok := newDownloadTransportLeafWithResolver(
		&http.Transport{Proxy: http.ProxyURL(proxyURL)},
		lookupIP,
	)
	if !ok {
		t.Fatal("newDownloadTransportLeafWithResolver() did not rebuild transport")
	}

	req, err := http.NewRequest(http.MethodGet, "http://public.example/file", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = transport.RoundTrip(req)
	if err == nil {
		t.Fatal("RoundTrip() error = nil, want plain HTTP hostname rejection")
	}
	if problem, ok := errs.ProblemOf(err); !ok ||
		problem.Category != errs.CategoryPolicy ||
		problem.Subtype != errs.SubtypeAccessDenied {
		t.Fatalf("RoundTrip() problem = %#v, %v; want policy/access_denied", problem, ok)
	} else if problem.Hint != "use HTTPS or a literal public IP" {
		t.Fatalf("RoundTrip() hint = %q, want recovery guidance", problem.Hint)
	}
	if proxyCalled.Load() {
		t.Fatal("proxy was called for a plain HTTP hostname target")
	}
}

func TestProxiedHTTPSDownloadTriesEveryValidatedTargetIP(t *testing.T) {
	connectTargets := make(chan string, 2)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		connectTargets <- req.Host
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(proxy.Close)
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := newDownloadTransportLeafWithResolver(
		&http.Transport{Proxy: http.ProxyURL(proxyURL)},
		func(context.Context, string, string) ([]net.IP, error) {
			return []net.IP{
				net.ParseIP("203.0.113.10"),
				net.ParseIP("203.0.113.11"),
			}, nil
		},
	)
	if !ok {
		t.Fatal("newDownloadTransportLeafWithResolver() did not rebuild transport")
	}

	req, err := http.NewRequest(http.MethodGet, "https://multi.example/file", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(req); err == nil {
		t.Fatal("RoundTrip() error = nil, want proxy rejection")
	}
	for _, want := range []string{"203.0.113.10:443", "203.0.113.11:443"} {
		select {
		case got := <-connectTargets:
			if got != want {
				t.Fatalf("proxy CONNECT target = %q, want %q", got, want)
			}
		default:
			t.Fatalf("proxy did not receive CONNECT target %q", want)
		}
	}
}

func TestCanRetryDownloadTargetOnlyAllowsBodylessReads(t *testing.T) {
	for _, test := range []struct {
		method string
		body   string
		want   bool
	}{
		{method: http.MethodGet, want: true},
		{method: http.MethodHead, want: true},
		{method: http.MethodPost},
		{method: http.MethodGet, body: "body"},
	} {
		req, err := http.NewRequest(test.method, "https://download.example/file", strings.NewReader(test.body))
		if err != nil {
			t.Fatal(err)
		}
		if test.body == "" {
			req.Body = nil
		}
		if got := canRetryDownloadTarget(req); got != test.want {
			t.Fatalf("canRetryDownloadTarget(%s, body=%q) = %v, want %v", test.method, test.body, got, test.want)
		}
	}
}

func TestProxiedHTTPSTargetPreservesOriginalTLSServerName(t *testing.T) {
	transport, ok := newDownloadTransportLeafWithResolver(
		&http.Transport{Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: "proxy.example:8080"})},
		func(context.Context, string, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("203.0.113.10")}, nil
		},
	)
	if !ok {
		t.Fatal("newDownloadTransportLeafWithResolver() did not rebuild transport")
	}
	proxyAware, ok := transport.(*proxyAwareDownloadTransport)
	if !ok {
		t.Fatalf("transport type = %T, want *proxyAwareDownloadTransport", transport)
	}

	pinned := proxyAware.proxiedTransportForTLSServer("download.example")
	if pinned.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig = nil")
	}
	if pinned.TLSClientConfig.ServerName != "download.example" {
		t.Fatalf("TLS ServerName = %q, want download.example", pinned.TLSClientConfig.ServerName)
	}
	if proxyAware.proxied.TLSClientConfig != nil && proxyAware.proxied.TLSClientConfig.ServerName != "" {
		t.Fatalf("base proxy TLS ServerName = %q, want unchanged", proxyAware.proxied.TLSClientConfig.ServerName)
	}
}

func TestHTTPSProxyTLSDialerUsesLegacyDial(t *testing.T) {
	wantErr := errors.New("legacy dial used")
	source := &http.Transport{
		Dial: func(string, string) (net.Conn, error) {
			return nil, wantErr
		},
	}
	target := source.Clone()
	configureHTTPSProxyTLSDialer(target, source)
	if target.DialTLSContext == nil {
		t.Fatal("DialTLSContext = nil")
	}
	if _, err := target.DialTLSContext(context.Background(), "tcp", "proxy.example:443"); !errors.Is(err, wantErr) {
		t.Fatalf("DialTLSContext() error = %v, want %v", err, wantErr)
	}
}

func TestDirectDownloadLegacyDialTLSClosesRestrictedConnection(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { serverConn.Close() })
	conn := &trackedDownloadConn{
		Conn:       clientConn,
		remoteAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 443},
	}
	rebuilt, ok := newDownloadTransportLeaf(&http.Transport{
		DialTLS: func(string, string) (net.Conn, error) {
			return conn, nil
		},
	})
	if !ok {
		t.Fatal("newDownloadTransportLeaf() did not rebuild transport")
	}
	transport, ok := rebuilt.(*http.Transport)
	if !ok {
		t.Fatalf("rebuilt transport = %T, want *http.Transport", rebuilt)
	}

	_, err := transport.DialTLS("tcp", "public.example:443")
	if err == nil || !strings.Contains(err.Error(), "local/internal host is not allowed") {
		t.Fatalf("DialTLS() error = %v, want restricted target rejection", err)
	}
	if problem, ok := errs.ProblemOf(err); !ok ||
		problem.Category != errs.CategoryPolicy ||
		problem.Subtype != errs.SubtypeAccessDenied {
		t.Fatalf("DialTLS() problem = %#v, %v; want policy/access_denied", problem, ok)
	}
	if !conn.closed {
		t.Fatal("restricted connection was not closed")
	}
}

func TestDirectDownloadLegacyDialTLSPreservesDialError(t *testing.T) {
	wantErr := errors.New("dial failed")
	rebuilt, ok := newDownloadTransportLeaf(&http.Transport{
		DialTLS: func(string, string) (net.Conn, error) {
			return nil, wantErr
		},
	})
	if !ok {
		t.Fatal("newDownloadTransportLeaf() did not rebuild transport")
	}
	transport, ok := rebuilt.(*http.Transport)
	if !ok {
		t.Fatalf("rebuilt transport = %T, want *http.Transport", rebuilt)
	}

	if _, err := transport.DialTLS("tcp", "public.example:443"); !errors.Is(err, wantErr) {
		t.Fatalf("DialTLS() error = %v, want %v", err, wantErr)
	}
}

func TestHTTPSProxyTLSDialerRetainsHandshakeTimeout(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})
	source := &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return clientConn, nil
		},
		TLSHandshakeTimeout: 50 * time.Millisecond,
	}
	target := source.Clone()
	configureHTTPSProxyTLSDialer(target, source)

	started := time.Now()
	if _, err := target.DialTLSContext(context.Background(), "tcp", "proxy.example:443"); err == nil {
		t.Fatal("DialTLSContext() error = nil, want TLS handshake timeout")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("TLS handshake timeout took %s, want under 1s", elapsed)
	}
}

func TestHTTPSProxyTLSDialerUsesProxyServerName(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(server.Close)

	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	proxySNI := make(chan string, 1)
	serverTLSConfig := server.TLS.Clone()
	serverTLSConfig.GetConfigForClient = func(info *tls.ClientHelloInfo) (*tls.Config, error) {
		proxySNI <- info.ServerName
		return nil, nil
	}
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- tls.Server(serverConn, serverTLSConfig).Handshake()
	}()

	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	source := &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return clientConn, nil
		},
		TLSClientConfig: &tls.Config{
			RootCAs:    roots,
			ServerName: "target.example.com",
		},
	}
	target := source.Clone()
	configureHTTPSProxyTLSDialer(target, source)

	conn, err := target.DialTLSContext(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
	if got := <-proxySNI; got != "example.com" {
		t.Fatalf("proxy TLS ServerName = %q, want example.com", got)
	}
}

type trackedDownloadConn struct {
	net.Conn
	remoteAddr net.Addr
	closed     bool
}

func (c *trackedDownloadConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *trackedDownloadConn) Close() error {
	c.closed = true
	return c.Conn.Close()
}
