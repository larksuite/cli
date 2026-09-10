// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package keysigner provides stable handles for signing keys. L1/L2
// backends keep private material inside platform security services; the L3
// backend decrypts an encrypted file in-process using a caller-supplied secret.
// Callers can obtain the public key and request signatures but cannot export
// private material through this API. ES256 is the default. RS256 is supported by
// Windows/Linux TPM, macOS Keychain, Windows CNG, and SoftwareSigner backends.
// Apple's Secure Enclave key API only supports P-256, so its L1 rejects RS256.
package keysigner

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode/utf8"
)

const (
	// AlgES256 is ECDSA with P-256 and SHA-256.
	AlgES256 = "ES256"
	// AlgRS256 is RSA PKCS#1 v1.5 with SHA-256 and a key of at least 2048 bits.
	AlgRS256 = "RS256"
)

// Stable backend names used by stored key metadata and backend directories.
const (
	MacOSSecureEnclaveSignerName = "macos-secure-enclave"
	MacOSKeychainSignerName      = "macos-keychain"
	WindowsPlatformKSPSignerName = "windows-platform-ksp"
	WindowsSoftwareKSPSignerName = "windows-software-ksp"
	LinuxTPMSignerName           = "linux-tpm"
	SoftwareSignerName           = "software-file"
)

// SecurityLevel is the protection class of a signing backend.
type SecurityLevel string

const (
	SecurityLevelL1 SecurityLevel = "L1"
	SecurityLevelL2 SecurityLevel = "L2"
	SecurityLevelL3 SecurityLevel = "L3"
)

var (
	// ErrUnavailable means this signer cannot be used on this build or host.
	// Only new bindings may try another signer; existing bindings fail closed.
	ErrUnavailable = errors.New("DPoP key signer is unavailable")
	// ErrKeyNotFound means the stable handle no longer resolves to its private
	// key. Callers must never recreate a key for an existing token binding.
	ErrKeyNotFound    = errors.New("DPoP signing key not found")
	ErrKeyExists      = errors.New("DPoP signing key already exists")
	ErrCorrupt        = errors.New("invalid or mismatched signing key record")
	ErrUnlock         = errors.New("wrong unlock secret or damaged signing key ciphertext")
	ErrUnlockRequired = errors.New("software signing requires a 16..1024-byte unlock secret")
	// ErrUnsupportedAlgorithm rejects an algorithm without accessing key storage.
	ErrUnsupportedAlgorithm = errors.New("unsupported signing algorithm")
)

// KeyRef is a stable backend handle.
type KeyRef struct {
	Label string
	// Algorithm is the required JOSE signing algorithm; empty means AlgES256.
	// It is not part of the label's identity and must never replace an existing key.
	Algorithm string
}

// Signer owns signing keys behind stable references. Sign hashes signingInput
// as required by ref.Algorithm and returns a JOSE signature and that algorithm.
type Signer interface {
	// Name identifies the exact backend used to restore an existing binding.
	Name() string
	SecurityLevel() SecurityLevel
	// EnsureKey is only for new bindings. Existing bindings must use PublicKey
	// and Sign, which never create replacements. KeyCreator rejects duplicates.
	EnsureKey(ctx context.Context, ref KeyRef) (crypto.PublicKey, error)
	PublicKey(ctx context.Context, ref KeyRef) (crypto.PublicKey, error)
	Sign(ctx context.Context, ref KeyRef, signingInput []byte) (signature []byte, algorithm string, err error)
	// DeleteKey removes a key; missing keys may return nil or ErrKeyNotFound.
	DeleteKey(ctx context.Context, ref KeyRef) error
}

// KeyCreator optionally supports creation without reusing an existing identity.
type KeyCreator interface {
	CreateKey(ctx context.Context, ref KeyRef) (crypto.PublicKey, error)
}

// signingAlgorithm owns cryptography, independently of key storage.
// Native adapters separately select the algorithms their platform implements.
type signingAlgorithm interface {
	name() string
	generateKey() (crypto.Signer, error)
	validatePublicKey(crypto.PublicKey) error
	sign(crypto.Signer, []byte) ([]byte, error)
	// Clear software private material without invalidating returned public keys.
	clearPrivateKey(crypto.Signer)
}

func algorithmForRef(ctx context.Context, ref KeyRef) (signingAlgorithm, error) {
	if err := validateRefContext(ctx, ref); err != nil {
		return nil, err
	}
	switch ref.Algorithm {
	case "", AlgES256:
		return es256Algorithm{}, nil
	case AlgRS256:
		return rs256Algorithm{}, nil
	default:
		return nil, fmt.Errorf("keysigner: %w: %q", ErrUnsupportedAlgorithm, ref.Algorithm)
	}
}

