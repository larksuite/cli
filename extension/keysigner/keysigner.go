// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package keysigner defines the pluggable signing abstraction used by the
// private_key_jwt registration and authentication flow.
//
// The open-source core only declares the Signer interface and pure-stdlib key
// helpers. The platform implementations that hold a non-exportable private key
// (TPM 2.0 via facebookincubator/sks on Linux/Windows, a non-extractable
// Keychain key on macOS) live OUTSIDE this core — in a build-tagged module or
// extension — and register themselves via Register from init(). This keeps
// CGO-heavy and license-sensitive dependencies out of the open-source build.
package keysigner

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"math/big"
)

// KeyRef identifies a non-exportable signing key held by a backend
// (TEE/TPM/Keychain). It is a stable handle (label), never the key material.
type KeyRef struct {
	// Label is the backend key label/tag (e.g. "larksuite-cli-agent").
	Label string
}

// Signer signs JWS signing inputs with a non-exportable key.
type Signer interface {
	// EnsureKey returns the public key for ref, creating the key if absent.
	EnsureKey(ctx context.Context, ref KeyRef) (crypto.PublicKey, error)
	// PublicKey returns the public key for ref without creating it.
	PublicKey(ctx context.Context, ref KeyRef) (crypto.PublicKey, error)
	// Sign signs signingInput and returns a JOSE-format signature plus the JWS
	// alg ("ES256"/"RS256"). Implementations apply the alg's hash and, for
	// ECDSA, MUST return the fixed-width r||s form required by RFC 7518 §3.4
	// (not ASN.1 DER), because the backend (TPM/Keychain) typically yields DER.
	Sign(ctx context.Context, ref KeyRef, signingInput []byte) (sig []byte, alg string, err error)
}

// Supported JWS algorithms.
const (
	AlgES256 = "ES256"
	AlgRS256 = "RS256"
)

// DefaultKeyLabel is the backend key label lark-cli uses for its device signing
// key. One non-exportable key is created on first private_key_jwt registration
// and reused across subsequent app registrations on the same device.
const DefaultKeyLabel = "larksuite-cli-agent"

// AlgForKey returns the JWS alg for a public key: EC P-256 -> ES256, RSA -> RS256.
// The signer backend chooses the key type (the macOS keychain signer uses an
// RSA-2048 key, hence RS256).
func AlgForKey(pub crypto.PublicKey) (string, error) {
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		if k.Curve == elliptic.P256() {
			return AlgES256, nil
		}
		return "", fmt.Errorf("keysigner: unsupported EC curve %q (only P-256/ES256)", k.Curve.Params().Name)
	case *rsa.PublicKey:
		return AlgRS256, nil
	default:
		return "", fmt.Errorf("keysigner: unsupported public key type %T", pub)
	}
}

// EncodePublicKey marshals pub to PKIX DER and base64-encodes it (std encoding),
// matching the public-key form the registration backend binds to the app.
func EncodePublicKey(pub crypto.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("keysigner: encode public key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

// PublicKeyJWK returns the RFC 7517 JSON Web Key for pub, used to embed the
// public key in the attestation JWT's "jwk" header so the registration backend
// can bind it to the app. EC keys use base64url fixed-width coordinates
// (RFC 7518 §6.2.1); RSA keys use base64url-encoded modulus and exponent.
func PublicKeyJWK(pub crypto.PublicKey) (map[string]any, error) {
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		if k.Curve != elliptic.P256() {
			return nil, fmt.Errorf("keysigner: JWK supports EC P-256 only, got %q", k.Curve.Params().Name)
		}
		const coordLen = 32 // P-256 field element size
		x := make([]byte, coordLen)
		y := make([]byte, coordLen)
		k.X.FillBytes(x)
		k.Y.FillBytes(y)
		return map[string]any{
			"use": "sig",
			"kty": "EC",
			"crv": "P-256",
			"x":   base64.RawURLEncoding.EncodeToString(x),
			"y":   base64.RawURLEncoding.EncodeToString(y),
		}, nil
	case *rsa.PublicKey:
		return map[string]any{
			"use": "sig",
			"kty": "RSA",
			"n":   base64.RawURLEncoding.EncodeToString(k.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.E)).Bytes()),
		}, nil
	default:
		return nil, fmt.Errorf("keysigner: unsupported public key type %T for JWK", pub)
	}
}
