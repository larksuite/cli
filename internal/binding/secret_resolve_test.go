// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"testing"
)

func makeGetenv(m map[string]string) func(string) string {
	return func(key string) string { return m[key] }
}

func TestResolve_PlainString(t *testing.T) {
	got, err := ResolveSecretInput(SecretInput{Plain: "my_secret"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "my_secret" {
		t.Errorf("got %q, want %q", got, "my_secret")
	}
}

func TestResolve_EmptyInput(t *testing.T) {
	_, err := ResolveSecretInput(SecretInput{}, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
	want := "appSecret is missing or empty"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolve_EnvTemplate_Found(t *testing.T) {
	getenv := makeGetenv(map[string]string{"MY_VAR": "resolved_value"})
	got, err := ResolveSecretInput(SecretInput{Plain: "${MY_VAR}"}, nil, getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "resolved_value" {
		t.Errorf("got %q, want %q", got, "resolved_value")
	}
}

func TestResolve_EnvTemplate_NotFound(t *testing.T) {
	getenv := makeGetenv(map[string]string{})
	_, err := ResolveSecretInput(SecretInput{Plain: "${MY_VAR}"}, nil, getenv)
	if err == nil {
		t.Fatal("expected error for unset env variable, got nil")
	}
	want := `env variable "MY_VAR" referenced in openclaw.json is not set or empty`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolve_EnvTemplate_InvalidFormat(t *testing.T) {
	getenv := makeGetenv(map[string]string{})
	got, err := ResolveSecretInput(SecretInput{Plain: "${lowercase}"}, nil, getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "${lowercase}" {
		t.Errorf("got %q, want %q (treated as plain string)", got, "${lowercase}")
	}
}

func TestResolve_EnvRef(t *testing.T) {
	getenv := makeGetenv(map[string]string{"MY_KEY": "env_val"})
	input := SecretInput{Ref: &SecretRef{Source: "env", Provider: "default", ID: "MY_KEY"}}
	got, err := ResolveSecretInput(input, nil, getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "env_val" {
		t.Errorf("got %q, want %q", got, "env_val")
	}
}

func TestResolve_EnvRef_NotFound(t *testing.T) {
	getenv := makeGetenv(map[string]string{})
	input := SecretInput{Ref: &SecretRef{Source: "env", Provider: "default", ID: "MY_KEY"}}
	_, err := ResolveSecretInput(input, nil, getenv)
	if err == nil {
		t.Fatal("expected error for missing env variable, got nil")
	}
}

func TestResolve_EnvRef_Allowlisted(t *testing.T) {
	getenv := makeGetenv(map[string]string{"MY_KEY": "allowed_val"})
	cfg := &SecretsConfig{
		Providers: map[string]*ProviderConfig{
			"default": {Source: "env", Allowlist: []string{"MY_KEY"}},
		},
	}
	input := SecretInput{Ref: &SecretRef{Source: "env", Provider: "default", ID: "MY_KEY"}}
	got, err := ResolveSecretInput(input, cfg, getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "allowed_val" {
		t.Errorf("got %q, want %q", got, "allowed_val")
	}
}

func TestResolve_EnvRef_NotAllowlisted(t *testing.T) {
	getenv := makeGetenv(map[string]string{"MY_KEY": "some_val"})
	cfg := &SecretsConfig{
		Providers: map[string]*ProviderConfig{
			"default": {Source: "env", Allowlist: []string{"OTHER"}},
		},
	}
	input := SecretInput{Ref: &SecretRef{Source: "env", Provider: "default", ID: "MY_KEY"}}
	_, err := ResolveSecretInput(input, cfg, getenv)
	if err == nil {
		t.Fatal("expected error for non-allowlisted key, got nil")
	}
	want := `environment variable "MY_KEY" is not allowlisted in provider`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolve_UnknownSource(t *testing.T) {
	getenv := makeGetenv(map[string]string{})
	cfg := &SecretsConfig{
		Providers: map[string]*ProviderConfig{
			"default": {Source: "unknown"},
		},
	}
	input := SecretInput{Ref: &SecretRef{Source: "unknown", Provider: "default", ID: "some_id"}}
	_, err := ResolveSecretInput(input, cfg, getenv)
	if err == nil {
		t.Fatal("expected error for unknown source, got nil")
	}
}

func TestResolve_ProviderNotConfigured(t *testing.T) {
	getenv := makeGetenv(map[string]string{})
	cfg := &SecretsConfig{
		Providers: map[string]*ProviderConfig{},
	}
	input := SecretInput{Ref: &SecretRef{Source: "file", Provider: "nonexistent", ID: "/some/path"}}
	_, err := ResolveSecretInput(input, cfg, getenv)
	if err == nil {
		t.Fatal("expected error for non-configured provider, got nil")
	}
	want := `secret provider "nonexistent" is not configured (ref: file:nonexistent:/some/path)`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}
