//go:build darwin

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"
)

func TestSecureEnclaveLifecycleAndNativeFailures(t *testing.T) {
	// CoreFoundation allocations are real; every keychain call is replaced.
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Cleanup(func() {
		if _, err := os.Stat(filepath.Join(root, "Library", "Application Support", "lark-cli", "keysigner")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Secure Enclave operations created local state: %v", err)
		}
	})
	if err := loadFFI(); err != nil {
		t.Fatal(err)
	}
	oldFind, oldCreate, oldAccess := secItemCopyMatching, secKeyCreateRandom, secAccessControlCreate
	oldAttrs, oldPublic, oldExport := secKeyCopyAttributes, secKeyCopyPublic, secKeyCopyExternal
	oldSign, oldDelete := secKeyCreateSignature, secItemDelete
	oldGetUI, oldSetUI := getKeychainUserInteractionAllowed, setKeychainUserInteractionAllowed
	t.Cleanup(func() {
		secItemCopyMatching, secKeyCreateRandom, secAccessControlCreate = oldFind, oldCreate, oldAccess
		secKeyCopyAttributes, secKeyCopyPublic, secKeyCopyExternal = oldAttrs, oldPublic, oldExport
		secKeyCreateSignature, secItemDelete = oldSign, oldDelete
		getKeychainUserInteractionAllowed, setKeychainUserInteractionAllowed = oldGetUI, oldSetUI
	})
	getKeychainUserInteractionAllowed = func() (bool, error) { return false, nil }
	setKeychainUserInteractionAllowed = func(bool) error { return nil }
	var cfErrorCreate func(allocator, domain uintptr, code int, info uintptr) uintptr
	cf, err := purego.Dlopen(cfFrameworkPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	defer purego.Dlclose(cf)
	purego.RegisterLibFunc(&cfErrorCreate, cf, "CFErrorCreate")
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rawPublic := elliptic.Marshal(elliptic.P256(), private.X, private.Y)
	ref := KeyRef{Label: `existing:native/label\key`}
	ctx := context.Background()
	signer := secureEnclaveSigner{}
	name := cfStringCreate(0, cstr(ref.Label), cfStringEncodingUTF8)
	defer cfRelease(name)
	tag := cfBytes([]byte("com.larksuite.cli"))
	defer cfRelease(tag)
	bits := int32(256)
	size := cfNumberCreate(0, 3, &bits)
	defer cfRelease(size)
	assertValue := func(dict, key, want uintptr) {
		t.Helper()
		got := cfDictGetValue(dict, key)
		if got == 0 || cfEqual(got, want) == 0 {
			t.Fatalf("native dictionary value for %x = %x, want %x", key, got, want)
		}
	}
	assertQuery := func(query uintptr) {
		t.Helper()
		assertValue(query, kSecClass, kSecClassKey)
		assertValue(query, kSecAttrKeyType, kSecAttrKeyTypeEC)
		assertValue(query, kSecAttrKeyClass, kSecAttrKeyClassPrivate)
		assertValue(query, kSecAttrTokenID, kSecAttrTokenIDSecureEnclave)
		assertValue(query, kSecAttrLabel, name)
		assertValue(query, kSecAttrApplicationTag, tag)
		assertValue(query, kSecUseAuthenticationUI, kSecUseAuthenticationUIFail)
	}
	bytesOf := func(data uintptr) []byte {
		return unsafe.Slice((*byte)(cfDataGetBytePtr(data)), cfDataGetLength(data))
	}
	var found, wrongToken, badSignature, badPublic bool
	var creates, deletes int
	var findStatus int32
	secItemCopyMatching = func(query uintptr, out *uintptr) int32 {
		assertQuery(query)
		assertValue(query, kSecReturnRef, kCFBooleanTrue)
		if findStatus != 0 {
			return findStatus
		}
		if !found {
			return -25300
		}
		*out = cfBytes([]byte("private"))
		return 0
	}
	secAccessControlCreate = func(_, protection, flags uintptr, _ *uintptr) uintptr {
		if protection != kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly || flags != 1<<30 {
			t.Fatal("changed device-bound protection policy or requested user presence")
		}
		return cfBytes([]byte("access"))
	}
	secKeyCreateRandom = func(attrs uintptr, out *uintptr) uintptr {
		creates++
		assertValue(attrs, kSecAttrLabel, name)
		assertValue(attrs, kSecAttrTokenID, kSecAttrTokenIDSecureEnclave)
		assertValue(attrs, kSecAttrKeyType, kSecAttrKeyTypeEC)
		assertValue(attrs, kSecAttrKeySizeInBits, size)
		assertValue(attrs, kSecUseAuthenticationUI, kSecUseAuthenticationUIFail)
		priv := cfDictGetValue(attrs, kSecPrivateKeyAttrs)
		if priv == 0 {
			t.Fatal("missing private-key policy")
		}
		assertValue(priv, kSecAttrIsPermanent, kCFBooleanTrue)
		assertValue(priv, kSecAttrApplicationTag, tag)
		if string(bytesOf(cfDictGetValue(priv, kSecAttrAccessControl))) != "access" {
			t.Fatal("missing access control")
		}
		found = true
		return cfBytes([]byte("private"))
	}
	secKeyCopyAttributes = func(uintptr) uintptr {
		attrs := cfDictCreateMutable(0, 0, cbDictKey, cbDictValue)
		if !wrongToken {
			cfDictSetValue(attrs, kSecAttrTokenID, kSecAttrTokenIDSecureEnclave)
		}
		return attrs
	}
	secKeyCopyPublic = func(uintptr) uintptr { return cfBytes([]byte("public")) }
	secKeyCopyExternal = func(key uintptr, _ *uintptr) uintptr {
		if string(bytesOf(key)) != "public" {
			t.Fatal("attempted private-key export")
		}
		if badPublic {
			return cfBytes([]byte("invalid"))
		}
		return cfBytes(rawPublic)
	}
	secKeyCreateSignature = func(_, algorithm, digest uintptr, out *uintptr) uintptr {
		if algorithm != algECDSASHA256 || len(bytesOf(digest)) != sha256.Size {
			t.Fatal("native signing did not receive SHA-256 digest and algorithm")
		}
		der, err := ecdsa.SignASN1(rand.Reader, private, bytesOf(digest))
		if err != nil {
			t.Fatal(err)
		}
		if badSignature {
			der[len(der)-1] ^= 1
		}
		return cfBytes(der)
	}
	secItemDelete = func(query uintptr) int32 {
		if value := cfDictGetValue(query, kSecValueRef); value != 0 {
			if string(bytesOf(value)) != "private" {
				t.Fatal("rollback selected an unrelated key")
			}
		} else {
			assertQuery(query)
			if cfDictGetValue(query, kSecReturnRef) != 0 {
				t.Fatal("delete query contains a return parameter")
			}
		}
		if !found {
			return -25300
		}
		deletes++
		found = false
		return 0
	}

	if _, err := signer.PublicKey(ctx, ref); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("missing public key: %v", err)
	}
	if _, _, err := signer.Sign(ctx, ref, nil); !errors.Is(err, ErrKeyNotFound) || creates != 0 {
		t.Fatalf("missing key was recreated by a read/sign: %v", err)
	}
	for i := 0; i < 2; i++ {
		public, err := signer.EnsureKey(ctx, ref)
		if err != nil || !private.PublicKey.Equal(public) || creates != 1 {
			t.Fatalf("create/reopen changed key identity: %v", err)
		}
	}
	message := []byte("single-hash Secure Enclave signature")
	signature, algorithm, err := signer.Sign(ctx, ref, message)
	digest := sha256.Sum256(message)
	if err != nil || algorithm != AlgES256 || len(signature) != 64 ||
		!ecdsa.Verify(&private.PublicKey, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
		t.Fatalf("JOSE signature failed independent verification: %v", err)
	}
	badSignature = true
	if _, _, err := signer.Sign(ctx, ref, message); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("accepted invalid native signature: %v", err)
	}
	badSignature, wrongToken = false, true
	if _, err := signer.EnsureKey(ctx, ref); !errors.Is(err, ErrCorrupt) || creates != 1 || deletes != 0 {
		t.Fatalf("non-hardware key accepted or existing key mutated: %v", err)
	}
	wrongToken = false
	findStatus = -34018
	beforeCreates := creates
	if _, err := signer.EnsureKey(ctx, ref); err == nil || errors.Is(err, ErrUnavailable) || creates != beforeCreates {
		t.Fatalf("entitlement failure permitted fallback or replacement: %v", err)
	}
	findStatus = 0
	found, badPublic = false, true
	if _, err := signer.EnsureKey(ctx, ref); !errors.Is(err, ErrCorrupt) || found || deletes != 1 {
		t.Fatalf("failed creation left a key behind: %v", err)
	}
	badPublic, found = false, true
	if err := signer.DeleteKey(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := signer.DeleteKey(ctx, ref); err != nil || deletes != 2 {
		t.Fatalf("deletion is not idempotent: %v", err)
	}
	beforeCreates = creates
	for _, invalid := range []KeyRef{{Label: ref.Label, Algorithm: AlgRS256}, {Label: ""}, {Label: "a\x00b"}} {
		if _, err := signer.EnsureKey(ctx, invalid); err == nil || creates != beforeCreates {
			t.Fatalf("invalid request reached storage: %v", err)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := signer.EnsureKey(canceled, ref); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request: %v", err)
	}
	foreignDomain := cfStringCreate(0, cstr("enclave-error.example"), cfStringEncodingUTF8)
	defer cfRelease(foreignDomain)
	if err := secureEnclaveCFError("fixture", cfErrorCreate(0, foreignDomain, -4, 0)); errors.Is(err, ErrUnavailable) {
		t.Fatal("unrelated CFError domain permitted fallback")
	}
	if err := secureEnclaveCFError("fixture", cfErrorCreate(0, kCFErrorDomainOSStatus, -4, 0)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("known OSStatus did not permit fallback: %v", err)
	}
}
