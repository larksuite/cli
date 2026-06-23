//go:build darwin

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"os"
	"testing"
)

// TestKeychainSignerRegistered confirms the keychain_signer build self-registers
// (init → Register), so keysigner.Active() is non-nil. No keychain access.
func TestKeychainSignerRegistered(t *testing.T) {
	if _, ok := Active().(keychainSigner); !ok {
		t.Fatalf("Active() = %T, want keychainSigner (keychain_signer build must self-register)", Active())
	}
}

// TestKeychainSignerRoundTrip creates a real non-extractable RSA key, signs, and
// verifies RS256 against the returned public key. Gated by LARK_KEYCHAIN_IT
// because it mutates the dedicated lark-cli keychain store. The signer is now
// cgo-free (purego runtime FFI), so it runs with CGO_ENABLED=0. Run with:
//
//	LARK_KEYCHAIN_IT=1 go test -run RoundTrip ./internal/keysigner/
func TestKeychainSignerRoundTrip(t *testing.T) {
	if os.Getenv("LARK_KEYCHAIN_IT") == "" {
		t.Skip("set LARK_KEYCHAIN_IT=1 to run (mutates the macOS keychain)")
	}
	s := keychainSigner{}
	ref := KeyRef{Label: "lark-cli-keychain-it"}

	pub, err := s.EnsureKey(context.Background(), ref)
	if err != nil {
		t.Fatalf("EnsureKey: %v", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("public key = %T, want *rsa.PublicKey", pub)
	}
	if alg, err := AlgForKey(pub); err != nil || alg != AlgRS256 {
		t.Fatalf("AlgForKey = %q, %v; want RS256", alg, err)
	}

	input := []byte("header.payload")
	sig, alg, err := s.Sign(context.Background(), ref, input)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if alg != AlgRS256 {
		t.Errorf("Sign alg = %q, want RS256", alg)
	}
	h := sha256.Sum256(input)
	if err := rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, h[:], sig); err != nil {
		t.Errorf("RS256 signature did not verify: %v", err)
	}
}
