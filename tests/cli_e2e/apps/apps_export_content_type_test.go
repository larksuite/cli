// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zipMagic is the local file header every real zip starts with. Asserting on it
// proves the bytes on disk are the archive itself rather than an error body that
// was streamed to the output path.
var zipMagic = []byte("PK\x03\x04")

// exportArchiveBody is the fake archive the gateway serves. It carries the zip
// magic so a saved file can be distinguished from a saved error blob.
var exportArchiveBody = append(append([]byte{}, zipMagic...), "E2E-ARCHIVE-PAYLOAD"...)

// TestAppsExportContentTypeE2E runs the real lark-cli binary against a local
// HTTPS gateway and pins which Content-Type is treated as the archive.
//
// The command's only failure mode here lives in response handling, not in
// argument parsing, so a dry-run cannot reach it: the request the CLI sends is
// byte-identical in every subtest and only the response Content-Type differs.
// That is why this is a full-process test with a live TLS endpoint rather than
// another dry-run shape assertion.
//
// The gate must be a JSON blacklist, not an archive whitelist. An archive
// served under a non-standard binary label — or with no Content-Type at all —
// is still the archive and must reach disk. Only a JSON body is the gateway's
// HTTP-200 error envelope.
func TestAppsExportContentTypeE2E(t *testing.T) {
	t.Run("StreamsArchive", func(t *testing.T) {
		// Labels a gateway or proxy may substitute for application/zip. Each one
		// used to be refused by the archive-Content-Type whitelist even though the
		// body is a valid archive.
		for _, tc := range []struct {
			name        string
			contentType string
		}{
			{"x-zip-compressed", "application/x-zip-compressed"},
			{"binary-octet-stream", "binary/octet-stream"},
			{"force-download", "application/force-download"},
			{"absent", ""},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if tc.contentType == "" {
					// Guard the fixture itself: if the gateway ever labels this
					// response, the case silently stops testing an absent
					// Content-Type and starts testing whatever was inferred.
					assertGatewayOmitsContentType(t, exportArchiveBody)
				}

				out, result := runExport(t, tc.contentType, exportArchiveBody)

				result.AssertExitCode(t, 0)
				saved, err := os.ReadFile(out)
				require.NoError(t, err, "archive must be written for content type %q", tc.contentType)
				assert.Equal(t, exportArchiveBody, saved,
					"saved bytes must be the archive verbatim")
			})
		}
	})

	t.Run("RefusesErrorEnvelope", func(t *testing.T) {
		// Both plain and RFC 6839 structured-suffix JSON are error envelopes and
		// must never be written to the output path.
		for _, tc := range []struct {
			name        string
			contentType string
		}{
			{"application-json", "application/json"},
			{"problem-json", "application/problem+json"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				envelope := []byte(`{"code":40901,"msg":"app not published"}`)
				out, result := runExport(t, tc.contentType, envelope)

				assert.NotEqual(t, 0, result.ExitCode,
					"a JSON error envelope must fail the command")
				_, statErr := os.Stat(out)
				assert.True(t, os.IsNotExist(statErr),
					"no file may be written for a JSON error envelope")
				combined := result.Stdout + result.Stderr
				assert.Contains(t, combined, "app not published",
					"the server's reason must reach the caller")
			})
		}
	})
}

// assertGatewayOmitsContentType proves the gateway really sends no Content-Type
// for the empty-type case.
//
// net/http sniffs the body when the header is unset, and this archive's PK magic
// infers application/zip — a type the old whitelist accepted. Without this check
// the "absent" case would pass against the very bug it is meant to catch.
func assertGatewayOmitsContentType(t *testing.T, body []byte) {
	t.Helper()

	proxyAddr, caPath := startExportGateway(t, "", body)

	pemBytes, err := os.ReadFile(caPath)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(pemBytes))

	proxyURL, err := url.Parse("http://" + proxyAddr)
	require.NoError(t, err)
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
	}

	resp, err := client.Get("https://open.feishu.cn/open-apis/spark/v1/apps/export")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	_, present := resp.Header["Content-Type"]
	require.False(t, present,
		"gateway must omit Content-Type; got %q", resp.Header.Get("Content-Type"))
}

