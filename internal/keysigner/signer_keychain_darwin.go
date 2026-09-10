//go:build darwin

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// macOS non-exportable Keychain signer (compiled into every darwin build).
//
// It does NOT use the Secure Enclave / hardware TEE (which would require
// code-signing entitlements that are unfriendly to open source). Instead it
// generates a P-256 or RSA-2048 key directly inside a dedicated app keychain. The
// private key is permanent, sensitive, sign-only, and non-extractable; it is
// never present in Go memory or a temporary file. Signing is
// ECDSA-SHA256 (ES256) or RSA PKCS#1 v1.5 SHA-256 (RS256).
//
// Security and CoreFoundation are called through runtime FFI
// (github.com/ebitengine/purego). Key generation and signing stay inside the OS
// APIs while the binary remains CGO-free and can be cross-compiled for darwin.
//
// Build with:  go build   (cgo-free; compiled into every darwin build, no tag)
package keysigner

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

// ---- Security / CoreFoundation runtime bindings (purego, no cgo) ----

const (
	cfFrameworkPath  = "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"
	secFrameworkPath = "/System/Library/Frameworks/Security.framework/Security"

	// kCFStringEncodingUTF8 (CFStringBuiltInEncodings).
	cfStringEncodingUTF8 = 0x08000100

	// OSStatus values.
	errSecSuccess = 0

	// Legacy Security.framework key-generation values from cssmtype.h. These
	// APIs can target the dedicated file keychain while keeping the private key
	// non-extractable and sign-only from the moment it is created.
	cssmKeyUseSign         = 0x00000004
	cssmKeyUseVerify       = 0x00000008
	cssmKeyAttrPermanent   = 0x00000001
	cssmKeyAttrSensitive   = 0x00000008
	cssmKeyAttrExtractable = 0x00000020

	publicKeyAttributes  = cssmKeyAttrPermanent | cssmKeyAttrExtractable
	privateKeyAttributes = cssmKeyAttrPermanent | cssmKeyAttrSensitive
)

var (
	ffiOnce sync.Once
	ffiErr  error

	cfEqual               func(a, b uintptr) uint8
	cfNumberCreate        func(alloc uintptr, numberType int, value *int32) uintptr
	cfDictGetValue        func(dict, key uintptr) uintptr
	cfErrorGetDomain      func(e uintptr) uintptr
	cfDataCreate          func(alloc uintptr, bytes *byte, length int) uintptr
	cfDataGetLength       func(d uintptr) int
	cfDataGetBytePtr      func(d uintptr) unsafe.Pointer
	cfStringCreate        func(alloc uintptr, cstr *byte, encoding uint32) uintptr
	cfArrayCreate         func(alloc uintptr, values *uintptr, numValues int, cb uintptr) uintptr
	cfArrayGetCount       func(array uintptr) int
	cfArrayGetValue       func(array uintptr, index int) uintptr
	cfDictCreateMutable   func(alloc uintptr, capacity int, keyCB, valCB uintptr) uintptr
	cfDictSetValue        func(dict, key, val uintptr)
	cfRelease             func(ref uintptr)
	cfErrorGetCode        func(e uintptr) int
	dlsymDataPointer      func(handle uintptr, name string) *uintptr
	secKeychainCreate     func(path *byte, passwordLength uint32, password unsafe.Pointer, promptUser uint8, initialAccess uintptr, out *uintptr) int32
	secKeychainOpen       func(path *byte, out *uintptr) int32
	secKeychainUnlock     func(keychain uintptr, passwordLength uint32, password unsafe.Pointer, usePassword uint8) int32
	secKeychainGetUI      func(state *uint8) int32
	secKeychainSetUI      func(state uint8) int32
	secAccessCreate       func(descriptor, trustedList uintptr, out *uintptr) int32
	secAccessCopyACLs     func(access, authorization uintptr) uintptr
	secACLSetContents     func(acl, applicationList, description uintptr, promptSelector uint32) int32
	secKeyCreatePair      func(keychain uintptr, algorithm, keySize uint32, contextHandle uint64, publicKeyUsage, publicKeyAttr, privateKeyUsage, privateKeyAttr uint32, initialAccess uintptr, publicKey, privateKey *uintptr) int32
	secKeyCopyExternal    func(key uintptr, errOut *uintptr) uintptr
	secKeychainItemDelete func(item uintptr) int32
	secItemCopyMatching   func(query uintptr, result *uintptr) int32
	secItemUpdate         func(query, attrs uintptr) int32
	secKeyCreateSignature func(key, algo, data uintptr, errOut *uintptr) uintptr

	// CFTypeRef data-symbol constants (deref to obtain the held ref value).
	kSecClass                uintptr
	kSecClassKey             uintptr
	kSecAttrKeyClass         uintptr
	kSecAttrKeyClassPrivate  uintptr
	kSecAttrKeyClassPublic   uintptr
	kSecAttrKeyType          uintptr
	kSecAttrKeyTypeEC        uintptr
	kSecAttrKeyTypeRSA       uintptr
	kSecAttrApplicationLabel uintptr
	kSecReturnRef            uintptr
	kSecMatchSearchList      uintptr
	kSecAttrLabel            uintptr
	kSecACLAuthorizationSign uintptr
	kCFBooleanTrue           uintptr
	algECDSASHA256           uintptr
	algRSASHA256             uintptr

	// Struct-symbol constants (passed BY ADDRESS, not dereferenced).
	cbTypeArray uintptr
	cbDictKey   uintptr
	cbDictValue uintptr
)

