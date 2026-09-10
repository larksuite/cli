// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package keysigner provides stable handles for signing keys. L1/L2
// backends keep private material inside platform security services; the L3
// backend decrypts an encrypted file in-process using a caller-supplied secret.
// Callers can obtain the public key and request signatures but cannot export
// private material through this API. ES256 is the default. SoftwareSigner also
// supports ES384, ES512, EdDSA, and RS256. Native adapters accept only algorithms
// implemented by their platform API; Apple's Secure Enclave is P-256 only.
package keysigner

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"strings"
	"unicode/utf8"
)

const (
	// AlgES256 is ECDSA with P-256 and SHA-256.
	AlgES256 = "ES256"
	// AlgES384 is ECDSA with P-384 and SHA-384.
	AlgES384 = "ES384"
	// AlgES512 is ECDSA with P-521 and SHA-512.
	AlgES512 = "ES512"
	// AlgEdDSA is EdDSA with Ed25519.
	AlgEdDSA = "EdDSA"
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

// NewKeyLabel returns a random 128-bit key label with the caller's prefix.
func NewKeyLabel(prefix string) (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("keysigner: generate key label: %w", err)
	}
	label := prefix + hex.EncodeToString(id[:])
	if err := validateRefContext(nil, KeyRef{Label: label}); err != nil {
		return "", err
	}
	return label, nil
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

// PlatformSignerNames returns the ordered native backends for the current OS.
func PlatformSignerNames() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{MacOSSecureEnclaveSignerName, MacOSKeychainSignerName}
	case "windows":
		return []string{WindowsPlatformKSPSignerName, WindowsSoftwareKSPSignerName}
	case "linux":
		return []string{LinuxTPMSignerName}
	default:
		return nil
	}
}

// IsPlatformSignerName reports whether name identifies a built-in native
// backend on any supported OS.
func IsPlatformSignerName(name string) bool {
	switch name {
	case MacOSSecureEnclaveSignerName, MacOSKeychainSignerName,
		WindowsPlatformKSPSignerName, WindowsSoftwareKSPSignerName,
		LinuxTPMSignerName:
		return true
	default:
		return false
	}
}

// NewPlatformSigners constructs the current OS's native backends in fallback
// order. Storage location remains caller policy through directory.
func NewPlatformSigners(directory func(string) (string, error)) []Signer {
	names := PlatformSignerNames()
	signers := make([]Signer, 0, len(names))
	for _, name := range names {
		if signer := NewSigner(name, directory); signer != nil {
			signers = append(signers, signer)
		}
	}
	return signers
}

// ResolvePlatformSigner restores one recorded native backend. An empty name
// selects the strongest backend available on the current OS for legacy records.
func ResolvePlatformSigner(name string, directory func(string) (string, error)) (Signer, error) {
	if name == "" {
		signers := NewPlatformSigners(directory)
		if len(signers) == 0 {
			return nil, ErrUnavailable
		}
		return signers[0], nil
	}
	if !IsPlatformSignerName(name) {
		return nil, fmt.Errorf("keysigner: unknown platform signer %q", name)
	}
	signer := NewSigner(name, directory)
	if signer == nil {
		return nil, fmt.Errorf("%w: platform signer %q is not supported on %s", ErrUnavailable, name, runtime.GOOS)
	}
	return signer, nil
}

// EnsureKeyWithFallback creates or opens a key with the strongest usable
// signer. It falls through only when a backend is unavailable.
func EnsureKeyWithFallback(ctx context.Context, signers []Signer, ref KeyRef) (Signer, crypto.PublicKey, error) {
	var unavailable []error
	for _, signer := range signers {
		public, err := signer.EnsureKey(ctx, ref)
		if err == nil {
			return signer, public, nil
		}
		if errors.Is(err, ErrUnavailable) {
			unavailable = append(unavailable, err)
			continue
		}
		return nil, nil, fmt.Errorf("keysigner: ensure key with %q: %w", signer.Name(), err)
	}
	return nil, nil, errors.Join(append([]error{ErrUnavailable}, unavailable...)...)
}

