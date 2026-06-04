// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Read-side back-compat: legacy AppUser JSON must unmarshal with all new fields zero.
func TestAppUser_LegacyConfigStillUnmarshals(t *testing.T) {
	legacy := []byte(`{"userOpenId":"ou_alice","userName":"Alice"}`)

	var got AppUser
	if err := json.Unmarshal(legacy, &got); err != nil {
		t.Fatalf("unmarshal legacy AppUser: %v", err)
	}
	if got.UserOpenId != "ou_alice" || got.UserName != "Alice" {
		t.Errorf("legacy fields lost: got %+v", got)
	}
	if got.UnionId != "" {
		t.Errorf("UnionId = %q, want empty (legacy has none)", got.UnionId)
	}
	if got.CachedAt != nil {
		t.Errorf("CachedAt = %v, want nil", got.CachedAt)
	}
	if got.FirstAuthAt != nil {
		t.Errorf("FirstAuthAt = %v, want nil", got.FirstAuthAt)
	}
	if got.LastUsed != nil {
		t.Errorf("LastUsed = %v, want nil", got.LastUsed)
	}
	if got.LastScopes != "" {
		t.Errorf("LastScopes = %q, want empty", got.LastScopes)
	}
}

// Write-side back-compat: zero-valued new fields must not appear in JSON (omitempty contract).
func TestAppUser_NewFieldsOmittedWhenZero(t *testing.T) {
	u := AppUser{UserOpenId: "ou_a", UserName: "Alice"}
	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)
	for _, banned := range []string{"unionId", "cachedAt", "firstAuthAt", "lastUsed", "lastScopes"} {
		if strings.Contains(got, banned) {
			t.Errorf("zero-valued AppUser JSON contains %q: %s", banned, got)
		}
	}
}