// loadFFI resolves the framework functions and constants once. Any failure
// (framework missing, symbol absent) is returned to every caller so signing
// fails cleanly rather than crashing.
func loadFFI() error {
	ffiOnce.Do(func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				ffiErr = fmt.Errorf("keysigner: load Security framework bindings: %v", recovered)
			}
		}()
		cf, err := purego.Dlopen(cfFrameworkPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			ffiErr = fmt.Errorf("keysigner: dlopen CoreFoundation: %w", err)
			return
		}
		sec, err := purego.Dlopen(secFrameworkPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			ffiErr = fmt.Errorf("keysigner: dlopen Security: %w", err)
			return
		}

		purego.RegisterLibFunc(&cfDataCreate, cf, "CFDataCreate")
		purego.RegisterLibFunc(&cfEqual, cf, "CFEqual")
		purego.RegisterLibFunc(&cfNumberCreate, cf, "CFNumberCreate")
		purego.RegisterLibFunc(&cfDictGetValue, cf, "CFDictionaryGetValue")
		purego.RegisterLibFunc(&cfErrorGetDomain, cf, "CFErrorGetDomain")
		purego.RegisterLibFunc(&cfDataGetLength, cf, "CFDataGetLength")
		purego.RegisterLibFunc(&cfDataGetBytePtr, cf, "CFDataGetBytePtr")
		purego.RegisterLibFunc(&cfStringCreate, cf, "CFStringCreateWithCString")
		purego.RegisterLibFunc(&cfArrayCreate, cf, "CFArrayCreate")
		purego.RegisterLibFunc(&cfArrayGetCount, cf, "CFArrayGetCount")
		purego.RegisterLibFunc(&cfArrayGetValue, cf, "CFArrayGetValueAtIndex")
		purego.RegisterLibFunc(&cfDictCreateMutable, cf, "CFDictionaryCreateMutable")
		purego.RegisterLibFunc(&cfDictSetValue, cf, "CFDictionarySetValue")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
		purego.RegisterLibFunc(&cfErrorGetCode, cf, "CFErrorGetCode")
		purego.RegisterLibFunc(&dlsymDataPointer, purego.RTLD_DEFAULT, "dlsym")
		purego.RegisterLibFunc(&secKeychainCreate, sec, "SecKeychainCreate")
		purego.RegisterLibFunc(&secKeychainOpen, sec, "SecKeychainOpen")
		purego.RegisterLibFunc(&secKeychainUnlock, sec, "SecKeychainUnlock")
		purego.RegisterLibFunc(&secKeychainGetUI, sec, "SecKeychainGetUserInteractionAllowed")
		purego.RegisterLibFunc(&secKeychainSetUI, sec, "SecKeychainSetUserInteractionAllowed")
		purego.RegisterLibFunc(&secAccessCreate, sec, "SecAccessCreate")
		purego.RegisterLibFunc(&secAccessCopyACLs, sec, "SecAccessCopyMatchingACLList")
		purego.RegisterLibFunc(&secACLSetContents, sec, "SecACLSetContents")
		purego.RegisterLibFunc(&secKeyCreatePair, sec, "SecKeyCreatePair")
		purego.RegisterLibFunc(&secKeyCopyExternal, sec, "SecKeyCopyExternalRepresentation")
		purego.RegisterLibFunc(&secKeychainItemDelete, sec, "SecKeychainItemDelete")
		purego.RegisterLibFunc(&secItemCopyMatching, sec, "SecItemCopyMatching")
		purego.RegisterLibFunc(&secItemUpdate, sec, "SecItemUpdate")
		purego.RegisterLibFunc(&secKeyCreateSignature, sec, "SecKeyCreateSignature")
		purego.RegisterLibFunc(&secAccessControlCreate, sec, "SecAccessControlCreateWithFlags")
		purego.RegisterLibFunc(&secKeyCreateRandom, sec, "SecKeyCreateRandomKey")
		purego.RegisterLibFunc(&secKeyCopyPublic, sec, "SecKeyCopyPublicKey")
		purego.RegisterLibFunc(&secKeyCopyAttributes, sec, "SecKeyCopyAttributes")
		purego.RegisterLibFunc(&secItemDelete, sec, "SecItemDelete")

		// CFStringRef/CFBooleanRef constants: Dlsym gives the address of the
		// exported variable; deref once to read the ref it holds.
		derefs := []struct {
			dst    *uintptr
			handle uintptr
			name   string
		}{
			{&kSecClass, sec, "kSecClass"},
			{&kSecClassKey, sec, "kSecClassKey"},
			{&kSecAttrKeyClass, sec, "kSecAttrKeyClass"},
			{&kSecAttrKeyClassPrivate, sec, "kSecAttrKeyClassPrivate"},
			{&kSecAttrKeyClassPublic, sec, "kSecAttrKeyClassPublic"},
			{&kSecAttrKeyType, sec, "kSecAttrKeyType"},
			{&kSecAttrKeyTypeEC, sec, "kSecAttrKeyTypeECSECPrimeRandom"},
			{&kSecAttrKeyTypeRSA, sec, "kSecAttrKeyTypeRSA"},
			{&kSecAttrApplicationLabel, sec, "kSecAttrApplicationLabel"},
			{&kSecReturnRef, sec, "kSecReturnRef"},
			{&kSecMatchSearchList, sec, "kSecMatchSearchList"},
			{&kSecAttrLabel, sec, "kSecAttrLabel"},
			{&kSecACLAuthorizationSign, sec, "kSecACLAuthorizationSign"},
			{&kCFBooleanTrue, cf, "kCFBooleanTrue"},
			{&kCFErrorDomainOSStatus, cf, "kCFErrorDomainOSStatus"},
			{&kSecAttrTokenID, sec, "kSecAttrTokenID"},
			{&kSecAttrTokenIDSecureEnclave, sec, "kSecAttrTokenIDSecureEnclave"},
			{&kSecAttrKeySizeInBits, sec, "kSecAttrKeySizeInBits"},
			{&kSecAttrAccessControl, sec, "kSecAttrAccessControl"},
			{&kSecAttrApplicationTag, sec, "kSecAttrApplicationTag"},
			{&kSecAttrIsPermanent, sec, "kSecAttrIsPermanent"},
			{&kSecPrivateKeyAttrs, sec, "kSecPrivateKeyAttrs"},
			{&kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly, sec, "kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly"},
			{&kSecUseAuthenticationUI, sec, "kSecUseAuthenticationUI"},
			{&kSecUseAuthenticationUIFail, sec, "kSecUseAuthenticationUIFail"},
			{&kSecValueRef, sec, "kSecValueRef"},
			{&algECDSASHA256, sec, "kSecKeyAlgorithmECDSASignatureDigestX962SHA256"},
			{&algRSASHA256, sec, "kSecKeyAlgorithmRSASignatureDigestPKCS1v15SHA256"},
		}
		for _, d := range derefs {
			sym := dlsymDataPointer(d.handle, d.name)
			if sym == nil {
				ffiErr = fmt.Errorf("keysigner: dlsym %s returned zero address", d.name)
				return
			}
			*d.dst = *sym
			if *d.dst == 0 {
				ffiErr = fmt.Errorf("keysigner: data symbol %s contains a zero reference", d.name)
				return
			}
		}

		// Callback structs are passed by address (no deref).
		addrs := []struct {
			dst    *uintptr
			handle uintptr
			name   string
		}{
			{&cbTypeArray, cf, "kCFTypeArrayCallBacks"},
			{&cbDictKey, cf, "kCFTypeDictionaryKeyCallBacks"},
			{&cbDictValue, cf, "kCFTypeDictionaryValueCallBacks"},
		}
		for _, a := range addrs {
			sym, e := purego.Dlsym(a.handle, a.name)
			if e != nil {
				ffiErr = fmt.Errorf("keysigner: dlsym %s: %w", a.name, e)
				return
			}
			if sym == 0 {
				ffiErr = fmt.Errorf("keysigner: dlsym %s returned zero address", a.name)
				return
			}
			*a.dst = sym
		}
	})
	return ffiErr
}