// runExport invokes `apps +export` against a local gateway that answers with the
// given Content-Type and body, and returns the output path and the CLI result.
// The output path is intentionally not created, so its absence after a failed
// run is meaningful.
func runExport(t *testing.T, contentType string, body []byte) (string, *clie2e.Result) {
	t.Helper()

	proxyAddr, caPath := startExportGateway(t, contentType, body)
	workDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"apps", "+export", "--app-id", "app_e2e", "--output", "src.zip"},
		DefaultAs: "user",
		WorkDir:   workDir,
		Env: map[string]string{
			// Route every outbound request to the local gateway and trust its CA.
			"LARKSUITE_CLI_PROXY_ENABLE":       "true",
			"LARKSUITE_CLI_PROXY_ADDRESS":      "http://" + proxyAddr,
			"LARKSUITE_CLI_CA_PATH":            caPath,
			"LARKSUITE_CLI_APP_ID":             "apps_export_e2e",
			"LARKSUITE_CLI_APP_SECRET":         "apps_export_e2e_secret",
			"LARKSUITE_CLI_USER_ACCESS_TOKEN":  "u-apps-export-e2e",
			"LARKSUITE_CLI_BRAND":              "feishu",
			"LARKSUITE_CLI_CONFIG_DIR":         filepath.Join(workDir, "config"),
			"LARKSUITE_CLI_DATA_DIR":           filepath.Join(workDir, "data"),
			"LARKSUITE_CLI_NO_UPDATE_NOTIFIER": "1",
			"LARKSUITE_CLI_NO_SKILLS_NOTIFIER": "1",
		},
	})
	require.NoError(t, err)

	return filepath.Join(workDir, "src.zip"), result
}

// startExportGateway runs a CONNECT-terminating proxy that answers the tunnelled
// HTTPS request itself. The CLI reaches the OpenAPI host over real TLS, so the
// response travels the same client path a live export does — the transport is
// real and only the peer is local.
//
// It returns the proxy address and the path to the CA PEM the CLI must trust.
func startExportGateway(t *testing.T, contentType string, body []byte) (string, string) {
	t.Helper()

	certPEM, keyPEM := selfSignedCert(t)
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caPath, certPEM, 0o600))

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}}

	// origin answers the requests the CLI makes once the tunnel is up: the token
	// fetch, then the export itself.
	origin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/auth/v3/") || strings.Contains(r.URL.Path, "access_token") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","tenant_access_token":"t-e2e","app_access_token":"a-e2e","expire":7200}`)
			return
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		} else {
			// A nil value suppresses net/http's body sniffing, which would
			// otherwise infer application/zip from the archive magic and turn
			// the "absent" case into a type the old whitelist already accepted.
			w.Header()["Content-Type"] = nil
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	proxy := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodConnect {
				origin(w, r)
				return
			}
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "hijack unsupported", http.StatusInternalServerError)
				return
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				return
			}
			if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
				_ = conn.Close()
				return
			}
			tlsConn := tls.Server(conn, tlsCfg)
			if err := tlsConn.Handshake(); err != nil {
				_ = tlsConn.Close()
				return
			}
			// Serve exactly one connection: the tunnel the CLI just opened.
			_ = (&http.Server{Handler: origin}).Serve(&singleConnListener{conn: tlsConn})
		}),
	}

	go func() { _ = proxy.Serve(listener) }()
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = proxy.Shutdown(shutdownCtx)
	})

	return listener.Addr().String(), caPath
}

// selfSignedCert issues a short-lived CA-capable certificate covering the
// OpenAPI hosts so the CLI's TLS verification succeeds against the local peer.
func selfSignedCert(t *testing.T) ([]byte, []byte) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "lark-cli-e2e-export-gateway"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"open.feishu.cn", "open.larksuite.com", "localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM
}

// singleConnListener hands an already-accepted connection to http.Server so one
// CONNECT tunnel can be served with the standard HTTP stack.
type singleConnListener struct {
	conn net.Conn
	done bool
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	if l.done {
		return nil, io.EOF
	}
	l.done = true
	return l.conn, nil
}

func (l *singleConnListener) Close() error   { return nil }
func (l *singleConnListener) Addr() net.Addr { return l.conn.LocalAddr() }