func TestAppUser_NewFieldsRoundTrip(t *testing.T) {
	cached := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	first := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

	in := AppUser{
		UserOpenId:  "ou_a",
		UserName:    "Alice",
		UnionId:     "on_a",
		CachedAt:    &cached,
		FirstAuthAt: &first,
		LastUsed:    &last,
		LastScopes:  "im:message,im:resource",
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got AppUser
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.UnionId != in.UnionId {
		t.Errorf("UnionId: got %q want %q", got.UnionId, in.UnionId)
	}
	if got.CachedAt == nil || !got.CachedAt.Equal(*in.CachedAt) {
		t.Errorf("CachedAt: got %v want %v", got.CachedAt, in.CachedAt)
	}
	if got.FirstAuthAt == nil || !got.FirstAuthAt.Equal(*in.FirstAuthAt) {
		t.Errorf("FirstAuthAt: got %v want %v", got.FirstAuthAt, in.FirstAuthAt)
	}
	if got.LastUsed == nil || !got.LastUsed.Equal(*in.LastUsed) {
		t.Errorf("LastUsed: got %v want %v", got.LastUsed, in.LastUsed)
	}
	if got.LastScopes != in.LastScopes {
		t.Errorf("LastScopes: got %q want %q", got.LastScopes, in.LastScopes)
	}
}

// Empty CurrentUser must be omitted from JSON to match Lang/StrictMode behaviour.
func TestAppConfig_CurrentUserOmitEmpty(t *testing.T) {
	app := AppConfig{
		AppId: "cli_x", AppSecret: PlainSecret("s"),
		Brand: BrandFeishu, Users: []AppUser{},
	}
	data, err := json.Marshal(app)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, exists := raw["currentUser"]; exists {
		t.Error("expected currentUser to be omitted when empty")
	}
}

// CurrentUser must round-trip — three-level resolution falls back to Users[0] without it.
func TestAppConfig_CurrentUserRoundTrip(t *testing.T) {
	app := AppConfig{
		AppId: "cli_x", AppSecret: PlainSecret("s"),
		Brand:       BrandFeishu,
		CurrentUser: "ou_alice",
		Users: []AppUser{
			{UserOpenId: "ou_alice", UserName: "Alice"},
			{UserOpenId: "ou_bob", UserName: "Bob"},
		},
	}
	data, err := json.Marshal(app)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got AppConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.CurrentUser != "ou_alice" {
		t.Errorf("CurrentUser = %q, want ou_alice", got.CurrentUser)
	}
}

// Legacy AppConfig (no currentUser) loads with CurrentUser zero — Users[0] becomes the default.
func TestAppConfig_LegacyConfigStillUnmarshals(t *testing.T) {
	legacy := []byte(`{
		"appId": "cli_x",
		"appSecret": "s",
		"brand": "feishu",
		"users": [
			{"userOpenId":"ou_a","userName":"Alice"}
		]
	}`)
	var got AppConfig
	if err := json.Unmarshal(legacy, &got); err != nil {
		t.Fatalf("unmarshal legacy AppConfig: %v", err)
	}
	if got.CurrentUser != "" {
		t.Errorf("CurrentUser = %q, want empty (legacy has none)", got.CurrentUser)
	}
	if len(got.Users) != 1 || got.Users[0].UserOpenId != "ou_a" {
		t.Errorf("Users[] not preserved: %+v", got.Users)
	}
}

// SchemaVersion=0 must be omitted from JSON; 0 is the legacy-file marker.
// SaveMultiAppConfig is the authoritative writer that stamps the live version.
func TestMultiAppConfig_SchemaVersionOmitEmpty(t *testing.T) {
	cfg := MultiAppConfig{Apps: []AppConfig{{
		AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
	}}}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, exists := raw["schemaVersion"]; exists {
		t.Error("expected schemaVersion to be omitted when zero")
	}
}

// Legacy file (no schemaVersion) must unmarshal as version 0 so the migrator fires once on upgrade.
func TestMultiAppConfig_LegacyFileLoadsAsVersionZero(t *testing.T) {
	legacy := []byte(`{
		"currentApp": "alpha",
		"apps": [
			{"appId":"cli_x","appSecret":"s","brand":"feishu","users":[]}
		]
	}`)
	var got MultiAppConfig
	if err := json.Unmarshal(legacy, &got); err != nil {
		t.Fatalf("unmarshal legacy MultiAppConfig: %v", err)
	}
	if got.SchemaVersion != 0 {
		t.Errorf("SchemaVersion = %d on legacy file, want 0", got.SchemaVersion)
	}
	if got.CurrentApp != "alpha" {
		t.Errorf("CurrentApp = %q, want alpha", got.CurrentApp)
	}
	if len(got.Apps) != 1 {
		t.Errorf("Apps count = %d, want 1", len(got.Apps))
	}
}

func TestMultiAppConfig_SchemaVersionRoundTrip(t *testing.T) {
	cfg := MultiAppConfig{
		SchemaVersion: 1,
		CurrentApp:    "alpha",
		Apps:          []AppConfig{{AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu}},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got MultiAppConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", got.SchemaVersion)
	}
}

// SaveMultiAppConfig must stamp SchemaVersion forward to CurrentSchemaVersion.
func TestSaveMultiAppConfig_StampsSchemaVersion(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	cfg := &MultiAppConfig{
		// SchemaVersion zero — simulates a freshly-loaded legacy file.
		Apps: []AppConfig{{
			AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
			Users: []AppUser{{UserOpenId: "ou_a", UserName: "Alice"}},
		}},
	}
	if err := SaveMultiAppConfig(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if cfg.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("after Save: SchemaVersion = %d, want %d", cfg.SchemaVersion, CurrentSchemaVersion)
	}

	got, err := LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("on-disk SchemaVersion = %d, want %d", got.SchemaVersion, CurrentSchemaVersion)
	}
}