// cstr returns a pointer to a NUL-terminated copy of s. The backing array stays
// alive while the returned pointer is reachable.
func cstr(s string) *byte {
	b := append([]byte(s), 0)
	return &b[0]
}

// cfBytes wraps Go bytes in a CFData (CFDataCreate copies the bytes). Caller
// releases the returned CFDataRef.
func cfBytes(b []byte) uintptr {
	var p *byte
	if len(b) > 0 {
		p = &b[0]
	}
	d := cfDataCreate(0, p, len(b))
	runtime.KeepAlive(b)
	return d
}

// keychainSearchArray opens the dedicated keychain file and wraps it in a
// CFArray for kSecMatchSearchList. Caller releases the returned array.
//
// NOTE: SecKeychainOpen / the file-based keychain are deprecated by Apple in
// favor of the data-protection keychain. They still function on current macOS;
// migrating off them is tracked separately and is independent of the cgo→purego
// change (the original cgo version used the same APIs).
func keychainSearchArray(keychainPath string) (uintptr, error) {
	var kc uintptr
	if st := secKeychainOpen(cstr(keychainPath), &kc); st != errSecSuccess {
		return 0, keychainError("open keychain", int(st))
	}
	vals := [1]uintptr{kc}
	arr := cfArrayCreate(0, &vals[0], 1, cbTypeArray)
	cfRelease(kc) // the array retains it
	if arr == 0 {
		return 0, fmt.Errorf("keysigner: CFArrayCreate(search list) failed")
	}
	return arr, nil
}

// findKey locates a key by its application label and key class within the
// dedicated keychain. Caller releases the returned SecKeyRef.
func findKey(appLabel []byte, keychainPath string, keyClass, keyType uintptr) (uintptr, error) {
	search, err := keychainSearchArray(keychainPath)
	if err != nil {
		return 0, err
	}
	defer cfRelease(search)

	labelData := cfBytes(appLabel)
	defer cfRelease(labelData)

	q := cfDictCreateMutable(0, 0, cbDictKey, cbDictValue)
	if q == 0 {
		return 0, fmt.Errorf("keysigner: CFDictionaryCreateMutable(query) failed")
	}
	defer cfRelease(q)
	cfDictSetValue(q, kSecClass, kSecClassKey)
	cfDictSetValue(q, kSecAttrKeyClass, keyClass)
	cfDictSetValue(q, kSecAttrKeyType, keyType)
	cfDictSetValue(q, kSecAttrApplicationLabel, labelData)
	cfDictSetValue(q, kSecReturnRef, kCFBooleanTrue)
	cfDictSetValue(q, kSecMatchSearchList, search)

	var keyRef uintptr
	if st := secItemCopyMatching(q, &keyRef); st != errSecSuccess {
		if st == -25300 {
			return 0, fmt.Errorf("%w: application label %s", ErrKeyNotFound, hex.EncodeToString(appLabel))
		}
		return 0, keychainError("find key", int(st))
	}
	return keyRef, nil
}

