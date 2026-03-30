// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin

package keychain

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestDecodeMasterKeyValue(t *testing.T) {
	rawKey := bytes.Repeat([]byte{0x5a}, masterKeyBytes)
	encodedKey := base64.StdEncoding.EncodeToString(rawKey)

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "plain base64",
			input: encodedKey,
		},
		{
			name:  "go-keyring base64 wrapper",
			input: goKeyringBase64Prefix + base64.StdEncoding.EncodeToString([]byte(encodedKey)),
		},
		{
			name:  "go-keyring hex wrapper",
			input: goKeyringEncodedPrefix + hex.EncodeToString([]byte(encodedKey)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := decodeMasterKeyValue(tt.input)
			if !ok {
				t.Fatalf("decodeMasterKeyValue(%q) returned ok=false", tt.name)
			}
			if !bytes.Equal(got, rawKey) {
				t.Fatalf("decodeMasterKeyValue(%q) returned unexpected key", tt.name)
			}
		})
	}
}
