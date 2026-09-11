// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"errors"
	"io"
	"math/big"
	"runtime"
	"strings"
	"testing"
)

func TestNewSignerSelectsOnlyRequestedBackend(t *testing.T) {
	for _, tc := range []struct {
		name, platform string
		level          SecurityLevel
	}{
		{MacOSSecureEnclaveSignerName, "darwin", SecurityLevelL1},
		{MacOSKeychainSignerName, "darwin", SecurityLevelL2},
		{WindowsPlatformKSPSignerName, "windows", SecurityLevelL1},
		{WindowsSoftwareKSPSignerName, "windows", SecurityLevelL2},
		{LinuxTPMSignerName, "linux", SecurityLevelL1},
		{"unknown", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			signer := NewSigner(tc.name, func(string) (string, error) {
				t.Fatal("constructing a signer accessed its storage directory")
				return "", nil
			})
			if runtime.GOOS != tc.platform {
				if signer != nil {
					t.Fatal("unsupported name selected a backend")
				}
				return
			}
			if signer == nil || signer.Name() != tc.name || signer.SecurityLevel() != tc.level {
				t.Fatalf("NewSigner(%q) = %v, want the requested %s backend", tc.name, signer, tc.level)
			}
		})
	}
}

func TestSigningAlgorithms(t *testing.T) {
	for _, algorithm := range []signingAlgorithm{es256Algorithm{}, rs256Algorithm{}} {
		t.Run(algorithm.name(), func(t *testing.T) {
			private, err := algorithm.generateKey()
			if err != nil {
				t.Fatal(err)
			}
			public := private.Public()
			if err := algorithm.validatePublicKey(public); err != nil {
				t.Fatal(err)
			}
			if got, err := AlgForKey(public); err != nil || got != algorithm.name() {
				t.Fatalf("AlgForKey() = %q, %v", got, err)
			}
			encoded, err := EncodePublicKey(public)
			if err != nil {
				t.Fatal(err)
			}
			der, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := x509.ParsePKIXPublicKey(der)
			if err != nil {
				t.Fatal(err)
			}
			if !publicKeysEqual(public, parsed) {
				t.Fatal("PKIX encoding changed public identity")
			}
			jwk, err := PublicKeyJWK(public)
			if err != nil {
				t.Fatal(err)
			}
			switch algorithm.name() {
			case AlgES256:
				if jwk.Kty != "EC" || jwk.Crv != "P-256" || jwk.X == "" || jwk.Y == "" || jwk.N != "" || jwk.E != "" {
					t.Fatalf("unexpected ES256 JWK: %+v", jwk)
				}
			case AlgRS256:
				if jwk.Kty != "RSA" || jwk.N == "" || jwk.E == "" || jwk.Crv != "" || jwk.X != "" || jwk.Y != "" {
					t.Fatalf("unexpected RS256 JWK: %+v", jwk)
				}
			}

			input := []byte("header.payload")
			signature, err := algorithm.sign(private, input)
			if err != nil {
				t.Fatal(err)
			}
			verifySignature(t, public, algorithm.name(), input, signature)
			algorithm.clearPrivateKey(private)
			if !publicKeysEqual(public, parsed) {
				t.Fatal("clearing private material changed public identity")
			}
			assertPrivateKeyCleared(t, private)
		})
	}
}

func TestSigningAlgorithmsRejectInvalidBackendOutput(t *testing.T) {
	cause := errors.New("backend signing failed")
	for _, algorithm := range []signingAlgorithm{es256Algorithm{}, rs256Algorithm{}} {
		private, err := algorithm.generateKey()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { algorithm.clearPrivateKey(private) })
		broken := brokenCryptoSigner{Signer: private}
		if _, err := algorithm.sign(broken, nil); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("%s accepted an invalid signature: %v", algorithm.name(), err)
		}
		broken.err = cause
		if _, err := algorithm.sign(broken, nil); !errors.Is(err, cause) {
			t.Fatalf("%s lost the backend error: %v", algorithm.name(), err)
		}
	}
}