// These seams keep lifecycle and interaction tests hermetic. Production calls
// Security.framework directly, so the generated keychain password never
// appears in process argv.
var (
	createKeychainFile                = createKeychainFileFFI
	unlockKeychainFile                = unlockKeychainFileFFI
	getKeychainUserInteractionAllowed = getKeychainUserInteractionAllowedFFI
	setKeychainUserInteractionAllowed = setKeychainUserInteractionAllowedFFI
	keychainUserInteractionOperation  = func() chan struct{} {
		lock := make(chan struct{}, 1)
		lock <- struct{}{}
		return lock
	}()
)

func getKeychainUserInteractionAllowedFFI() (bool, error) {
	var allowed uint8
	if status := secKeychainGetUI(&allowed); status != errSecSuccess {
		return false, keychainError("read Keychain user-interaction state", int(status))
	}
	return allowed != 0, nil
}

func setKeychainUserInteractionAllowedFFI(allowed bool) error {
	var state uint8
	if allowed {
		state = 1
	}
	if status := secKeychainSetUI(state); status != errSecSuccess {
		return keychainError("set Keychain user-interaction state", int(status))
	}
	return nil
}

// withKeychainUserInteractionDisabled makes the no-system-dialog requirement
// an execution invariant. If an operation would otherwise need UI, Keychain
// Services returns an error instead. The process-global setting is serialized
// and restored even when the operation fails.
func withKeychainUserInteractionDisabled(ctx context.Context, operation func() error) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-keychainUserInteractionOperation:
	}
	defer func() { keychainUserInteractionOperation <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return err
	}

	previous, err := getKeychainUserInteractionAllowed()
	if err != nil {
		return err
	}
	if err := setKeychainUserInteractionAllowed(false); err != nil {
		return err
	}
	defer func() {
		if restoreErr := setKeychainUserInteractionAllowed(previous); restoreErr != nil {
			if err == nil {
				err = restoreErr
			} else {
				err = fmt.Errorf("%w; additionally failed to restore Keychain user-interaction state: %w", err, restoreErr)
			}
		}
	}()
	return operation()
}

// keychainSigner implements Signer using a macOS non-exportable Keychain key.
type keychainSigner struct{}

// Native support requires Keychain key creation, metadata validation, and
// signing; implementing a software algorithm alone does not enable it here.
type keychainAlgorithm interface {
	name() string
	keychainKeyType() uintptr
	keychainKeyParameters() (uint32, uint32)
	parseKeychainPublicKey([]byte) (crypto.PublicKey, error)
	keychainMetadataPublicKey(*keyMetadata) (crypto.PublicKey, error)
	signKeychain(uintptr, []byte) ([]byte, error)
}

const cssmAlgIDECDSA = 73

var _ keychainAlgorithm = es256Algorithm{}

func (es256Algorithm) keychainKeyType() uintptr { return kSecAttrKeyTypeEC }

func (es256Algorithm) keychainKeyParameters() (uint32, uint32) { return cssmAlgIDECDSA, 256 }

