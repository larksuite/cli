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
	"github.com/larksuite/cli/internal/envvars"
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

	typ, assertion, err := SignClientAssertion(context.Background(), "agent-key", "cli_app", "open.feishu.cn")
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

func TestProbeUsesConfiguredHelper(t *testing.T) {
	t.Setenv(envvars.CliKeylessSignerCmd, `/helper`)
	prev := run
	t.Cleanup(func() { run = prev })

	var got request
	run = func(ctx context.Context, argv []string, req request) (response, error) {
		got = req
		return response{OK: true}, nil
	}

	if err := Probe(context.Background(), "agent-key"); err != nil {
		t.Fatal(err)
	}
	if got.Op != "pubkey" || got.KeyRef != "agent-key" {
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
