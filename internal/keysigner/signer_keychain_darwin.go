//go:build darwin

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// macOS non-exportable Keychain signer (compiled into every darwin build).
//
// It does NOT use the Secure Enclave / hardware TEE (which would require
// code-signing entitlements that are unfriendly to open source). Instead it
// generates an RSA-2048 key directly inside a dedicated app keychain. The
// private key is permanent, sensitive, and non-extractable; it is never present
// in Go memory or a temporary file. Its access list trusts only the creating
// application by default. Signing is RSASSA-PKCS1v15-SHA256 (RS256).
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
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
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

	// Legacy Security.framework key-generation values from cssmtype.h. The
	// legacy API is used because it can target a dedicated file keychain and set
	// both a non-extractable attribute and an application-only SecAccess ACL.
	cssmAlgIDRSA           = 42
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

	cfDataCreate          func(alloc uintptr, bytes *byte, length int) uintptr
	cfDataGetLength       func(d uintptr) int
	cfDataGetBytePtr      func(d uintptr) unsafe.Pointer
	cfStringCreate        func(alloc uintptr, cstr *byte, encoding uint32) uintptr
	cfArrayCreate         func(alloc uintptr, values *uintptr, numValues int, cb uintptr) uintptr
	cfDictCreateMutable   func(alloc uintptr, capacity int, keyCB, valCB uintptr) uintptr
	cfDictSetValue        func(dict, key, val uintptr)
	cfRelease             func(ref uintptr)
	cfErrorGetCode        func(e uintptr) int
	dlsymDataPointer      func(handle uintptr, name string) *uintptr
	secKeychainCreate     func(path *byte, passwordLength uint32, password unsafe.Pointer, promptUser uint8, initialAccess uintptr, out *uintptr) int32
	secKeychainOpen       func(path *byte, out *uintptr) int32
	secKeychainUnlock     func(keychain uintptr, passwordLength uint32, password unsafe.Pointer, usePassword uint8) int32
	secAccessCreate       func(descriptor, trustedList uintptr, out *uintptr) int32
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
	kSecAttrKeyType          uintptr
	kSecAttrKeyTypeRSA       uintptr
	kSecAttrApplicationLabel uintptr
	kSecReturnRef            uintptr
	kSecMatchSearchList      uintptr
	kSecAttrLabel            uintptr
	kCFBooleanTrue           uintptr
	algRSAPKCS1SHA256        uintptr

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
		// RegisterLibFunc panics when a symbol is unavailable. Convert that into
		// the same stable availability error as dlopen/dlsym failures so doctor
		// and auth commands never crash on a future macOS without a legacy symbol.
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
		purego.RegisterLibFunc(&cfDataGetLength, cf, "CFDataGetLength")
		purego.RegisterLibFunc(&cfDataGetBytePtr, cf, "CFDataGetBytePtr")
		purego.RegisterLibFunc(&cfStringCreate, cf, "CFStringCreateWithCString")
		purego.RegisterLibFunc(&cfArrayCreate, cf, "CFArrayCreate")
		purego.RegisterLibFunc(&cfDictCreateMutable, cf, "CFDictionaryCreateMutable")
		purego.RegisterLibFunc(&cfDictSetValue, cf, "CFDictionarySetValue")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
		purego.RegisterLibFunc(&cfErrorGetCode, cf, "CFErrorGetCode")
		// purego.Dlsym exposes symbol addresses as uintptr. Bind dlsym with a
		// pointer return for data symbols so reading exported CFTypeRef variables
		// never performs a uintptr-to-pointer conversion in Go.
		purego.RegisterLibFunc(&dlsymDataPointer, purego.RTLD_DEFAULT, "dlsym")
		purego.RegisterLibFunc(&secKeychainCreate, sec, "SecKeychainCreate")
		purego.RegisterLibFunc(&secKeychainOpen, sec, "SecKeychainOpen")
		purego.RegisterLibFunc(&secKeychainUnlock, sec, "SecKeychainUnlock")
		purego.RegisterLibFunc(&secAccessCreate, sec, "SecAccessCreate")
		purego.RegisterLibFunc(&secKeyCreatePair, sec, "SecKeyCreatePair")
		purego.RegisterLibFunc(&secKeyCopyExternal, sec, "SecKeyCopyExternalRepresentation")
		purego.RegisterLibFunc(&secKeychainItemDelete, sec, "SecKeychainItemDelete")
		purego.RegisterLibFunc(&secItemCopyMatching, sec, "SecItemCopyMatching")
		purego.RegisterLibFunc(&secItemUpdate, sec, "SecItemUpdate")
		purego.RegisterLibFunc(&secKeyCreateSignature, sec, "SecKeyCreateSignature")

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
			{&kSecAttrKeyType, sec, "kSecAttrKeyType"},
			{&kSecAttrKeyTypeRSA, sec, "kSecAttrKeyTypeRSA"},
			{&kSecAttrApplicationLabel, sec, "kSecAttrApplicationLabel"},
			{&kSecReturnRef, sec, "kSecReturnRef"},
			{&kSecMatchSearchList, sec, "kSecMatchSearchList"},
			{&kSecAttrLabel, sec, "kSecAttrLabel"},
			{&kCFBooleanTrue, cf, "kCFBooleanTrue"},
			{&algRSAPKCS1SHA256, sec, "kSecKeyAlgorithmRSASignatureDigestPKCS1v15SHA256"},
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