// ProbeSigner verifies one signer's create/sign/delete path without retaining
// the probe key.
func ProbeSigner(ctx context.Context, signer Signer, ref KeyRef, input []byte) error {
	if signer == nil {
		return ErrUnavailable
	}
	public, err := signer.EnsureKey(ctx, ref)
	if err != nil {
		return err
	}
	cleanupCtx := context.WithoutCancel(ctx)
	algorithm, err := algorithmForRef(ctx, ref)
	if err != nil {
		return errors.Join(err, signer.DeleteKey(cleanupCtx, ref))
	}
	if err := algorithm.validatePublicKey(public); err != nil {
		return errors.Join(err, signer.DeleteKey(cleanupCtx, ref))
	}
	signature, signedAlgorithm, signErr := signer.Sign(ctx, ref, input)
	deleteErr := signer.DeleteKey(cleanupCtx, ref)
	if signErr != nil {
		return errors.Join(signErr, deleteErr)
	}
	validSize := len(signature) > 0
	switch key := public.(type) {
	case *ecdsa.PublicKey:
		validSize = len(signature) == 2*((key.Curve.Params().BitSize+7)/8)
	case ed25519.PublicKey:
		validSize = len(signature) == ed25519.SignatureSize
	case *rsa.PublicKey:
		validSize = len(signature) == key.Size()
	}
	if signedAlgorithm != algorithm.name() || !validSize {
		return errors.Join(fmt.Errorf("keysigner: probe returned algorithm %q and %d-byte signature", signedAlgorithm, len(signature)), deleteErr)
	}
	return deleteErr
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
	case AlgES384, AlgES512:
		return newECDSAAlgorithm(ref.Algorithm)
	case AlgEdDSA:
		return edDSAAlgorithm{}, nil
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

func (es256Algorithm) common() ecdsaAlgorithm {
	algorithm, _ := newECDSAAlgorithm(AlgES256)
	return algorithm
}

func (es256Algorithm) generateKey() (crypto.Signer, error) {
	return es256Algorithm{}.common().generateKey()
}

func (es256Algorithm) validatePublicKey(public crypto.PublicKey) error {
	return es256Algorithm{}.common().validatePublicKey(public)
}

func (a es256Algorithm) sign(key crypto.Signer, input []byte) ([]byte, error) {
	return a.common().sign(key, input)
}

// Best effort only: Go and the crypto implementation may retain other copies.
func (es256Algorithm) clearPrivateKey(key crypto.Signer) {
	es256Algorithm{}.common().clearPrivateKey(key)
}

func (es256Algorithm) digest(input []byte) []byte {
	return es256Algorithm{}.common().digest(input)
}

// signatureToJOSE converts ASN.1 into the fixed-width R || S representation
// required by RFC 7518 for ES256.
func (es256Algorithm) signatureToJOSE(der []byte) ([]byte, error) {
	return ecdsaSignatureToJOSE(es256Algorithm{}.common().parameters, der)
}

type ecdsaAlgorithm struct {
	parameters ecKeyParameters
}

func newECDSAAlgorithm(name string) (ecdsaAlgorithm, error) {
	parameters, ok := ecParametersForAlgorithm(name)
	if !ok {
		return ecdsaAlgorithm{}, fmt.Errorf("keysigner: %w: %q", ErrUnsupportedAlgorithm, name)
	}
	return ecdsaAlgorithm{parameters: parameters}, nil
}

func (a ecdsaAlgorithm) name() string { return a.parameters.algorithm }

func (a ecdsaAlgorithm) generateKey() (crypto.Signer, error) {
	return ecdsa.GenerateKey(a.parameters.ellipticCurve, rand.Reader)
}

func (a ecdsaAlgorithm) validatePublicKey(public crypto.PublicKey) error {
	key, ok := public.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("keysigner: public key is %T, want a valid %s ECDSA key", public, a.parameters.curve)
	}
	parameters, err := ecParametersForPublicKey(key)
	if err != nil {
		return err
	}
	if parameters.algorithm != a.name() {
		return fmt.Errorf("keysigner: public key uses %s, want %s", parameters.algorithm, a.name())
	}
	return nil
}

func (a ecdsaAlgorithm) sign(key crypto.Signer, input []byte) ([]byte, error) {
	public, ok := key.Public().(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("keysigner: public key is %T, want a valid %s ECDSA key", key.Public(), a.parameters.curve)
	}
	if err := a.validatePublicKey(public); err != nil {
		return nil, err
	}
	digest := a.digest(input)
	der, err := key.Sign(rand.Reader, digest, a.parameters.hash)
	if err != nil {
		return nil, err
	}
	if !ecdsa.VerifyASN1(public, digest, der) {
		return nil, ErrCorrupt
	}
	return ecdsaSignatureToJOSE(a.parameters, der)
}

// Best effort only: Go and the crypto implementation may retain other copies.
func (ecdsaAlgorithm) clearPrivateKey(key crypto.Signer) {
	if private, ok := key.(*ecdsa.PrivateKey); ok && private != nil && private.D != nil {
		clear(private.D.Bits())
		private.D.SetInt64(0)
	}
}

func (a ecdsaAlgorithm) digest(input []byte) []byte {
	switch a.parameters.hash {
	case crypto.SHA256:
		digest := sha256.Sum256(input)
		return digest[:]
	case crypto.SHA384:
		digest := sha512.Sum384(input)
		return digest[:]
	case crypto.SHA512:
		digest := sha512.Sum512(input)
		return digest[:]
	default:
		panic("keysigner: unsupported ECDSA hash")
	}
}

type edDSAAlgorithm struct{}

func (edDSAAlgorithm) name() string { return AlgEdDSA }

func (edDSAAlgorithm) generateKey() (crypto.Signer, error) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	return private, err
}

func (edDSAAlgorithm) validatePublicKey(public crypto.PublicKey) error {
	key, ok := public.(ed25519.PublicKey)
	if !ok || len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("keysigner: public key is %T, want a valid Ed25519 key", public)
	}
	return nil
}

func (a edDSAAlgorithm) sign(key crypto.Signer, input []byte) ([]byte, error) {
	public := key.Public()
	if err := a.validatePublicKey(public); err != nil {
		return nil, err
	}
	signature, err := key.Sign(rand.Reader, input, crypto.Hash(0))
	if err != nil {
		return nil, err
	}
	if len(signature) != ed25519.SignatureSize || !ed25519.Verify(public.(ed25519.PublicKey), input, signature) {
		return nil, ErrCorrupt
	}
	return signature, nil
}

// Best effort only: Go and the crypto implementation may retain other copies.
func (edDSAAlgorithm) clearPrivateKey(key crypto.Signer) {
	if private, ok := key.(ed25519.PrivateKey); ok {
		clear(private)
	}
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
