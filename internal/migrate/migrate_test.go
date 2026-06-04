// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package migrate

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
)

// withTempConfigDir points GetConfigDir at a fresh temp dir and returns it with a Root.
func withTempConfigDir(t *testing.T) (string, larkauth.Root) {
	t.Helper()
	keyring.MockInit()
	cfgDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", cfgDir)
	t.Setenv("HOME", t.TempDir())
	return cfgDir, larkauth.NewLocalRoot(cfgDir)
}

// writeLegacyConfigRaw writes JSON directly, bypassing SaveMultiAppConfig
// which would stamp SchemaVersion=1.
func writeLegacyConfigRaw(t *testing.T, json string) {
	t.Helper()
	cfgPath := core.GetConfigPath()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(json), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// TestMaybeMigrate_NoConfig_NoOp tests fresh install with no config.json.
func TestMaybeMigrate_NoConfig_NoOp(t *testing.T) {
	_, root := withTempConfigDir(t)
	err := MaybeMigrate(root, io.Discard)
	if !IsNoOp(err) {
		t.Errorf("expected noOp, got %v", err)
	}
}

// TestMaybeMigrate_AlreadyCurrent_NoOp tests schema already at v1.
func TestMaybeMigrate_AlreadyCurrent_NoOp(t *testing.T) {
	_, root := withTempConfigDir(t)
	multi := &core.MultiAppConfig{
		SchemaVersion: core.CurrentSchemaVersion,
		CurrentApp:    "p",
		Apps: []core.AppConfig{{
			Name: "p", AppId: "app-x", Brand: core.BrandFeishu,
			Users: []core.AppUser{{UserOpenId: "ou_a", UserName: "Alice"}},
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}
	if err := MaybeMigrate(root, io.Discard); !IsNoOp(err) {
		t.Errorf("expected noOp, got %v", err)
	}
}

// TestMaybeMigrate_LegacyConfig_StampsSchemaAndBackfillsSidecarAndIndex covers the headline migration path.
func TestMaybeMigrate_LegacyConfig_StampsSchemaAndBackfillsSidecarAndIndex(t *testing.T) {
	cfgDir, root := withTempConfigDir(t)
	writeLegacyConfigRaw(t, `{
  "currentApp": "p",
  "apps": [
    {
      "name": "p",
      "appId": "app-x",
      "appSecret": "secret",
      "brand": "lark",
      "users": [
        {"userOpenId": "ou_a", "userName": "Alice"},
        {"userOpenId": "ou_b", "userName": "Bob"}
      ]
    }
  ]
}`)

	// Sanity check: legacy file really has SchemaVersion=0.
	pre, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("Load (pre): %v", err)
	}
	if pre.SchemaVersion != 0 {
		t.Fatalf("pre-migrate SchemaVersion = %d, want 0", pre.SchemaVersion)
	}

	var warns bytes.Buffer
	if err := MaybeMigrate(root, &warns); err != nil {
		t.Fatalf("MaybeMigrate: %v", err)
	}

	post, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("Load (post): %v", err)
	}
	if post.SchemaVersion != core.CurrentSchemaVersion {
		t.Errorf("post-migrate SchemaVersion = %d, want %d", post.SchemaVersion, core.CurrentSchemaVersion)
	}

	for _, u := range post.Apps[0].Users {
		if u.FirstAuthAt == nil {
			t.Errorf("AppUser %s: FirstAuthAt nil after migrate", u.UserOpenId)
		}
	}

	for _, oid := range []string{"ou_a", "ou_b"} {
		path := filepath.Join(cfgDir, "users", "app-x", oid, "user_profile.json")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("sidecar profile missing at %s: %v", path, err)
		}
	}

	entries, err := larkauth.UserIndexEntries(root)
	if err != nil {
		t.Fatalf("UserIndexEntries: %v", err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.UserOpenId] = true
	}
	for _, oid := range []string{"ou_a", "ou_b"} {
		if !seen[oid] {
			t.Errorf("index row missing for %s; got: %#v", oid, entries)
		}
	}

	if warns.Len() > 0 {
		t.Errorf("unexpected WARN output: %s", warns.String())
	}
}

