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
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
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
	method := core.AuthMethodPrivateKeyJWTLocalKeyPair
	cfg := &core.CliConfig{AppID: "cli_test", AuthMethod: method, KeySource: core.KeySourceTEE,
		KeyProvider: keysigner.SoftwareSignerName, KeyLabel: ref.Label}
	reopened, err := ResolveConfigSigner(cfg, kc)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{}
	used, err := ClientAuthFromConfig(cfg, reopened).ApplyClientAssertion(ctx, form, "https://example.com/token")
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

// fakeAuthSigner is a real in-memory ECDSA P-256 signer for client-auth tests.
type fakeAuthSigner struct {
	key                *ecdsa.PrivateKey
	publicErr, signErr error
	ensureCalls        int
}

func (*fakeAuthSigner) Name() string { return keysigner.MacOSKeychainSignerName }

func (*fakeAuthSigner) SecurityLevel() keysigner.SecurityLevel { return keysigner.SecurityLevelL3 }

func newFakeAuthSigner(t *testing.T) *fakeAuthSigner {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"HOME", "LOCALAPPDATA", "LARKSUITE_CLI_DATA_DIR", "LARKSUITE_CLI_CONFIG_DIR"} {
		t.Setenv(name, root)
	}
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeAuthSigner{key: k}
}

func (f *fakeAuthSigner) EnsureKey(context.Context, keysigner.KeyRef) (crypto.PublicKey, error) {
	f.ensureCalls++
	return f.key.Public(), nil
}
func (f *fakeAuthSigner) PublicKey(context.Context, keysigner.KeyRef) (crypto.PublicKey, error) {
	if f.publicErr != nil {
		return nil, f.publicErr
	}
	return f.key.Public(), nil
}
func (f *fakeAuthSigner) Sign(_ context.Context, _ keysigner.KeyRef, in []byte) ([]byte, string, error) {
	if f.signErr != nil {
		return nil, "", f.signErr
	}
	return keysigner.SignWithPrivateKey(f.key, in)
}

func (*fakeAuthSigner) DeleteKey(context.Context, keysigner.KeyRef) error { return nil }

func jwtTestPart(t *testing.T, value string, index int) map[string]any {
	t.Helper()
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		t.Fatal("invalid compact JWT")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[index])
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func checkPrivateKeyJWTLocalKeyPairRequest(t *testing.T, assertion, proof, appID, kid string, seen map[string]bool) {
	t.Helper()
	header, claims := jwtTestPart(t, assertion, 0), jwtTestPart(t, assertion, 1)
	if header["typ"] != "JWT" || header["kid"] != kid || claims["iss"] != appID || claims["sub"] != appID || claims["aud"] != core.ClientAssertionAudience(core.BrandFeishu) {
		t.Fatalf("unexpected assertion identity: header=%v claims=%v", header, claims)
	}
	jti, _ := claims["jti"].(string)
	if jti == "" || seen[jti] {
		t.Fatal("client assertion was reused")
	}
	seen[jti] = true
	if proof != "" {
		proofHeader := jwtTestPart(t, proof, 0)
		if proofHeader["typ"] != "dpop+jwt" {
			t.Fatal("assertion used as DPoP proof")
		}
		proofClaims := jwtTestPart(t, proof, 1)
		proofJTI, _ := proofClaims["jti"].(string)
		if proofJTI == "" || seen[proofJTI] {
			t.Fatal("DPoP proof was reused")
		}
		seen[proofJTI] = true
	}
}

func TestClientAuthSigningKeyMissing(t *testing.T) {
	policyErr := errs.NewSecurityPolicyError(errs.SubtypeAccessDenied, "signing denied").WithCause(keysigner.ErrKeyNotFound)
	for _, tc := range []struct {
		name    string
		cause   error
		missing bool
	}{
		{"missing", fmt.Errorf("backend: %w", keysigner.ErrKeyNotFound), true},
		{"locked", keysigner.ErrUnlockRequired, false},
		{"unavailable", keysigner.ErrUnavailable, false},
		{"corrupt", keysigner.ErrCorrupt, false},
		{"typed policy", policyErr, false},
	} {
		for _, stage := range []string{"public key", "sign"} {
			t.Run(tc.name+"/"+stage, func(t *testing.T) {
				signer := newFakeAuthSigner(t)
				if stage == "public key" {
					signer.publicErr = tc.cause
				} else {
					signer.signErr = tc.cause
				}
				ca := ClientAuth{AppID: "cli_missing_key", AppSecret: "must-not-send", AuthMethod: core.AuthMethodPrivateKeyJWTLocalKeyPair, Signer: signer, KeyLabel: "lost-key"}
				calls := 0
				client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					calls++
					return nil, errors.New("unexpected HTTP request")
				})}
				result, err := RequestDeviceAuthorization(context.Background(), client, ca, core.BrandFeishu, "", nil)
				if result != nil || !errors.Is(err, tc.cause) || calls != 0 || signer.ensureCalls != 0 {
					t.Fatalf("result=%v err=%v HTTP calls=%d key creations=%d", result, err, calls, signer.ensureCalls)
				}
				problem, typed := errs.ProblemOf(err)
				if tc.missing {
					if !typed || problem.Category != errs.CategoryAuthentication || problem.Subtype != errs.SubtypeSigningKeyMissing || problem.Retryable ||
						!strings.Contains(problem.Hint, "re-register") || !strings.Contains(problem.Hint, "lark-cli config init --private-key-jwt") ||
						!strings.Contains(problem.Hint, "in a terminal") || !strings.Contains(problem.Hint, "existing-app") || !strings.Contains(problem.Hint, ca.AppID) || strings.Contains(problem.Hint, "--new") {
						t.Fatalf("missing key error lacks authentication classification or registration hint: %v", err)
					}
				} else if tc.cause == policyErr {
					if !typed || problem.Category != errs.CategoryPolicy || problem.Subtype != errs.SubtypeAccessDenied || problem.Hint != "" {
						t.Fatalf("typed policy error changed: %v", err)
					}
				} else if typed {
					t.Fatalf("non-missing error reclassified: %v", err)
				}
			})
		}
	}
}

func TestResolveConfigSignerRestoresRegistrationBackends(t *testing.T) {
	cfg := &core.CliConfig{AuthMethod: core.AuthMethodPrivateKeyJWTLocalKeyPair, KeySource: core.KeySourceTEE, KeyLabel: "key-1"}
	kc := softwareTestKeychain{}
	for _, candidate := range keylesshelper.RegistrationSigners(kc) {
		cfg.KeyProvider = candidate.Name()
		resolved, err := ResolveConfigSigner(cfg, kc)
		if err != nil || resolved == nil || resolved.Name() != cfg.KeyProvider {
			t.Fatalf("resolve %q: got %v, err = %v", cfg.KeyProvider, resolved, err)
		}
	}

	cfg.KeyProvider = keysigner.MacOSSecureEnclaveSignerName
	if runtime.GOOS == "darwin" {
		cfg.KeyProvider = keysigner.LinuxTPMSignerName
	}
	if signer, err := ResolveConfigSigner(cfg, kc); signer != nil || !errors.Is(err, keysigner.ErrUnavailable) {
		t.Fatalf("cross-platform provider %q: got %v, err = %v", cfg.KeyProvider, signer, err)
	}
}
