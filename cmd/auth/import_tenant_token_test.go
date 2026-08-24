// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

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

type importTATKeychain struct {
	service string
	account string
	value   string
	setErr  error
}

func (k *importTATKeychain) Get(string, string) (string, error) { return "", nil }
func (k *importTATKeychain) Set(service, account, value string) error {
	k.service = service
	k.account = account
	k.value = value
	return k.setErr
}
func (k *importTATKeychain) Remove(string, string) error { return nil }

func isolateImportTATConfig(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
}

func TestAuthImportTenantToken_StoresSecretAndPrintsEnvelope(t *testing.T) {
	isolateImportTATConfig(t)
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	f.IOStreams.In = strings.NewReader("tenant-token\n")
	kc := &importTATKeychain{}
	f.Keychain = kc

	cmd := NewCmdAuthImportTenantToken(f, nil)
	cmd.SetArgs([]string{"--app-id", "cli_test", "--token-stdin"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if kc.service != keychain.LarkCliService || kc.account != "tat:cli_test" || kc.value != "tenant-token" {
		t.Fatalf("stored = (%q, %q, %q), want lark-cli/tat:cli_test with original token", kc.service, kc.account, kc.value)
	}

	var envelope output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	data, ok := envelope.Data.(map[string]any)
	if !envelope.OK || !ok || data["appId"] != "cli_test" || data["stored"] != true {
		t.Fatalf("envelope = %#v, want ok data appId/stored", envelope)
	}
	if strings.Contains(stdout.String(), "tenant-token") {
		t.Fatalf("stdout leaked token: %s", stdout.String())
	}
}

func TestAuthImportTenantToken_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		in        string
		wantParam string
	}{
		{name: "missing token stdin flag", args: []string{"--app-id", "cli_test"}, wantParam: "--token-stdin"},
		{name: "empty stdin", args: []string{"--app-id", "cli_test", "--token-stdin"}, wantParam: "--token-stdin"},
		{name: "empty token line", args: []string{"--app-id", "cli_test", "--token-stdin"}, in: "\n", wantParam: "--token-stdin"},
		{name: "leading token whitespace", args: []string{"--app-id", "cli_test", "--token-stdin"}, in: " tenant-token\n", wantParam: "--token-stdin"},
		{name: "trailing token whitespace", args: []string{"--app-id", "cli_test", "--token-stdin"}, in: "tenant-token \n", wantParam: "--token-stdin"},
		{name: "internal token whitespace", args: []string{"--app-id", "cli_test", "--token-stdin"}, in: "tenant token\n", wantParam: "--token-stdin"},
		{name: "multi-line token", args: []string{"--app-id", "cli_test", "--token-stdin"}, in: "tenant-token\nsecond-line\n", wantParam: "--token-stdin"},
		{name: "whitespace app id", args: []string{"--app-id", "   ", "--token-stdin"}, in: "tenant-token\n", wantParam: "--app-id"},
		{name: "surrounding app id whitespace", args: []string{"--app-id", " cli_test", "--token-stdin"}, in: "tenant-token\n", wantParam: "--app-id"},
		{name: "unsafe app id", args: []string{"--app-id", "cli/test", "--token-stdin"}, in: "tenant-token\n", wantParam: "--app-id"},
		{name: "uppercase app id", args: []string{"--app-id", "cli_Test", "--token-stdin"}, in: "tenant-token\n", wantParam: "--app-id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isolateImportTATConfig(t)
			f, _, _, _ := cmdutil.TestFactory(t, nil)
			f.IOStreams.In = strings.NewReader(tc.in)
			cmd := NewCmdAuthImportTenantToken(f, nil)
			cmd.SilenceErrors = true
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil, want validation error")
			}
			if got := output.ExitCodeOf(err); got != output.ExitValidation {
				t.Fatalf("exit code = %d, want validation", got)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T %v, want *errs.ValidationError", err, err)
			}
			if validationErr.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != tc.wantParam {
				t.Fatalf("validation metadata = (subtype=%q, param=%q), want invalid_argument/%s", validationErr.Subtype, validationErr.Param, tc.wantParam)
			}
		})
	}
}

