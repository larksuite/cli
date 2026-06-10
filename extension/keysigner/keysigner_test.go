// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"reflect"
	"testing"
)

func TestAlgForKey(t *testing.T) {
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if alg, err := AlgForKey(ec.Public()); err != nil || alg != AlgES256 {
		t.Errorf("P-256: alg=%q err=%v, want ES256/nil", alg, err)
	}

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if alg, err := AlgForKey(rsaKey.Public()); err != nil || alg != AlgRS256 {
		t.Errorf("RSA: alg=%q err=%v, want RS256/nil", alg, err)
	}

	ec384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AlgForKey(ec384.Public()); err == nil {
		t.Error("P-384: expected unsupported-curve error")
	}

	if _, err := AlgForKey("not a key"); err == nil {
		t.Error("string: expected unsupported-type error")
	}
}

func TestEncodePublicKeyRoundTrip(t *testing.T) {
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := EncodePublicKey(ec.Public())
	if err != nil {
		t.Fatal(err)
	}
	der, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("not valid base64: %v", err)
	}
	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		t.Fatalf("not valid PKIX: %v", err)
	}
	if !reflect.DeepEqual(pub, ec.Public()) {
		t.Error("public key did not round-trip")
	}
}

func TestPublicKeyJWK_EC(t *testing.T) {
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	jwk, err := PublicKeyJWK(ec.Public())
	if err != nil {
		t.Fatal(err)
	}
	if jwk["kty"] != "EC" || jwk["crv"] != "P-256" {
		t.Errorf("jwk = %v, want kty=EC crv=P-256", jwk)
	}
	if jwk["use"] != "sig" {
		t.Errorf("jwk use = %v, want sig", jwk["use"])
	}
	x, _ := jwk["x"].(string)
	xb, err := base64.RawURLEncoding.DecodeString(x)
	if err != nil || len(xb) != 32 {
		t.Errorf("x = %q (decoded %d bytes), want 32-byte base64url", x, len(xb))
	}
	if _, ok := jwk["y"].(string); !ok {
		t.Error("jwk missing y")
	}
}

func TestPublicKeyJWK_RSA(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwk, err := PublicKeyJWK(rsaKey.Public())
	if err != nil {
		t.Fatal(err)
	}
	if jwk["kty"] != "RSA" || jwk["n"] == "" || jwk["e"] == "" {
		t.Errorf("jwk = %v, want kty=RSA with n,e", jwk)
	}
	if jwk["use"] != "sig" {
		t.Errorf("jwk use = %v, want sig", jwk["use"])
	}
}

func TestPublicKeyJWK_UnsupportedCurve(t *testing.T) {
	ec384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PublicKeyJWK(ec384.Public()); err == nil {
		t.Error("P-384: expected error")
	}
}

type stubSigner struct{}

func (stubSigner) EnsureKey(context.Context, KeyRef) (crypto.PublicKey, error)  { return nil, nil }
func (stubSigner) PublicKey(context.Context, KeyRef) (crypto.PublicKey, error)  { return nil, nil }
func (stubSigner) Sign(context.Context, KeyRef, []byte) ([]byte, string, error) { return nil, "", nil }

func TestRegistry(t *testing.T) {
	if Active() != nil {
		t.Skip("a signer is already registered in this build")
	}
	Register(stubSigner{})
	if _, ok := Active().(stubSigner); !ok {
		t.Error("Active did not return the registered signer")
	}
}
