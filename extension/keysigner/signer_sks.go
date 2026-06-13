//go:build (linux || windows) && sks_signer

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// TPM 2.0 signer (build tag `sks_signer`), backed by
// github.com/facebookincubator/sks.
//
// sks holds a non-exportable ECDSA P-256 key in the platform TPM and signs
// SHA-256 digests. On Linux it talks to /dev/tpmrm0; on Windows it uses the
// Microsoft Platform Crypto Provider (CNG). Both backends return an ASN.1 DER
// ECDSA signature, which we convert to the fixed-width r||s form JWS requires for
// ES256 (see ecdsaDERToJOSE). One key is created on the first private_key_jwt
// registration (DefaultKeyLabel) and reused for subsequent app registrations and
// every client_assertion on the same device.
//
// Build with:  go build -tags sks_signer
// Without the tag this file is excluded, no signer registers (keysigner.Active()
// is nil), and the build stays free of the TPM dependency stack — client_secret
// auth only. This mirrors the macOS keychain signer's `keychain_signer` gating.
package keysigner

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/facebookincubator/flog"
	"github.com/facebookincubator/sks"
)

// p256ByteLen is the P-256 coordinate width. sks regular keys are always ECDSA
// P-256, so ES256 signatures are 2*p256ByteLen bytes of r||s.
const p256ByteLen = 32

// keyTag is the sks key tag. Both the Linux and Windows sks backends address
// keys by label and ignore the tag, but the macOS backend uses it, so we set a
// stable namespaced value for forward compatibility.
const keyTag = "com.larksuite.cli"

// sksSigner implements Signer (and HardwareProber) using a non-exportable
// TPM 2.0 ECDSA key via sks.
type sksSigner struct{}

func init() {
	Register(sksSigner{})
	// This sks version logs verbose TPM-operation chatter to stderr via flog (a
	// glog fork it owns exclusively) — e.g. "Loaded TPM device", "Found handle
	// for key" on every sign. The CLI does not use flog, so silence it
	// process-wide here; real failures are returned as errors, never relied upon
	// from these logs. (Newer sks switched to slog, but that lands only on its
	// go-1.24 line, which we avoid to keep the module on go 1.23.)
	flog.SetOutput(io.Discard)
}

// EnsureKey returns the public key for ref, creating the TPM key if absent.
// sks.NewKey is find-or-create: it returns the existing key when one is present.
func (sksSigner) EnsureKey(_ context.Context, ref KeyRef) (crypto.PublicKey, error) {
	key, err := sks.NewKey(ref.Label, keyTag, false, true, nil)
	if err != nil {
		return nil, fmt.Errorf("keysigner: ensure TPM key %q: %w", ref.Label, err)
	}
	defer key.Close()
	return ecdsaPublic(ref.Label, key.Public())
}

// PublicKey returns the public key for ref without creating it. FromLabelTag does
// not touch the TPM until Public() loads the sealed key; a missing key yields a
// nil public key, which we surface as an error — at runtime the key MUST already
// exist (it was bound to the app at registration), so we never silently mint a
// new, unbound one here.
func (sksSigner) PublicKey(_ context.Context, ref KeyRef) (crypto.PublicKey, error) {
	pub := sks.FromLabelTag(ref.Label).Public()
	if pub == nil {
		return nil, fmt.Errorf("keysigner: TPM key %q not found", ref.Label)
	}
	return ecdsaPublic(ref.Label, pub)
}

// Sign signs signingInput with the TPM key and returns a JOSE-format ES256
// signature (fixed-width r||s) plus its alg.
func (sksSigner) Sign(_ context.Context, ref KeyRef, signingInput []byte) ([]byte, string, error) {
	key, err := sks.NewKey(ref.Label, keyTag, false, true, nil)
	if err != nil {
		return nil, "", fmt.Errorf("keysigner: load TPM key %q: %w", ref.Label, err)
	}
	defer key.Close()

	// ES256 signs the SHA-256 digest of the JWS signing input.
	digest := sha256.Sum256(signingInput)
	der, err := key.Sign(nil, digest[:], crypto.SHA256)
	if err != nil {
		return nil, "", fmt.Errorf("keysigner: TPM sign with key %q: %w", ref.Label, err)
	}
	// Both sks backends emit ASN.1 DER; JWS ES256 requires fixed-width r||s
	// (RFC 7518 §3.4).
	rs, err := ecdsaDERToJOSE(der, p256ByteLen)
	if err != nil {
		return nil, "", err
	}
	return rs, AlgES256, nil
}

// ProbeHardware reports on the TPM backing this signer without touching any key.
// A failure to reach the TPM (no device, permission denied, not TPM 2.0) is
// reported as Available=false with Reason set, NOT as a Go error — the probe
// still succeeded in determining that the TEE is currently unusable.
func (sksSigner) ProbeHardware(_ context.Context) (HardwareInfo, error) {
	info := HardwareInfo{Backend: "tpm2"}
	data, err := sks.GetSecureHardwareVendorData()
	if err != nil {
		info.Reason = cleanProbeError(err)
		return info, nil
	}
	info.VendorName = data.VendorName
	info.VendorInfo = data.VendorInfo
	info.Available = data.IsTPM20CompliantDevice
	if !info.Available {
		info.Reason = "secure hardware is not a TPM 2.0 compliant device"
	}
	return info, nil
}

// ecdsaPublic asserts that an sks public key is an ECDSA key (it always is for
// regular sks keys) so the caller gets the concrete type AlgForKey/PublicKeyJWK expect.
func ecdsaPublic(label string, pub crypto.PublicKey) (*ecdsa.PublicKey, error) {
	ecPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("keysigner: TPM key %q public is %T, want *ecdsa.PublicKey", label, pub)
	}
	return ecPub, nil
}
