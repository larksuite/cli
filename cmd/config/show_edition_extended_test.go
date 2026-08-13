// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/runtimeplan"
)

func TestExtendedConfigShowAllowedWithManagedSource(t *testing.T) {
	f := newConfigFactoryWithExternalProvider(t)
	cmd := NewCmdConfig(f)
	matched, _, err := cmd.Find([]string{"show"})
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.PersistentPreRunE(matched, nil); err != nil {
		t.Fatalf("config show blocked: %v", err)
	}
}

func TestExtendedConfigShowProjectsManagedSource(t *testing.T) {
	f := newConfigFactoryWithExternalProvider(t)
	var stdout bytes.Buffer
	f.IOStreams.Out = &stdout
	cmdutil.TestSetRuntimePlan(t, f, runtimeplan.New(runtimeplan.Options{
		Description: runtimeplan.Description{
			Managed:           true,
			Variant:           "managed-test",
			ProxiesRequests:   true,
			DataPlaneEndpoint: "https://managed.example.test",
		},
	}))

	if err := configShowRun(&ConfigShowOptions{Factory: f}); err != nil {
		t.Fatalf("configShowRun() error = %v", err)
	}
	var got editionConfigShowResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Source != "external" ||
		got.CredentialProvider != "env" ||
		got.Manageable ||
		got.AppID != "test-app" ||
		got.ExternalCredentialMode == nil ||
		*got.ExternalCredentialMode != "managed-test" ||
		got.RemoteEndpoint == nil ||
		*got.RemoteEndpoint != "https://managed.example.test" {
		t.Fatalf("output = %#v", got)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["appSecret"]; ok {
		t.Fatalf("managed output must not invent appSecret: %s", stdout.String())
	}
	if _, ok := fields["users"]; ok {
		t.Fatalf("managed output must not invent users: %s", stdout.String())
	}
}

func TestExtendedConfigShowTypesManagedSourceFailure(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	providerErr := errors.New("provider failed")
	cred := credential.NewCredentialProvider(
		[]extcred.Provider{&stubConfigExtProvider{name: "broken", err: providerErr}},
		nil, nil, nil,
	)
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.Credential = cred

	err := configShowRun(&ConfigShowOptions{Factory: f})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeUnknown {
		t.Fatalf("error = %#v, want internal/unknown", err)
	}
	if !errors.Is(err, providerErr) {
		t.Fatalf("error does not preserve provider failure: %v", err)
	}
}