func validateRefContext(ctx context.Context, ref KeyRef) error {
	if len(ref.Label) == 0 || len(ref.Label) > 256 || !utf8.ValidString(ref.Label) ||
		strings.ContainsFunc(ref.Label, func(r rune) bool { return r < 32 || r == 127 }) {
		return errors.New("keysigner: key reference must be 1..256 UTF-8 bytes without control characters")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}

type es256Algorithm struct{}

func (es256Algorithm) name() string { return AlgES256 }

func (es256Algorithm) generateKey() (crypto.Signer, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

func (es256Algorithm) validatePublicKey(public crypto.PublicKey) error {
	_, err := P256PublicKey(public)
	return err
}

func (a es256Algorithm) sign(key crypto.Signer, input []byte) ([]byte, error) {
	public, err := P256PublicKey(key.Public())
	if err != nil {
		return nil, err
	}
	digest := a.digest(input)
	der, err := key.Sign(rand.Reader, digest, crypto.SHA256)
	if err != nil {
		return nil, err
	}
	if !ecdsa.VerifyASN1(public, digest, der) {
		return nil, ErrCorrupt
	}
	return a.signatureToJOSE(der)
}

// Best effort only: Go and the crypto implementation may retain other copies.
func (es256Algorithm) clearPrivateKey(key crypto.Signer) {
	if private, ok := key.(*ecdsa.PrivateKey); ok && private != nil && private.D != nil {
		clear(private.D.Bits())
		private.D.SetInt64(0)
	}
}

func (es256Algorithm) digest(input []byte) []byte {
	digest := sha256.Sum256(input)
	return digest[:]
}

// signatureToJOSE converts ASN.1 into the fixed-width R || S representation
// required by RFC 7518 for ES256.
func (es256Algorithm) signatureToJOSE(der []byte) ([]byte, error) {
	var parsed struct {
		R *big.Int
		S *big.Int
	}
	rest, err := asn1.Unmarshal(der, &parsed)
	if err != nil {
		return nil, fmt.Errorf("keysigner: parse ECDSA signature: %w", err)
	}
	if len(rest) != 0 || parsed.R == nil || parsed.S == nil || parsed.R.Sign() <= 0 || parsed.S.Sign() <= 0 {
		return nil, errors.New("keysigner: invalid ECDSA signature")
	}
	const coordinateBytes = 32
	if parsed.R.Cmp(elliptic.P256().Params().N) >= 0 || parsed.S.Cmp(elliptic.P256().Params().N) >= 0 {
		return nil, errors.New("keysigner: ECDSA signature scalar outside P-256 order")
	}
	out := make([]byte, coordinateBytes*2)
	parsed.R.FillBytes(out[:coordinateBytes])
	parsed.S.FillBytes(out[coordinateBytes:])
	return out, nil
}

type rs256Algorithm struct{}

func (rs256Algorithm) name() string { return AlgRS256 }

func (rs256Algorithm) generateKey() (crypto.Signer, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}

func (rs256Algorithm) validatePublicKey(public crypto.PublicKey) error {
	key, ok := public.(*rsa.PublicKey)
	if !ok || key == nil || key.N == nil || key.N.Sign() <= 0 || key.N.BitLen() < 2048 ||
		key.N.Bit(0) == 0 || key.E < 3 || key.E > 1<<31-1 || key.E%2 == 0 {
		return fmt.Errorf("keysigner: public key is %T, want a valid RSA key of at least 2048 bits", public)
	}
	return nil
}

func (a rs256Algorithm) sign(key crypto.Signer, input []byte) ([]byte, error) {
	public := key.Public()
	if err := a.validatePublicKey(public); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(input)
	signature, err := key.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		return nil, err
	}
	if err := rsa.VerifyPKCS1v15(public.(*rsa.PublicKey), crypto.SHA256, digest[:], signature); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	return signature, nil
}

// Best effort only: Go and the crypto implementation may retain other copies.
// The public modulus is deliberately preserved for callers retaining Public().
func (rs256Algorithm) clearPrivateKey(key crypto.Signer) {
	private, ok := key.(*rsa.PrivateKey)
	if !ok || private == nil {
		return
	}
	values := []*big.Int{private.D, private.Precomputed.Dp, private.Precomputed.Dq, private.Precomputed.Qinv}
	values = append(values, private.Primes...)
	for _, crt := range private.Precomputed.CRTValues {
		values = append(values, crt.Exp, crt.Coeff, crt.R)
	}
	for _, value := range values {
		if value != nil {
			clear(value.Bits())
			value.SetInt64(0)
		}
	}
}