// TestMaybeMigrate_Idempotent: second run is a noOp and leaves state unchanged.
func TestMaybeMigrate_Idempotent(t *testing.T) {
	_, root := withTempConfigDir(t)
	writeLegacyConfigRaw(t, `{
  "currentApp": "p",
  "apps": [{"name":"p","appId":"app-x","appSecret":"s","brand":"lark","users":[{"userOpenId":"ou_a","userName":"Alice"}]}]
}`)

	if err := MaybeMigrate(root, io.Discard); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	first, _ := core.LoadMultiAppConfig()
	firstFirstAuthAt := first.Apps[0].Users[0].FirstAuthAt
	if firstFirstAuthAt == nil {
		t.Fatal("first migrate did not stamp FirstAuthAt")
	}

	// Second run: should be noOp because schema is now current.
	err := MaybeMigrate(root, io.Discard)
	if !IsNoOp(err) {
		t.Errorf("second migrate: expected noOp, got %v", err)
	}

	second, _ := core.LoadMultiAppConfig()
	if second.Apps[0].Users[0].FirstAuthAt == nil ||
		!second.Apps[0].Users[0].FirstAuthAt.Equal(*firstFirstAuthAt) {
		t.Errorf("FirstAuthAt mutated by no-op rerun: was %v, now %v",
			firstFirstAuthAt, second.Apps[0].Users[0].FirstAuthAt)
	}
}

// TestMaybeMigrate_PreservesExistingFirstAuthAt: pre-populated FirstAuthAt must not be clobbered.
func TestMaybeMigrate_PreservesExistingFirstAuthAt(t *testing.T) {
	_, root := withTempConfigDir(t)
	pinned := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	writeLegacyConfigRaw(t, `{
  "currentApp": "p",
  "apps": [{
    "name":"p","appId":"app-x","appSecret":"s","brand":"lark",
    "users":[{"userOpenId":"ou_a","userName":"Alice","firstAuthAt":"`+pinned+`"}]
  }]
}`)

	if err := MaybeMigrate(root, io.Discard); err != nil {
		t.Fatalf("MaybeMigrate: %v", err)
	}
	got, _ := core.LoadMultiAppConfig()
	gotTS := got.Apps[0].Users[0].FirstAuthAt
	if gotTS == nil {
		t.Fatal("FirstAuthAt nil after migrate")
	}
	want, _ := time.Parse(time.RFC3339Nano, pinned)
	if !gotTS.Equal(want) {
		t.Errorf("FirstAuthAt = %v, want %v (preserved)", gotTS, want)
	}
}

// TestMaybeMigrate_PreservesExistingSidecar: an existing sidecar may have richer data
// (real CachedAt/FirstAuthAt) than the synthesized one, so it must not be overwritten.
func TestMaybeMigrate_PreservesExistingSidecar(t *testing.T) {
	_, root := withTempConfigDir(t)
	writeLegacyConfigRaw(t, `{
  "currentApp": "p",
  "apps": [{"name":"p","appId":"app-x","appSecret":"s","brand":"lark","users":[{"userOpenId":"ou_a","userName":"Alice"}]}]
}`)
	pinned := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	ctx := larkauth.ForUser("app-x", "ou_a")
	if err := larkauth.SaveUserProfileFor(root, ctx, larkauth.UserProfile{
		UserOpenId: "ou_a", UserName: "Alice (cached)", CachedAt: pinned, FirstAuthAt: pinned,
	}); err != nil {
		t.Fatalf("SaveUserProfileFor: %v", err)
	}

	if err := MaybeMigrate(root, io.Discard); err != nil {
		t.Fatalf("MaybeMigrate: %v", err)
	}
	got, err := larkauth.LoadUserProfileFor(root, ctx)
	if err != nil {
		t.Fatalf("LoadUserProfileFor: %v", err)
	}
	if got == nil {
		t.Fatal("sidecar deleted by migrator")
	}
	if !got.FirstAuthAt.Equal(pinned) {
		t.Errorf("sidecar FirstAuthAt = %v, want %v (preserved)", got.FirstAuthAt, pinned)
	}
	if got.UserName != "Alice (cached)" {
		t.Errorf("sidecar UserName = %q, want %q (preserved)", got.UserName, "Alice (cached)")
	}
}

