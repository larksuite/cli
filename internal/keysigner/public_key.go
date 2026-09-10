// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"math/big"
)

// PublicJWK contains the public key members for EC and RSA signing keys.
type PublicJWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv,omitempty"`
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`
	N   string `json:"n,omitempty"`
	E   string `json:"e,omitempty"`
}

// P256PublicKey validates and projects a backend public key to P-256 ECDSA.
func P256PublicKey(public crypto.PublicKey) (*ecdsa.PublicKey, error) {
	ec, ok := public.(*ecdsa.PublicKey)
	if !ok || ec == nil || ec.Curve != elliptic.P256() || ec.X == nil || ec.Y == nil || !ec.Curve.IsOnCurve(ec.X, ec.Y) {
		return nil, fmt.Errorf("keysigner: public key is %T, want a valid P-256 ECDSA key", public)
	}
	return ec, nil
}

// AlgForKey returns the JOSE algorithm for a validated public signing key.
func AlgForKey(public crypto.PublicKey) (string, error) {
	var algorithm signingAlgorithm
	switch public.(type) {
	case *ecdsa.PublicKey:
		algorithm = es256Algorithm{}
	case *rsa.PublicKey:
		algorithm = rs256Algorithm{}
	default:
		return "", fmt.Errorf("keysigner: unsupported public key type %T", public)
	}
	if err := algorithm.validatePublicKey(public); err != nil {
		return "", err
	}
	return algorithm.name(), nil
}

// EncodePublicKey returns a validated public signing key as standard-base64 PKIX DER.
func EncodePublicKey(public crypto.PublicKey) (string, error) {
	if _, err := AlgForKey(public); err != nil {
		return "", err
	}
	der, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return "", fmt.Errorf("keysigner: encode public key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

// PublicKeyJWK returns only the public key members of a validated signing key.
func PublicKeyJWK(public crypto.PublicKey) (PublicJWK, error) {
	if _, err := AlgForKey(public); err != nil {
		return PublicJWK{}, err
	}
	switch key := public.(type) {
	case *ecdsa.PublicKey:
		return PublicJWK{
			Kty: "EC", Crv: "P-256",
			X: base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))),
			Y: base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
		}, nil
	case *rsa.PublicKey:
		return PublicJWK{
			Kty: "RSA",
			N:   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}, nil
	default:
		return PublicJWK{}, fmt.Errorf("keysigner: unsupported public key type %T", public)
	}
}
