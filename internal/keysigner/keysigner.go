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
	"encoding/asn1"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
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

// HardwareInfo describes the secure hardware backing a Signer, as reported by a
// HardwareProber. It is advisory/diagnostic: it tells a user whether
// private_key_jwt can use a real TEE on this device.
type HardwareInfo struct {
	Backend    string // backing technology, e.g. "tpm2" or "keychain"
	Available  bool   // the hardware is present and usable for signing
	VendorName string // hardware vendor/manufacturer, when known
	VendorInfo string // additional vendor detail, when known
	Reason     string // when Available is false, a human-readable cause
}

// HardwareProber is an optional capability a Signer may implement to report on
// the secure hardware backing it (TPM/TEE vendor and availability) WITHOUT
// creating or using a key. Probing never mutates key state.
type HardwareProber interface {
	ProbeHardware(ctx context.Context) (HardwareInfo, error)
}

// ProbeActiveHardware probes the active signer's secure hardware. ok is false
// when there is no active signer or it does not implement HardwareProber — in
// which case private_key_jwt is unsupported on this build. When ok is true, info
// reports availability and, if unavailable, info.Reason explains why.
func ProbeActiveHardware(ctx context.Context) (info HardwareInfo, ok bool, err error) {
	return probeHardware(ctx, Active())
}

// probeHardware is the registry-independent core of ProbeActiveHardware, so it
// can be unit-tested without touching the global signer.
func probeHardware(ctx context.Context, s Signer) (HardwareInfo, bool, error) {
	p, ok := s.(HardwareProber)
	if !ok {
		return HardwareInfo{}, false, nil
	}
	info, err := p.ProbeHardware(ctx)
	return info, true, err
}

// cleanProbeError renders err's message with redundant re-wraps collapsed. Some
// backends (e.g. facebookincubator/sks) wrap an error twice with the SAME "%w"
// prefix, yielding "P: P: cause"; this peels each outer layer whose only
// contribution is to repeat the prefix already present in the wrapped error,
// leaving a single "P: cause". A layer that adds genuinely new context is kept.
func cleanProbeError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	for {
		inner := errors.Unwrap(err)
		if inner == nil {
			break
		}
		innerMsg := inner.Error()
		prefix, ok := strings.CutSuffix(msg, innerMsg)
		if !ok || prefix == "" || !strings.HasPrefix(innerMsg, prefix) {
			break
		}
		msg, err = innerMsg, inner
	}
	return msg
}

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

// ecdsaDERToJOSE converts an ASN.1 DER-encoded ECDSA signature — the form most
// TEE/TPM backends emit (e.g. facebookincubator/sks marshals the TPM's r,s with
// asn1.Marshal) — into the fixed-width r||s form JWS requires for ES256
// (RFC 7518 §3.4). byteLen is the curve coordinate size (32 for P-256), so the
// result is exactly 2*byteLen bytes with r and s each left-zero-padded.
//
// This is intentionally part of the pure-stdlib core (not a platform signer) so
// it can be unit-tested with a software key on any machine, including TPM-less CI.
func ecdsaDERToJOSE(der []byte, byteLen int) ([]byte, error) {
	var sig struct{ R, S *big.Int }
	rest, err := asn1.Unmarshal(der, &sig)
	if err != nil {
		return nil, fmt.Errorf("keysigner: parse ECDSA DER signature: %w", err)
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("keysigner: %d trailing byte(s) after ECDSA DER signature", len(rest))
	}
	if sig.R == nil || sig.S == nil || sig.R.Sign() <= 0 || sig.S.Sign() <= 0 {
		return nil, fmt.Errorf("keysigner: ECDSA signature has non-positive r/s")
	}
	// Guard before FillBytes, which panics if the scalar does not fit in byteLen.
	if sig.R.BitLen() > byteLen*8 || sig.S.BitLen() > byteLen*8 {
		return nil, fmt.Errorf("keysigner: ECDSA r/s exceeds %d-byte coordinate", byteLen)
	}
	out := make([]byte, 2*byteLen)
	sig.R.FillBytes(out[:byteLen])
	sig.S.FillBytes(out[byteLen:])
	return out, nil
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