// TestMaybeMigrate_NoUsers_StillStampsSchema: schema stamp on empty Users keeps
// subsequent loads from re-triggering the migrator.
func TestMaybeMigrate_NoUsers_StillStampsSchema(t *testing.T) {
	_, root := withTempConfigDir(t)
	writeLegacyConfigRaw(t, `{
  "currentApp": "p",
  "apps": [{"name":"p","appId":"app-x","appSecret":"s","brand":"lark","users":[]}]
}`)
	if err := MaybeMigrate(root, io.Discard); err != nil {
		t.Fatalf("MaybeMigrate: %v", err)
	}
	got, _ := core.LoadMultiAppConfig()
	if got.SchemaVersion != core.CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", got.SchemaVersion, core.CurrentSchemaVersion)
	}
}

// TestMaybeMigrate_SkipsUsersWithEmptyOpenId: empty UserOpenId is the gate;
// such rows are left alone, not stamped or sidecared.
func TestMaybeMigrate_SkipsUsersWithEmptyOpenId(t *testing.T) {
	cfgDir, root := withTempConfigDir(t)
	writeLegacyConfigRaw(t, `{
  "currentApp": "p",
  "apps": [{"name":"p","appId":"app-x","appSecret":"s","brand":"lark","users":[{"userOpenId":"","userName":"Ghost"},{"userOpenId":"ou_a","userName":"Alice"}]}]
}`)
	if err := MaybeMigrate(root, io.Discard); err != nil {
		t.Fatalf("MaybeMigrate: %v", err)
	}
	got, _ := core.LoadMultiAppConfig()
	if got.Apps[0].Users[0].FirstAuthAt != nil {
		t.Error("ghost AppUser was stamped despite empty open_id")
	}
	if got.Apps[0].Users[1].FirstAuthAt == nil {
		t.Error("Alice was NOT stamped despite valid open_id")
	}
	ghostDir := filepath.Join(cfgDir, "users", "app-x", "")
	entries, err := os.ReadDir(ghostDir)
	if err != nil {
		t.Fatalf("ReadDir users/app-x: %v", err)
	}
	for _, e := range entries {
		if e.Name() == "" {
			t.Error("migrator created a sidecar dir for empty-openId user")
		}
	}
}

// TestIsNoOp covers the sentinel API.
func TestIsNoOp(t *testing.T) {
	if !IsNoOp(noOp) {
		t.Error("IsNoOp(noOp) = false, want true")
	}
	if IsNoOp(errors.New("other")) {
		t.Error("IsNoOp(other) = true, want false")
	}
	if IsNoOp(nil) {
		t.Error("IsNoOp(nil) = true, want false")
	}
}

// TestMaybeMigrate_ForwardIncompatSchema_NoOp: when SchemaVersion > current,
// LoadMultiAppConfig returns *core.ConfigError and the migrator must treat that
// as a silent no-op so bootstrap (which swallows the return) doesn't crash or
// emit warnings on every invocation. The next config-touching command surfaces
// the upgrade hint at its own load site.
//
// Symmetric to TestLoadMultiAppConfig_RejectsForwardIncompatSchema in internal/core.
func TestMaybeMigrate_ForwardIncompatSchema_NoOp(t *testing.T) {
	_, root := withTempConfigDir(t)
	writeLegacyConfigRaw(t, `{
		"schemaVersion": 99,
		"apps": [
			{"appId":"cli_x","appSecret":"s","brand":"feishu","users":[{"userOpenId":"ou_a","userName":"Alice"}]}
		]
	}`)

	var errBuf bytes.Buffer
	err := MaybeMigrate(root, &errBuf)
	if !IsNoOp(err) {
		t.Fatalf("MaybeMigrate on SchemaVersion=99: want noOp, got %v", err)
	}
	// Migrator must stay silent: bootstrap fires this on every invocation.
	if errBuf.Len() != 0 {
		t.Errorf("MaybeMigrate emitted unexpected output on forward-incompat config: %q", errBuf.String())
	}

	// On-disk config must remain untouched. Read raw rather than via the
	// now-rejecting loader.
	raw, err := os.ReadFile(filepath.Join(core.GetConfigDir(), "config.json"))
	if err != nil {
		t.Fatalf("read raw config: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"schemaVersion": 99`)) {
		t.Errorf("MaybeMigrate mutated a forward-incompat config: %s", raw)
	}
}
