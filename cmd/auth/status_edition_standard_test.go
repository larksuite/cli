// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package auth

import (
	"encoding/json"
	"strings"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
)

func TestStandardAuthStatusPreservesExistingProjection(t *testing.T) {
	cfg := &core.CliConfig{
		AppID: "cli_env", Brand: core.BrandFeishu, DefaultAs: core.AsBot,
		SupportedIdentities: uint8(extcred.SupportsBot),
	}
	f, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	f.Credential = credential.NewCredentialProvider(
		[]extcred.Provider{&stubExternalProvider{name: "env"}},
		nil, nil, f.HttpClient,
	)

	if err := authStatusRun(&StatusOptions{Factory: f}, nil); err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"source", "credentialProvider", "externalCredentialMode"} {
		if _, exists := got[field]; exists {
			t.Fatalf("Standard auth status contains edition field %q: %s", field, stdout.String())
		}
	}
	var note string
	if err := json.Unmarshal(got["note"], &note); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(note, "lark-cli auth login") {
		t.Fatalf("Standard note = %q, want established login guidance", note)
	}
}
