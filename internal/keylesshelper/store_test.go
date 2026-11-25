// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keysigner"
)

type storeTestSigner struct {
	keysigner.Signer
	name                                             string
	ensureErr, signErr                               error
	ensureCalls, publicCalls, signCalls, deleteCalls int
	onSign                                           func()
}

func (s *storeTestSigner) Name() string                           { return s.name }
func (s *storeTestSigner) SecurityLevel() keysigner.SecurityLevel { return keysigner.SecurityLevelL3 }
func (s *storeTestSigner) EnsureKey(ctx context.Context, ref keysigner.KeyRef) (crypto.PublicKey, error) {
	s.ensureCalls++
	if s.ensureErr != nil {
		return nil, s.ensureErr
	}
	return s.Signer.EnsureKey(ctx, ref)
}
func (s *storeTestSigner) PublicKey(ctx context.Context, ref keysigner.KeyRef) (crypto.PublicKey, error) {
	s.publicCalls++
	return s.Signer.PublicKey(ctx, ref)
}
func (s *storeTestSigner) Sign(ctx context.Context, ref keysigner.KeyRef, input []byte) ([]byte, string, error) {
	s.signCalls++
	if s.onSign != nil {
		s.onSign()
	}
	if s.signErr != nil {
		return nil, "", s.signErr
	}
	return s.Signer.Sign(ctx, ref, input)
}
func (s *storeTestSigner) DeleteKey(ctx context.Context, ref keysigner.KeyRef) error {
	s.deleteCalls++
	return s.Signer.DeleteKey(ctx, ref)
}

func newStoreTestSigner(t *testing.T) *storeTestSigner {
	t.Helper()
	signer, err := keysigner.NewSoftwareSigner(t.TempDir(), func(context.Context) ([]byte, error) { return bytes.Repeat([]byte{1}, 32), nil })
	if err != nil {
		t.Fatal(err)
	}
	return &storeTestSigner{Signer: signer, name: "test-software"}
}

func TestKeyStoreLifecycle(t *testing.T) {
	isolateSoftwareStorage(t)
	ctx := context.Background()
	signer := newStoreTestSigner(t)
	unavailable := &storeTestSigner{name: "unavailable", ensureErr: keysigner.ErrUnavailable}
	store := NewKeyStore(unavailable, signer)
	// Attestation, client assertion and the health probe each sign under the store lock.
	signer.onSign = func() {
		wait, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
		defer cancel()
		err := withKeyStoreLockContext(wait, func() error { t.Fatal("signing escaped transaction"); return nil })
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("nested lock: %v", err)
		}
	}
	key, attestation, err := store.CreateAttestationContext(ctx, "registration", "nonce", time.Now())
	if err != nil || key == nil || attestation == "" {
		t.Fatalf("create: %v", err)
	}
	if unavailable.ensureCalls != 1 || key.Provider() != signer.Name() || key.Signer() != signer {
		t.Fatal("fallback not selected")
	}
	if signer.ensureCalls != 1 || signer.publicCalls != 0 {
		t.Fatalf("attestation key lookup: EnsureKey=%d PublicKey=%d", signer.ensureCalls, signer.publicCalls)
	}
	if _, err := key.Thumbprint(); err != nil {
		t.Fatal(err)
	}
	before := signer.ensureCalls
	assertion, err := SignClientAssertionContext(ctx, key.Signer(), key.ID(), "cli_test", "https://example.invalid", time.Now())
	if err != nil || assertion == "" || signer.ensureCalls != before {
		t.Fatalf("assertion recreated key: %v", err)
	}
	if _, err := ProbeKeyContext(ctx, key.Signer(), key.ID()); err != nil {
		t.Fatal(err)
	}
	if signer.signCalls != 3 {
		t.Fatalf("attestation, assertion and probe signed %d times, want 3", signer.signCalls)
	}
	if err := key.DeleteContext(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := ProbeKeyContext(ctx, key.Signer(), key.ID()); !errors.Is(err, keysigner.ErrKeyNotFound) {
		t.Fatalf("missing key: %v", err)
	}
	if signer.ensureCalls != before {
		t.Fatal("load recreated key")
	}
}

