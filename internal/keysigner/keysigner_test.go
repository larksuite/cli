// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
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
	es384, err := newECDSAAlgorithm(AlgES384)
	if err != nil {
		t.Fatal(err)
	}
	es512, err := newECDSAAlgorithm(AlgES512)
	if err != nil {
		t.Fatal(err)
	}
	for _, algorithm := range []signingAlgorithm{
		es256Algorithm{},
		es384,
		es512,
		edDSAAlgorithm{},
		rs256Algorithm{},
	} {
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
			case AlgES256, AlgES384, AlgES512:
				parameters, _ := ecParametersForAlgorithm(algorithm.name())
				if jwk.Kty != "EC" || jwk.Crv != parameters.curve || jwk.X == "" || jwk.Y == "" || jwk.N != "" || jwk.E != "" {
					t.Fatalf("unexpected %s JWK: %+v", algorithm.name(), jwk)
				}
			case AlgEdDSA:
				if jwk.Kty != "OKP" || jwk.Crv != "Ed25519" || jwk.X == "" || jwk.Y != "" || jwk.N != "" || jwk.E != "" {
					t.Fatalf("unexpected EdDSA JWK: %+v", jwk)
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
	es384, err := newECDSAAlgorithm(AlgES384)
	if err != nil {
		t.Fatal(err)
	}
	es512, err := newECDSAAlgorithm(AlgES512)
	if err != nil {
		t.Fatal(err)
	}
	for _, algorithm := range []signingAlgorithm{
		es256Algorithm{},
		es384,
		es512,
		edDSAAlgorithm{},
		rs256Algorithm{},
	} {
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

func TestPublicKeyThumbprintRFC7638RSA(t *testing.T) {
	const modulus = "" +
		"0vx7agoebGcQSuuPiLJXZptN9nndrQmbXEps2aiAFbWhM78LhWx4cbbfAAt" +
		"VT86zwu1RK7aPFFxuhDR1L6tSoc_BJECPebWKRXjBZCiFV4n3oknjhMstn6" +
		"4tZ_2W-5JsGY4Hc5n9yBXArwl93lqt7_RN5w6Cf0h4QyQ5v-65YGjQR0_FD" +
		"W2QvzqY368QQMicAtaSqzs8KJZgnYb9c7d0zgdAZHzu6qMQvRL5hajrn1n9" +
		"1CbOpbISD08qNLyrdkt-bFTWhAI4vMQFh6WeZu0fM4lFd2NcRwr3XPksINH" +
		"aQ-G_xBniIqbw0Ls1jF44-csFCur-kEgU8awapJzKnqDKgw"
	n, err := base64.RawURLEncoding.DecodeString(modulus)
	if err != nil {
		t.Fatal(err)
	}
	got, err := PublicKeyThumbprint(&rsa.PublicKey{N: new(big.Int).SetBytes(n), E: 65537})
	if err != nil {
		t.Fatal(err)
	}
	const want = "NzbLsXh8uDCcd-6MNwXF4W_7noWXFZAfHkxZsRGC9Xs"
	if got != want {
		t.Fatalf("thumbprint = %q, want RFC 7638 value %q", got, want)
	}
}

func TestRejectsInvalidKeysAndReferences(t *testing.T) {
	modulus := new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 2047), big.NewInt(1))
	for _, public := range []crypto.PublicKey{
		nil,
		(*ecdsa.PublicKey)(nil),
		&ecdsa.PublicKey{Curve: elliptic.P224(), X: elliptic.P224().Params().Gx, Y: elliptic.P224().Params().Gy},
		&ecdsa.PublicKey{Curve: elliptic.P256(), X: big.NewInt(1), Y: big.NewInt(1)},
		ed25519.PublicKey{1},
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
	switch key := public.(type) {
	case *ecdsa.PublicKey:
		parameters, err := ecParametersForPublicKey(key)
		if err != nil {
			t.Fatal(err)
		}
		var digest []byte
		switch algorithm {
		case AlgES256:
			sum := sha256.Sum256(input)
			digest = sum[:]
		case AlgES384:
			sum := sha512.Sum384(input)
			digest = sum[:]
		case AlgES512:
			sum := sha512.Sum512(input)
			digest = sum[:]
		default:
			t.Fatalf("unexpected ECDSA algorithm %q", algorithm)
		}
		if algorithm != parameters.algorithm || len(signature) != 2*parameters.coordinateBytes ||
			!ecdsa.Verify(key, digest,
				new(big.Int).SetBytes(signature[:parameters.coordinateBytes]),
				new(big.Int).SetBytes(signature[parameters.coordinateBytes:])) {
			t.Fatalf("%s signature failed independent verification", algorithm)
		}
	case ed25519.PublicKey:
		if algorithm != AlgEdDSA || !ed25519.Verify(key, input, signature) {
			t.Fatal("EdDSA signature failed independent verification")
		}
	case *rsa.PublicKey:
		digest := sha256.Sum256(input)
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
	case ed25519.PublicKey:
		other, ok := b.(ed25519.PublicKey)
		return ok && key.Equal(other)
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
	case ed25519.PrivateKey:
		if !bytes.Equal(key, make([]byte, len(key))) {
			t.Fatal("retained Ed25519 private material")
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
