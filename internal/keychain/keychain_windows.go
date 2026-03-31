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

func registryPathForService(service string) string {
	return regRootPath + `\` + safeRegistryComponent(service)
}

var safeRegRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func safeRegistryComponent(s string) string {
	// Registry key path uses '\\' separators; avoid accidental nesting and odd chars.
	s = strings.ReplaceAll(s, "\\", "_")
	return safeRegRe.ReplaceAllString(s, "_")
}

func valueNameForAccount(account string) string {
	// Avoid any special characters; keep deterministic.
	return base64.RawURLEncoding.EncodeToString([]byte(account))
}

func dpapiEntropy(service, account string) *windows.DataBlob {
	// Bind ciphertext to (service, account) to reduce swap/replay risks.
	// Note: empty entropy is allowed, but we intentionally use deterministic entropy.
	data := []byte(service + "\x00" + account)
	if len(data) == 0 {
		return nil
	}
	return &windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
}

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

func freeDataBlob(b *windows.DataBlob) {
	if b == nil || b.Data == nil {
		return
	}
	// Per DPAPI contract, output buffers must be freed with LocalFree.
	_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(b.Data)))
	b.Data = nil
	b.Size = 0
}

func platformGet(service, account string) (string, error) {
	v, ok := registryGet(service, account)
	if !ok {
		return "", fmt.Errorf("not found")
	}
	return v, nil
}

func platformSet(service, account, data string) error {
	entropy := dpapiEntropy(service, account)
	protected, err := dpapiProtect([]byte(data), entropy)
	if err != nil {
		return fmt.Errorf("dpapi protect failed: %w", err)
	}
	return registrySet(service, account, protected)
}

func platformRemove(service, account string) error {
	return registryRemove(service, account)
}

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
