// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package doctor

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/identitydiag"
)

func TestExtendedProxyNetworkCheckUsesAuthenticatedDiagnostics(t *testing.T) {
	verified := true
	endpoint := "https://credentials.example"
	got := editionProxyNetworkCheck(&DoctorOptions{}, endpoint, identitydiag.Result{
		User: identitydiag.Identity{Verified: &verified},
	})
	if got.Status != "pass" || got.Name != "endpoint_external_platform" {
		t.Fatalf("check = %#v", got)
	}

	got = editionProxyNetworkCheck(&DoctorOptions{}, endpoint, identitydiag.Result{})
	if got.Status != "fail" {
		t.Fatalf("unverified check = %#v, want fail", got)
	}
}

func TestExtendedDoctorManagedSourceDoesNotRequireLocalConfig(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := &core.CliConfig{
		AppID: "cli_env", Brand: core.BrandFeishu,
		SupportedIdentities: uint8(extcred.SupportsBot), DefaultAs: core.AsBot,
	}
	f, out, _, _ := cmdutil.TestFactory(t, cfg)
	f.Credential = credential.NewCredentialProvider(
		[]extcred.Provider{&fakeExtProvider{
			name: "env",
			account: &extcred.Account{
				AppID:               "cli_env",
				SupportedIdentities: extcred.SupportsBot,
			},
		}},
		nil, nil, nil,
	)

	if err := doctorRun(&DoctorOptions{Factory: f, Ctx: context.Background(), Offline: true}, nil); err != nil {
		t.Fatalf("doctorRun() error = %v", err)
	}
	var got struct {
		OK     bool          `json:"ok"`
		Checks []checkResult `json:"checks"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK {
		t.Fatalf("checks = %#v", got.Checks)
	}
	assertCheck(t, got.Checks, "credential_source", "pass")
	configCheck := findCheck(t, got.Checks, "config_file")
	if configCheck.Status != "skip" ||
		!strings.Contains(configCheck.Message, "local config") ||
		strings.Contains(configCheck.Message, "config init") {
		t.Fatalf("config_file = %#v", configCheck)
	}
}
