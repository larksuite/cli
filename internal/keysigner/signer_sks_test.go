//go:build linux || (windows && amd64)

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"io"
	"math/big"
	"strings"
	"testing"

	"github.com/facebookincubator/flog"
	"github.com/facebookincubator/sks"
)

// TestFlogSilenced verifies the mechanism init() relies on to keep sks's flog
// TPM chatter off the CLI's stderr: SetOutput redirects flog, and io.Discard
// drops it. Cleanup restores io.Discard so init()'s silencing holds for the
// rest of the package's tests.
func TestFlogSilenced(t *testing.T) {
	var buf bytes.Buffer
	flog.SetOutput(&buf)
	t.Cleanup(func() { flog.SetOutput(io.Discard) })

	flog.Info("captured-line")
	if !strings.Contains(buf.String(), "captured-line") {
		t.Fatalf("flog.SetOutput(buffer) did not capture output: %q", buf.String())
	}

	flog.SetOutput(io.Discard)
	buf.Reset()
	flog.Info("should-be-discarded")
	if buf.Len() != 0 {
		t.Errorf("flog output not discarded: %q", buf.String())
	}
}

// requireTEE skips the test unless the TPM is present and usable. On a Linux
// machine with a TPM but a restrictive device owner (`/dev/tpmrm0` is `tss:tss`
// by default), grant access with `sudo usermod -aG tss $USER` then re-login, or
// run the test under sudo.
func requireTEE(t *testing.T) {
	t.Helper()
	info, err := sksSigner{}.ProbeHardware(context.Background())
	if err != nil || !info.Available {
		reason := info.Reason
		if err != nil {
			reason = err.Error()
		}
		t.Skipf("TEE not available (%s)", reason)
	}
}

// TestSKSSignerRoundTrip exercises the full registration→assertion contract
// against the real TPM: create the key, read it back without creating, derive
// the JWS alg + JWK, sign, and verify the fixed-width r||s output.
func TestSKSSignerRoundTrip(t *testing.T) {
	requireTEE(t)

	var s sksSigner
	ctx := context.Background()
	ref := KeyRef{Label: "larksuite-cli-test"}

	// Best-effort cleanup so the test key does not linger in the TPM-sealed store.
	t.Cleanup(func() {
		if k, err := sks.NewKey(ref.Label, keyTag, false, true, nil); err == nil {
			_ = k.Remove()
			_ = k.Close()
		}
	})

	pub, err := s.EnsureKey(ctx, ref)
	if err != nil {
		t.Fatalf("EnsureKey: %v", err)
	}
	ecPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("EnsureKey returned %T, want *ecdsa.PublicKey", pub)
	}

	// PublicKey (no-create) must return the same key bound at EnsureKey.
	pub2, err := s.PublicKey(ctx, ref)
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	if !ecPub.Equal(pub2) {
		t.Fatal("PublicKey returned a different key than EnsureKey")
	}

	// The JWT layer derives alg + JWK from the public key; both must work.
	if alg, err := AlgForKey(pub); err != nil || alg != AlgES256 {
		t.Fatalf("AlgForKey = %q, %v; want ES256", alg, err)
	}
	if _, err := PublicKeyJWK(pub); err != nil {
		t.Fatalf("PublicKeyJWK: %v", err)
	}

	// Sign a representative JWS signing input and verify the converted r||s.
	input := []byte("eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJjbGkifQ")
	sig, alg, err := s.Sign(ctx, ref, input)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if alg != AlgES256 {
		t.Fatalf("Sign alg = %q, want ES256", alg)
	}
	if len(sig) != 2*p256ByteLen {
		t.Fatalf("len(sig) = %d, want %d (fixed-width r||s)", len(sig), 2*p256ByteLen)
	}
	digest := sha256.Sum256(input)
	r := new(big.Int).SetBytes(sig[:p256ByteLen])
	ss := new(big.Int).SetBytes(sig[p256ByteLen:])
	if !ecdsa.Verify(ecPub, digest[:], r, ss) {
		t.Fatal("TPM signature did not verify against the public key")
	}
}
