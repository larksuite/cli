// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"testing"

	"github.com/larksuite/cli/internal/core"
)

func TestTokenTypeString(t *testing.T) {
	tests := []struct {
		tt   TokenType
		want string
	}{
		{TokenTypeUAT, "uat"},
		{TokenTypeTAT, "tat"},
		{TokenType("custom"), "custom"},
	}
	for _, tc := range tests {
		if got := tc.tt.String(); got != tc.want {
			t.Errorf("TokenType(%q).String() = %q, want %q", tc.tt, got, tc.want)
		}
	}
}

func TestParseTokenType(t *testing.T) {
	tests := []struct {
		s    string
		want TokenType
		ok   bool
	}{
		{"uat", TokenTypeUAT, true},
		{"tat", TokenTypeTAT, true},
		{"UAT", TokenTypeUAT, true},
		{"bad", "", false},
	}
	for _, tc := range tests {
		got, ok := ParseTokenType(tc.s)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("ParseTokenType(%q) = (%v, %v), want (%v, %v)", tc.s, got, ok, tc.want, tc.ok)
		}
	}
}

func TestAccountFromCliConfigAndBack_ReturnCopies(t *testing.T) {
	cfg := &core.CliConfig{
		ProfileName:         "target",
		AppID:               "app-1",
		AppSecret:           "secret-1",
		Brand:               core.BrandLark,
		DefaultAs:           "user",
		UserOpenId:          "ou_123",
		UserName:            "alice",
		SupportedIdentities: 3,
	}

	acct := AccountFromCliConfig(cfg)
	if acct == nil {
		t.Fatal("AccountFromCliConfig() = nil")
	}
	if acct.AppID != cfg.AppID || acct.ProfileName != cfg.ProfileName || acct.UserName != cfg.UserName {
		t.Fatalf("AccountFromCliConfig() = %#v, want copied fields from %#v", acct, cfg)
	}

	roundtrip := acct.ToCliConfig()
	if roundtrip == nil {
		t.Fatal("ToCliConfig() = nil")
	}
	if roundtrip.AppID != cfg.AppID || roundtrip.ProfileName != cfg.ProfileName || roundtrip.UserName != cfg.UserName {
		t.Fatalf("ToCliConfig() = %#v, want copied fields from %#v", roundtrip, cfg)
	}

	roundtrip.AppID = "mutated-cli"
	acct.AppID = "mutated-account"

	if cfg.AppID != "app-1" {
		t.Fatalf("cfg.AppID = %q, want original value", cfg.AppID)
	}
	if roundtrip.AppID != "mutated-cli" {
		t.Fatalf("roundtrip.AppID = %q, want mutated value", roundtrip.AppID)
	}
	if acct.AppID != "mutated-account" {
		t.Fatalf("acct.AppID = %q, want mutated value", acct.AppID)
	}
}

func TestAccountToCliConfig_TokenOnlySecretPreservesNoAppSecret(t *testing.T) {
	acct := &Account{
		ProfileName: "env",
		AppID:       "app-1",
		AppSecret:   "",
		Brand:       core.BrandFeishu,
	}

	cfg := acct.ToCliConfig()
	if cfg == nil {
		t.Fatal("ToCliConfig() = nil")
	}
	if cfg.AppSecret != "" {
		t.Fatalf("AppSecret = %q, want empty string", cfg.AppSecret)
	}

	roundtrip := AccountFromCliConfig(cfg)
	if roundtrip == nil {
		t.Fatal("AccountFromCliConfig() = nil")
	}
	if roundtrip.AppSecret != "" {
		t.Fatalf("roundtrip.AppSecret = %q, want empty string", roundtrip.AppSecret)
	}
}

func TestRuntimeAppSecret_TokenOnlyUsesPlaceholder(t *testing.T) {
	if got := RuntimeAppSecret(""); got == "" {
		t.Fatal("RuntimeAppSecret(\"\") = empty, want non-empty placeholder")
	}
	if HasRealAppSecret(RuntimeAppSecret("")) {
		t.Fatalf("HasRealAppSecret(RuntimeAppSecret(\"\")) = true, want false")
	}
	if got := RuntimeAppSecret("secret-1"); got != "secret-1" {
		t.Fatalf("RuntimeAppSecret(real) = %q, want %q", got, "secret-1")
	}
}
