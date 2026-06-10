//go:build darwin && keychain_signer

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// macOS non-exportable Keychain signer (build tag `keychain_signer`).
// Ported from github.com/JackZhao10086/tee-test.
//
// It does NOT use the Secure Enclave / hardware TEE (which would require
// code-signing entitlements that are unfriendly to open source). Instead it
// generates an RSA-2048 key in software, imports it into a dedicated app
// keychain as NON-EXTRACTABLE (`security import -x`), then deletes the software
// copy — so the private key can sign but can never be exported. Signing is
// RSASSA-PKCS1v15-SHA256 (RS256).
//
// Build with:  go build -tags keychain_signer
// Without the tag, this file is excluded and the open-source build stays
// CGO-free with no registered signer (client_secret only).
package keysigner

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>

static CFDataRef cf_data(const void *bytes, int len) {
	return CFDataCreate(kCFAllocatorDefault, bytes, len);
}

static int add_keychain_search_list(CFMutableDictionaryRef query, const char *keychainPath) {
	SecKeychainRef keychain = NULL;
	OSStatus status = SecKeychainOpen(keychainPath, &keychain);
	if (status != errSecSuccess) {
		return (int)status;
	}

	const void *values[] = { keychain };
	CFArrayRef searchList = CFArrayCreate(kCFAllocatorDefault, values, 1, &kCFTypeArrayCallBacks);
	CFRelease(keychain);
	if (searchList == NULL) {
		return -2;
	}

	CFDictionarySetValue(query, kSecMatchSearchList, searchList);
	CFRelease(searchList);
	return 0;
}

