// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dpop

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keysigner"
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
	endpoint := core.ResolveEndpoints(core.BrandFeishu).Open
	for _, tc := range []struct {
		name, url, auth              string
		bound, tokenEndpoint, signed bool
		wantAuth                     string
	}{
		{"resource", endpoint + "/open-apis/test?cursor=1", "Bearer fixture-token", true, false, true, "DPoP fixture-token"},
		{"already_dpop", endpoint + "/open-apis/test", "DPoP fixture-token", true, false, true, "DPoP fixture-token"},
		{"external", "https://example.com/upload", "Bearer fixture-token", true, false, true, "DPoP fixture-token"},
		{"token_endpoint", endpoint + "/authen/v2/oauth/token", "", false, true, true, ""},
		{"unbound", endpoint + "/open-apis/test", "Bearer fixture-token", false, false, false, "Bearer fixture-token"},
		{"presigned", "https://example.com/upload", "AWS4-HMAC-SHA256 fixture-signature", false, false, false, "AWS4-HMAC-SHA256 fixture-signature"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tc.url, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.bound {
				req = req.WithContext(WithBinding(req.Context(), binding))
			}
			if tc.tokenEndpoint {
				req = req.WithContext(WithTokenEndpointKey(req.Context(), binding.Key()))
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
