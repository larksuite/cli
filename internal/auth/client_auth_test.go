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
	"fmt"
	"net/url"
	"testing"

	"github.com/larksuite/cli/internal/auth/jwt"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keysigner"
)

// fakeAuthSigner is a real in-memory ECDSA P-256 signer for client-auth tests.
type fakeAuthSigner struct{ key *ecdsa.PrivateKey }

type fakeExternalAssertionSigner struct {
	keyRef, clientID, audience string
	calls                      int
}

func (f *fakeExternalAssertionSigner) SignClientAssertion(_ context.Context, keyRef, clientID, audience string) (string, string, error) {
	f.keyRef, f.clientID, f.audience = keyRef, clientID, audience
	f.calls++
	return jwt.ClientAssertionType, fmt.Sprintf("external.jwt.%d", f.calls), nil
}

func newFakeAuthSigner(t *testing.T) *fakeAuthSigner {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
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
	ca := ClientAuth{AppID: "cli_a", AppSecret: "test-secret"} // AuthMethod "" => client_secret
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
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	ca := ClientAuth{AppID: "cli_a", AuthMethod: core.AuthMethodPrivateKeyJWT} // Signer nil
	if _, err := ca.applyClientAssertion(context.Background(), url.Values{}, "aud"); err == nil {
		t.Fatal("expected error when private_key_jwt has no signer")
	}
}

func TestClientAuth_applyClientAssertion_UnknownProviderFailsClosed(t *testing.T) {
	ca := ClientAuth{AppID: "cli_a", AuthMethod: core.AuthMethodPrivateKeyJWT, Signer: newFakeAuthSigner(t), KeyLabel: "k", KeyProvider: "evil.provider"}
	form := url.Values{}
	used, err := ca.applyClientAssertion(context.Background(), form, "aud")
	if err == nil || used || form.Has("client_assertion") {
		t.Fatalf("unknown provider must fail closed: used=%v form=%v err=%v", used, form, err)
	}
}

func TestClientAuth_applyClientAssertion_NilExternalProviderDoesNotFallback(t *testing.T) {
	previous := resolveExternalAssertionSigner
	resolveExternalAssertionSigner = func(context.Context, string) (clientAssertionSigner, error) {
		return nil, nil
	}
	t.Cleanup(func() { resolveExternalAssertionSigner = previous })

	ca := ClientAuth{
		AppID: "cli_a", AppSecret: "must-not-send", AuthMethod: core.AuthMethodPrivateKeyJWT,
		Signer: newFakeAuthSigner(t), KeyLabel: "k", KeyProvider: core.KeylessProviderLarkSuite,
	}
	form := url.Values{}
	used, err := ca.applyClientAssertion(context.Background(), form, "aud")
	if err == nil || used || form.Has("client_assertion") || form.Has("client_secret") {
		t.Fatalf("nil external provider must fail closed: used=%v form=%v err=%v", used, form, err)
	}
}

func TestClientAuth_applyClientAssertion_ExplicitProviderDoesNotUseBuiltinOrSecret(t *testing.T) {
	fake := &fakeExternalAssertionSigner{}
	previous := resolveExternalAssertionSigner
	resolveExternalAssertionSigner = func(_ context.Context, provider string) (clientAssertionSigner, error) {
		if provider != core.KeylessProviderLarkSuite {
			t.Fatalf("provider = %q", provider)
		}
		return fake, nil
	}
	t.Cleanup(func() { resolveExternalAssertionSigner = previous })

	ca := ClientAuth{
		AppID: "cli_external", AppSecret: "must-not-send", AuthMethod: core.AuthMethodPrivateKeyJWT,
		Signer: newFakeAuthSigner(t), KeyLabel: "openclaw-lark", KeyProvider: core.KeylessProviderLarkSuite,
	}
	form := url.Values{}
	used, err := ca.applyClientAssertion(context.Background(), form, "open.feishu.cn")
	if err != nil || !used {
		t.Fatalf("applyClientAssertion = used %v err %v", used, err)
	}
	if form.Get("client_assertion") != "external.jwt.1" || form.Has("client_secret") ||
		fake.keyRef != "openclaw-lark" || fake.clientID != "cli_external" || fake.audience != "open.feishu.cn" {
		t.Fatalf("form=%v signer=(%q,%q,%q)", form, fake.keyRef, fake.clientID, fake.audience)
	}
}

func TestClientAuth_ResolveSignerPreparedCopyReusesResolutionAndRemintsAssertions(t *testing.T) {
	fake := &fakeExternalAssertionSigner{}
	resolveCalls := 0
	previous := resolveExternalAssertionSigner
	resolveExternalAssertionSigner = func(_ context.Context, provider string) (clientAssertionSigner, error) {
		resolveCalls++
		if provider != core.KeylessProviderLarkSuite {
			t.Fatalf("provider = %q", provider)
		}
		return fake, nil
	}
	t.Cleanup(func() { resolveExternalAssertionSigner = previous })

	original := ClientAuth{
		AppID: "cli_external", AuthMethod: core.AuthMethodPrivateKeyJWT,
		KeyLabel: "openclaw-lark", KeyProvider: core.KeylessProviderLarkSuite,
	}
	prepared, err := original.ResolveSigner(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if original.externalSigner != nil {
		t.Fatal("ResolveSigner must return a prepared copy without mutating the original")
	}

	forms := []url.Values{{}, {}}
	for _, form := range forms {
		used, err := prepared.applyClientAssertion(context.Background(), form, "open.feishu.cn")
		if err != nil || !used {
			t.Fatalf("applyClientAssertion = used %v err %v", used, err)
		}
	}

	if resolveCalls != 1 {
		t.Fatalf("provider resolution calls = %d, want 1", resolveCalls)
	}
	if fake.calls != 2 {
		t.Fatalf("assertion signing calls = %d, want 2", fake.calls)
	}
	first := forms[0].Get("client_assertion")
	second := forms[1].Get("client_assertion")
	if first == "" || second == "" || first == second {
		t.Fatalf("assertions = (%q, %q), want two fresh values", first, second)
	}
	for _, form := range forms {
		if form.Has("client_secret") {
			t.Fatalf("private_key_jwt form leaked client_secret: %v", form)
		}
	}
}

func TestClientAuthFromConfig(t *testing.T) {
	ca := ClientAuthFromConfig(&core.CliConfig{
		AppID:      "cli_x",
		AppSecret:  "test-secret",
		AuthMethod: core.AuthMethodPrivateKeyJWT,
		KeyLabel:   "label-1",
	})
	if ca.AppID != "cli_x" || ca.AppSecret != "test-secret" || ca.AuthMethod != core.AuthMethodPrivateKeyJWT || ca.KeyLabel != "label-1" {
		t.Errorf("ClientAuth = %+v", ca)
	}
}
