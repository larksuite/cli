//go:build darwin

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestKeychainSignerRegistered confirms every Darwin build self-registers the
// signer (init → Register), so keysigner.Active() is non-nil. No keychain access.
func TestKeychainSignerRegistered(t *testing.T) {
	if _, ok := Active().(keychainSigner); !ok {
		t.Fatalf("Active() = %T, want keychainSigner (Darwin build must self-register)", Active())
	}
}

func TestKeychainFFIBindings(t *testing.T) {
	if err := loadFFI(); err != nil {
		t.Fatalf("loadFFI: %v", err)
	}
	if secKeychainCreate == nil || secKeychainOpen == nil || secKeychainUnlock == nil ||
		secAccessCreate == nil || secKeyCreatePair == nil || secKeyCopyExternal == nil || secKeychainItemDelete == nil {
		t.Fatal("one or more keychain functions were not registered")
	}
}

func TestPrivateKeyGenerationAttributesAreNonExtractable(t *testing.T) {
	if privateKeyAttributes&cssmKeyAttrPermanent == 0 {
		t.Fatal("private key must be stored permanently in the dedicated keychain")
	}
	if privateKeyAttributes&cssmKeyAttrSensitive == 0 {
		t.Fatal("private key must be marked sensitive")
	}
	if privateKeyAttributes&cssmKeyAttrExtractable != 0 {
		t.Fatal("private key must not be extractable")
	}
	if publicKeyAttributes&cssmKeyAttrExtractable == 0 {
		t.Fatal("public key must remain exportable")
	}
}

// TestEnsureKeychainUnlocksExistingKeychain covers the recovery path for a
// dedicated file keychain that was created in an earlier process and has since
// become locked. No real Security.framework call is executed.
func TestEnsureKeychainUnlocksExistingKeychain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, "Library", "Application Support", "lark-cli", "keysigner")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	keychainPath := filepath.Join(dir, "lark-cli.keychain")
	if err := os.WriteFile(keychainPath, []byte("not-a-real-keychain"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "keychain.pass"), []byte("test-password\n"), 0600); err != nil {
		t.Fatal(err)
	}

	previousCreate := createKeychainFile
	previousUnlock := unlockKeychainFile
	createKeychainFile = func(string, []byte) error {
		t.Fatal("createKeychainFile called for an existing keychain")
		return nil
	}
	type unlockCall struct {
		path     string
		password []byte
	}
	var calls []unlockCall
	var borrowedPassword []byte
	unlockKeychainFile = func(path string, password []byte) error {
		borrowedPassword = password
		calls = append(calls, unlockCall{path: path, password: append([]byte(nil), password...)})
		return nil
	}
	t.Cleanup(func() {
		createKeychainFile = previousCreate
		unlockKeychainFile = previousUnlock
	})

	got, err := ensureKeychain()
	if err != nil {
		t.Fatalf("ensureKeychain: %v", err)
	}
	if got != keychainPath {
		t.Fatalf("ensureKeychain path = %q, want %q", got, keychainPath)
	}
	want := []unlockCall{{path: keychainPath, password: []byte("test-password")}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unlock calls = %#v, want %#v", calls, want)
	}
	for i, value := range borrowedPassword {
		if value != 0 {
			t.Fatalf("borrowed password byte %d was not cleared after use", i)
		}
	}
}

func TestEnsureKeychainNewKeychainStillUnlocksAfterCreation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	type lifecycleCall struct {
		op       string
		path     string
		password []byte
	}
	var calls []lifecycleCall
	previousCreate := createKeychainFile
	previousUnlock := unlockKeychainFile
	createKeychainFile = func(path string, password []byte) error {
		calls = append(calls, lifecycleCall{op: "create", path: path, password: append([]byte(nil), password...)})
		return nil
	}
	unlockKeychainFile = func(path string, password []byte) error {
		calls = append(calls, lifecycleCall{op: "unlock", path: path, password: append([]byte(nil), password...)})
		return nil
	}
	t.Cleanup(func() {
		createKeychainFile = previousCreate
		unlockKeychainFile = previousUnlock
	})

	keychainPath, err := ensureKeychain()
	if err != nil {
		t.Fatalf("ensureKeychain: %v", err)
	}
	if len(calls) != 2 || calls[0].op != "create" || calls[1].op != "unlock" {
		t.Fatalf("lifecycle calls = %#v, want create then unlock", calls)
	}
	for _, call := range calls {
		if call.path != keychainPath || len(call.password) != 64 {
			t.Fatalf("lifecycle call = %#v, want path %q and generated 64-byte password", call, keychainPath)
		}
	}
	if !reflect.DeepEqual(calls[0].password, calls[1].password) {
		t.Fatal("create and unlock did not receive the same generated password")
	}
}

func TestEnsureKeychainExistingUnlockErrorIsReturned(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, "Library", "Application Support", "lark-cli", "keysigner")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lark-cli.keychain"), []byte("not-a-real-keychain"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "keychain.pass"), []byte("test-password\n"), 0600); err != nil {
		t.Fatal(err)
	}

	previousCreate := createKeychainFile
	previousUnlock := unlockKeychainFile
	createKeychainFile = func(string, []byte) error {
		t.Fatal("createKeychainFile called for an existing keychain")
		return nil
	}
	unlockKeychainFile = func(string, []byte) error {
		return errors.New("keysigner: unlock keychain: Security framework status -25293")
	}
	t.Cleanup(func() {
		createKeychainFile = previousCreate
		unlockKeychainFile = previousUnlock
	})

	_, err := ensureKeychain()
	if err == nil {
		t.Fatal("ensureKeychain returned nil error")
	}
	if got := err.Error(); !strings.Contains(got, "unlock keychain") || !strings.Contains(got, "-25293") {
		t.Fatalf("ensureKeychain error = %q", got)
	}
}

func TestEnsureKeychainCreateErrorStopsBeforeUnlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	previousCreate := createKeychainFile
	previousUnlock := unlockKeychainFile
	createKeychainFile = func(string, []byte) error {
		return errors.New("keysigner: create keychain: Security framework status -50")
	}
	unlockKeychainFile = func(string, []byte) error {
		t.Fatal("unlockKeychainFile called after creation failed")
		return nil
	}
	t.Cleanup(func() {
		createKeychainFile = previousCreate
		unlockKeychainFile = previousUnlock
	})

	_, err := ensureKeychain()
	if err == nil || !strings.Contains(err.Error(), "create keychain") || !strings.Contains(err.Error(), "-50") {
		t.Fatalf("ensureKeychain error = %v", err)
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
		t.Skip("set LARK_KEYCHAIN_IT=1 to run the macOS Keychain integration test")
	}
	t.Setenv("HOME", t.TempDir())
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

	md, err := readKeyMetadata(ref.Label)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	appLabel, err := hex.DecodeString(md.AppLabel)
	if err != nil {
		t.Fatalf("decode app label: %v", err)
	}
	keychain, err := ensureKeychain()
	if err != nil {
		t.Fatalf("ensure keychain for export check: %v", err)
	}
	privateKeyRef, err := findPrivateKey(appLabel, keychain)
	if err != nil {
		t.Fatalf("find private key for export check: %v", err)
	}
	defer cfRelease(privateKeyRef)
	var exportErr uintptr
	if exported := secKeyCopyExternal(privateKeyRef, &exportErr); exported != 0 {
		cfRelease(exported)
		t.Fatal("private key was exportable")
	}
	if exportErr != 0 {
		cfRelease(exportErr)
	}
}
