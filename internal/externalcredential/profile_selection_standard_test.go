// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package externalcredential

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/vfs"
)

func TestStandardEditionRejectsExternalCredentialProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	systemPath := filepath.Join(dir, "external-credential.json")
	t.Setenv(envvars.CliExternalCredentialConfig, systemPath)
	profile := []byte(`{
	  "currentApp": "sandbox",
	  "apps": [{
	    "name": "sandbox",
	    "appId": "cli_example",
	    "brand": "feishu",
	    "users": []
	  }]
	}`)
	system := []byte(`{
	  "version": 1,
	  "mode": "platform_proxy",
	  "remoteEndpoint": "https://proxy.example",
	  "applications": [{"brand": "feishu", "appId": "cli_example"}]
	}`)
	if err := vfs.WriteFile(filepath.Join(dir, "config.json"), profile, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := vfs.WriteFile(systemPath, system, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := SelectProfile("")
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("SelectProfile error = %T %v, want validation/failed_precondition", err, err)
	}
}
