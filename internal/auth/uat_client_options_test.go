// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"testing"

	"github.com/larksuite/cli/internal/core"
)

// TestNewUATCallOptions validates the extraction of options from CLI config.
func TestNewUATCallOptions(t *testing.T) {
	cfg := &core.CliConfig{
		AppID:      "app123",
		AppSecret:  "secret",
		Brand:      core.BrandLark,
		UserOpenId: "ou_test",
		AuthMethod: core.AuthMethodPrivateKeyJWTLocalKeyPair, KeyLabel: "key-1",
		DPoPMode: core.DPoPModeRequired, CredentialSource: core.CredentialSourceLocal,
	}
	errOut := &bytes.Buffer{}

	signer := newFakeAuthSigner(t)
	opts := NewUATCallOptions(cfg, errOut, signer)

	if opts.AppId != "app123" {
		t.Errorf("AppId = %q, want app123", opts.AppId)
	}
	if opts.AppSecret != "secret" {
		t.Errorf("AppSecret = %q, want secret", opts.AppSecret)
	}
	if opts.Domain != core.BrandLark {
		t.Errorf("Domain = %q, want lark", opts.Domain)
	}
	if opts.UserOpenId != "ou_test" {
		t.Errorf("UserOpenId = %q, want ou_test", opts.UserOpenId)
	}
	if opts.ErrOut != errOut {
		t.Error("ErrOut not set correctly")
	}
	if opts.AuthMethod != cfg.AuthMethod || opts.KeyLabel != cfg.KeyLabel || opts.Signer != signer {
		t.Fatal("lost signing-key configuration")
	}
	if opts.DPoPMode != cfg.DPoPMode || opts.DPoPKeyStore == nil {
		t.Fatal("lost local DPoP policy")
	}
}
