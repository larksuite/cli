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
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/auth/jwt"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/keylesshelper"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/vfs"
)

// fakeAuthSigner is a real in-memory ECDSA P-256 signer for client-auth tests.
type fakeAuthSigner struct{ key *ecdsa.PrivateKey }

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

func TestClientAuth_applyClientAssertion_MalformedConfigDoesNotFallback(t *testing.T) {
	signer := newFakeAuthSigner(t)
	t.Setenv(envvars.CliKeylessSignerCmd, "")
	if err := vfs.WriteFile(core.GetConfigPath(), []byte("{not-json"), 0600); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}

	ca := ClientAuth{AppID: "cli_a", AuthMethod: core.AuthMethodPrivateKeyJWT, Signer: signer, KeyLabel: "k"}
	form := url.Values{}
	used, err := ca.applyClientAssertion(context.Background(), form, "aud")
	if err == nil {
		t.Fatal("expected malformed config error instead of platform-signer fallback")
	}
	if used || form.Has("client_assertion") {
		t.Fatalf("malformed config used fallback signer: used=%v form=%v", used, form)
	}
	if !strings.Contains(err.Error(), "config.json keylessSignerCmd") || !strings.Contains(err.Error(), core.GetConfigPath()) {
		t.Fatalf("error = %q, want signer source and config path", err)
	}
}

func TestSignClientAssertion_KeylessHelper(t *testing.T) {
	t.Setenv(envvars.CliKeylessSignerCmd, keylessHelperTestCommand(t))
	t.Setenv("LARKSUITE_CLI_KEYLESS_HELPER_ASSERT", `{"op":"sign-assertion","keyRef":"agent-key","clientId":"cli_a","aud":"aud"}`)

	helper, err := keylesshelper.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	assertionType, assertion, err := SignClientAssertion(context.Background(), nil, helper, "agent-key", "cli_a", "aud")
	if err != nil {
		t.Fatal(err)
	}
	if assertionType != jwt.ClientAssertionType || assertion != "helper.jwt" {
		t.Fatalf("assertion = (%q, %q)", assertionType, assertion)
	}
}

func keylessHelperTestCommand(t *testing.T) string {
	t.Helper()
	argv, err := json.Marshal([]string{os.Args[0], "-test.run=TestKeylessHelperProcess", "--"})
	if err != nil {
		t.Fatal(err)
	}
	return string(argv)
}

func TestKeylessHelperProcess(t *testing.T) {
	want := os.Getenv("LARKSUITE_CLI_KEYLESS_HELPER_ASSERT")
	if want == "" {
		return
	}
	var got map[string]any
	if err := json.NewDecoder(os.Stdin).Decode(&got); err != nil {
		t.Fatal(err)
	}
	var expected map[string]any
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatal(err)
	}
	for k, v := range expected {
		if got[k] != v {
			t.Fatalf("%s = %v, want %v; request=%v", k, got[k], v, got)
		}
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"ok":                    true,
		"client_assertion_type": jwt.ClientAssertionType,
		"client_assertion":      "helper.jwt",
	})
	os.Exit(0)
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
