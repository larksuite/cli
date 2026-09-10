// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dpop

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keysigner"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

type transportTestSigner struct {
	keysigner.Signer
	key     *ecdsa.PrivateKey
	signErr error
}

func (s *transportTestSigner) PublicKey(context.Context, keysigner.KeyRef) (crypto.PublicKey, error) {
	return &s.key.PublicKey, nil
}

func (s *transportTestSigner) Sign(_ context.Context, _ keysigner.KeyRef, message []byte) ([]byte, string, error) {
	if s.signErr != nil {
		return nil, "", s.signErr
	}
	digest := sha256.Sum256(message)
	r, v, err := ecdsa.Sign(rand.Reader, s.key, digest[:])
	if err != nil {
		return nil, "", err
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	v.FillBytes(signature[32:])
	return signature, keysigner.AlgES256, nil
}

func transportTestBinding(t *testing.T) (*Binding, *transportTestSigner) {
	t.Helper()
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer := &transportTestSigner{Signer: newTestStoreSigner("injected", keysigner.SecurityLevelL3), key: private}
	key := newKey("transport-test-key", &private.PublicKey, signer, NewClock(nil))
	binding, err := NewBinding("fixture-token", key)
	if err != nil {
		t.Fatal(err)
	}
	return binding, signer
}

type transportTestRoundTripper func(*http.Request) (*http.Response, error)

func (f transportTestRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type transportTestBody struct {
	io.Reader
	closes int
}

func (b *transportTestBody) Close() error { b.closes++; return nil }

func TestTransportSignsBoundRequestsRegardlessOfDestination(t *testing.T) {
	binding, _ := transportTestBinding(t)
	for _, brand := range []core.LarkBrand{core.BrandFeishu, core.BrandLark} {
		endpoints := core.ResolveEndpoints(brand)
		for _, tc := range []struct {
			name, url, auth string
			bound, signed   bool
			wantAuth        string
		}{
			{"resource", endpoints.Open + "/open-apis/test?cursor=1", "Bearer fixture-token", true, true, "DPoP fixture-token"},
			{"already_dpop", endpoints.Open + "/open-apis/test", "DPoP fixture-token", true, true, "DPoP fixture-token"},
			{"default_port", endpoints.Open + ":443/open-apis/test", "Bearer fixture-token", true, true, "DPoP fixture-token"},
			{"unbound", endpoints.Open + "/open-apis/test", "Bearer fixture-token", false, false, "Bearer fixture-token"},
			{"external", "https://example.com/open-apis/test", "Bearer fixture-token", true, true, "DPoP fixture-token"},
			{"lookalike", endpoints.Open + ".example.com/open-apis/test", "DPoP fixture-token", true, true, "DPoP fixture-token"},
			{"plaintext", strings.Replace(endpoints.Open, "https:", "http:", 1) + "/open-apis/test", "Bearer fixture-token", true, true, "DPoP fixture-token"},
			{"other_port", endpoints.Open + ":8443/open-apis/test", "Bearer fixture-token", true, true, "DPoP fixture-token"},
			{"accounts", endpoints.Accounts + "/open-apis/test", "Bearer fixture-token", true, true, "DPoP fixture-token"},
			{"outside_namespace", endpoints.Open + "/open-apis-other/test", "Bearer fixture-token", true, true, "DPoP fixture-token"},
			{"presigned", "https://example.com/upload", "AWS4-HMAC-SHA256 fixture-signature", false, false, "AWS4-HMAC-SHA256 fixture-signature"},
		} {
			t.Run(string(brand)+"/"+tc.name, func(t *testing.T) {
				req, err := http.NewRequest(http.MethodGet, tc.url, nil)
				if err != nil {
					t.Fatal(err)
				}
				if tc.bound {
					req = req.WithContext(WithBinding(req.Context(), binding))
				}
				req.Header.Set("Authorization", tc.auth)
				req.Header.Set(ProofHeader, "stale-proof")
				original := req.Header.Clone()
				calls := 0
				transport := &Transport{Base: transportTestRoundTripper(func(sent *http.Request) (*http.Response, error) {
					calls++
					if got := sent.Header.Get("Authorization"); got != tc.wantAuth {
						t.Errorf("authorization = %q, want %q", got, tc.wantAuth)
					}
					proof := sent.Header.Get(ProofHeader)
					if tc.signed && (proof == "" || proof == "stale-proof") || !tc.signed && proof != original.Get(ProofHeader) {
						t.Errorf("unexpected proof for signed=%v: %q", tc.signed, proof)
					}
					return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
				})}
				resp, err := transport.RoundTrip(req)
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
				if calls != 1 || !reflect.DeepEqual(req.Header, original) {
					t.Fatal("transport changed original headers or forwarded more than once")
				}
			})
		}
	}
}

func TestTransport_ClosesBodyBeforeReturningLocalError(t *testing.T) {
	for _, scenario := range []string{"token_signing", "resource_signing", "binding_mismatch"} {
		t.Run(scenario, func(t *testing.T) {
			binding, signer := transportTestBinding(t)
			signErr := errors.New("injected keychain locked")
			signer.signErr = signErr
			ctx := WithBinding(context.Background(), binding)
			if scenario == "token_signing" {
				ctx = WithTokenEndpointKey(context.Background(), binding.Key())
			}
			body := &transportTestBody{Reader: strings.NewReader("upload")}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, core.ResolveEndpoints(core.BrandFeishu).Open+"/open-apis/upload", body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+binding.Token)
			wantSubtype := errs.SubtypeDPoPProofFailed
			if scenario == "binding_mismatch" {
				req.Header.Set("Authorization", "Bearer different-token")
				wantSubtype = errs.SubtypeDPoPBindingMismatch
			}
			base := transportTestRoundTripper(func(*http.Request) (*http.Response, error) {
				t.Fatal("local DPoP failure reached the network transport")
				return nil, nil
			})
			_, err = (&Transport{Base: base}).RoundTrip(req)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Subtype != wantSubtype || problem.Hint == "" {
				t.Fatalf("error = %v, want typed %s with recovery", err, wantSubtype)
			}
			if scenario != "binding_mismatch" && !errors.Is(err, signErr) {
				t.Fatal("signing error cause was lost")
			}
			if body.closes != 1 {
				t.Fatalf("body closed %d times, want exactly once", body.closes)
			}
		})
	}
}

func TestTransport_DoesNotReplayProofAfterConnectionLoss(t *testing.T) {
	for _, scenario := range []struct {
		name, method, payload string
		noBody, tokenEndpoint bool
	}{
		{name: "get_nil", method: http.MethodGet},
		{name: "get_no_body", method: http.MethodGet, noBody: true},
		{name: "get_with_body", method: http.MethodGet, payload: "query"},
		{name: "post_idempotency", method: http.MethodPost, payload: "upload"},
		{name: "post_empty", method: http.MethodPost},
		{name: "token_endpoint", method: http.MethodPost, payload: "grant_type=refresh_token", tokenEndpoint: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			binding, _ := transportTestBinding(t)
			var mu sync.Mutex
			var proofs []string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/warm" {
					_, _ = io.WriteString(w, "warm")
					return
				}
				payload, err := io.ReadAll(r.Body)
				if err != nil || string(payload) != scenario.payload || r.Method != scenario.method {
					t.Errorf("wire method/body = %s/%q, error = %v", r.Method, payload, err)
				}
				if r.Header.Get("Idempotency-Key") != "fixture-idempotency-key" {
					t.Error("idempotency header was lost")
				}
				if scenario.method == http.MethodGet && scenario.payload == "" && (r.ContentLength != 0 || len(r.TransferEncoding) != 0) {
					t.Error("bodyless GET acquired a body on the wire")
				}
				mu.Lock()
				proofs = append(proofs, r.Header.Get(ProofHeader))
				attempt := len(proofs)
				mu.Unlock()
				if attempt == 1 {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = conn.Close() // Request reached the server; response was lost.
					return
				}
				_, _ = io.WriteString(w, "ok")
			}))
			defer server.Close()
			native := server.Client().Transport.(*http.Transport).Clone()
			native.TLSClientConfig.ServerName = server.Certificate().DNSNames[0]
			native.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			}
			defer native.CloseIdleConnections()
			client := &http.Client{Transport: &Transport{Base: native}, Timeout: 5 * time.Second}
			origin := core.ResolveEndpoints(core.BrandFeishu).Open
			warm, err := client.Get(origin + "/warm")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, warm.Body)
			_ = warm.Body.Close()

			ctx := WithBinding(context.Background(), binding)
			if scenario.tokenEndpoint {
				ctx = WithTokenEndpointKey(context.Background(), binding.Key())
			}
			var body io.Reader
			if scenario.payload != "" {
				body = strings.NewReader(scenario.payload)
			} else if scenario.noBody {
				body = http.NoBody
			}
			req, err := http.NewRequestWithContext(ctx, scenario.method, origin+"/open-apis/resource", body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+binding.Token)
			req.Header.Set("Idempotency-Key", "fixture-idempotency-key")
			originalBody, hadGetBody := req.Body, req.GetBody != nil
			resp, err := client.Do(req)
			if resp != nil {
				_ = resp.Body.Close()
			}
			if err == nil {
				t.Fatal("native transport replayed the signed request after losing its response")
			}
			if req.Body != originalBody || (req.GetBody != nil) != hadGetBody || req.Header.Get(ProofHeader) != "" || req.Header.Get("Authorization") != "Bearer "+binding.Token {
				t.Fatal("DPoP transport mutated the caller's replayable request")
			}
			// An explicit caller retry passes through the signer again.
			retry := req.Clone(ctx)
			if retry.GetBody != nil {
				retry.Body, err = retry.GetBody()
				if err != nil {
					t.Fatal(err)
				}
			}
			resp, err = client.Do(retry)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			mu.Lock()
			defer mu.Unlock()
			if len(proofs) != 2 || proofs[0] == "" || proofs[0] == proofs[1] {
				t.Fatalf("received %d attempts; expected two distinct proofs", len(proofs))
			}
		})
	}
}

