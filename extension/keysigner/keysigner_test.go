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
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
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

func TestECDSADERToJOSE(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	// Iterate so we hit signatures whose r or s has its high bit set (ASN.1 pads
	// those with a leading 0x00) and whose scalars are short (need left-zero
	// padding) — verifying fixed-width conversion in both directions.
	for i := 0; i < 64; i++ {
		digest := sha256.Sum256([]byte{byte(i), byte(i >> 8), 'j', 'w', 't'})
		der, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		jose, err := ecdsaDERToJOSE(der, 32)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		if len(jose) != 64 {
			t.Fatalf("iter %d: len(jose)=%d, want 64 (fixed-width r||s)", i, len(jose))
		}
		r := new(big.Int).SetBytes(jose[:32])
		s := new(big.Int).SetBytes(jose[32:])
		if !ecdsa.Verify(&key.PublicKey, digest[:], r, s) {
			t.Fatalf("iter %d: converted r||s did not verify against the public key", i)
		}
	}
}

func TestECDSADERToJOSE_Errors(t *testing.T) {
	if _, err := ecdsaDERToJOSE([]byte{0x01, 0x02, 0x03}, 32); err == nil {
		t.Error("garbage DER: expected error")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("trailing"))
	der, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ecdsaDERToJOSE(append(der, 0x00), 32); err == nil {
		t.Error("DER with trailing byte: expected error")
	}
}

type stubSigner struct{}

func (stubSigner) EnsureKey(context.Context, KeyRef) (crypto.PublicKey, error)  { return nil, nil }
func (stubSigner) PublicKey(context.Context, KeyRef) (crypto.PublicKey, error)  { return nil, nil }
func (stubSigner) Sign(context.Context, KeyRef, []byte) ([]byte, string, error) { return nil, "", nil }

func TestCleanProbeError(t *testing.T) {
	cause := errors.New("open /dev/tpmrm0: permission denied")
	const p = "sks: error fetching Secure Hardware Vendor Data: "

	// sks double-wraps with the same %w prefix → collapse to a single prefix.
	doubled := fmt.Errorf(p+"%w", fmt.Errorf(p+"%w", cause))
	if got, want := cleanProbeError(doubled), p+cause.Error(); got != want {
		t.Errorf("doubled: got %q, want %q", got, want)
	}
	// Triple wrap collapses too.
	if got, want := cleanProbeError(fmt.Errorf(p+"%w", doubled)), p+cause.Error(); got != want {
		t.Errorf("tripled: got %q, want %q", got, want)
	}
	// A layer adding genuinely new context is preserved.
	if got, want := cleanProbeError(fmt.Errorf("load: %w", cause)), "load: "+cause.Error(); got != want {
		t.Errorf("distinct prefix: got %q, want %q", got, want)
	}
	// nil and unwrapped-leaf cases.
	if got := cleanProbeError(nil); got != "" {
		t.Errorf("nil: got %q, want empty", got)
	}
	if got := cleanProbeError(cause); got != cause.Error() {
		t.Errorf("leaf: got %q, want %q", got, cause.Error())
	}
}

type proberSigner struct {
	stubSigner
	info HardwareInfo
}

func (p proberSigner) ProbeHardware(context.Context) (HardwareInfo, error) { return p.info, nil }

func TestProbeHardware(t *testing.T) {
	// nil signer and a signer that does not implement HardwareProber both yield ok=false.
	if _, ok, _ := probeHardware(context.Background(), nil); ok {
		t.Error("nil signer: ok should be false")
	}
	if _, ok, _ := probeHardware(context.Background(), stubSigner{}); ok {
		t.Error("non-prober signer: ok should be false")
	}

	want := HardwareInfo{Backend: "tpm2", Available: true, VendorName: "ACME"}
	info, ok, err := probeHardware(context.Background(), proberSigner{info: want})
	if err != nil || !ok {
		t.Fatalf("prober: ok=%v err=%v, want true/nil", ok, err)
	}
	if info != want {
		t.Errorf("info = %+v, want %+v", info, want)
	}
}

func TestRegistry(t *testing.T) {
	if Active() != nil {
		t.Skip("a signer is already registered in this build")
	}
	Register(stubSigner{})
	if _, ok := Active().(stubSigner); !ok {
		t.Error("Active did not return the registered signer")
	}
}