func (es256Algorithm) parseKeychainPublicKey(data []byte) (crypto.PublicKey, error) {
	x, y := elliptic.Unmarshal(elliptic.P256(), data)
	if x == nil || y == nil {
		return nil, errors.New("keysigner: invalid P-256 ANSI X9.63 public key")
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
}

func (es256Algorithm) keychainMetadataPublicKey(md *keyMetadata) (crypto.PublicKey, error) {
	publicKey, err := decodePublicKey(md.PublicKey)
	if err != nil {
		return nil, err
	}
	p256, err := P256PublicKey(publicKey)
	if err != nil {
		return nil, err
	}
	appLabel, err := metadataAppLabel(md)
	if err != nil {
		return nil, err
	}
	expected := sha1.Sum(elliptic.Marshal(elliptic.P256(), p256.X, p256.Y))
	if !bytes.Equal(appLabel, expected[:]) {
		return nil, errors.New("keysigner: public key does not match its keychain application label")
	}
	return p256, nil
}

func (a es256Algorithm) signKeychain(keyRef uintptr, signingInput []byte) ([]byte, error) {
	der, err := signKeychainDigest(keyRef, algECDSASHA256, a.digest(signingInput))
	if err != nil {
		return nil, err
	}
	return a.signatureToJOSE(der)
}

const cssmAlgIDRSA = 42

var _ keychainAlgorithm = rs256Algorithm{}

func (rs256Algorithm) keychainKeyType() uintptr { return kSecAttrKeyTypeRSA }

func (rs256Algorithm) keychainKeyParameters() (uint32, uint32) { return cssmAlgIDRSA, 2048 }

func (a rs256Algorithm) parseKeychainPublicKey(data []byte) (crypto.PublicKey, error) {
	public, err := x509.ParsePKCS1PublicKey(data)
	if err != nil {
		return nil, err
	}
	if err := a.validatePublicKey(public); err != nil {
		return nil, err
	}
	return public, nil
}

func (a rs256Algorithm) keychainMetadataPublicKey(md *keyMetadata) (crypto.PublicKey, error) {
	public, err := decodePublicKey(md.PublicKey)
	if err != nil {
		return nil, err
	}
	if err := a.validatePublicKey(public); err != nil {
		return nil, err
	}
	appLabel, err := metadataAppLabel(md)
	if err != nil {
		return nil, err
	}
	// Application labels hash native PKCS#1 bytes; persisted public keys use PKIX.
	expected := sha1.Sum(x509.MarshalPKCS1PublicKey(public.(*rsa.PublicKey)))
	if !bytes.Equal(appLabel, expected[:]) {
		return nil, errors.New("keysigner: public key does not match its keychain application label")
	}
	return public, nil
}

func (rs256Algorithm) signKeychain(keyRef uintptr, signingInput []byte) ([]byte, error) {
	digest := sha256.Sum256(signingInput)
	return signKeychainDigest(keyRef, algRSASHA256, digest[:])
}

func keychainAlgorithmForRef(ctx context.Context, ref KeyRef) (keychainAlgorithm, error) {
	algorithm, err := algorithmForRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	native, ok := algorithm.(keychainAlgorithm)
	if !ok {
		return nil, fmt.Errorf("keysigner: Keychain: %w: %q", ErrUnsupportedAlgorithm, ref.Algorithm)
	}
	return native, nil
}

// NewSigner constructs only the named native backend, or returns nil if this
// platform does not support it. macOS does not use the directory resolver.
func NewSigner(name string, _ func(string) (string, error)) Signer {
	switch name {
	case MacOSSecureEnclaveSignerName:
		return secureEnclaveSigner{}
	case MacOSKeychainSignerName:
		return keychainSigner{}
	default:
		return nil
	}
}

func (keychainSigner) Name() string { return MacOSKeychainSignerName }

func (keychainSigner) SecurityLevel() SecurityLevel { return SecurityLevelL2 }

func (keychainSigner) EnsureKey(ctx context.Context, ref KeyRef) (crypto.PublicKey, error) {
	algorithm, err := keychainAlgorithmForRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	if md, err := readKeyMetadata(ref.Label); err == nil {
		publicKey, err := algorithm.keychainMetadataPublicKey(md)
		if err != nil {
			return nil, err
		}
		if err := privateKeyAvailable(ctx, md, algorithm); err == nil {
			return publicKey, nil
		} else if !errors.Is(err, ErrKeyNotFound) {
			return nil, err
		}
		// EnsureKey is only used while creating a new binding. A stale metadata
		// file may therefore be removed and replaced. PublicKey and Sign never
		// take this path, so an existing token can never be rebound silently.
		metadataPath, pathErr := keyMetadataPath(ref.Label)
		if pathErr != nil {
			return nil, pathErr
		}
		if removeErr := vfs.Remove(metadataPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return nil, fmt.Errorf("keysigner: remove stale key metadata: %w", removeErr)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return createKeychainKey(ctx, ref.Label, algorithm)
}

func (keychainSigner) PublicKey(ctx context.Context, ref KeyRef) (crypto.PublicKey, error) {
	algorithm, err := keychainAlgorithmForRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	md, err := readKeyMetadata(ref.Label)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", ErrKeyNotFound, ref.Label)
	}
	if err != nil {
		return nil, err
	}
	publicKey, err := algorithm.keychainMetadataPublicKey(md)
	if err != nil {
		return nil, err
	}
	if err := privateKeyAvailable(ctx, md, algorithm); err != nil {
		return nil, err
	}
	return publicKey, nil
}

func (keychainSigner) Sign(ctx context.Context, ref KeyRef, signingInput []byte) (signature []byte, algorithm string, err error) {
	selected, err := keychainAlgorithmForRef(ctx, ref)
	if err != nil {
		return nil, "", err
	}
	if err := requireFFI(); err != nil {
		return nil, "", err
	}
	err = withKeychainUserInteractionDisabled(ctx, func() error {
		signature, algorithm, err = signWithKeychain(ctx, ref, signingInput, selected)
		return err
	})
	return signature, algorithm, err
}

func (keychainSigner) DeleteKey(ctx context.Context, ref KeyRef) error {
	algorithm, err := keychainAlgorithmForRef(ctx, ref)
	if err != nil {
		return err
	}
	md, err := readKeyMetadata(ref.Label)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := algorithm.keychainMetadataPublicKey(md); err != nil {
		return err
	}
	if err := requireFFI(); err != nil {
		return err
	}
	metadataPath, err := keyMetadataPath(ref.Label)
	if err != nil {
		return err
	}
	keychainPath, err := keychainFilePath()
	if err != nil {
		return err
	}
	if _, err := vfs.Stat(keychainPath); os.IsNotExist(err) {
		return removeKeyMetadata(metadataPath)
	} else if err != nil {
		return fmt.Errorf("keysigner: stat keychain before key deletion: %w", err)
	}
	appLabel, err := metadataAppLabel(md)
	if err != nil {
		return err
	}
	if err := withKeychainUserInteractionDisabled(ctx, func() error {
		keychain, err := ensureKeychain(ctx)
		if err != nil {
			return err
		}
		for _, keyClass := range []uintptr{kSecAttrKeyClassPrivate, kSecAttrKeyClassPublic} {
			keyRef, err := findKey(appLabel, keychain, keyClass, algorithm.keychainKeyType())
			if errors.Is(err, ErrKeyNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			status := secKeychainItemDelete(keyRef)
			cfRelease(keyRef)
			if status != errSecSuccess && status != -25300 {
				return keychainError("delete keychain key", int(status))
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return removeKeyMetadata(metadataPath)
}

func signWithKeychain(ctx context.Context, ref KeyRef, signingInput []byte, algorithm keychainAlgorithm) ([]byte, string, error) {
	md, err := readKeyMetadata(ref.Label)
	if os.IsNotExist(err) {
		return nil, "", fmt.Errorf("%w: %s", ErrKeyNotFound, ref.Label)
	}
	if err != nil {
		return nil, "", err
	}
	if _, err := algorithm.keychainMetadataPublicKey(md); err != nil {
		return nil, "", err
	}
	appLabel, err := metadataAppLabel(md)
	if err != nil {
		return nil, "", err
	}
	keychain, err := existingKeychain(ctx)
	if err != nil {
		return nil, "", err
	}

	keyRef, err := findKey(appLabel, keychain, kSecAttrKeyClassPrivate, algorithm.keychainKeyType())
	if err != nil {
		return nil, "", err
	}
	defer cfRelease(keyRef)
	signature, err := algorithm.signKeychain(keyRef, signingInput)
	if err != nil {
		return nil, "", err
	}
	return signature, algorithm.name(), nil
}

// keyMetadata records the public key + the keychain application-label used to
// locate the non-extractable private key.
type keyMetadata struct {
	PublicKey string `json:"public_key"` // PKIX DER, standard base64
	AppLabel  string `json:"app_label"`  // hex(sha1(native public key bytes))
}

func requireFFI() error {
	if err := loadFFI(); err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return nil
}

func metadataAppLabel(md *keyMetadata) ([]byte, error) {
	appLabel, err := hex.DecodeString(md.AppLabel)
	if err != nil {
		return nil, fmt.Errorf("keysigner: decode app label: %w", err)
	}
	if len(appLabel) != sha1.Size {
		return nil, fmt.Errorf("keysigner: invalid app label length %d", len(appLabel))
	}
	return appLabel, nil
}

func privateKeyAvailable(ctx context.Context, md *keyMetadata, algorithm keychainAlgorithm) error {
	if err := requireFFI(); err != nil {
		return err
	}
	appLabel, err := metadataAppLabel(md)
	if err != nil {
		return err
	}
	return withKeychainUserInteractionDisabled(ctx, func() error {
		keychain, err := existingKeychain(ctx)
		if err != nil {
			return err
		}
		keyRef, err := findKey(appLabel, keychain, kSecAttrKeyClassPrivate, algorithm.keychainKeyType())
		if err != nil {
			return err
		}
		cfRelease(keyRef)
		return nil
	})
}

// configureNoPromptSigningAccess preserves the existing `security import -A`
// UX contract without creating a software private-key copy. SecAccessCreate
// groups restricted operations in one ACL; making its application list nil
// allows any application to use it without confirmation. The private key is
// nevertheless sign-only and non-extractable, so its effective capability is
// limited to signing.
func configureNoPromptSigningAccess(access, descriptor uintptr) error {
	aclList := secAccessCopyACLs(access, kSecACLAuthorizationSign)
	if aclList == 0 {
		return fmt.Errorf("keysigner: find signing access policy failed")
	}
	defer cfRelease(aclList)

	count := cfArrayGetCount(aclList)
	if count == 0 {
		return fmt.Errorf("keysigner: signing access policy is empty")
	}
	for i := 0; i < count; i++ {
		acl := cfArrayGetValue(aclList, i)
		if acl == 0 {
			return fmt.Errorf("keysigner: signing access policy contains an empty ACL")
		}
		// applicationList=nil means any application may use this ACL without a
		// confirmation dialog. promptSelector=0 never requests a passphrase.
		if status := secACLSetContents(acl, 0, descriptor, 0); status != errSecSuccess {
			return keychainError("configure no-prompt signing access", int(status))
		}
	}
	return nil
}

func setKeychainKeyLabel(appLabel []byte, keychain, label string, keyType uintptr) error {
	if err := loadFFI(); err != nil {
		return err
	}
	search, err := keychainSearchArray(keychain)
	if err != nil {
		return err
	}
	defer cfRelease(search)

	labelData := cfBytes(appLabel)
	defer cfRelease(labelData)

	q := cfDictCreateMutable(0, 0, cbDictKey, cbDictValue)
	if q == 0 {
		return fmt.Errorf("keysigner: CFDictionaryCreateMutable(query) failed")
	}
	defer cfRelease(q)
	cfDictSetValue(q, kSecClass, kSecClassKey)
	cfDictSetValue(q, kSecAttrKeyClass, kSecAttrKeyClassPrivate)
	cfDictSetValue(q, kSecAttrKeyType, keyType)
	cfDictSetValue(q, kSecAttrApplicationLabel, labelData)
	cfDictSetValue(q, kSecMatchSearchList, search)

	cfLabel := cfStringCreate(0, cstr(label), cfStringEncodingUTF8)
	if cfLabel == 0 {
		return fmt.Errorf("keysigner: CFStringCreateWithCString failed")
	}
	defer cfRelease(cfLabel)
	attrs := cfDictCreateMutable(0, 0, cbDictKey, cbDictValue)
	if attrs == 0 {
		return fmt.Errorf("keysigner: CFDictionaryCreateMutable(attrs) failed")
	}
	defer cfRelease(attrs)
	cfDictSetValue(attrs, kSecAttrLabel, cfLabel)

	if st := secItemUpdate(q, attrs); st != errSecSuccess {
		return keychainError("set keychain key label", int(st))
	}
	return nil
}

func decodePublicKey(encoded string) (crypto.PublicKey, error) {
	der, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("keysigner: decode public key: %w", err)
	}
	return x509.ParsePKIXPublicKey(der)
}

func readKeyMetadata(label string) (*keyMetadata, error) {
	path, err := keyMetadataPath(label)
	if err != nil {
		return nil, err
	}
	data, err := vfs.ReadFile(path)
	if err != nil {
		return nil, err // preserves os.ErrNotExist for EnsureKey
	}
	var md keyMetadata
	if err := json.Unmarshal(data, &md); err != nil {
		return nil, fmt.Errorf("keysigner: parse key metadata: %w", err)
	}
	return &md, nil
}

func writeKeyMetadata(path string, md keyMetadata) error {
	if err := vfs.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return err
	}
	return validate.AtomicWrite(path, data, 0600)
}

func removeKeyMetadata(path string) error {
	if err := vfs.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("keysigner: remove key metadata: %w", err)
	}
	return nil
}

func existingKeychain(ctx context.Context) (string, error) {
	path, err := keychainFilePath()
	if err != nil {
		return "", err
	}
	if _, err := vfs.Stat(path); os.IsNotExist(err) {
		return "", fmt.Errorf("%w: dedicated macOS keychain is missing", ErrKeyNotFound)
	} else if err != nil {
		return "", fmt.Errorf("keysigner: stat dedicated keychain: %w", err)
	}
	return ensureKeychain(ctx)
}

func ensureKeychain(ctx context.Context) (string, error) {
	if ctx != nil && ctx.Err() != nil {
		return "", ctx.Err()
	}
	keychainPath, err := keychainFilePath()
	if err != nil {
		return "", err
	}
	password, err := keychainPassword()
	if err != nil {
		return "", err
	}
	defer clear(password)
	if _, err := vfs.Stat(keychainPath); err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("keysigner: stat keychain: %w", err)
		}
		if err := createKeychainFile(keychainPath, password); err != nil {
			return "", err
		}
	}

	// A dedicated file keychain can be locked after logout, reboot, an idle
	// interval, or an explicit lock. Always unlock it with the generated password
	// before use; interaction is disabled by the caller, so failure returns an
	// error instead of displaying a system dialog.
	if err := unlockKeychainFile(keychainPath, password); err != nil {
		return "", err
	}
	return keychainPath, nil
}

func createKeychainFileFFI(path string, password []byte) error {
	pathBytes := append([]byte(path), 0)
	var keychain uintptr
	status := secKeychainCreate(
		&pathBytes[0],
		uint32(len(password)),
		byteSlicePointer(password),
		0, // promptUser=false: unattended creation must never display UI.
		0, // Apple documents passing NULL for initialAccess.
		&keychain,
	)
	runtime.KeepAlive(pathBytes)
	runtime.KeepAlive(password)
	if status != errSecSuccess {
		return keychainError("create keychain", int(status))
	}
	if keychain == 0 {
		return fmt.Errorf("keysigner: create keychain returned an empty reference")
	}
	cfRelease(keychain)
	return nil
}

func unlockKeychainFileFFI(path string, password []byte) error {
	pathBytes := append([]byte(path), 0)
	var keychain uintptr
	status := secKeychainOpen(&pathBytes[0], &keychain)
	runtime.KeepAlive(pathBytes)
	if status != errSecSuccess {
		return keychainError("open keychain for unlock", int(status))
	}
	if keychain == 0 {
		return fmt.Errorf("keysigner: open keychain for unlock returned an empty reference")
	}
	defer cfRelease(keychain)

	status = secKeychainUnlock(keychain, uint32(len(password)), byteSlicePointer(password), 1)
	runtime.KeepAlive(password)
	if status != errSecSuccess {
		return keychainError("unlock keychain", int(status))
	}
	return nil
}

func byteSlicePointer(data []byte) unsafe.Pointer {
	if len(data) == 0 {
		return nil
	}
	return unsafe.Pointer(&data[0])
}

func keysignerDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("keysigner: resolve config dir: %w", err)
	}
	return filepath.Join(configDir, "lark-cli", "keysigner"), nil
}

