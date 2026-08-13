// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package externalcredential

import (
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/vfs"
)

func TestLoadSystemConfigRejectsUnknownField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "external-credential.json")
	t.Setenv(envvars.CliExternalCredentialConfig, path)
	if err := vfs.WriteFile(path, []byte(`{
	  "version":1,
	  "mode":"platform_proxy",
	  "remoteEndpoint":"https://proxy.example",
	  "applications":[{"brand":"feishu","appId":"cli_test"}],
	  "fallback":"direct"
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, present, err := loadSystemConfig(); err == nil || !present {
		t.Fatal("expected closed system configuration schema to reject unknown field")
	}
	selection, err := SelectProfile("")
	if err == nil {
		t.Fatal("expected profile selection to preserve the system configuration error")
	}
	if selection == nil || !selection.SystemConfigPresent {
		t.Fatalf("selection = %#v, want system configuration presence preserved on error", selection)
	}
}
