//go:build darwin

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"unsafe"
)

// Keep the application tag stable so existing native keys remain discoverable.
const hardwareKeyTag = "com.larksuite.cli"

// These bindings are resolved by loadFFI alongside the L2 Keychain bindings.
var (
	secAccessControlCreate func(allocator, protection, flags uintptr, errOut *uintptr) uintptr
	secKeyCreateRandom     func(attributes uintptr, errOut *uintptr) uintptr
	secKeyCopyPublic       func(key uintptr) uintptr
	secKeyCopyAttributes   func(key uintptr) uintptr
	secItemDelete          func(query uintptr) int32

	kCFErrorDomainOSStatus                           uintptr
	kSecAttrTokenID                                  uintptr
	kSecAttrTokenIDSecureEnclave                     uintptr
	kSecAttrKeySizeInBits                            uintptr
	kSecAttrAccessControl                            uintptr
	kSecAttrApplicationTag                           uintptr
	kSecAttrIsPermanent                              uintptr
	kSecPrivateKeyAttrs                              uintptr
	kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly uintptr
	kSecUseAuthenticationUI                          uintptr
	kSecUseAuthenticationUIFail                      uintptr
	kSecValueRef                                     uintptr
)

type secureEnclaveSigner struct{}

func (secureEnclaveSigner) Name() string                 { return MacOSSecureEnclaveSignerName }
func (secureEnclaveSigner) SecurityLevel() SecurityLevel { return SecurityLevelL1 }

func withSecureEnclaveOperation(ctx context.Context, ref KeyRef, operation func() error) error {
	algorithm, err := algorithmForRef(ctx, ref)
	if err != nil {
		return err
	}
	// Security.framework's Secure Enclave key API only exposes P-256 keys.
	if algorithm.name() != AlgES256 {
		return fmt.Errorf("keysigner: Secure Enclave: %w: %q", ErrUnsupportedAlgorithm, ref.Algorithm)
	}
	if err := requireFFI(); err != nil {
		return err
	}
	return withKeychainUserInteractionDisabled(ctx, operation)
}

func (secureEnclaveSigner) EnsureKey(ctx context.Context, ref KeyRef) (public crypto.PublicKey, err error) {
	err = withSecureEnclaveOperation(ctx, ref, func() error {
		key, err := findSecureEnclaveKey(ref.Label)
		created := errors.Is(err, ErrKeyNotFound)
		if created {
			key, err = createSecureEnclaveKey(ref.Label)
		}
		if err != nil {
			return err
		}
		defer cfRelease(key)
		public, err = secureEnclavePublicKey(key)
		if err != nil && created {
			// Roll back only the key this call created, never a reused identity.
			query := cfDictCreateMutable(0, 0, cbDictKey, cbDictValue)
			if query == 0 {
				return errors.Join(err, errors.New("keysigner: create Secure Enclave cleanup query failed"))
			}
			defer cfRelease(query)
			cfDictSetValue(query, kSecValueRef, key)
			cfDictSetValue(query, kSecUseAuthenticationUI, kSecUseAuthenticationUIFail)
			if status := secItemDelete(query); status != 0 && status != -25300 {
				return errors.Join(err, &secureEnclaveError{operation: "roll back key", code: int(status), osStatus: true})
			}
		}
		return err
	})
	return public, err
}

func (secureEnclaveSigner) PublicKey(ctx context.Context, ref KeyRef) (public crypto.PublicKey, err error) {
	err = withSecureEnclaveOperation(ctx, ref, func() error {
		key, err := findSecureEnclaveKey(ref.Label)
		if err != nil {
			return err
		}
		defer cfRelease(key)
		public, err = secureEnclavePublicKey(key)
		return err
	})
	return public, err
}