func keychainFilePath() (string, error) {
	dir, err := keysignerDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lark-cli.keychain"), nil
}

func keychainPassword() ([]byte, error) {
	dir, err := keysignerDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "keychain.pass")
	if data, err := vfs.ReadFile(path); err == nil {
		defer clear(data)
		if pw := bytes.TrimSpace(data); len(pw) != 0 {
			return append([]byte(nil), pw...), nil
		}
		return nil, fmt.Errorf("keysigner: empty keychain password")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	defer clear(buf)
	pw := make([]byte, hex.EncodedLen(len(buf)))
	hex.Encode(pw, buf)
	if err := vfs.MkdirAll(filepath.Dir(path), 0700); err != nil {
		clear(pw)
		return nil, err
	}
	stored := append(append([]byte(nil), pw...), '\n')
	if err := validate.AtomicWrite(path, stored, 0600); err != nil {
		clear(stored)
		clear(pw)
		return nil, err
	}
	clear(stored)
	return pw, nil
}

func keyMetadataPath(label string) (string, error) {
	dir, err := keysignerDir()
	if err != nil {
		return "", err
	}
	id := sha256.Sum256([]byte(label))
	return filepath.Join(dir, "keys", hex.EncodeToString(id[:])+".json"), nil
}

func keychainError(operation string, status int) error {
	switch status {
	case -25299:
		return fmt.Errorf("keysigner: %s: key already exists", operation)
	case -25300:
		return fmt.Errorf("keysigner: %s: key not found", operation)
	case -2:
		return fmt.Errorf("keysigner: %s: allocation failed", operation)
	default:
		return fmt.Errorf("keysigner: %s: Security framework status %d", operation, status)
	}
}

