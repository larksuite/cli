// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestPluginUninstall_Basic(t *testing.T) {
	dir := t.TempDir()
	writeTestPkgJSON(t, dir, map[string]interface{}{
		"actionPlugins": map[string]interface{}{
			"@test/my-plugin": "1.0.0",
		},
	})
	pluginDir := filepath.Join(dir, "node_modules", "@test/my-plugin")
	os.MkdirAll(pluginDir, 0o755)
	os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte("{}"), 0o644)
	chdirTest(t, dir)

	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsPluginUninstall, []string{
		"+plugin-uninstall", "--name", "@test/my-plugin",
		"--format", "json", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify node_modules removed
	if _, err := os.Stat(pluginDir); !os.IsNotExist(err) {
		t.Error("node_modules plugin dir should be removed")
	}

	// Verify package.json updated
	pkg, _ := pluginReadPackageJSON(dir)
	ap := pluginGetActionPlugins(pkg)
	if _, ok := ap["@test/my-plugin"]; ok {
		t.Error("actionPlugins should no longer contain @test/my-plugin")
	}
}

func TestPluginUninstall_NotInstalled(t *testing.T) {
	dir := t.TempDir()
	writeTestPkgJSON(t, dir, map[string]interface{}{})
	chdirTest(t, dir)

	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsPluginUninstall, []string{
		"+plugin-uninstall", "--name", "@test/not-here",
		"--format", "json", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("uninstalling non-existent plugin should succeed: %v", err)
	}

	var env map[string]interface{}
	json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]interface{})
	if data["removed"] != true {
		t.Errorf("removed = %v, want true", data["removed"])
	}
}

func TestPluginUninstall_BlockedByDependentInstance(t *testing.T) {
	dir := t.TempDir()
	writeTestPkgJSON(t, dir, map[string]interface{}{
		"actionPlugins": map[string]interface{}{
			"@test/my-plugin": "1.0.0",
		},
	})
	// Install plugin
	pluginDir := filepath.Join(dir, "node_modules", "@test/my-plugin")
	os.MkdirAll(pluginDir, 0o755)
	os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte("{}"), 0o644)

	// Create a capability that references this plugin
	capDir := filepath.Join(dir, "server", "capabilities")
	os.MkdirAll(capDir, 0o755)
	writeTestCapJSON(t, capDir, "my-instance.json", map[string]interface{}{
		"id":        "my-instance",
		"pluginKey": "@test/my-plugin",
		"name":      "My Instance",
	})
	chdirTest(t, dir)

	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsPluginUninstall, []string{
		"+plugin-uninstall", "--name", "@test/my-plugin",
		"--format", "json", "--as", "user",
	}, factory, stdout)
	if err == nil {
		t.Fatal("expected error when uninstalling a plugin with dependent instances, got nil")
	}

	// Verify plugin directory still exists (blocked)
	if _, err := os.Stat(pluginDir); err != nil {
		t.Errorf("plugin directory should still exist after blocked uninstall: %v", err)
	}

	// Verify error mentions the dependent instance
	prob, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed error, got %v", err)
	}
	if prob.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %s, want %s", prob.Subtype, errs.SubtypeFailedPrecondition)
	}
	if prob.Hint == "" {
		t.Error("hint should be non-empty")
	}
}

func TestPluginUninstall_WithUnrelatedInstances(t *testing.T) {
	dir := t.TempDir()
	writeTestPkgJSON(t, dir, map[string]interface{}{
		"actionPlugins": map[string]interface{}{
			"@test/my-plugin": "1.0.0",
		},
	})
	pluginDir := filepath.Join(dir, "node_modules", "@test/my-plugin")
	os.MkdirAll(pluginDir, 0o755)
	os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte("{}"), 0o644)

	// Create a capability that references a DIFFERENT plugin — should not block
	capDir := filepath.Join(dir, "server", "capabilities")
	os.MkdirAll(capDir, 0o755)
	writeTestCapJSON(t, capDir, "other-instance.json", map[string]interface{}{
		"id":        "other-instance",
		"pluginKey": "@test/other-plugin",
		"name":      "Other Instance",
	})
	chdirTest(t, dir)

	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsPluginUninstall, []string{
		"+plugin-uninstall", "--name", "@test/my-plugin",
		"--format", "json", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("uninstall should succeed when instances reference different plugins: %v", err)
	}

	// Verify plugin was removed
	if _, err := os.Stat(pluginDir); !os.IsNotExist(err) {
		t.Error("plugin directory should be removed")
	}
}

func TestPluginUninstall_PreservesOtherPlugins(t *testing.T) {
	dir := t.TempDir()
	writeTestPkgJSON(t, dir, map[string]interface{}{
		"name": "my-app",
		"actionPlugins": map[string]interface{}{
			"@test/remove-me": "1.0.0",
			"@test/keep-me":   "2.0.0",
		},
	})
	chdirTest(t, dir)

	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsPluginUninstall, []string{
		"+plugin-uninstall", "--name", "@test/remove-me",
		"--format", "json", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pkg, _ := pluginReadPackageJSON(dir)
	ap := pluginGetActionPlugins(pkg)
	if _, ok := ap["@test/remove-me"]; ok {
		t.Error("@test/remove-me should be removed from actionPlugins")
	}
	if v, ok := ap["@test/keep-me"]; !ok || v != "2.0.0" {
		t.Errorf("@test/keep-me should be preserved, got %q", v)
	}
	if name, _ := pkg["name"].(string); name != "my-app" {
		t.Errorf("other fields should be preserved, name = %q", name)
	}
}