func (secureEnclaveSigner) Sign(ctx context.Context, ref KeyRef, input []byte) (signature []byte, algorithm string, err error) {
	err = withSecureEnclaveOperation(ctx, ref, func() error {
		key, err := findSecureEnclaveKey(ref.Label)
		if err != nil {
			return err
		}
		defer cfRelease(key)
		public, err := secureEnclavePublicKey(key)
		if err != nil {
			return err
		}
		a := es256Algorithm{}
		digest := a.digest(input)
		data := cfBytes(digest)
		if data == 0 {
			return errors.New("keysigner: create Secure Enclave digest failed")
		}
		defer cfRelease(data)
		var errRef uintptr
		signed := secKeyCreateSignature(key, algECDSASHA256, data, &errRef)
		if signed == 0 {
			return secureEnclaveCFError("sign", errRef)
		}
		defer cfRelease(signed)
		n, p := cfDataGetLength(signed), cfDataGetBytePtr(signed)
		if n <= 0 || n > 72 || p == nil {
			return ErrCorrupt
		}
		der := unsafe.Slice((*byte)(p), n)
		if !ecdsa.VerifyASN1(public, digest, der) {
			return ErrCorrupt
		}
		signature, err = a.signatureToJOSE(der)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return signature, AlgES256, nil
}

func (secureEnclaveSigner) DeleteKey(ctx context.Context, ref KeyRef) error {
	return withSecureEnclaveOperation(ctx, ref, func() error {
		query, err := secureEnclaveQuery(ref.Label)
		if err != nil {
			return err
		}
		defer cfRelease(query)
		status := secItemDelete(query)
		if status == 0 || status == -25300 { // Missing keys are already deleted.
			return nil
		}
		return &secureEnclaveError{operation: "delete key", code: int(status), osStatus: true}
	})
}

// Secure Enclave keys are found by their stable label and application tag in
// the system keychain, without a dedicated file keychain or public-key sidecar.
func secureEnclaveQuery(label string) (uintptr, error) {
	tag := cfBytes([]byte(hardwareKeyTag))
	if tag == 0 {
		return 0, errors.New("keysigner: create Secure Enclave tag failed")
	}
	defer cfRelease(tag)
	name := cfStringCreate(0, cstr(label), cfStringEncodingUTF8)
	if name == 0 {
		return 0, errors.New("keysigner: create Secure Enclave label failed")
	}
	defer cfRelease(name)
	query := cfDictCreateMutable(0, 0, cbDictKey, cbDictValue)
	if query == 0 {
		return 0, errors.New("keysigner: create Secure Enclave query failed")
	}
	cfDictSetValue(query, kSecClass, kSecClassKey)
	cfDictSetValue(query, kSecAttrKeyType, kSecAttrKeyTypeEC)
	cfDictSetValue(query, kSecAttrKeyClass, kSecAttrKeyClassPrivate)
	cfDictSetValue(query, kSecAttrTokenID, kSecAttrTokenIDSecureEnclave)
	cfDictSetValue(query, kSecAttrApplicationTag, tag)
	cfDictSetValue(query, kSecAttrLabel, name)
	// Fail, never skip protected keys: skipping could look like a missing key.
	cfDictSetValue(query, kSecUseAuthenticationUI, kSecUseAuthenticationUIFail)
	return query, nil
}

func findSecureEnclaveKey(label string) (uintptr, error) {
	query, err := secureEnclaveQuery(label)
	if err != nil {
		return 0, err
	}
	defer cfRelease(query)
	cfDictSetValue(query, kSecReturnRef, kCFBooleanTrue)
	var key uintptr
	if status := secItemCopyMatching(query, &key); status != 0 {
		return 0, &secureEnclaveError{operation: "find key", code: int(status), osStatus: true}
	}
	if key == 0 {
		return 0, ErrCorrupt
	}
	return key, nil
}

func createSecureEnclaveKey(label string) (uintptr, error) {
	var errRef uintptr
	// Keep the device-bound policy with kSecAccessControlPrivateKeyUsage
	// (1 << 30), without a user-presence or biometric requirement.
	access := secAccessControlCreate(0, kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly, 1<<30, &errRef)
	if access == 0 {
		return 0, secureEnclaveCFError("create access control", errRef)
	}
	defer cfRelease(access)
	tag := cfBytes([]byte(hardwareKeyTag))
	if tag == 0 {
		return 0, errors.New("keysigner: create Secure Enclave tag failed")
	}
	defer cfRelease(tag)
	name := cfStringCreate(0, cstr(label), cfStringEncodingUTF8)
	if name == 0 {
		return 0, errors.New("keysigner: create Secure Enclave label failed")
	}
	defer cfRelease(name)
	bits := int32(256)
	size := cfNumberCreate(0, 3, &bits) // kCFNumberSInt32Type
	if size == 0 {
		return 0, errors.New("keysigner: create Secure Enclave key size failed")
	}
	defer cfRelease(size)
	private := cfDictCreateMutable(0, 0, cbDictKey, cbDictValue)
	if private == 0 {
		return 0, errors.New("keysigner: create Secure Enclave private attributes failed")
	}
	defer cfRelease(private)
	cfDictSetValue(private, kSecAttrIsPermanent, kCFBooleanTrue)
	cfDictSetValue(private, kSecAttrApplicationTag, tag)
	cfDictSetValue(private, kSecAttrAccessControl, access)
	attrs := cfDictCreateMutable(0, 0, cbDictKey, cbDictValue)
	if attrs == 0 {
		return 0, errors.New("keysigner: create Secure Enclave attributes failed")
	}
	defer cfRelease(attrs)
	cfDictSetValue(attrs, kSecAttrLabel, name)
	cfDictSetValue(attrs, kSecAttrKeyType, kSecAttrKeyTypeEC)
	cfDictSetValue(attrs, kSecAttrKeySizeInBits, size)
	cfDictSetValue(attrs, kSecAttrTokenID, kSecAttrTokenIDSecureEnclave)
	cfDictSetValue(attrs, kSecPrivateKeyAttrs, private)
	cfDictSetValue(attrs, kSecUseAuthenticationUI, kSecUseAuthenticationUIFail)
	key := secKeyCreateRandom(attrs, &errRef)
	if key == 0 {
		return 0, secureEnclaveCFError("create key", errRef)
	}
	return key, nil
}

func secureEnclavePublicKey(key uintptr) (*ecdsa.PublicKey, error) {
	attrs := secKeyCopyAttributes(key)
	if attrs == 0 {
		return nil, ErrCorrupt
	}
	defer cfRelease(attrs)
	token := cfDictGetValue(attrs, kSecAttrTokenID)
	if token == 0 || cfEqual(token, kSecAttrTokenIDSecureEnclave) == 0 {
		return nil, fmt.Errorf("keysigner: key is not backed by Secure Enclave: %w", ErrCorrupt)
	}
	public := secKeyCopyPublic(key)
	if public == 0 {
		return nil, errors.New("keysigner: copy Secure Enclave public key failed")
	}
	defer cfRelease(public)
	var errRef uintptr
	data := secKeyCopyExternal(public, &errRef) // Only the public key is exported.
	if data == 0 {
		return nil, secureEnclaveCFError("export public key", errRef)
	}
	defer cfRelease(data)
	n, p := cfDataGetLength(data), cfDataGetBytePtr(data)
	if n != 65 || p == nil {
		return nil, ErrCorrupt
	}
	parsed, err := (es256Algorithm{}).parseKeychainPublicKey(unsafe.Slice((*byte)(p), n))
	if err != nil {
		return nil, err
	}
	return P256PublicKey(parsed)
}

// Preserve the native code and its namespace; only documented OSStatus values
// permit fallback. Permission, entitlement, interaction, and unknown failures
// remain errors, even if another CFError domain happens to reuse the same code.
type secureEnclaveError struct {
	operation string
	code      int
	osStatus  bool
}

func (e *secureEnclaveError) Error() string {
	return fmt.Sprintf("keysigner: Secure Enclave %s failed (code %d, OSStatus=%t)", e.operation, e.code, e.osStatus)
}

func (e *secureEnclaveError) Is(target error) bool {
	if !e.osStatus {
		return false
	}
	switch e.code {
	case -25300:
		return target == ErrKeyNotFound
	case -4, -25291:
		return target == ErrUnavailable
	default:
		return false
	}
}

func secureEnclaveCFError(operation string, ref uintptr) error {
	if ref == 0 {
		return fmt.Errorf("keysigner: Secure Enclave %s returned neither a result nor an error", operation)
	}
	defer cfRelease(ref)
	domain := cfErrorGetDomain(ref)
	return &secureEnclaveError{
		operation: operation,
		code:      cfErrorGetCode(ref),
		osStatus:  domain != 0 && cfEqual(domain, kCFErrorDomainOSStatus) != 0,
	}
}
