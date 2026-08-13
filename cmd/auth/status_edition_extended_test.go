// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package auth

import (
	"encoding/json"
	"strings"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/runtimeplan"
)

func TestExtendedAuthStatusReportsManagedSource(t *testing.T) {
	cfg := &core.CliConfig{
		AppID: "cli_env", Brand: core.BrandFeishu, DefaultAs: core.AsBot,
		SupportedIdentities: uint8(extcred.SupportsBot),
	}
	f, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	f.Credential = credential.NewCredentialProvider(
		[]extcred.Provider{&stubExternalProvider{name: "env"}},
		nil, nil, f.HttpClient,
	)
	cmdutil.TestSetRuntimePlan(t, f, runtimeplan.New(runtimeplan.Options{
		Description: runtimeplan.Description{
			Managed: true,
			Variant: "managed-test",
		},
	}))

	if err := authStatusRun(&StatusOptions{Factory: f}, nil); err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["source"] != "external" ||
		got["credentialProvider"] != "env" ||
		got["externalCredentialMode"] != "managed-test" ||
		got["identity"] != "bot" {
		t.Fatalf("output = %#v", got)
	}
	if note, _ := got["note"].(string); strings.Contains(note, "auth login") ||
		!strings.Contains(note, "external credential provider env") {
		t.Fatalf("note = %q", note)
	}
}
