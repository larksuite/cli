// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package doctor

import (
	"context"
	"encoding/json"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
)

func TestStandardDoctorPreservesConfigFirstDiagnostics(t *testing.T) {
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

	if err := doctorRun(&DoctorOptions{Factory: f, Ctx: context.Background(), Offline: true}, nil); err == nil {
		t.Fatal("doctorRun() = nil, want established missing-config failure")
	}
	var got struct {
		Checks []checkResult `json:"checks"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	assertCheck(t, got.Checks, "config_file", "fail")
	for _, check := range got.Checks {
		if check.Name == "credential_source" {
			t.Fatalf("Standard doctor exposed edition diagnostic: %#v", got.Checks)
		}
	}
}
