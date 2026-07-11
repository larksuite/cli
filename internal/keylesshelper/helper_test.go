// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/auth/jwt"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/vfs"
)

func TestParseCommand(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "plain path", raw: "/opt/lark-keyless-signer", want: []string{"/opt/lark-keyless-signer"}},
		{name: "json argv", raw: `["/opt/lark-keyless-signer","--mode","local"]`, want: []string{"/opt/lark-keyless-signer", "--mode", "local"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCommand(tc.raw)
			if err != nil {
				t.Fatalf("parseCommand: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("argv = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestValidateConfiguredRejectsInvalidJSONArgv(t *testing.T) {
	t.Setenv(envvars.CliKeylessSignerCmd, `[""]`)

	err := ValidateConfigured()
	if err == nil {
		t.Fatal("ValidateConfigured() error = nil, want invalid argv error")
	}
	if !strings.Contains(err.Error(), "must name a helper binary") {
		t.Fatalf("ValidateConfigured() error = %q", err)
	}
}

func TestResolveFromConfig(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliKeylessSignerCmd, "")
	seedKeylessSignerConfig(t, "/config/helper")

	helper, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if helper == nil {
		t.Fatal("Resolve() = nil, want config.json helper")
	}
	if err := ValidateConfigured(); err != nil {
		t.Fatalf("ValidateConfigured() error = %v", err)
	}
}

func TestCommandResolutionUsesConfigJSONArgv(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliKeylessSignerCmd, "")
	seedKeylessSignerConfig(t, `["/config/helper","--mode","config"]`)
	prev := run
	t.Cleanup(func() { run = prev })

	run = func(_ context.Context, argv []string, _ request) (response, error) {
		want := []string{"/config/helper", "--mode", "config"}
		if !reflect.DeepEqual(argv, want) {
			t.Fatalf("argv = %#v, want %#v", argv, want)
		}
		return response{OK: true}, nil
	}

	helper, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err := helper.Probe(context.Background(), "agent-key"); err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
}

func TestCommandResolutionPrefersEnvironment(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliKeylessSignerCmd, "/env/helper")
	seedKeylessSignerConfig(t, "/config/helper")
	prev := run
	t.Cleanup(func() { run = prev })

	run = func(_ context.Context, argv []string, _ request) (response, error) {
		if !reflect.DeepEqual(argv, []string{"/env/helper"}) {
			t.Fatalf("argv = %#v, want environment helper", argv)
		}
		return response{OK: true}, nil
	}

	helper, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err := helper.Probe(context.Background(), "agent-key"); err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
}

func TestCommandResolutionInvalidEnvironmentShadowsConfig(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliKeylessSignerCmd, `[""]`)
	seedKeylessSignerConfig(t, "/config/helper")

	err := ValidateConfigured()
	if err == nil || !strings.Contains(err.Error(), "must name a helper binary") {
		t.Fatalf("ValidateConfigured() error = %v, want invalid environment command", err)
	}
}

func TestCommandResolutionInvalidConfigNamesConfigSource(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliKeylessSignerCmd, "")
	seedKeylessSignerConfig(t, `[""]`)

	err := ValidateConfigured()
	if err == nil {
		t.Fatal("ValidateConfigured() error = nil, want invalid config command")
	}
	if !strings.Contains(err.Error(), "config.json keylessSignerCmd") {
		t.Fatalf("ValidateConfigured() error = %q, want config source", err)
	}
}

func TestResolveUnconfigured(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliKeylessSignerCmd, "")

	helper, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if helper != nil {
		t.Fatal("Resolve() != nil, want no helper without env or config")
	}
}

func TestResolveFailsClosedWhenConfigMalformed(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliKeylessSignerCmd, "")
	if err := vfs.WriteFile(core.GetConfigPath(), []byte("{not-json"), 0600); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}

	helper, err := Resolve()
	if err == nil {
		t.Fatalf("Resolve() = %v, nil; want config load error", helper)
	}
	if !strings.Contains(err.Error(), "config.json keylessSignerCmd") || !strings.Contains(err.Error(), core.GetConfigPath()) {
		t.Fatalf("Resolve() error = %q, want signer source and config path", err)
	}
}