func signKeychainDigest(keyRef, algorithm uintptr, digest []byte) ([]byte, error) {
	digestData := cfBytes(digest)
	defer cfRelease(digestData)

	var errRef uintptr
	sigRef := secKeyCreateSignature(keyRef, algorithm, digestData, &errRef)
	if sigRef == 0 {
		code := 0
		if errRef != 0 {
			code = cfErrorGetCode(errRef)
			cfRelease(errRef)
		}
		return nil, fmt.Errorf("keysigner: SecKeyCreateSignature failed (CFError %d)", code)
	}
	defer cfRelease(sigRef)

	n := cfDataGetLength(sigRef)
	bp := cfDataGetBytePtr(sigRef)
	if n <= 0 || bp == nil {
		return nil, errors.New("keysigner: SecKeyCreateSignature returned an empty signature")
	}
	der := make([]byte, n)
	copy(der, unsafe.Slice((*byte)(bp), n))
	return der, nil
}

func createKeychainKey(ctx context.Context, label string, a keychainAlgorithm) (publicKey crypto.PublicKey, err error) {
	if err := requireFFI(); err != nil {
		return nil, err
	}
	err = withKeychainUserInteractionDisabled(ctx, func() error {
		publicKey, err = createKeychainKeyWithoutUI(ctx, label, a)
		return err
	})
	return publicKey, err
}

