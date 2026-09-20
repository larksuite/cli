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
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/auth/jwt"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keylesshelper"
	"github.com/larksuite/cli/internal/keysigner"
)

type softwareTestKeychain struct{ keychain.KeychainAccess }

func (softwareTestKeychain) Get(string, string) (string, error) {
	return "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", nil
}

func TestClientAuthRestoresSoftwareSigner(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	root := t.TempDir()
	for _, name := range []string{"HOME", "USERPROFILE", "LOCALAPPDATA", "LARKSUITE_CLI_DATA_DIR"} {
		t.Setenv(name, root)
	}
	kc := softwareTestKeychain{}
	ctx := context.Background()
	signer, err := keylesshelper.ResolveSigner(keysigner.SoftwareSignerName, kc)
	if err != nil {
		t.Fatal(err)
	}
	ref := keysigner.KeyRef{Label: "software-auth-test"}
	public, err := signer.EnsureKey(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{core.AuthMethodPrivateKeyJWT, core.AuthMethodPrivateKeyJWTLocalKeyPair} {
		cfg := &core.CliConfig{AppID: "cli_test", AuthMethod: method, KeySource: core.SecretSourceTEE,
			KeyProvider: keysigner.SoftwareSignerName, KeyLabel: ref.Label}
		reopened, err := ResolveConfigSigner(cfg, kc)
		if err != nil {
			t.Fatal(err)
		}
		form := url.Values{}
		used, err := ClientAuthFromConfig(cfg, reopened).applyClientAssertion(ctx, form, "https://example.com/token")
		if err != nil || !used || form.Get("client_assertion_type") != jwt.ClientAssertionType || form.Has("client_secret") {
			t.Fatalf("software client assertion: used=%v, error=%v", used, err)
		}
		parts := strings.Split(form.Get("client_assertion"), ".")
		if len(parts) != 3 {
			t.Fatal("invalid JWT structure")
		}
		sig, err := base64.RawURLEncoding.DecodeString(parts[2])
		if err != nil || len(sig) != 64 {
			t.Fatalf("invalid ES256 signature: %v", err)
		}
		digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if !ecdsa.Verify(public.(*ecdsa.PublicKey), digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
			t.Fatal("restored software signer used a different key")
		}
	}
	if err := signer.DeleteKey(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := signer.PublicKey(ctx, ref); !errors.Is(err, keysigner.ErrKeyNotFound) {
		t.Fatalf("missing software key must not be recreated: %v", err)
	}
}

// fakeAuthSigner is a real in-memory ECDSA P-256 signer for client-auth tests.
type fakeAuthSigner struct{ key *ecdsa.PrivateKey }

func (*fakeAuthSigner) Name() string { return keysigner.MacOSKeychainSignerName }

func (*fakeAuthSigner) SecurityLevel() keysigner.SecurityLevel { return keysigner.SecurityLevelL3 }

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

func (*fakeAuthSigner) DeleteKey(context.Context, keysigner.KeyRef) error { return nil }

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
	for _, method := range []string{
		core.AuthMethodPrivateKeyJWT,
		core.AuthMethodPrivateKeyJWTLocalKeyPair,
	} {
		t.Run(method, func(t *testing.T) {
			ca := ClientAuth{
				AppID:      "cli_a",
				AuthMethod: method,
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
				t.Errorf("client_secret must not be present for %s", method)
			}
		})
	}
}

func TestClientAuth_applyClientAssertion_NilSigner(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, method := range []string{
		core.AuthMethodPrivateKeyJWT,
		core.AuthMethodPrivateKeyJWTLocalKeyPair,
	} {
		ca := ClientAuth{AppID: "cli_a", AuthMethod: method}
		if _, err := ca.applyClientAssertion(context.Background(), url.Values{}, "aud"); err == nil {
			t.Fatalf("expected error when %s has no signer", method)
		}
	}
}

func TestClientAuth_applyClientAssertion_UnknownProviderFailsClosed(t *testing.T) {
	ca := ClientAuth{AppID: "cli_a", AuthMethod: core.AuthMethodPrivateKeyJWTLocalKeyPair, Signer: newFakeAuthSigner(t), KeyLabel: "k", KeyProvider: "evil.provider"}
	form := url.Values{}
	used, err := ca.applyClientAssertion(context.Background(), form, "aud")
	if err == nil || used || form.Has("client_assertion") {
		t.Fatalf("unknown provider must fail closed: used=%v form=%v err=%v", used, form, err)
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
		return fake, nil
	}
	t.Cleanup(func() { resolveExternalAssertionSigner = previous })

	original := ClientAuth{AppID: "cli_external", AuthMethod: core.AuthMethodPrivateKeyJWTLocalKeyPair, KeyLabel: "openclaw-lark", KeyProvider: core.KeylessProviderLarkSuite}
	prepared, err := original.ResolveSigner(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	forms := []url.Values{{}, {}}
	for _, form := range forms {
		used, err := prepared.applyClientAssertion(context.Background(), form, "open.feishu.cn")
		if err != nil || !used {
			t.Fatalf("applyClientAssertion = used %v err %v", used, err)
		}
	}
	if resolveCalls != 1 || fake.calls != 2 || forms[0].Get("client_assertion") == forms[1].Get("client_assertion") {
		t.Fatalf("resolveCalls=%d signCalls=%d assertions=(%q,%q)", resolveCalls, fake.calls, forms[0].Get("client_assertion"), forms[1].Get("client_assertion"))
	}
}

func TestClientAuthFromConfig(t *testing.T) {
	signer := newFakeAuthSigner(t)
	ca := ClientAuthFromConfig(&core.CliConfig{
		AppID:       "cli_x",
		AppSecret:   "test-secret",
		AuthMethod:  core.AuthMethodPrivateKeyJWTLocalKeyPair,
		KeyLabel:    "label-1",
		KeyProvider: core.KeylessProviderLarkSuite,
	}, signer)
	if ca.AppID != "cli_x" || ca.AppSecret != "test-secret" || ca.AuthMethod != core.AuthMethodPrivateKeyJWTLocalKeyPair || ca.KeyLabel != "label-1" || ca.KeyProvider != core.KeylessProviderLarkSuite {
		t.Errorf("ClientAuth = %+v", ca)
	}
	if ca.Signer != signer {
		t.Fatal("ClientAuth did not retain the invocation signer")
	}
}

func TestResolveConfigSignerRestoresRecordedNativeBackend(t *testing.T) {
	names := keysigner.PlatformSignerNames()
	if len(names) == 0 {
		t.Skip("current platform has no native signer")
	}

	for _, method := range []string{
		core.AuthMethodPrivateKeyJWT,
		core.AuthMethodPrivateKeyJWTLocalKeyPair,
	} {
		t.Run(method, func(t *testing.T) {
			resolved, err := ResolveConfigSigner(&core.CliConfig{
				AuthMethod:  method,
				KeySource:   core.SecretSourceTEE,
				KeyProvider: names[0],
				KeyLabel:    "key-1",
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if resolved == nil || resolved.Name() != names[0] {
				t.Fatalf("resolved signer = %T, want %s", resolved, names[0])
			}
		})
	}
}