func TestResolveFailsClosedWhenConfigUnreadable(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliKeylessSignerCmd, "")
	if err := vfs.MkdirAll(core.GetConfigPath(), 0700); err != nil {
		t.Fatalf("create directory at config path: %v", err)
	}

	helper, err := Resolve()
	if err == nil {
		t.Fatalf("Resolve() = %v, nil; want config load error", helper)
	}
	if !strings.Contains(err.Error(), "config.json keylessSignerCmd") || !strings.Contains(err.Error(), core.GetConfigPath()) {
		t.Fatalf("Resolve() error = %q, want signer source and config path", err)
	}
}

func TestResolvePinsSingleConfigSnapshot(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliKeylessSignerCmd, "")
	seedKeylessSignerConfig(t, "/first/helper")

	helper, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if helper == nil {
		t.Fatal("Resolve() = nil, want configured helper")
	}

	// A concurrent atomic config replacement after resolution must not change
	// the command used by this operation.
	seedKeylessSignerConfig(t, "/second/helper")
	prev := run
	t.Cleanup(func() { run = prev })
	run = func(_ context.Context, argv []string, _ request) (response, error) {
		if !reflect.DeepEqual(argv, []string{"/first/helper"}) {
			t.Fatalf("argv = %#v, want command from the resolved snapshot", argv)
		}
		return response{OK: true}, nil
	}

	if err := helper.Probe(context.Background(), "agent-key"); err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
}

func seedKeylessSignerConfig(t *testing.T, command string) {
	t.Helper()
	err := core.SaveMultiAppConfig(&core.MultiAppConfig{
		KeylessSignerCmd: command,
		Apps: []core.AppConfig{{
			AppId: "cli_test", AppSecret: core.PlainSecret("secret"), Brand: core.BrandFeishu,
		}},
	})
	if err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}
}

func TestSignAttestationUsesConfiguredHelper(t *testing.T) {
	t.Setenv(envvars.CliKeylessSignerCmd, `["/helper"]`)
	prev := run
	t.Cleanup(func() { run = prev })

	var got request
	run = func(ctx context.Context, argv []string, req request) (response, error) {
		got = req
		if !reflect.DeepEqual(argv, []string{"/helper"}) {
			t.Fatalf("argv = %#v", argv)
		}
		return response{OK: true, Attestation: "att.jwt"}, nil
	}

	att, err := SignAttestation(context.Background(), "agent-key", "nonce-1")
	if err != nil {
		t.Fatal(err)
	}
	if att != "att.jwt" {
		t.Fatalf("attestation = %q", att)
	}
	if got.Op != "sign-attestation" || got.KeyRef != "agent-key" || got.Nonce != "nonce-1" {
		t.Fatalf("request = %+v", got)
	}
}

func TestSignClientAssertionUsesConfiguredHelper(t *testing.T) {
	t.Setenv(envvars.CliKeylessSignerCmd, `/helper`)
	prev := run
	t.Cleanup(func() { run = prev })

	var got request
	run = func(ctx context.Context, argv []string, req request) (response, error) {
		got = req
		return response{
			OK:                  true,
			ClientAssertionType: jwt.ClientAssertionType,
			ClientAssertion:     "client.jwt",
		}, nil
	}

	helper, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	typ, assertion, err := helper.SignClientAssertion(context.Background(), "agent-key", "cli_app", "open.feishu.cn")
	if err != nil {
		t.Fatal(err)
	}
	if typ != jwt.ClientAssertionType || assertion != "client.jwt" {
		t.Fatalf("got (%q, %q)", typ, assertion)
	}
	if got.Op != "sign-assertion" || got.KeyRef != "agent-key" || got.ClientID != "cli_app" || got.Audience != "open.feishu.cn" {
		t.Fatalf("request = %+v", got)
	}
}

func TestRunCommandOmitsStderrOnFailure(t *testing.T) {
	err := helperRunError(errors.New("exit status 1"), "secret.jwt")
	if err == nil {
		t.Fatal("helperRunError() error = nil")
	}
	if strings.Contains(err.Error(), "secret.jwt") {
		t.Fatalf("error leaked stderr: %q", err)
	}
}

func TestHelperRunErrorPreservesCause(t *testing.T) {
	cause := errors.New("boom")
	err := helperRunError(cause, "")
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(err, cause) = false; err=%v", err)
	}
}