func createKeychainKeyWithoutUI(ctx context.Context, label string, a keychainAlgorithm) (crypto.PublicKey, error) {
	metadataPath, err := keyMetadataPath(label)
	if err != nil {
		return nil, err
	}
	keychain, err := ensureKeychain(ctx)
	if err != nil {
		return nil, err
	}

	var keychainRef uintptr
	if status := secKeychainOpen(cstr(keychain), &keychainRef); status != errSecSuccess {
		return nil, keychainError("open keychain for key generation", int(status))
	}
	if keychainRef == 0 {
		return nil, fmt.Errorf("keysigner: open keychain for key generation returned an empty reference")
	}
	defer cfRelease(keychainRef)

	descriptor := cfStringCreate(0, cstr(label), cfStringEncodingUTF8)
	if descriptor == 0 {
		return nil, fmt.Errorf("keysigner: create key access descriptor failed")
	}
	defer cfRelease(descriptor)

	var access uintptr
	if status := secAccessCreate(descriptor, 0, &access); status != errSecSuccess {
		return nil, keychainError("create key access policy", int(status))
	}
	if access == 0 {
		return nil, fmt.Errorf("keysigner: create key access policy returned an empty reference")
	}
	defer cfRelease(access)
	if err := configureNoPromptSigningAccess(access, descriptor); err != nil {
		return nil, err
	}

	algorithmID, keyBits := a.keychainKeyParameters()
	var publicKeyRef, privateKeyRef uintptr
	status := secKeyCreatePair(
		keychainRef,
		algorithmID,
		keyBits,
		0,
		cssmKeyUseVerify,
		publicKeyAttributes,
		cssmKeyUseSign,
		privateKeyAttributes,
		access,
		&publicKeyRef,
		&privateKeyRef,
	)
	deleteAndRelease := func(keyRef uintptr) {
		if keyRef != 0 {
			_ = secKeychainItemDelete(keyRef)
			cfRelease(keyRef)
		}
	}
	if status != errSecSuccess {
		deleteAndRelease(privateKeyRef)
		deleteAndRelease(publicKeyRef)
		return nil, keychainError("generate non-extractable "+a.name()+" key", int(status))
	}
	if publicKeyRef == 0 || privateKeyRef == 0 {
		deleteAndRelease(privateKeyRef)
		deleteAndRelease(publicKeyRef)
		return nil, fmt.Errorf("keysigner: key generation returned an empty key reference")
	}
	defer cfRelease(publicKeyRef)
	defer cfRelease(privateKeyRef)

	committed := false
	defer func() {
		if !committed {
			_ = secKeychainItemDelete(privateKeyRef)
			_ = secKeychainItemDelete(publicKeyRef)
		}
	}()

	var exportErr uintptr
	publicDERRef := secKeyCopyExternal(publicKeyRef, &exportErr)
	if publicDERRef == 0 {
		code := 0
		if exportErr != 0 {
			code = cfErrorGetCode(exportErr)
			cfRelease(exportErr)
		}
		return nil, fmt.Errorf("keysigner: export public key failed (CFError %d)", code)
	}
	defer cfRelease(publicDERRef)
	publicDERLength := cfDataGetLength(publicDERRef)
	publicDERPointer := cfDataGetBytePtr(publicDERRef)
	if publicDERLength <= 0 || publicDERPointer == nil {
		return nil, fmt.Errorf("keysigner: exported public key is empty")
	}
	publicDER := make([]byte, publicDERLength)
	copy(publicDER, unsafe.Slice((*byte)(publicDERPointer), publicDERLength))
	publicKey, err := a.parseKeychainPublicKey(publicDER)
	if err != nil {
		return nil, err
	}
	appLabel := sha1.Sum(publicDER)

	if err := setKeychainKeyLabel(appLabel[:], keychain, label, a.keychainKeyType()); err != nil {
		return nil, err
	}

	encodedPub, err := EncodePublicKey(publicKey)
	if err != nil {
		return nil, err
	}
	if err := writeKeyMetadata(metadataPath, keyMetadata{PublicKey: encodedPub, AppLabel: hex.EncodeToString(appLabel[:])}); err != nil {
		return nil, err
	}
	committed = true
	return publicKey, nil
}
