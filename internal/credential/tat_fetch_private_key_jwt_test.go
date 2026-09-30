// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keysigner"
)

func TestFetchTATWithAssertionForProvider_UnknownProviderFailsBeforeHTTP(t *testing.T) {
	rt := &stubRoundTripper{respCode: http.StatusOK, respBody: `{"code":0,"access_token":"must-not-use"}`}
	hc := &http.Client{Transport: rt}

	_, err := FetchTATWithAssertionForProvider(context.Background(), hc, core.BrandFeishu, "cli_app", nil, "unknown.provider", "key-1")
	if err == nil {
		t.Fatal("expected unknown provider error")
	}
	if rt.gotReq != nil {
		t.Fatalf("provider failure sent HTTP request to %s", rt.gotReq.URL)
	}
}

// fakeTATSigner is a real in-memory ECDSA P-256 signer for assertion tests.
type fakeTATSigner struct{ key *ecdsa.PrivateKey }

func (*fakeTATSigner) Name() string { return "fake" }

func (*fakeTATSigner) SecurityLevel() keysigner.SecurityLevel { return keysigner.SecurityLevelL3 }

func newFakeTATSigner(t *testing.T) *fakeTATSigner {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeTATSigner{key: k}
}

func (f *fakeTATSigner) EnsureKey(context.Context, keysigner.KeyRef) (crypto.PublicKey, error) {
	return f.key.Public(), nil
}

func (f *fakeTATSigner) PublicKey(context.Context, keysigner.KeyRef) (crypto.PublicKey, error) {
	return f.key.Public(), nil
}

func (f *fakeTATSigner) Sign(_ context.Context, _ keysigner.KeyRef, in []byte) ([]byte, string, error) {
	h := sha256.Sum256(in)
	r, s, err := ecdsa.Sign(rand.Reader, f.key, h[:])
	if err != nil {
		return nil, "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return sig, keysigner.AlgES256, nil
}

func (*fakeTATSigner) DeleteKey(context.Context, keysigner.KeyRef) error { return nil }

func TestFetchTATWithAssertion_Success(t *testing.T) {
	rt := &stubRoundTripper{respCode: 200, respBody: `{"access_token":"test-token","token_type":"Bearer","expires_in":7200}`}
	hc := &http.Client{Transport: rt}

	token, err := FetchTATWithAssertion(context.Background(), hc, core.BrandFeishu, "cli_app", newFakeTATSigner(t), "agent-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "test-token" {
		t.Errorf("token = %q, want test-token", token)
	}
	if rt.gotReq.URL.String() != "https://open.feishu.cn/open-apis/authen/v2/oauth/token" {
		t.Errorf("url = %s", rt.gotReq.URL.String())
	}

	form, err := url.ParseQuery(rt.gotBody)
	if err != nil {
		t.Fatal(err)
	}
	if form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
		t.Errorf("grant_type = %q", form.Get("grant_type"))
	}
	if form.Get("client_assertion_type") != "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" {
		t.Errorf("client_assertion_type = %q", form.Get("client_assertion_type"))
	}
	if form.Get("client_assertion") == "" {
		t.Error("client_assertion is empty")
	}
	if form.Has("client_secret") {
		t.Error("client_secret must NOT be sent for private_key_jwt_local_keypair")
	}
	if form.Get("client_id") != "cli_app" {
		t.Errorf("client_id = %q", form.Get("client_id"))
	}
}

func TestFetchTATWithAssertion_NilSigner(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	hc := &http.Client{Transport: &stubRoundTripper{respCode: 200, respBody: `{}`}}
	if _, err := FetchTATWithAssertion(context.Background(), hc, core.BrandFeishu, "cli_app", nil, "k"); err == nil {
		t.Fatal("expected error when signer is nil")
	}
}