// findPrivateKey locates the non-extractable private key by its application
// label within the dedicated keychain. Caller releases the returned SecKeyRef.
func findPrivateKey(appLabel []byte, keychainPath string) (uintptr, error) {
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
	cfDictSetValue(q, kSecAttrKeyClass, kSecAttrKeyClassPrivate)
	cfDictSetValue(q, kSecAttrKeyType, kSecAttrKeyTypeRSA)
	cfDictSetValue(q, kSecAttrApplicationLabel, labelData)
	cfDictSetValue(q, kSecReturnRef, kCFBooleanTrue)
	cfDictSetValue(q, kSecMatchSearchList, search)

	var keyRef uintptr
	if st := secItemCopyMatching(q, &keyRef); st != errSecSuccess {
		return 0, keychainError("find private key", int(st))
	}
	return keyRef, nil
}

// These seams keep lifecycle tests hermetic. Production calls Security.framework
// directly, so the generated keychain password never appears in process argv.
var (
	createKeychainFile = createKeychainFileFFI
	unlockKeychainFile = unlockKeychainFileFFI
)

// keychainSigner implements Signer using a macOS non-exportable Keychain key.
type keychainSigner struct{}

func init() { Register(keychainSigner{}) }

// ProbeHardware reports the macOS Keychain backend backing this signer. The
// keychain signer is compiled into every darwin build and needs no special
// hardware, so it reports available whenever its framework bindings load.
// It performs no key access, so it never prompts. Implementing HardwareProber
// is what lets `doctor` report the signer as present rather than treating the
// (prober-less) signer as "no platform signer in this build".
func (keychainSigner) ProbeHardware(_ context.Context) (HardwareInfo, error) {
	info := HardwareInfo{Backend: "keychain", VendorName: "macOS Keychain"}
	// A missing framework or symbol is a status (Available=false via Reason),
	// not a probe error. Loading symbols does not touch the keychain or prompt.
	if err := loadFFI(); err != nil {
		info.Reason = err.Error()
		return info, nil //nolint:nilerr // absence is reported via Reason, not as an error
	}
	info.Available = true
	return info, nil
}

