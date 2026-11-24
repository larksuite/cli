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

	"github.com/larksuite/cli/internal/keysigner"
	"github.com/stretchr/testify/require"
)

// fakeSigner is a real in-memory ECDSA P-256 signer, so tests exercise the full
// JWS path and the produced token is actually cryptographically verifiable.
type fakeSigner struct{ key *ecdsa.PrivateKey }

func (*fakeSigner) Name() string { return "fake" }

func (*fakeSigner) SecurityLevel() keysigner.SecurityLevel { return keysigner.SecurityLevelL3 }

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
	return keysigner.SignWithPrivateKey(f.key, in)
}

func (*fakeSigner) DeleteKey(context.Context, keysigner.KeyRef) error { return nil }

func verifyTestJWT(t *testing.T, token string, key *ecdsa.PublicKey) (header, claims map[string]any) {
	t.Helper()
	parts := strings.Split(token, ".")
	require.Len(t, parts, 3)
	decode := func(part string) []byte {
		t.Helper()
		data, err := base64.RawURLEncoding.DecodeString(part)
		require.NoError(t, err)
		return data
	}
	require.NoError(t, json.Unmarshal(decode(parts[0]), &header))
	require.NoError(t, json.Unmarshal(decode(parts[1]), &claims))
	signature := decode(parts[2])
	require.Len(t, signature, 64)
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	require.True(t, ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])))
	require.Equal(t, "JWT", header["typ"])
	require.Equal(t, "ES256", header["alg"])
	kid, err := keysigner.PublicKeyThumbprint(key)
	require.NoError(t, err)
	require.Equal(t, kid, header["kid"])
	require.NotEmpty(t, claims["jti"])
	return header, claims
}

func TestBuildSignedJWT_AlgMismatch(t *testing.T) {
	f := newFakeSigner(t) // always reports ES256
	if _, err := buildSignedJWT(context.Background(), f, keysigner.KeyRef{}, keysigner.AlgRS256, map[string]any{}, nil); err == nil {
		t.Fatal("expected error when header alg != signer alg")
	}
}

func TestSignClientAssertion(t *testing.T) {
	signer := newFakeSigner(t)
	now := time.Unix(1700000000, 0)
	const aud = "https://accounts.example"
	token, err := SignClientAssertion(context.Background(), signer, keysigner.KeyRef{Label: "key"}, "cli_app", aud, now)
	require.NoError(t, err)
	_, claims := verifyTestJWT(t, token, &signer.key.PublicKey)
	require.Equal(t, "cli_app", claims["iss"])
	require.Equal(t, "cli_app", claims["sub"])
	require.Equal(t, aud, claims["aud"])
	require.Equal(t, float64(now.Unix()), claims["iat"])
	require.Equal(t, float64(now.Add(5*time.Minute).Unix()), claims["exp"])
}

func TestSignAttestation(t *testing.T) {
	signer := newFakeSigner(t)
	now := time.Unix(1700000000, 0)
	token, err := SignAttestation(context.Background(), signer, keysigner.KeyRef{Label: "key"}, signer.key.Public(), "nonce-abc", now)
	require.NoError(t, err)
	header, claims := verifyTestJWT(t, token, &signer.key.PublicKey)
	jwk, err := keysigner.PublicKeyJWK(signer.key.Public())
	require.NoError(t, err)
	want := jwk.Map()
	want["alg"], want["use"] = keysigner.AlgES256, "sig"
	require.Equal(t, want, header["jwk"])
	require.Equal(t, "nonce-abc", claims["nonce"])
	require.Equal(t, float64(now.Unix()), claims["iat"])
	require.Equal(t, float64(now.Add(5*time.Minute).Unix()), claims["exp"])
}

func TestSigningRequiresSigner(t *testing.T) {
	_, err := SignClientAssertion(context.Background(), nil, keysigner.KeyRef{}, "cli_app", "aud", time.Now())
	require.Error(t, err)
	_, err = SignAttestation(context.Background(), nil, keysigner.KeyRef{}, newFakeSigner(t).key.Public(), "nonce", time.Now())
	require.Error(t, err)
}
