// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPluginList_Empty(t *testing.T) {
	dir := t.TempDir()
	writeTestPkgJSON(t, dir, map[string]interface{}{})
	chdirTest(t, dir)

	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsPluginList, []string{
		"+plugin-list", "--format", "json", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var env map[string]interface{}
	json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]interface{})
	plugins, _ := data["plugins"].([]interface{})
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestPluginList_Installed(t *testing.T) {
	dir := t.TempDir()
	writeTestPkgJSON(t, dir, map[string]interface{}{
		"actionPlugins": map[string]interface{}{
			"@test/my-plugin": "1.0.0",
		},
	})
	manifestDir := filepath.Join(dir, "node_modules", "@test/my-plugin")
	os.MkdirAll(manifestDir, 0o755)
	os.WriteFile(filepath.Join(manifestDir, "package.json"), []byte(`{"version":"1.0.0"}`), 0o644)
	chdirTest(t, dir)

	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsPluginList, []string{
		"+plugin-list", "--format", "json", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var env map[string]interface{}
	json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]interface{})
	plugins, _ := data["plugins"].([]interface{})
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	p := plugins[0].(map[string]interface{})
	if p["status"] != "installed" {
		t.Errorf("status = %v, want installed", p["status"])
	}
}

func TestPluginList_DeclaredNotInstalled(t *testing.T) {
	dir := t.TempDir()
	writeTestPkgJSON(t, dir, map[string]interface{}{
		"actionPlugins": map[string]interface{}{
			"@test/missing": "1.0.0",
		},
	})
	chdirTest(t, dir)

	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsPluginList, []string{
		"+plugin-list", "--format", "json", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var env map[string]interface{}
	json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]interface{})
	plugins, _ := data["plugins"].([]interface{})
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	p := plugins[0].(map[string]interface{})
	if p["status"] != "declared_not_installed" {
		t.Errorf("status = %v, want declared_not_installed", p["status"])
	}
}

// --- helpers ---

func chdirTest(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) }) //nolint:errcheck
}

func writeTestPkgJSON(t *testing.T, dir string, pkg map[string]interface{}) {
	t.Helper()
	data, err := json.Marshal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
