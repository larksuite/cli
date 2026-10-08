// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
)

// PublicJWK contains the public members of a supported signing key.
type PublicJWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv,omitempty"`
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`
	N   string `json:"n,omitempty"`
	E   string `json:"e,omitempty"`
}

// Map returns a mutable JSON object containing only populated JWK members.
func (j PublicJWK) Map() map[string]any {
	values := map[string]any{"kty": j.Kty}
	for name, value := range map[string]string{
		"crv": j.Crv,
		"x":   j.X,
		"y":   j.Y,
		"n":   j.N,
		"e":   j.E,
	} {
		if value != "" {
			values[name] = value
		}
	}
	return values
}

type ecKeyParameters struct {
	algorithm       string
	curve           string
	ellipticCurve   elliptic.Curve
	coordinateBytes int
	hash            crypto.Hash
}

func ecParametersForAlgorithm(algorithm string) (ecKeyParameters, bool) {
	switch algorithm {
	case AlgES256:
		return ecKeyParameters{AlgES256, "P-256", elliptic.P256(), 32, crypto.SHA256}, true
	case AlgES384:
		return ecKeyParameters{AlgES384, "P-384", elliptic.P384(), 48, crypto.SHA384}, true
	case AlgES512:
		return ecKeyParameters{AlgES512, "P-521", elliptic.P521(), 66, crypto.SHA512}, true
	default:
		return ecKeyParameters{}, false
	}
}

func ecParametersForPublicKey(key *ecdsa.PublicKey) (ecKeyParameters, error) {
	if key == nil || key.Curve == nil || key.X == nil || key.Y == nil ||
		!key.Curve.IsOnCurve(key.X, key.Y) {
		return ecKeyParameters{}, errors.New("keysigner: invalid ECDSA public key")
	}
	for _, algorithm := range []string{AlgES256, AlgES384, AlgES512} {
		parameters, _ := ecParametersForAlgorithm(algorithm)
		if key.Curve == parameters.ellipticCurve {
			return parameters, nil
		}
	}
	return ecKeyParameters{}, fmt.Errorf("keysigner: unsupported EC curve %q", key.Curve.Params().Name)
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
	switch key := public.(type) {
	case *ecdsa.PublicKey:
		parameters, err := ecParametersForPublicKey(key)
		if err != nil {
			return "", err
		}
		return parameters.algorithm, nil
	case ed25519.PublicKey:
		if len(key) != ed25519.PublicKeySize {
			return "", errors.New("keysigner: invalid Ed25519 public key")
		}
		return AlgEdDSA, nil
	case *rsa.PublicKey:
		algorithm := rs256Algorithm{}
		if err := algorithm.validatePublicKey(key); err != nil {
			return "", err
		}
		return algorithm.name(), nil
	default:
		return "", fmt.Errorf("keysigner: unsupported public key type %T", public)
	}
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
		parameters, err := ecParametersForPublicKey(key)
		if err != nil {
			return PublicJWK{}, err
		}
		return PublicJWK{
			Kty: "EC", Crv: parameters.curve,
			X: base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, parameters.coordinateBytes))),
			Y: base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, parameters.coordinateBytes))),
		}, nil
	case ed25519.PublicKey:
		return PublicJWK{
			Kty: "OKP", Crv: "Ed25519",
			X: base64.RawURLEncoding.EncodeToString(key),
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

// PublicKeyThumbprint returns an RFC 7638 SHA-256 JWK thumbprint.
func PublicKeyThumbprint(public crypto.PublicKey) (string, error) {
	jwk, err := PublicKeyJWK(public)
	if err != nil {
		return "", err
	}
	var canonical []byte
	switch jwk.Kty {
	case "EC":
		canonical, err = json.Marshal(struct {
			Crv string `json:"crv"`
			Kty string `json:"kty"`
			X   string `json:"x"`
			Y   string `json:"y"`
		}{jwk.Crv, jwk.Kty, jwk.X, jwk.Y})
	case "OKP":
		canonical, err = json.Marshal(struct {
			Crv string `json:"crv"`
			Kty string `json:"kty"`
			X   string `json:"x"`
		}{jwk.Crv, jwk.Kty, jwk.X})
	case "RSA":
		canonical, err = json.Marshal(struct {
			E   string `json:"e"`
			Kty string `json:"kty"`
			N   string `json:"n"`
		}{jwk.E, jwk.Kty, jwk.N})
	default:
		return "", fmt.Errorf("keysigner: unsupported JWK key type %q", jwk.Kty)
	}
	if err != nil {
		return "", fmt.Errorf("keysigner: marshal JWK thumbprint input: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

func ecdsaSignatureToJOSE(parameters ecKeyParameters, der []byte) ([]byte, error) {
	var signature struct{ R, S *big.Int }
	rest, err := asn1.Unmarshal(der, &signature)
	if err != nil {
		return nil, fmt.Errorf("keysigner: parse ECDSA DER signature: %w", err)
	}
	if len(rest) != 0 || signature.R == nil || signature.S == nil ||
		signature.R.Sign() <= 0 || signature.S.Sign() <= 0 {
		return nil, errors.New("keysigner: invalid ECDSA signature")
	}
	order := parameters.ellipticCurve.Params().N
	if signature.R.Cmp(order) >= 0 || signature.S.Cmp(order) >= 0 {
		return nil, fmt.Errorf("keysigner: ECDSA signature scalar outside %s order", parameters.curve)
	}
	out := make([]byte, 2*parameters.coordinateBytes)
	signature.R.FillBytes(out[:parameters.coordinateBytes])
	signature.S.FillBytes(out[parameters.coordinateBytes:])
	return out, nil
}
