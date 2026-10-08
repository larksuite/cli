// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dpop

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/keysigner"
)

func TestDefaultTPMSignerUsesDataDirectory(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(fmt.Sprintf("override=%t", override), func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config"))
			t.Setenv("LARKSUITE_CLI_DATA_DIR", "")
			dataDirectory := filepath.Join(root, ".local", "share")
			if override {
				dataDirectory = filepath.Join(root, "data")
				t.Setenv("LARKSUITE_CLI_DATA_DIR", dataDirectory)
			}
			store := NewKeyStore(&testMetadataStore{values: map[string]string{}})
			signer := store.signerByName("linux-tpm")
			if signer == nil || store.signers[0] != signer || signer.SecurityLevel() != keysigner.SecurityLevelL1 {
				t.Fatal("default store did not register the TPM as L1")
			}
			directory := filepath.Join(dataDirectory, "lark-cli", "keysigner", "linux-tpm")
			if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("constructing a store accessed the TPM directory: %v", err)
			}
			// Missing key files fail before opening /dev/tpmrm0.
			ref := keysigner.KeyRef{Label: "path-check"}
			if _, err := signer.PublicKey(context.Background(), ref); !errors.Is(err, keysigner.ErrKeyNotFound) {
				t.Fatalf("missing key = %v", err)
			}
			if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing-key lookup created local state: %v", err)
			}
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, fmt.Sprintf("%x.json", sha256.Sum256([]byte(ref.Label))))
			if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := signer.PublicKey(context.Background(), ref); !errors.Is(err, keysigner.ErrCorrupt) {
				t.Fatalf("TPM signer did not read the expected key file: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "config", "signing-keys")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("TPM key operation accessed the configuration directory: %v", err)
			}
		})
	}
}