func TestAuthImportTenantToken_StorageFailureIsTypedAndDoesNotLeak(t *testing.T) {
	isolateImportTATConfig(t)
	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	f.IOStreams.In = strings.NewReader("super-secret-token\n")
	underlying := errors.New("set failed")
	f.Keychain = &importTATKeychain{setErr: underlying}

	cmd := NewCmdAuthImportTenantToken(f, nil)
	cmd.SetArgs([]string{"--app-id", "cli_test", "--token-stdin"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want storage error")
	}
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) || internalErr.Subtype != errs.SubtypeStorage || !errors.Is(err, underlying) {
		t.Fatalf("error = %T %v, want internal/storage with cause", err, err)
	}
	combined := stdout.String() + stderr.String() + err.Error()
	if strings.Contains(combined, "super-secret-token") {
		t.Fatalf("command output leaked token: %s", combined)
	}
}

func TestAuthImportTenantToken_RejectsPositionalToken(t *testing.T) {
	isolateImportTATConfig(t)
	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	f.IOStreams.In = strings.NewReader("stdin-token\n")
	kc := &importTATKeychain{}
	f.Keychain = kc
	cmd := NewCmdAuthImportTenantToken(f, nil)
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"--app-id", "cli_test", "--token-stdin", "positional-secret"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want positional argument rejection")
	}
	if kc.value != "" {
		t.Fatalf("stored token = %q, want no write", kc.value)
	}
	if strings.Contains(stdout.String()+stderr.String()+err.Error(), "positional-secret") {
		t.Fatal("command output leaked rejected positional token")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != "" || !strings.Contains(validationErr.Hint, "--token-stdin") {
		t.Fatalf("error = %T %v, want positional validation with stdin hint and no flag attribution", err, err)
	}
}

func TestAuthImportTenantToken_BypassesExternalProviderGuardOnlyForLeaf(t *testing.T) {
	isolateImportTATConfig(t)
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	f.IOStreams.In = strings.NewReader("tenant-token\n")
	f.Keychain = &importTATKeychain{}
	stub := &stubExternalProvider{name: "env"}
	f.Credential = credential.NewCredentialProvider([]extcred.Provider{stub}, nil, nil, nil)

	cmd := NewCmdAuth(f)
	cmd.SetArgs([]string{"import-tenant-token", "--app-id", "cli_test", "--token-stdin"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("import under external provider error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"stored": true`) {
		t.Fatalf("stdout = %s, want stored success", stdout.String())
	}

	blocked := NewCmdAuth(f)
	blocked.SetArgs([]string{"status"})
	if err := blocked.Execute(); err == nil {
		t.Fatal("auth status under external provider error = nil, want existing guard")
	}
}

func TestAuthImportTenantToken_RunHookReceivesValidatedOptions(t *testing.T) {
	isolateImportTATConfig(t)
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.IOStreams.In = strings.NewReader("ignored-by-hook\n")
	var got *ImportTenantTokenOptions
	cmd := NewCmdAuthImportTenantToken(f, func(opts *ImportTenantTokenOptions) error {
		got = opts
		return nil
	})
	cmd.SetArgs([]string{"--app-id", "cli_test", "--token-stdin"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got == nil || got.AppID != "cli_test" || !got.TokenStdin || got.Ctx == nil {
		t.Fatalf("options = %#v, want populated options", got)
	}
}

type importTATFailingReader struct {
	err error
}

func (r importTATFailingReader) Read([]byte) (int, error) { return 0, r.err }

func TestAuthImportTenantToken_StdinFailurePreservesCause(t *testing.T) {
	isolateImportTATConfig(t)
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	underlying := io.ErrUnexpectedEOF
	f.IOStreams.In = importTATFailingReader{err: underlying}
	cmd := NewCmdAuthImportTenantToken(f, nil)
	cmd.SetArgs([]string{"--app-id", "cli_test", "--token-stdin"})

	err := cmd.Execute()
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T %v, want *errs.ValidationError", err, err)
	}
	if validationErr.Subtype != errs.SubtypeFailedPrecondition || validationErr.Param != "--token-stdin" {
		t.Fatalf("validation metadata = (subtype=%q, param=%q), want failed_precondition/--token-stdin", validationErr.Subtype, validationErr.Param)
	}
	if !errors.Is(err, underlying) {
		t.Fatalf("error = %v, want stdin cause", err)
	}
}

var _ keychain.KeychainAccess = (*importTATKeychain)(nil)