static int find_private_key_by_app_label(const unsigned char *appLabel, int appLabelLen, const char *keychainPath, SecKeyRef *outKey) {
	CFDataRef cfAppLabel = cf_data(appLabel, appLabelLen);
	if (cfAppLabel == NULL) {
		return -2;
	}

	CFMutableDictionaryRef query = CFDictionaryCreateMutable(
		kCFAllocatorDefault,
		0,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	CFDictionarySetValue(query, kSecClass, kSecClassKey);
	CFDictionarySetValue(query, kSecAttrKeyClass, kSecAttrKeyClassPrivate);
	CFDictionarySetValue(query, kSecAttrKeyType, kSecAttrKeyTypeRSA);
	CFDictionarySetValue(query, kSecAttrApplicationLabel, cfAppLabel);
	CFDictionarySetValue(query, kSecReturnRef, kCFBooleanTrue);
	int searchResult = add_keychain_search_list(query, keychainPath);
	if (searchResult != 0) {
		CFRelease(query);
		CFRelease(cfAppLabel);
		return searchResult;
	}

	CFTypeRef item = NULL;
	OSStatus status = SecItemCopyMatching(query, &item);
	CFRelease(query);
	CFRelease(cfAppLabel);
	if (status != errSecSuccess) {
		return (int)status;
	}

	*outKey = (SecKeyRef)item;
	return 0;
}

static int set_private_key_label(const unsigned char *appLabel, int appLabelLen, const char *keychainPath, const char *label) {
	CFDataRef cfAppLabel = cf_data(appLabel, appLabelLen);
	CFStringRef cfLabel = CFStringCreateWithCString(kCFAllocatorDefault, label, kCFStringEncodingUTF8);
	if (cfAppLabel == NULL || cfLabel == NULL) {
		if (cfAppLabel != NULL) CFRelease(cfAppLabel);
		if (cfLabel != NULL) CFRelease(cfLabel);
		return -2;
	}

	CFMutableDictionaryRef query = CFDictionaryCreateMutable(
		kCFAllocatorDefault,
		0,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	CFDictionarySetValue(query, kSecClass, kSecClassKey);
	CFDictionarySetValue(query, kSecAttrKeyClass, kSecAttrKeyClassPrivate);
	CFDictionarySetValue(query, kSecAttrKeyType, kSecAttrKeyTypeRSA);
	CFDictionarySetValue(query, kSecAttrApplicationLabel, cfAppLabel);
	int searchResult = add_keychain_search_list(query, keychainPath);
	if (searchResult != 0) {
		CFRelease(query);
		CFRelease(cfAppLabel);
		CFRelease(cfLabel);
		return searchResult;
	}

	CFMutableDictionaryRef attrs = CFDictionaryCreateMutable(
		kCFAllocatorDefault,
		0,
		&kCFTypeDictionaryKeyCallBacks,
		&kCFTypeDictionaryValueCallBacks
	);
	CFDictionarySetValue(attrs, kSecAttrLabel, cfLabel);

	OSStatus status = SecItemUpdate(query, attrs);
	CFRelease(query);
	CFRelease(attrs);
	CFRelease(cfAppLabel);
	CFRelease(cfLabel);
	return (int)status;
}

static int sign_with_nonextractable_key(const unsigned char *appLabel, int appLabelLen, const char *keychainPath, const unsigned char *digest, int digestLen, unsigned char **sigOut, long *sigLen) {
	SecKeyRef privateKey = NULL;
	int result = find_private_key_by_app_label(appLabel, appLabelLen, keychainPath, &privateKey);
	if (result != 0) {
		return result;
	}

	CFDataRef digestData = CFDataCreate(kCFAllocatorDefault, digest, digestLen);
	if (digestData == NULL) {
		CFRelease(privateKey);
		return -2;
	}

	CFErrorRef error = NULL;
	CFDataRef signature = SecKeyCreateSignature(
		privateKey,
		kSecKeyAlgorithmRSASignatureDigestPKCS1v15SHA256,
		digestData,
		&error
	);
	CFRelease(digestData);
	if (signature == NULL) {
		CFRelease(privateKey);
		if (error != NULL) {
			int code = (int)CFErrorGetCode(error);
			CFRelease(error);
			return code;
		}
		return -1;
	}

	CFIndex len = CFDataGetLength(signature);
	unsigned char *sigBuf = malloc((size_t)len);
	if (sigBuf == NULL) {
		CFRelease(signature);
		CFRelease(privateKey);
		return -2;
	}
	memcpy(sigBuf, CFDataGetBytePtr(signature), (size_t)len);
	CFRelease(signature);
	*sigOut = sigBuf;
	*sigLen = len;

	CFRelease(privateKey);
	return 0;
}
*/
import "C"

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
	"strings"
	"unsafe"
)

// keychainSigner implements Signer using a macOS non-exportable Keychain key.
type keychainSigner struct{}

func init() { Register(keychainSigner{}) }

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
	md, err := readKeyMetadata(ref.Label)
	if err != nil {
		return nil, "", err
	}
	appLabel, err := hex.DecodeString(md.AppLabel)
	if err != nil {
		return nil, "", fmt.Errorf("keysigner: decode app label: %w", err)
	}
	if len(appLabel) == 0 {
		// Guard the &appLabel[0] CGO pointer below against corrupted metadata.
		return nil, "", fmt.Errorf("keysigner: key metadata for %q has empty app label", ref.Label)
	}
	keychain, err := ensureKeychain()
	if err != nil {
		return nil, "", err
	}
	cKeychain := C.CString(keychain)
	defer C.free(unsafe.Pointer(cKeychain))

	digest := sha256.Sum256(signingInput)
	var sig *C.uchar
	var sigLen C.long
	status := C.sign_with_nonextractable_key(
		(*C.uchar)(unsafe.Pointer(&appLabel[0])),
		C.int(len(appLabel)),
		cKeychain,
		(*C.uchar)(unsafe.Pointer(&digest[0])),
		C.int(len(digest)),
		&sig,
		&sigLen,
	)
	if status != 0 {
		return nil, "", keychainError("sign with non-extractable key", int(status))
	}
	defer C.free(unsafe.Pointer(sig))

	// RS256: the SecKey PKCS1v15-SHA256 signature is the JOSE signature as-is.
	return C.GoBytes(unsafe.Pointer(sig), C.int(sigLen)), AlgRS256, nil
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

	pemFile, err := os.CreateTemp("", "lark-keysigner-*.pem")
	if err != nil {
		return nil, fmt.Errorf("keysigner: temp key file: %w", err)
	}
	pemPath := pemFile.Name()
	defer os.Remove(pemPath)
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

	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("keysigner: resolve executable: %w", err)
	}
	keychain, err := ensureKeychain()
	if err != nil {
		return nil, err
	}
	// -x: import as NON-EXTRACTABLE; the software copy (pemPath) is then removed.
	importCmd := exec.Command("security", "import", pemPath, "-k", keychain, "-t", "priv", "-f", "openssl", "-x", "-A", "-T", executable)
	if out, err := importCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("keysigner: import non-extractable key: %w: %s", err, string(out))
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
	cKeychain := C.CString(keychain)
	defer C.free(unsafe.Pointer(cKeychain))
	cLabel := C.CString(label)
	defer C.free(unsafe.Pointer(cLabel))
	status := C.set_private_key_label(
		(*C.uchar)(unsafe.Pointer(&appLabel[0])),
		C.int(len(appLabel)),
		cKeychain,
		cLabel,
	)
	if status != 0 {
		return keychainError("set keychain key label", int(status))
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
	data, err := os.ReadFile(path)
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
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
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
	if _, err := os.Stat(keychainPath); err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("keysigner: stat keychain: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(keychainPath), 0700); err != nil {
			return "", err
		}
		for _, args := range [][]string{
			{"create-keychain", "-p", password, keychainPath},
			{"set-keychain-settings", keychainPath},
			{"unlock-keychain", "-p", password, keychainPath},
		} {
			if out, err := exec.Command("security", args...).CombinedOutput(); err != nil {
				return "", fmt.Errorf("keysigner: security %s: %w: %s", args[0], err, string(out))
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
	if data, err := os.ReadFile(path); err == nil {
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
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(pw+"\n"), 0600); err != nil {
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
