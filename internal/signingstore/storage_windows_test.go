// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signingstore

import (
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/validate"
)

func TestWindowsUnlockScopeIgnoresDataOverride(t *testing.T) {
	cache, data := t.TempDir(), t.TempDir()
	t.Setenv("LOCALAPPDATA", cache)
	t.Setenv("LARKSUITE_CLI_DATA_DIR", data)
	root, err := StorageDir()
	if err != nil || root != filepath.Join(data, keychain.LarkCliService) {
		t.Fatalf("key root=%s error=%v", root, err)
	}
	for _, tc := range []struct{ namespace, label string }{
		{"software-file", "DPoP unlock lock directory"},
		{filepath.Join("keyless", "software-file"), "software unlock lock directory"},
	} {
		want, err := validate.SafeEnvDirPath(filepath.Join(cache, keychain.LarkCliService, "keysigner", tc.namespace), tc.label)
		if err != nil {
			t.Fatal(err)
		}
		for _, directory := range []string{filepath.Join(data, "first"), filepath.Join(t.TempDir(), "second")} {
			got, err := UnlockLockDirectory(directory, tc.namespace, tc.label)
			if err != nil || got != want {
				t.Fatalf("unlock namespace=%s got=%s want=%s error=%v", tc.namespace, got, want, err)
			}
		}
	}
}
