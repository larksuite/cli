// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package jwt

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/extension/keysigner"
)

// fakeSigner is a real in-memory ECDSA P-256 signer, so tests exercise the full
// JWS path and the produced token is actually cryptographically verifiable.
type fakeSigner struct{ key *ecdsa.PrivateKey }

func newFakeSigner(t *testing.T) *fakeSigner {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeSigner{key: k}
}

func (f *fakeSigner) EnsureKey(context.Context, keysigner.KeyRef) (crypto.PublicKey, error) {
	return f.key.Public(), nil
}
func (f *fakeSigner) PublicKey(context.Context, keysigner.KeyRef) (crypto.PublicKey, error) {
	return f.key.Public(), nil
}
func (f *fakeSigner) Sign(_ context.Context, _ keysigner.KeyRef, in []byte) ([]byte, string, error) {
	h := sha256.Sum256(in)
	r, s, err := ecdsa.Sign(rand.Reader, f.key, h[:])
	if err != nil {
		return nil, "", err
	}
	// JOSE ES256: fixed-width big-endian r||s (32 bytes each for P-256).
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return sig, keysigner.AlgES256, nil
}

func TestBuildSignedJWT_VerifiableES256(t *testing.T) {
	f := newFakeSigner(t)
	now := time.Unix(1700000000, 0)

	tok, err := buildSignedJWT(context.Background(), f, keysigner.KeyRef{Label: "x"}, keysigner.AlgES256,
		map[string]any{}, clientAssertionClaims("cli_app", "https://accounts.example/token", now, 5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("want 3 JWS parts, got %d", len(parts))
	}

	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("header not base64url: %v", err)
	}
	var hdr map[string]any
	if err := json.Unmarshal(hb, &hdr); err != nil {
		t.Fatal(err)
	}
	if hdr["alg"] != "ES256" || hdr["typ"] != "JWT" {
		t.Errorf("header = %v, want alg=ES256 typ=JWT (server generalizedValidation requires typ)", hdr)
	}

	cb, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	if err := json.Unmarshal(cb, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["iss"] != "cli_app" || claims["sub"] != "cli_app" || claims["aud"] != "https://accounts.example/token" {
		t.Errorf("claims = %v", claims)
	}

	// Cryptographically verify the signature against the signing input.
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("sig not base64url: %v", err)
	}
	if len(sig) != 64 {
		t.Fatalf("ES256 sig len = %d, want 64", len(sig))
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(f.key.Public().(*ecdsa.PublicKey), h[:], r, s) {
		t.Error("signature did not verify")
	}
}

func TestBuildSignedJWT_NilSigner(t *testing.T) {
	if _, err := buildSignedJWT(context.Background(), nil, keysigner.KeyRef{}, "ES256", nil, nil); err == nil {
		t.Fatal("expected error for nil signer")
	}
}

func TestBuildSignedJWT_AlgMismatch(t *testing.T) {
	f := newFakeSigner(t) // always reports ES256
	if _, err := buildSignedJWT(context.Background(), f, keysigner.KeyRef{}, keysigner.AlgRS256, nil, nil); err == nil {
		t.Fatal("expected error when header alg != signer alg")
	}
}

func TestSignClientAssertion(t *testing.T) {
	f := newFakeSigner(t)
	now := time.Unix(1700000000, 0)
	const aud = "https://accounts.feishu.cn/open-apis/authen/v2/oauth/token"

	tok, err := SignClientAssertion(context.Background(), f, keysigner.KeyRef{Label: "k"}, "cli_app", aud, now)
	if err != nil {
		t.Fatal(err)
	}

	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("want 3 parts, got %d", len(parts))
	}
	cb, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	if err := json.Unmarshal(cb, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["iss"] != "cli_app" || claims["aud"] != aud {
		t.Errorf("claims = %v", claims)
	}

	// Signature must verify against the key's public half.
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(f.key.Public().(*ecdsa.PublicKey), h[:], r, s) {
		t.Error("client_assertion signature did not verify")
	}
}

func TestSignClientAssertion_NilSigner(t *testing.T) {
	if _, err := SignClientAssertion(context.Background(), nil, keysigner.KeyRef{}, "cli_app", "aud", time.Unix(0, 0)); err == nil {
		t.Fatal("expected error for nil signer")
	}
}

func TestSignAttestation(t *testing.T) {
	f := newFakeSigner(t)
	now := time.Unix(1700000000, 0)

	tok, err := SignAttestation(context.Background(), f, keysigner.KeyRef{Label: "k"}, "nonce-abc", now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("want 3 parts, got %d", len(parts))
	}

	hb, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var hdr map[string]any
	if err := json.Unmarshal(hb, &hdr); err != nil {
		t.Fatal(err)
	}
	jwk, ok := hdr["jwk"].(map[string]any)
	if !ok {
		t.Fatalf("attestation header missing jwk: %v", hdr)
	}
	if jwk["kty"] != "EC" || jwk["crv"] != "P-256" || jwk["use"] != "sig" {
		t.Errorf("jwk = %v", jwk)
	}

	cb, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	if err := json.Unmarshal(cb, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["nonce"] != "nonce-abc" {
		t.Errorf("nonce = %v", claims["nonce"])
	}
	// jti, iat, exp are all required by the attestation spec.
	iat, iatOK := claims["iat"].(float64)
	exp, expOK := claims["exp"].(float64)
	if !iatOK || !expOK || exp <= iat {
		t.Errorf("claims iat/exp invalid: iat=%v exp=%v", claims["iat"], claims["exp"])
	}
	if jti, _ := claims["jti"].(string); jti == "" {
		t.Error("claims jti empty")
	}

	// Signature verifies against the embedded key.
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(f.key.Public().(*ecdsa.PublicKey), h[:], r, s) {
		t.Error("attestation signature did not verify")
	}
}

func TestSignAttestation_NilSigner(t *testing.T) {
	if _, err := SignAttestation(context.Background(), nil, keysigner.KeyRef{}, "n", time.Unix(0, 0)); err == nil {
		t.Fatal("expected error for nil signer")
	}
}

func TestClaimFactories(t *testing.T) {
	now := time.Unix(1700000000, 0)

	a := attestationClaims("nonce-xyz", now)
	if a["nonce"] != "nonce-xyz" || a["iat"] != now.Unix() {
		t.Errorf("attestation claims = %v", a)
	}
	if a["exp"] != now.Add(attestationTTL).Unix() {
		t.Errorf("attestation exp = %v, want %v", a["exp"], now.Add(attestationTTL).Unix())
	}
	if jti, _ := a["jti"].(string); jti == "" {
		t.Error("attestation jti empty")
	}

	c := clientAssertionClaims("cli_app", "aud", now, time.Minute)
	if c["exp"].(int64) != now.Add(time.Minute).Unix() {
		t.Errorf("client_assertion exp = %v", c["exp"])
	}
}