func TestTransport_DoesNotReplayProofOnHTTP2RefusedStream(t *testing.T) {
	for _, tokenEndpoint := range []bool{false, true} {
		name := "resource_get"
		if tokenEndpoint {
			name = "token_post"
		}
		t.Run(name, func(t *testing.T) {
			binding, _ := transportTestBinding(t)
			var mu sync.Mutex
			var proofs []string
			server := httptest.NewUnstartedServer(nil)
			server.EnableHTTP2 = true
			// Inject REFUSED_STREAM after reading the actual proof. The standard
			// HTTP/2 client normally retries this without re-entering RoundTrip.
			server.Config.TLSNextProto = map[string]func(*http.Server, *tls.Conn, http.Handler){
				"h2": func(_ *http.Server, conn *tls.Conn, _ http.Handler) {
					defer conn.Close()
					_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
					preface := make([]byte, len(http2.ClientPreface))
					if _, err := io.ReadFull(conn, preface); err != nil || string(preface) != http2.ClientPreface {
						t.Errorf("invalid HTTP/2 preface: %v", err)
						return
					}
					framer := http2.NewFramer(conn, conn)
					framer.ReadMetaHeaders = hpack.NewDecoder(4096, nil)
					if err := framer.WriteSettings(); err != nil {
						t.Error(err)
						return
					}
					for {
						frame, err := framer.ReadFrame()
						if err != nil {
							return
						}
						switch frame := frame.(type) {
						case *http2.SettingsFrame:
							if !frame.IsAck() {
								if err := framer.WriteSettingsAck(); err != nil {
									return
								}
							}
						case *http2.MetaHeadersFrame:
							proof := ""
							for _, field := range frame.Fields {
								if field.Name == "dpop" {
									proof = field.Value
								}
							}
							mu.Lock()
							proofs = append(proofs, proof)
							attempt := len(proofs)
							mu.Unlock()
							if attempt == 1 {
								if err := framer.WriteRSTStream(frame.StreamID, http2.ErrCodeRefusedStream); err != nil {
									t.Error(err)
									return
								}
							} else {
								var headers bytes.Buffer
								if err := hpack.NewEncoder(&headers).WriteField(hpack.HeaderField{Name: ":status", Value: "200"}); err != nil {
									t.Error(err)
									return
								}
								if err := framer.WriteHeaders(http2.HeadersFrameParam{
									StreamID: frame.StreamID, EndHeaders: true, EndStream: true, BlockFragment: headers.Bytes(),
								}); err != nil {
									t.Error(err)
									return
								}
							}
						}
					}
				},
			}
			server.StartTLS()
			defer server.Close()
			native := server.Client().Transport.(*http.Transport).Clone()
			native.ForceAttemptHTTP2 = true
			native.TLSClientConfig.ServerName = server.Certificate().DNSNames[0]
			native.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			}
			defer native.CloseIdleConnections()
			ctx := WithBinding(context.Background(), binding)
			method := http.MethodGet
			var body io.Reader
			if tokenEndpoint {
				ctx = WithTokenEndpointKey(context.Background(), binding.Key())
				method, body = http.MethodPost, strings.NewReader("grant_type=refresh_token")
			}
			req, err := http.NewRequestWithContext(ctx, method, core.ResolveEndpoints(core.BrandFeishu).Open+"/open-apis/resource", body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+binding.Token)
			resp, err := (&http.Client{Transport: &Transport{Base: native}, Timeout: 5 * time.Second}).Do(req)
			if resp != nil {
				_ = resp.Body.Close()
			}
			if err == nil {
				t.Fatal("HTTP/2 transparently retried the signed request")
			}
			mu.Lock()
			defer mu.Unlock()
			if len(proofs) != 1 || proofs[0] == "" {
				t.Fatalf("received %d HTTP/2 proofs, want exactly one", len(proofs))
			}
		})
	}
}
