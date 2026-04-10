// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"strings"
	"testing"
)

func TestValidateSecretKeyMatch_KeychainMatches(t *testing.T) {
	secret := SecretInput{Ref: &SecretRef{Source: "keychain", ID: "appsecret:cli_abc123"}}
	if err := ValidateSecretKeyMatch("cli_abc123", secret); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateSecretKeyMatch_KeychainMismatch(t *testing.T) {
	secret := SecretInput{Ref: &SecretRef{Source: "keychain", ID: "appsecret:cli_old_app"}}
	err := ValidateSecretKeyMatch("cli_new_app", secret)
	if err == nil {
		t.Fatal("expected error for mismatched appId and keychain key")
	}
	// Verify the error message contains useful context
	msg := err.Error()
	for _, want := range []string{"cli_old_app", "cli_new_app", "appsecret:cli_new_app", "config init"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q: %s", want, msg)
		}
	}
}

func TestValidateSecretKeyMatch_PlainSecret_Skipped(t *testing.T) {
	secret := PlainSecret("some-secret")
	if err := ValidateSecretKeyMatch("cli_abc123", secret); err != nil {
		t.Errorf("plain secret should be skipped, got: %v", err)
	}
}

func TestValidateSecretKeyMatch_FileRef_Skipped(t *testing.T) {
	secret := SecretInput{Ref: &SecretRef{Source: "file", ID: "/tmp/secret.txt"}}
	if err := ValidateSecretKeyMatch("cli_abc123", secret); err != nil {
		t.Errorf("file ref should be skipped, got: %v", err)
	}
}

func TestValidateSecretKeyMatch_ZeroValue_Skipped(t *testing.T) {
	if err := ValidateSecretKeyMatch("cli_abc123", SecretInput{}); err != nil {
		t.Errorf("zero SecretInput should be skipped, got: %v", err)
	}
}

func TestValidateSecretKeyMatch_EmptyAppId_Mismatch(t *testing.T) {
	secret := SecretInput{Ref: &SecretRef{Source: "keychain", ID: "appsecret:cli_abc123"}}
	err := ValidateSecretKeyMatch("", secret)
	if err == nil {
		t.Fatal("expected error when appId is empty but keychain key references a real app")
	}
}
