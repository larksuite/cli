// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"encoding/json"
	"testing"
)

func TestSecretInput_MarshalJSON_PlainString(t *testing.T) {
	input := SecretInput{Plain: "my_secret"}
	data, err := input.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `"my_secret"`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestSecretInput_MarshalJSON_SecretRef(t *testing.T) {
	input := SecretInput{Ref: &SecretRef{Source: "env", ID: "MY_VAR"}}
	data, err := input.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var ref SecretRef
	if err := json.Unmarshal(data, &ref); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ref.Source != "env" {
		t.Errorf("source = %q, want %q", ref.Source, "env")
	}
	if ref.ID != "MY_VAR" {
		t.Errorf("id = %q, want %q", ref.ID, "MY_VAR")
	}
}

func TestSecretInput_UnmarshalJSON_InvalidSource(t *testing.T) {
	data := []byte(`{"source":"invalid","id":"key"}`)
	var input SecretInput
	err := json.Unmarshal(data, &input)
	if err == nil {
		t.Fatal("expected error for invalid source, got nil")
	}
}

func TestSecretInput_UnmarshalJSON_EmptyID(t *testing.T) {
	data := []byte(`{"source":"env","id":""}`)
	var input SecretInput
	err := json.Unmarshal(data, &input)
	if err == nil {
		t.Fatal("expected error for empty id, got nil")
	}
}

func TestSecretInput_UnmarshalJSON_InvalidType(t *testing.T) {
	data := []byte(`42`)
	var input SecretInput
	err := json.Unmarshal(data, &input)
	if err == nil {
		t.Fatal("expected error for numeric input, got nil")
	}
	want := "appSecret must be a string or {source, provider?, id} object"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveDefaultProvider_ExplicitProvider(t *testing.T) {
	ref := &SecretRef{Source: "env", Provider: "my-custom", ID: "KEY"}
	got := ResolveDefaultProvider(ref, nil)
	if got != "my-custom" {
		t.Errorf("got %q, want %q", got, "my-custom")
	}
}

func TestResolveDefaultProvider_FromDefaults(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		defaults *ProviderDefaults
		want     string
	}{
		{
			name:     "env default",
			source:   "env",
			defaults: &ProviderDefaults{Env: "my-env-prov"},
			want:     "my-env-prov",
		},
		{
			name:     "file default",
			source:   "file",
			defaults: &ProviderDefaults{File: "my-file-prov"},
			want:     "my-file-prov",
		},
		{
			name:     "exec default",
			source:   "exec",
			defaults: &ProviderDefaults{Exec: "my-exec-prov"},
			want:     "my-exec-prov",
		},
		{
			name:     "no defaults configured",
			source:   "env",
			defaults: &ProviderDefaults{},
			want:     DefaultProviderAlias,
		},
		{
			name:     "nil defaults",
			source:   "env",
			defaults: nil,
			want:     DefaultProviderAlias,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := &SecretRef{Source: tt.source, ID: "KEY"}
			cfg := &SecretsConfig{Defaults: tt.defaults}
			got := ResolveDefaultProvider(ref, cfg)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveDefaultProvider_NilConfig(t *testing.T) {
	ref := &SecretRef{Source: "env", ID: "KEY"}
	got := ResolveDefaultProvider(ref, nil)
	if got != DefaultProviderAlias {
		t.Errorf("got %q, want %q", got, DefaultProviderAlias)
	}
}

func TestLookupProvider_SourceMismatch(t *testing.T) {
	cfg := &SecretsConfig{
		Providers: map[string]*ProviderConfig{
			"default": {Source: "file"},
		},
	}
	ref := &SecretRef{Source: "env", ID: "KEY"}
	_, err := LookupProvider(ref, cfg)
	if err == nil {
		t.Fatal("expected error for source mismatch, got nil")
	}
	want := `secret provider "default" has source "file" but ref requests "env"`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestLookupProvider_ImplicitDefaultEnv(t *testing.T) {
	// Default env provider is implicitly available even without explicit config
	ref := &SecretRef{Source: "env", ID: "KEY"}
	pc, err := LookupProvider(ref, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pc.Source != "env" {
		t.Errorf("source = %q, want %q", pc.Source, "env")
	}
}

func TestListCandidateApps_NilChannel(t *testing.T) {
	got := ListCandidateApps(nil)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestListCandidateApps_SingleAccount(t *testing.T) {
	ch := &FeishuChannel{
		AppID:     "cli_single",
		AppSecret: SecretInput{Plain: "secret"},
		Brand:     "feishu",
	}
	got := ListCandidateApps(ch)
	if len(got) != 1 {
		t.Fatalf("count = %d, want 1", len(got))
	}
	if got[0].AppID != "cli_single" {
		t.Errorf("appId = %q, want %q", got[0].AppID, "cli_single")
	}
	if got[0].Label != "" {
		t.Errorf("label = %q, want empty", got[0].Label)
	}
	if got[0].Brand != "feishu" {
		t.Errorf("brand = %q, want %q", got[0].Brand, "feishu")
	}
}

func TestListCandidateApps_SingleAccount_Disabled(t *testing.T) {
	disabled := false
	ch := &FeishuChannel{
		Enabled:   &disabled,
		AppID:     "cli_disabled",
		AppSecret: SecretInput{Plain: "secret"},
	}
	got := ListCandidateApps(ch)
	if len(got) != 0 {
		t.Errorf("expected 0 apps for disabled channel, got %d", len(got))
	}
}

func TestListCandidateApps_MultiAccount_InheritTopLevel(t *testing.T) {
	ch := &FeishuChannel{
		AppID: "cli_top_level",
		Brand: "lark",
		Accounts: map[string]*FeishuAccount{
			"work": {
				// No AppID → inherits from top-level
				AppSecret: SecretInput{Plain: "secret"},
				// No Brand → inherits from top-level
			},
		},
	}
	got := ListCandidateApps(ch)
	if len(got) != 1 {
		t.Fatalf("count = %d, want 1", len(got))
	}
	if got[0].AppID != "cli_top_level" {
		t.Errorf("inherited appId = %q, want %q", got[0].AppID, "cli_top_level")
	}
	if got[0].Brand != "lark" {
		t.Errorf("inherited brand = %q, want %q", got[0].Brand, "lark")
	}
	if got[0].Label != "work" {
		t.Errorf("label = %q, want %q", got[0].Label, "work")
	}
}

func TestListCandidateApps_MultiAccount_InheritAppSecret(t *testing.T) {
	// Reproduces the "default": {} edge case from real openclaw.json configs
	// where an empty account object should inherit appSecret from the top-level channel.
	ch := &FeishuChannel{
		AppID:     "cli_fake_top_level",
		AppSecret: SecretInput{Plain: "fake_top_level_secret"},
		Brand:     "feishu",
		Accounts: map[string]*FeishuAccount{
			"default": {}, // empty — should inherit everything from top-level
			"other": {
				Enabled:   boolPtr(true),
				AppID:     "cli_fake_other",
				AppSecret: SecretInput{Plain: "fake_other_secret"},
			},
		},
	}
	got := ListCandidateApps(ch)
	if len(got) != 2 {
		t.Fatalf("count = %d, want 2", len(got))
	}
	// Find the "default" account
	var def *CandidateApp
	for i := range got {
		if got[i].Label == "default" {
			def = &got[i]
		}
	}
	if def == nil {
		t.Fatal("default account not found in candidates")
	}
	if def.AppID != "cli_fake_top_level" {
		t.Errorf("default appId = %q, want inherited top-level", def.AppID)
	}
	if def.AppSecret.IsZero() {
		t.Error("default appSecret should inherit from top-level, got zero")
	}
	if def.AppSecret.Plain != "fake_top_level_secret" {
		t.Errorf("default appSecret = %q, want inherited top-level", def.AppSecret.Plain)
	}
	if def.Brand != "feishu" {
		t.Errorf("default brand = %q, want inherited top-level", def.Brand)
	}
}

func TestListCandidateApps_ImplicitDefault_WhenTopLevelHasCredentials(t *testing.T) {
	// When accounts exist but none is named "default", and top-level has
	// its own appId+appSecret, the top-level should be included as a
	// synthetic "default" candidate (aligned with openclaw-lark plugin).
	ch := &FeishuChannel{
		AppID:     "cli_top",
		AppSecret: SecretInput{Plain: "top_secret"},
		Brand:     "feishu",
		Accounts: map[string]*FeishuAccount{
			"ethan": {
				AppID:     "cli_ethan",
				AppSecret: SecretInput{Plain: "ethan_secret"},
				Brand:     "lark",
			},
		},
	}
	got := ListCandidateApps(ch)
	if len(got) != 2 {
		t.Fatalf("count = %d, want 2 (default + ethan)", len(got))
	}
	var def, ethan *CandidateApp
	for i := range got {
		switch got[i].Label {
		case "default":
			def = &got[i]
		case "ethan":
			ethan = &got[i]
		}
	}
	if def == nil {
		t.Fatal("implicit default candidate not found")
	}
	if def.AppID != "cli_top" {
		t.Errorf("default appId = %q, want %q", def.AppID, "cli_top")
	}
	if ethan == nil {
		t.Fatal("ethan candidate not found")
	}
	if ethan.AppID != "cli_ethan" {
		t.Errorf("ethan appId = %q, want %q", ethan.AppID, "cli_ethan")
	}
}

func TestListCandidateApps_NoImplicitDefault_WhenExplicitDefaultExists(t *testing.T) {
	// When accounts already contain a "default" entry, don't duplicate it.
	ch := &FeishuChannel{
		AppID:     "cli_top",
		AppSecret: SecretInput{Plain: "top_secret"},
		Accounts: map[string]*FeishuAccount{
			"default": {}, // inherits top-level
			"other":   {AppID: "cli_other", AppSecret: SecretInput{Plain: "s"}},
		},
	}
	got := ListCandidateApps(ch)
	defaultCount := 0
	for _, c := range got {
		if c.Label == "default" {
			defaultCount++
		}
	}
	if defaultCount != 1 {
		t.Errorf("expected exactly 1 default candidate, got %d", defaultCount)
	}
}

func TestListCandidateApps_NoImplicitDefault_WhenTopLevelMissingSecret(t *testing.T) {
	// Top-level has appId but no appSecret → no implicit default.
	ch := &FeishuChannel{
		AppID: "cli_top",
		// no appSecret
		Accounts: map[string]*FeishuAccount{
			"ethan": {AppID: "cli_ethan", AppSecret: SecretInput{Plain: "s"}},
		},
	}
	got := ListCandidateApps(ch)
	if len(got) != 1 {
		t.Fatalf("count = %d, want 1 (only ethan)", len(got))
	}
	if got[0].Label != "ethan" {
		t.Errorf("label = %q, want %q", got[0].Label, "ethan")
	}
}

func boolPtr(v bool) *bool { return &v }

func TestListCandidateApps_MultiAccount_DisabledFiltered(t *testing.T) {
	disabled := false
	ch := &FeishuChannel{
		Accounts: map[string]*FeishuAccount{
			"active": {
				AppID:     "cli_active",
				AppSecret: SecretInput{Plain: "secret"},
			},
			"disabled": {
				Enabled:   &disabled,
				AppID:     "cli_disabled",
				AppSecret: SecretInput{Plain: "secret"},
			},
			"nil_acct": nil,
		},
	}
	got := ListCandidateApps(ch)
	if len(got) != 1 {
		t.Fatalf("count = %d, want 1 (disabled and nil filtered out)", len(got))
	}
	if got[0].AppID != "cli_active" {
		t.Errorf("appId = %q, want %q", got[0].AppID, "cli_active")
	}
}

func TestListCandidateApps_EmptyAppID(t *testing.T) {
	ch := &FeishuChannel{
		AppID: "",
		// No accounts, no appId → no candidates
	}
	got := ListCandidateApps(ch)
	if len(got) != 0 {
		t.Errorf("expected 0 apps for empty appId, got %d", len(got))
	}
}

func TestIsEnabled_Nil(t *testing.T) {
	if !isEnabled(nil) {
		t.Error("nil should default to enabled")
	}
}

func TestIsEnabled_True(t *testing.T) {
	v := true
	if !isEnabled(&v) {
		t.Error("explicit true should be enabled")
	}
}

func TestIsEnabled_False(t *testing.T) {
	v := false
	if isEnabled(&v) {
		t.Error("explicit false should be disabled")
	}
}
