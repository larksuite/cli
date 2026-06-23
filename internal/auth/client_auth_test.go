// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"net/url"
	"testing"

	"github.com/larksuite/cli/internal/auth/jwt"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keysigner"
)

// fakeAuthSigner is a real in-memory ECDSA P-256 signer for client-auth tests.
type fakeAuthSigner struct{ key *ecdsa.PrivateKey }

func newFakeAuthSigner(t *testing.T) *fakeAuthSigner {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeAuthSigner{key: k}
}

func (f *fakeAuthSigner) EnsureKey(context.Context, keysigner.KeyRef) (crypto.PublicKey, error) {
	return f.key.Public(), nil
}
func (f *fakeAuthSigner) PublicKey(context.Context, keysigner.KeyRef) (crypto.PublicKey, error) {
	return f.key.Public(), nil
}
func (f *fakeAuthSigner) Sign(_ context.Context, _ keysigner.KeyRef, in []byte) ([]byte, string, error) {
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

func TestClientAuth_applyClientAssertion_ClientSecret(t *testing.T) {
	ca := ClientAuth{AppID: "cli_a", AppSecret: "sec"} // AuthMethod "" => client_secret
	form := url.Values{}
	used, err := ca.applyClientAssertion(context.Background(), form, "https://aud/token")
	if err != nil {
		t.Fatal(err)
	}
	if used {
		t.Error("client_secret must not produce a client_assertion")
	}
	if form.Has("client_assertion") || form.Has("client_assertion_type") {
		t.Errorf("form should be untouched, got %v", form)
	}
}

func TestClientAuth_applyClientAssertion_PrivateKeyJWT(t *testing.T) {
	ca := ClientAuth{
		AppID:      "cli_a",
		AuthMethod: core.AuthMethodPrivateKeyJWT,
		Signer:     newFakeAuthSigner(t),
		KeyLabel:   "k",
	}
	form := url.Values{}
	used, err := ca.applyClientAssertion(context.Background(), form, "https://accounts.feishu.cn/open-apis/authen/v2/oauth/token")
	if err != nil {
		t.Fatal(err)
	}
	if !used {
		t.Fatal("expected client_assertion to be applied")
	}
	if form.Get("client_assertion_type") != jwt.ClientAssertionType {
		t.Errorf("client_assertion_type = %q", form.Get("client_assertion_type"))
	}
	if form.Get("client_assertion") == "" {
		t.Error("client_assertion is empty")
	}
	if form.Has("client_secret") {
		t.Error("client_secret must NOT be present for private_key_jwt")
	}
}

func TestClientAuth_applyClientAssertion_NilSigner(t *testing.T) {
	ca := ClientAuth{AppID: "cli_a", AuthMethod: core.AuthMethodPrivateKeyJWT} // Signer nil
	if _, err := ca.applyClientAssertion(context.Background(), url.Values{}, "aud"); err == nil {
		t.Fatal("expected error when private_key_jwt has no signer")
	}
}

func TestClientAuthFromConfig(t *testing.T) {
	ca := ClientAuthFromConfig(&core.CliConfig{
		AppID:      "cli_x",
		AppSecret:  "s",
		AuthMethod: core.AuthMethodPrivateKeyJWT,
		KeyLabel:   "label-1",
	})
	if ca.AppID != "cli_x" || ca.AppSecret != "s" || ca.AuthMethod != core.AuthMethodPrivateKeyJWT || ca.KeyLabel != "label-1" {
		t.Errorf("ClientAuth = %+v", ca)
	}
}
