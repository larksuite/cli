//go:build darwin

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// macOS non-exportable Keychain signer (compiled into every darwin build).
//
// It does NOT use the Secure Enclave / hardware TEE (which would require
// code-signing entitlements that are unfriendly to open source). Instead it
// generates an RSA-2048 key in software, imports it into a dedicated app
// keychain as NON-EXTRACTABLE (`security import -x`), then deletes the software
// copy — so the private key can sign but can never be exported. Signing is
// RSASSA-PKCS1v15-SHA256 (RS256).
//
// Unlike the original revision, this implementation calls the Security and
// CoreFoundation frameworks via RUNTIME FFI (github.com/ebitengine/purego)
// instead of cgo. The security model is identical (the private key is still a
// non-extractable keychain key and every signature is produced by the OS via
// SecKeyCreateSignature), but the binary builds with CGO_ENABLED=0 and can be
// cross-compiled for darwin from any host — so release binaries no longer
// require a native macOS build runner.
//
// Build with:  go build   (cgo-free; compiled into every darwin build, no tag)
package keysigner

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	secKeychainOpen       func(path *byte, out *uintptr) int32
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
		purego.RegisterLibFunc(&secKeychainOpen, sec, "SecKeychainOpen")
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
			sym, e := purego.Dlsym(d.handle, d.name)
			if e != nil || sym == 0 {
				ffiErr = fmt.Errorf("keysigner: dlsym %s: %v", d.name, e)
				return
			}
			// deref of a stable dylib data-symbol address (not Go-managed memory), so safe.
			*d.dst = *(*uintptr)(unsafe.Pointer(sym)) //nolint:govet // unsafeptr: see comment above
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
			if e != nil || sym == 0 {
				ffiErr = fmt.Errorf("keysigner: dlsym %s: %v", a.name, e)
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

// securityBin is invoked by absolute path so a poisoned PATH cannot hijack it.
const securityBin = "/usr/bin/security"

// keychainSigner implements Signer using a macOS non-exportable Keychain key.
type keychainSigner struct{}

func init() { Register(keychainSigner{}) }

// ProbeHardware reports the macOS Keychain backend backing this signer. The
// keychain signer is compiled into every darwin build and needs no special
// hardware, so it reports available whenever the Security tooling is present.
// It performs no key access, so it never prompts. Implementing HardwareProber
// is what lets `doctor` report the signer as present rather than treating the
// (prober-less) signer as "no TEE signer in this build".
func (keychainSigner) ProbeHardware(_ context.Context) (HardwareInfo, error) {
	info := HardwareInfo{Backend: "keychain", VendorName: "macOS Keychain"}
	// A missing security tool is a status (Available=false via Reason), not a
	// probe error — so we deliberately return a nil error here.
	if _, err := vfs.Stat(securityBin); err != nil {
		info.Reason = securityBin + " not found"
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

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("keysigner: generate RSA key: %w", err)
	}
	appLabel := sha1.Sum(x509.MarshalPKCS1PublicKey(&privateKey.PublicKey))

	pemFile, err := vfs.CreateTemp("", "lark-keysigner-*.pem")
	if err != nil {
		return nil, fmt.Errorf("keysigner: temp key file: %w", err)
	}
	pemPath := pemFile.Name()
	defer vfs.Remove(pemPath)
	if err := pemFile.Chmod(0600); err != nil {
		pemFile.Close()
		return nil, err
	}
	der := x509.MarshalPKCS1PrivateKey(privateKey)
	if _, err := pemFile.WriteString("-----BEGIN RSA PRIVATE KEY-----\n" +
		base64Wrap(der) + "-----END RSA PRIVATE KEY-----\n"); err != nil {
		pemFile.Close()
		return nil, err
	}
	if err := pemFile.Close(); err != nil {
		return nil, err
	}

	executable, err := vfs.Executable()
	if err != nil {
		return nil, fmt.Errorf("keysigner: resolve executable: %w", err)
	}
	keychain, err := ensureKeychain()
	if err != nil {
		return nil, err
	}
	// -x: import as NON-EXTRACTABLE; the software copy (pemPath) is then removed.
	importCmd := exec.Command(securityBin, "import", pemPath, "-k", keychain, "-t", "priv", "-f", "openssl", "-x", "-A", "-T", executable)
	if out, err := importCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("keysigner: import non-extractable key: %w: %s", err, summarizeCmdOutput(out))
	}
	if err := setKeychainKeyLabel(appLabel[:], keychain, label); err != nil {
		return nil, err
	}

	encodedPub, err := EncodePublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, err
	}
	if err := writeKeyMetadata(metadataPath, keyMetadata{PublicKey: encodedPub, AppLabel: hex.EncodeToString(appLabel[:])}); err != nil {
		return nil, err
	}
	return &privateKey.PublicKey, nil
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

// base64Wrap PEM-wraps DER bytes at 64 columns.
func base64Wrap(der []byte) string {
	enc := base64.StdEncoding.EncodeToString(der)
	var b strings.Builder
	for i := 0; i < len(enc); i += 64 {
		end := i + 64
		if end > len(enc) {
			end = len(enc)
		}
		b.WriteString(enc[i:end])
		b.WriteByte('\n')
	}
	return b.String()
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
	if _, err := vfs.Stat(keychainPath); err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("keysigner: stat keychain: %w", err)
		}
		if err := vfs.MkdirAll(filepath.Dir(keychainPath), 0700); err != nil {
			return "", err
		}
		for _, args := range [][]string{
			{"create-keychain", "-p", password, keychainPath},
			{"set-keychain-settings", keychainPath},
			{"unlock-keychain", "-p", password, keychainPath},
		} {
			if out, err := exec.Command(securityBin, args...).CombinedOutput(); err != nil {
				return "", fmt.Errorf("keysigner: security %s: %w: %s", args[0], err, summarizeCmdOutput(out))
			}
		}
	}
	return keychainPath, nil
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

func keychainPassword() (string, error) {
	dir, err := keysignerDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "keychain.pass")
	if data, err := vfs.ReadFile(path); err == nil {
		if pw := strings.TrimSpace(string(data)); pw != "" {
			return pw, nil
		}
		return "", fmt.Errorf("keysigner: empty keychain password")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	pw := hex.EncodeToString(buf)
	if err := vfs.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	if err := vfs.WriteFile(path, []byte(pw+"\n"), 0600); err != nil {
		return "", err
	}
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

// summarizeCmdOutput bounds external command output before it is embedded in
// an error: first line only, capped at 200 chars.
func summarizeCmdOutput(out []byte) string {
	s := strings.TrimSpace(string(out))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	const maxLen = 200
	if len(s) > maxLen {
		s = s[:maxLen] + "..."
	}
	return s
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