func TestES256JOSEEncodingBoundaries(t *testing.T) {
	order := elliptic.P256().Params().N
	for _, tc := range []struct {
		name     string
		r, s     *big.Int
		trailing bool
		wantErr  bool
	}{
		{name: "fixed width", r: big.NewInt(1), s: big.NewInt(128)},
		{name: "zero", r: big.NewInt(0), s: big.NewInt(1), wantErr: true},
		{name: "outside curve order", r: new(big.Int).Set(order), s: big.NewInt(1), wantErr: true},
		{name: "trailing data", r: big.NewInt(1), s: big.NewInt(1), trailing: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			der, err := asn1.Marshal(struct{ R, S *big.Int }{tc.r, tc.s})
			if err != nil {
				t.Fatal(err)
			}
			if tc.trailing {
				der = append(der, 0)
			}
			signature, err := (es256Algorithm{}).signatureToJOSE(der)
			if tc.wantErr {
				if err == nil {
					t.Fatal("accepted malformed ECDSA signature")
				}
				return
			}
			if err != nil || len(signature) != 64 || new(big.Int).SetBytes(signature[:32]).Cmp(tc.r) != 0 ||
				new(big.Int).SetBytes(signature[32:]).Cmp(tc.s) != 0 {
				t.Fatalf("signatureToJOSE() = %x, %v", signature, err)
			}
		})
	}
}

func TestRejectsInvalidKeysAndReferences(t *testing.T) {
	modulus := new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 2047), big.NewInt(1))
	for _, public := range []crypto.PublicKey{
		nil,
		(*ecdsa.PublicKey)(nil),
		&ecdsa.PublicKey{Curve: elliptic.P384(), X: elliptic.P384().Params().Gx, Y: elliptic.P384().Params().Gy},
		&ecdsa.PublicKey{Curve: elliptic.P256(), X: big.NewInt(1), Y: big.NewInt(1)},
		&rsa.PublicKey{N: big.NewInt(17), E: 65537},
		&rsa.PublicKey{N: modulus, E: 2},
	} {
		if _, err := AlgForKey(public); err == nil {
			t.Errorf("accepted invalid public key %T", public)
		}
	}
	for _, label := range []string{"", strings.Repeat("a", 257), "a\x00b", string([]byte{0xff})} {
		if _, err := algorithmForRef(context.Background(), KeyRef{Label: label}); err == nil {
			t.Errorf("accepted invalid key label %q", label)
		}
	}
	if _, err := algorithmForRef(context.Background(), KeyRef{Label: strings.Repeat("a", 256)}); err != nil {
		t.Fatalf("rejected maximum-length key label: %v", err)
	}
	if _, err := algorithmForRef(context.Background(), KeyRef{Label: "valid", Algorithm: "PS256"}); !errors.Is(err, ErrUnsupportedAlgorithm) {
		t.Fatalf("unsupported algorithm: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := algorithmForRef(ctx, KeyRef{Label: "valid"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled validation: %v", err)
	}
}

type brokenCryptoSigner struct {
	crypto.Signer
	err error
}

func (s brokenCryptoSigner) Sign(io.Reader, []byte, crypto.SignerOpts) ([]byte, error) {
	return []byte{0x30, 0}, s.err
}

func verifySignature(t *testing.T, public crypto.PublicKey, algorithm string, input, signature []byte) {
	t.Helper()
	digest := sha256.Sum256(input)
	switch key := public.(type) {
	case *ecdsa.PublicKey:
		if algorithm != AlgES256 || len(signature) != 64 ||
			!ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
			t.Fatal("ES256 signature failed independent verification")
		}
	case *rsa.PublicKey:
		if algorithm != AlgRS256 || rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) != nil {
			t.Fatal("RS256 signature failed independent verification")
		}
	default:
		t.Fatalf("unsupported verification key %T", public)
	}
}

func publicKeysEqual(a, b crypto.PublicKey) bool {
	switch key := a.(type) {
	case *ecdsa.PublicKey:
		return key.Equal(b)
	case *rsa.PublicKey:
		return key.Equal(b)
	default:
		return false
	}
}

func assertPrivateKeyCleared(t *testing.T, key crypto.Signer) {
	t.Helper()
	switch key := key.(type) {
	case *ecdsa.PrivateKey:
		if key.D.Sign() != 0 {
			t.Fatal("retained ECDSA private scalar")
		}
	case *rsa.PrivateKey:
		values := append([]*big.Int{key.D, key.Precomputed.Dp, key.Precomputed.Dq, key.Precomputed.Qinv}, key.Primes...)
		for _, value := range values {
			if value != nil && value.Sign() != 0 {
				t.Fatal("retained RSA private material")
			}
		}
	default:
		t.Fatalf("unsupported private key %T", key)
	}
}

func assertCleared(t *testing.T, value []byte) {
	t.Helper()
	if !bytes.Equal(value, make([]byte, len(value))) {
		t.Fatal("sensitive buffer was retained")
	}
}