func TestKeyStoreProbeAndFailedAttestationCleanup(t *testing.T) {
	isolateSoftwareStorage(t)
	signer := newStoreTestSigner(t)
	store := NewKeyStore(signer)
	if err := store.ProbeWritableContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if signer.deleteCalls != 1 {
		t.Fatal("probe key leaked")
	}
	signer.signErr = errors.New("signing failed")
	key, _, err := store.CreateAttestationContext(context.Background(), "failed", "nonce", time.Now())
	if !errors.Is(err, signer.signErr) || key == nil {
		t.Fatalf("lost cleanup handle: %v", err)
	}
	if err := key.DeleteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if signer.deleteCalls != 2 {
		t.Fatal("attestation key leaked")
	}
}

func TestKeySnapshotsRegisteredIdentity(t *testing.T) {
	signer := newStoreTestSigner(t)
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := newKey("key", private.Public(), signer)
	if err != nil {
		t.Fatal(err)
	}
	before, err := key.Thumbprint()
	if err != nil {
		t.Fatal(err)
	}
	name := signer.Name()
	private.X.SetInt64(1)
	signer.name = "changed"
	after, err := key.Thumbprint()
	if err != nil || before != after || key.Provider() != name {
		t.Fatalf("registered key identity changed: %v", err)
	}
}

func TestCleanupUnreferencedSigningKeyRoutesSecureEnclave(t *testing.T) {
	isolateSoftwareStorage(t)
	previous := core.AppConfig{
		AuthMethod: core.AuthMethodPrivateKeyJWTLocalKeyPair,
		KeyRef:     &core.KeyRef{Source: core.KeySourceTEE, Provider: keysigner.MacOSSecureEnclaveSignerName, ID: "key-1"},
	}
	// A canceled operation tests the cleanup route without accessing hardware.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	current := &core.MultiAppConfig{Apps: []core.AppConfig{previous}}
	if err := CleanupUnreferencedSigningKey(ctx, &testMetadataStore{}, previous, current); err != nil {
		t.Fatalf("cleanup reached a referenced key: %v", err)
	}
	want := keysigner.ErrUnavailable
	if runtime.GOOS == "darwin" {
		want = context.Canceled
	}
	if err := CleanupUnreferencedSigningKey(ctx, &testMetadataStore{}, previous, nil); !errors.Is(err, want) {
		t.Fatalf("cleanup unreferenced key: got %v, want %v", err, want)
	}
}

func TestCleanupSoftwareKeyAfterUnlockSecretLost(t *testing.T) {
	directory := isolateSoftwareStorage(t)
	kc := &testMetadataStore{values: map[string]string{}}
	ctx := context.Background()
	signer := newSoftwareSigner(kc)
	ref := &core.KeyRef{Source: core.KeySourceTEE, Provider: signer.Name(), ID: "cleanup-test"}
	if _, err := signer.EnsureKey(ctx, keysigner.KeyRef{Label: ref.ID}); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected one encrypted key, got %v, err = %v", files, err)
	}
	delete(kc.values, softwareUnlockAccount)
	previous := core.AppConfig{AuthMethod: core.AuthMethodPrivateKeyJWTLocalKeyPair, KeyRef: ref}
	shared := &core.MultiAppConfig{Apps: []core.AppConfig{previous}}
	if err := CleanupUnreferencedSigningKey(ctx, kc, previous, shared); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(files[0]); err != nil {
		t.Fatalf("shared key was not retained: %v", err)
	}
	if err := CleanupUnreferencedSigningKey(ctx, kc, previous, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(files[0]); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unreferenced encrypted key: stat error = %v", err)
	}
	if err := CleanupUnreferencedSigningKey(ctx, kc, previous, nil); err != nil {
		t.Fatalf("repeat cleanup: %v", err)
	}
}

func TestKeyStoreSharedLockLocation(t *testing.T) {
	isolateSoftwareStorage(t)
	if err := withKeyStoreLockContext(context.Background(), func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	directory, err := signerStorageDir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(directory, "keysigner"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "key_store.lock" {
		t.Fatalf("shared lock: %v %v", entries, err)
	}
}
