// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package keychain

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// ---------------------------------------------------------------------------
// Windows backend: DPAPI + HKCU registry
// ---------------------------------------------------------------------------

const regRootPath = `Software\LarkCli\keychain`

// registryPathForService returns the registry path for a given service.
func registryPathForService(service string) string {
	return regRootPath + `\` + safeRegistryComponent(service)
}

var safeRegRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// safeRegistryComponent sanitizes a string to be used as a registry key component.
func safeRegistryComponent(s string) string {
	// Registry key path uses '\\' separators; avoid accidental nesting and odd chars.
	s = strings.ReplaceAll(s, "\\", "_")
	return safeRegRe.ReplaceAllString(s, "_")
}

func valueNameForAccount(account string) string {
	// Avoid any special characters; keep deterministic.
	return base64.RawURLEncoding.EncodeToString([]byte(account))
}

// dpapiEntropy generates entropy for DPAPI encryption based on the service and account names.
func dpapiEntropy(service, account string) *windows.DataBlob {
	// Bind ciphertext to (service, account) to reduce swap/replay risks.
	// Note: empty entropy is allowed, but we intentionally use deterministic entropy.
	data := []byte(service + "\x00" + account)
	if len(data) == 0 {
		return nil
	}
	return &windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
}

// dpapiProtect encrypts data using Windows DPAPI.
func dpapiProtect(plaintext []byte, entropy *windows.DataBlob) ([]byte, error) {
	var in windows.DataBlob
	if len(plaintext) > 0 {
		in = windows.DataBlob{Size: uint32(len(plaintext)), Data: &plaintext[0]}
	}
	var out windows.DataBlob
	err := windows.CryptProtectData(&in, nil, entropy, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, err
	}
	defer freeDataBlob(&out)

	if out.Data == nil || out.Size == 0 {
		return []byte{}, nil
	}
	buf := unsafe.Slice(out.Data, int(out.Size))
	res := make([]byte, len(buf))
	copy(res, buf)
	return res, nil
}

// dpapiUnprotect decrypts data using Windows DPAPI.
func dpapiUnprotect(ciphertext []byte, entropy *windows.DataBlob) ([]byte, error) {
	var in windows.DataBlob
	if len(ciphertext) > 0 {
		in = windows.DataBlob{Size: uint32(len(ciphertext)), Data: &ciphertext[0]}
	}
	var out windows.DataBlob
	err := windows.CryptUnprotectData(&in, nil, entropy, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, err
	}
	defer freeDataBlob(&out)

	if out.Data == nil || out.Size == 0 {
		return []byte{}, nil
	}
	buf := unsafe.Slice(out.Data, int(out.Size))
	res := make([]byte, len(buf))
	copy(res, buf)
	return res, nil
}

// freeDataBlob frees the memory allocated for a DataBlob.
func freeDataBlob(b *windows.DataBlob) {
	if b == nil || b.Data == nil {
		return
	}
	// Per DPAPI contract, output buffers must be freed with LocalFree.
	_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(b.Data)))
	b.Data = nil
	b.Size = 0
}

// platformGet retrieves a value from the Windows registry.
func platformGet(service, account string) (string, error) {
	v, ok := registryGet(service, account)
	if !ok {
		return "", nil
	}
	return v, nil
}

// platformSet stores a value in the Windows registry.
func platformSet(service, account, data string) error {
	entropy := dpapiEntropy(service, account)
	protected, err := dpapiProtect([]byte(data), entropy)
	if err != nil {
		return fmt.Errorf("dpapi protect failed: %w", err)
	}
	return registrySet(service, account, protected)
}

// platformRemove deletes a value from the Windows registry.
func platformRemove(service, account string) error {
	return registryRemove(service, account)
}

// registryGet retrieves a string value from the registry under the given service and account.
func registryGet(service, account string) (string, bool) {
	keyPath := registryPathForService(service)
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()

	b64, _, err := k.GetStringValue(valueNameForAccount(account))
	if err != nil || b64 == "" {
		return "", false
	}
	blob, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", false
	}
	entropy := dpapiEntropy(service, account)
	plain, err := dpapiUnprotect(blob, entropy)
	if err != nil {
		return "", false
	}
	return string(plain), true
}

// registrySet stores a string value in the registry under the given service and account.
func registrySet(service, account string, protected []byte) error {
	keyPath := registryPathForService(service)
	k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("registry create/open failed: %w", err)
	}
	defer k.Close()

	b64 := base64.StdEncoding.EncodeToString(protected)
	if err := k.SetStringValue(valueNameForAccount(account), b64); err != nil {
		return fmt.Errorf("registry set failed: %w", err)
	}
	return nil
}

// registryRemove deletes a value from the registry under the given service and account.
func registryRemove(service, account string) error {
	keyPath := registryPathForService(service)
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	_ = k.DeleteValue(valueNameForAccount(account))
	return nil
}