func (keychainSigner) EnsureKey(_ context.Context, ref KeyRef) (crypto.PublicKey, error) {
	if md, err := readKeyMetadata(ref.Label); err == nil {
		return decodePublicKey(md.PublicKey)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return createKeychainKey(ref.Label)
}

func (keychainSigner) PublicKey(_ context.Context, ref KeyRef) (crypto.PublicKey, error) {
	md, err := readKeyMetadata(ref.Label)
	if err != nil {
		return nil, err
	}
	return decodePublicKey(md.PublicKey)
}

func (keychainSigner) Sign(_ context.Context, ref KeyRef, signingInput []byte) ([]byte, string, error) {
	if err := loadFFI(); err != nil {
		return nil, "", err
	}
	md, err := readKeyMetadata(ref.Label)
	if err != nil {
		return nil, "", err
	}
	appLabel, err := hex.DecodeString(md.AppLabel)
	if err != nil {
		return nil, "", fmt.Errorf("keysigner: decode app label: %w", err)
	}
	if len(appLabel) == 0 {
		// Guard the &appLabel[0] pointer below against corrupted metadata.
		return nil, "", fmt.Errorf("keysigner: key metadata for %q has empty app label", ref.Label)
	}
	keychain, err := ensureKeychain()
	if err != nil {
		return nil, "", err
	}

	keyRef, err := findPrivateKey(appLabel, keychain)
	if err != nil {
		return nil, "", err
	}
	defer cfRelease(keyRef)

	digest := sha256.Sum256(signingInput)
	digestData := cfBytes(digest[:])
	defer cfRelease(digestData)

	var errRef uintptr
	sigRef := secKeyCreateSignature(keyRef, algRSAPKCS1SHA256, digestData, &errRef)
	if sigRef == 0 {
		code := 0
		if errRef != 0 {
			code = cfErrorGetCode(errRef)
			cfRelease(errRef)
		}
		return nil, "", fmt.Errorf("keysigner: SecKeyCreateSignature failed (CFError %d)", code)
	}
	defer cfRelease(sigRef)

	n := cfDataGetLength(sigRef)
	bp := cfDataGetBytePtr(sigRef)
	out := make([]byte, n)
	copy(out, unsafe.Slice((*byte)(bp), n))
	// RS256: the SecKey PKCS1v15-SHA256 signature is the JOSE signature as-is.
	return out, AlgRS256, nil
}

// keyMetadata records the public key + the keychain application-label used to
// locate the non-extractable private key.
type keyMetadata struct {
	PublicKey string `json:"public_key"` // PKIX DER, std base64 (see EncodePublicKey)
	AppLabel  string `json:"app_label"`  // hex(sha1(PKCS1 public key))
}

func createKeychainKey(label string) (crypto.PublicKey, error) {
	metadataPath, err := keyMetadataPath(label)
	if err != nil {
		return nil, err
	}
	if err := loadFFI(); err != nil {
		return nil, err
	}
	keychain, err := ensureKeychain()
	if err != nil {
		return nil, err
	}

	var keychainRef uintptr
	if st := secKeychainOpen(cstr(keychain), &keychainRef); st != errSecSuccess {
		return nil, keychainError("open keychain for key generation", int(st))
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

	// A nil trusted list means only this application is trusted without a
	// confirmation dialog. This is intentionally stricter than security(1) -A.
	var access uintptr
	if st := secAccessCreate(descriptor, 0, &access); st != errSecSuccess {
		return nil, keychainError("create key access policy", int(st))
	}
	if access == 0 {
		return nil, fmt.Errorf("keysigner: create key access policy returned an empty reference")
	}
	defer cfRelease(access)

	var publicKeyRef, privateKeyRef uintptr
	status := secKeyCreatePair(
		keychainRef,
		cssmAlgIDRSA,
		2048,
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
		return nil, keychainError("generate non-extractable RSA key", int(status))
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
	publicKey, err := x509.ParsePKCS1PublicKey(publicDER)
	if err != nil {
		return nil, fmt.Errorf("keysigner: parse generated RSA public key: %w", err)
	}
	appLabel := sha1.Sum(publicDER)

	if err := setKeychainKeyLabel(appLabel[:], keychain, label); err != nil {
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

func setKeychainKeyLabel(appLabel []byte, keychain, label string) error {
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
	cfDictSetValue(q, kSecAttrKeyType, kSecAttrKeyTypeRSA)
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
	return vfs.WriteFile(path, data, 0600)
}

func ensureKeychain() (string, error) {
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
		if err := vfs.MkdirAll(filepath.Dir(keychainPath), 0700); err != nil {
			return "", err
		}
		if err := createKeychainFile(keychainPath, password); err != nil {
			return "", err
		}
	}

	// A file keychain can be locked after logout, reboot, an idle interval, or an
	// explicit `security lock-keychain`. SecKeychainOpen may still succeed while
	// it is locked; the failure then appears later at signing time (commonly as
	// SecKeyCreateSignature CFError -128). Always unlock the dedicated keychain
	// before returning it, including when the file already existed.
	if err := unlockKeychainFile(keychainPath, password); err != nil {
		return "", err
	}
	return keychainPath, nil
}

func createKeychainFileFFI(path string, password []byte) error {
	if err := loadFFI(); err != nil {
		return err
	}
	pathBytes := append([]byte(path), 0)
	var keychain uintptr
	status := secKeychainCreate(
		&pathBytes[0],
		uint32(len(password)),
		byteSlicePointer(password),
		0, // promptUser=false: unattended creation must never display UI.
		0, // initialAccess is ignored; Apple documents passing NULL.
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
	defer cfRelease(keychain)

	// Keep the system's default lock policy. ensureKeychain explicitly unlocks
	// this dedicated keychain with its generated password before every use, so
	// changing settings here would be unnecessary and could trigger system UI.
	return nil
}

func unlockKeychainFileFFI(path string, password []byte) error {
	if err := loadFFI(); err != nil {
		return err
	}
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
	if err := vfs.WriteFile(path, stored, 0600); err != nil {
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