func TestFetchTATWithAssertion_ServerError(t *testing.T) {
	rt := &stubRoundTripper{respCode: 200, respBody: `{"error":"invalid_client","error_description":"unknown key"}`}
	hc := &http.Client{Transport: rt}
	if _, err := FetchTATWithAssertion(context.Background(), hc, core.BrandFeishu, "cli_app", newFakeTATSigner(t), "k"); err == nil {
		t.Fatal("expected error for invalid_client response")
	}
}

func TestFetchTATWithAssertionPreservesPolicyClassification(t *testing.T) {
	for _, tc := range []struct {
		code    int
		subtype errs.Subtype
	}{
		{21000, errs.SubtypeChallengeRequired}, {21001, errs.SubtypeAccessDenied},
	} {
		rt := &stubRoundTripper{respCode: 400, respBody: fmt.Sprintf(`{"code":%d,"error":"access_denied","error_description":"policy rejected"}`, tc.code)}
		_, err := FetchTATWithAssertion(context.Background(), &http.Client{Transport: rt}, core.BrandFeishu, "cli_app", newFakeTATSigner(t), "key")
		problem, ok := errs.ProblemOf(err)
		if !ok || problem.Category != errs.CategoryPolicy || problem.Subtype != tc.subtype {
			t.Fatalf("policy error downgraded: %v", err)
		}
	}
}

func TestFetchTATWithAssertion_LimitsErrorBody(t *testing.T) {
	rt := &stubRoundTripper{respCode: 502, respBody: strings.Repeat("x", 2<<20)}
	hc := &http.Client{Transport: rt}

	_, err := FetchTATWithAssertion(context.Background(), hc, core.BrandFeishu, "cli_app", newFakeTATSigner(t), "k")
	if err == nil {
		t.Fatal("expected error")
	}
	if len(err.Error()) > (1<<20)+512 {
		t.Fatalf("error length = %d, want bounded", len(err.Error()))
	}
}

// Deterministic OAuth client rejections must be typed (ConfigError /
// SubtypeInvalidClient) so runProbePKJWT can tell "the key is not bound to this
// app" apart from transport noise.
func TestFetchTATWithAssertion_DeterministicReject_Typed(t *testing.T) {
	for _, oauthErr := range []string{"invalid_client", "unauthorized_client", "invalid_grant"} {
		rt := &stubRoundTripper{respCode: 401, respBody: `{"error":"` + oauthErr + `","error_description":"bad key"}`}
		hc := &http.Client{Transport: rt}
		_, err := FetchTATWithAssertion(context.Background(), hc, core.BrandFeishu, "cli_app", newFakeTATSigner(t), "k")
		if err == nil {
			t.Fatalf("%s: expected error", oauthErr)
		}
		if !errs.IsTyped(err) {
			t.Errorf("%s: must be typed, got %T", oauthErr, err)
		}
		var cfgErr *errs.ConfigError
		if !errors.As(err, &cfgErr) || cfgErr.Subtype != errs.SubtypeInvalidClient {
			t.Errorf("%s: want ConfigError/InvalidClient, got %T %v", oauthErr, err, err)
		}
	}
}

// Unrecognized OAuth errors and non-payload noise stay UNTYPED so the probe
// treats them as upstream noise and stays silent.
func TestFetchTATWithAssertion_AmbiguousError_Untyped(t *testing.T) {
	cases := []string{
		`{"error":"temporarily_unavailable","error_description":"retry"}`,
		`{"code":99999,"msg":"weird"}`,
		`not json`,
	}
	for _, body := range cases {
		rt := &stubRoundTripper{respCode: 503, respBody: body}
		hc := &http.Client{Transport: rt}
		_, err := FetchTATWithAssertion(context.Background(), hc, core.BrandFeishu, "cli_app", newFakeTATSigner(t), "k")
		if err == nil {
			t.Fatalf("body %q: expected error", body)
		}
		if errs.IsTyped(err) {
			t.Errorf("body %q: must be UNTYPED, got typed %T", body, err)
		}
	}
}