// Forward-compat: Save must not downgrade a future SchemaVersion (only the migrator changes it).
func TestSaveMultiAppConfig_DoesNotDowngradeSchemaVersion(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	cfg := &MultiAppConfig{
		SchemaVersion: 99, // pretend a future build wrote this
		Apps: []AppConfig{{
			AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
		}},
	}
	if err := SaveMultiAppConfig(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if cfg.SchemaVersion != 99 {
		t.Errorf("Save downgraded SchemaVersion: got %d, want preserved 99", cfg.SchemaVersion)
	}
}

// Unknown JSON fields must be tolerated so rolling forward then back doesn't brick the config.
func TestMultiAppConfig_UnknownFieldsTolerated(t *testing.T) {
	withFuture := []byte(`{
		"schemaVersion": 1,
		"currentApp": "alpha",
		"futurePolicy": {"x": 1},
		"apps": [
			{"appId":"cli_x","appSecret":"s","brand":"feishu","users":[]}
		]
	}`)
	var got MultiAppConfig
	if err := json.Unmarshal(withFuture, &got); err != nil {
		t.Fatalf("unmarshal with unknown future field: %v", err)
	}
	if got.SchemaVersion != 1 || got.CurrentApp != "alpha" {
		t.Errorf("unexpected parse result: %+v", got)
	}
}

// Forward-incompat schema must reject with a *ConfigError (code 3, type "config")
// so the root command's exit-error adapter renders the structured envelope and upgrade hint.
func TestLoadMultiAppConfig_RejectsForwardIncompatSchema(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	// Bypass SaveMultiAppConfig so we can stamp a version this binary wouldn't produce.
	future := []byte(`{
		"schemaVersion": 99,
		"apps": [
			{"appId":"cli_x","appSecret":"s","brand":"feishu","users":[{"userOpenId":"ou_a","userName":"Alice"}]}
		]
	}`)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), future, 0600); err != nil {
		t.Fatalf("write future config: %v", err)
	}

	got, err := LoadMultiAppConfig()
	if err == nil {
		t.Fatalf("LoadMultiAppConfig: want error for SchemaVersion=99, got nil; result=%+v", got)
	}
	if got != nil {
		t.Errorf("LoadMultiAppConfig: want nil result alongside error, got %+v", got)
	}

	var ce *ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("LoadMultiAppConfig: want *ConfigError, got %T: %v", err, err)
	}
	if ce.Code != 3 || ce.Type != "config" {
		t.Errorf("ConfigError shape mismatch: code=%d type=%q (want 3 / \"config\")", ce.Code, ce.Type)
	}
	// Message must name both versions so the user knows whether to upgrade or use --profile.
	if !strings.Contains(ce.Message, "99") || !strings.Contains(ce.Message, fmt.Sprint(CurrentSchemaVersion)) {
		t.Errorf("Message does not mention both versions: %q", ce.Message)
	}
	if ce.Hint == "" {
		t.Errorf("ConfigError.Hint must be populated so AI agents and humans see the next-step guidance")
	}
}

// Regression guard paired with the rejection test: a file at exactly CurrentSchemaVersion loads.
func TestLoadMultiAppConfig_AcceptsCurrentSchema(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	cfg := &MultiAppConfig{
		Apps: []AppConfig{{
			AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
			Users: []AppUser{{UserOpenId: "ou_a", UserName: "Alice"}},
		}},
	}
	if err := SaveMultiAppConfig(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig at CurrentSchemaVersion: unexpected error: %v", err)
	}
	if got.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("loaded SchemaVersion = %d, want %d", got.SchemaVersion, CurrentSchemaVersion)
	}
}

// Legacy configs (no schemaVersion) must load unchanged; the migrator stamps them forward.
func TestLoadMultiAppConfig_AcceptsLegacyZeroSchema(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	legacy := []byte(`{
		"apps": [
			{"appId":"cli_x","appSecret":"s","brand":"feishu","users":[{"userOpenId":"ou_a","userName":"Alice"}]}
		]
	}`)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), legacy, 0600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	got, err := LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig on legacy SchemaVersion=0: unexpected error: %v", err)
	}
	if got.SchemaVersion != 0 {
		t.Errorf("legacy file should load with SchemaVersion=0, got %d", got.SchemaVersion)
	}
}
