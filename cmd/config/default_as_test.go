// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"strings"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
)

func TestDefaultAs_Show_PrefersRuntimeCredentialValue(t *testing.T) {
	setupStrictModeTestConfig(t)

	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	multi.Apps[0].DefaultAs = "auto"
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatal(err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret", DefaultAs: "auto"})
	f.Credential = credential.NewCredentialProvider(
		[]extcred.Provider{&stubStrictModeProvider{
			name: "env",
			account: &extcred.Account{
				AppID:     "env-app",
				AppSecret: "env-secret",
				Brand:     string(core.BrandFeishu),
				DefaultAs: extcred.IdentityUser,
			},
		}},
		nil,
		nil,
		nil,
	)
	f.Config = func() (*core.CliConfig, error) {
		return f.Credential.ResolveAccount(context.Background())
	}

	cmd := NewCmdConfigDefaultAs(f)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(stdout.String(), "default-as: user") {
		t.Fatalf("output = %q, want runtime default-as", stdout.String())
	}
}
