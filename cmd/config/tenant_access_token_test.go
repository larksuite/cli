// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/output"
)

type configTenantTokenKeychain struct {
	values      map[string]string
	setCalls    int
	setValues   []string
	removeCalls int
}

func (k *configTenantTokenKeychain) Get(service, account string) (string, error) {
	return k.values[service+"/"+account], nil
}

func (k *configTenantTokenKeychain) Set(service, account, value string) error {
	k.setCalls++
	k.setValues = append(k.setValues, value)
	if k.values == nil {
		k.values = make(map[string]string)
	}
	k.values[service+"/"+account] = value
	return nil
}

func TestConfigTenantAccessTokenSetPreservesOpaqueLine(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	token := "opaque token\t雪"
	f.IOStreams.In = strings.NewReader(token + "\n")
	kc := &configTenantTokenKeychain{}
	f.Keychain = kc
	cmd := newCmdConfigTenantAccessTokenSet(f)
	cmd.SetArgs([]string{"--app-id", "cli_test"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(kc.setValues) != 1 || kc.setValues[0] != token {
		t.Fatalf("stored values = %q, want opaque input preserved verbatim", kc.setValues)
	}
}

func TestConfigTenantAccessTokenSetHelpHasNoSourceIndentation(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	cmd := newCmdConfigTenantAccessTokenSet(f)
	if strings.Contains(cmd.Long, "\t") || !strings.Contains(cmd.Long, "\nfor bot requests") {
		t.Fatalf("Long help contains source indentation: %q", cmd.Long)
	}
}

func (k *configTenantTokenKeychain) Remove(service, account string) error {
	k.removeCalls++
	delete(k.values, service+"/"+account)
	return nil
}

func TestConfigTenantAccessTokenSetStoresWithoutLeaking(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	f.IOStreams.In = strings.NewReader("super-secret-tat\n")
	kc := &configTenantTokenKeychain{}
	f.Keychain = kc
	cmd := newCmdConfigTenantAccessTokenSet(f)
	cmd.SetArgs([]string{"--app-id", "cli_Test/opaque"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if kc.setCalls != 1 {
		t.Fatalf("Set calls = %d, want 1", kc.setCalls)
	}
	if strings.Contains(stdout.String()+stderr.String(), "super-secret-tat") {
		t.Fatalf("output leaked token: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	var envelope output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	data, ok := envelope.Data.(map[string]any)
	if !envelope.OK || !ok || data["appId"] != "cli_Test/opaque" || data["stored"] != true {
		t.Fatalf("envelope = %#v, want stored success", envelope)
	}
}

func TestConfigTenantAccessTokenSetRejectsInvalidInputBeforeStorage(t *testing.T) {
	tests := []struct {
		name string
		in   io.Reader
		args []string
	}{
		{name: "empty stdin", in: strings.NewReader(""), args: []string{"--app-id", "cli_test"}},
		{name: "empty line", in: strings.NewReader("\n"), args: []string{"--app-id", "cli_test"}},
		{name: "multiple lines", in: strings.NewReader("token\nsecond\n"), args: []string{"--app-id", "cli_test"}},
		{name: "positional", in: strings.NewReader("token\n"), args: []string{"--app-id", "cli_test", "secret-positional"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
			f.IOStreams.In = tc.in
			kc := &configTenantTokenKeychain{}
			f.Keychain = kc
			cmd := newCmdConfigTenantAccessTokenSet(f)
			cmd.SilenceErrors = true
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) || kc.setCalls != 0 {
				t.Fatalf("error=%T %v setCalls=%d, want validation before storage", err, err, kc.setCalls)
			}
			combined := stdout.String() + stderr.String() + err.Error()
			if strings.Contains(combined, "secret-positional") {
				t.Fatalf("error leaked positional value: %s", combined)
			}
		})
	}
}

type failingTenantTokenReader struct{ err error }

func (r failingTenantTokenReader) Read([]byte) (int, error) { return 0, r.err }

func TestConfigTenantAccessTokenSetPreservesStdinFailure(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	sentinel := io.ErrUnexpectedEOF
	f.IOStreams.In = failingTenantTokenReader{err: sentinel}
	kc := &configTenantTokenKeychain{}
	f.Keychain = kc
	cmd := newCmdConfigTenantAccessTokenSet(f)
	cmd.SetArgs([]string{"--app-id", "cli_test"})

	err := cmd.Execute()
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeFailedPrecondition || !errors.Is(err, sentinel) || kc.setCalls != 0 {
		t.Fatalf("error=%T %v setCalls=%d, want failed_precondition preserving cause", err, err, kc.setCalls)
	}
}

func TestConfigTenantAccessTokenRemoveIsIdempotent(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	kc := &configTenantTokenKeychain{}
	f.Keychain = kc
	cmd := newCmdConfigTenantAccessTokenRemove(f)
	cmd.SetArgs([]string{"--app-id", "cli_test"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if kc.removeCalls != 1 || !strings.Contains(stdout.String(), `"removed": true`) {
		t.Fatalf("removeCalls=%d stdout=%s, want idempotent removed success", kc.removeCalls, stdout.String())
	}
}

func TestConfigTenantAccessTokenGroupBypassesExternalProviderGuard(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	f.IOStreams.In = strings.NewReader("tenant-token\n")
	f.Keychain = &configTenantTokenKeychain{}
	f.Credential = credential.NewCredentialProvider(
		[]extcred.Provider{&stubConfigExtProvider{name: "env"}}, nil, nil, nil,
	)
	cmd := NewCmdConfig(f)
	cmd.SetArgs([]string{"tenant-access-token", "set", "--app-id", "cli_test"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"stored": true`) {
		t.Fatalf("stdout = %s, want stored success", stdout.String())
	}
}

var _ keychain.KeychainAccess = (*configTenantTokenKeychain)(nil)
