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

// --- pluginResolveProjectPath ---

func TestPluginResolveProjectPath_DefaultToCwd(t *testing.T) {
	got, err := pluginResolveProjectPath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cwd, _ := os.Getwd()
	if got != cwd {
		t.Errorf("got %q, want cwd %q", got, cwd)
	}
}

func TestPluginResolveProjectPath_ExplicitPath(t *testing.T) {
	got, err := pluginResolveProjectPath("/tmp/myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/myapp" {
		t.Errorf("got %q, want /tmp/myapp", got)
	}
}

// --- pluginCheckProjectDir ---

func TestPluginCheckProjectDir_OK(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := pluginCheckProjectDir(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPluginCheckProjectDir_Missing(t *testing.T) {
	dir := t.TempDir()
	err := pluginCheckProjectDir(dir)
	if err == nil {
		t.Fatal("expected error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q, want failed_precondition", p.Subtype)
	}
}

// --- pluginResolveCapDir ---

func TestPluginResolveCapDir_EnvVar(t *testing.T) {
	t.Setenv("MIAODA_CAPABILITIES_DIR", "envdir/caps")
	got, err := pluginResolveCapDir("/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join("/proj", "envdir/caps"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPluginResolveCapDir_AppTypeEnv(t *testing.T) {
	t.Setenv("MIAODA_APP_TYPE", "2")
	got, err := pluginResolveCapDir("/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join("/proj", "server", "capabilities"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPluginResolveCapDir_AppTypeEnvShared(t *testing.T) {
	t.Setenv("MIAODA_APP_TYPE", "6")
	got, err := pluginResolveCapDir("/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join("/proj", "shared", "capabilities"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPluginResolveCapDir_EnvLocal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env.local"), []byte("MIAODA_APP_TYPE=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := pluginResolveCapDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(dir, "server", "capabilities"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPluginResolveCapDir_DetectServer(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "server", "capabilities"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := pluginResolveCapDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(dir, "server", "capabilities"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPluginResolveCapDir_DetectShared(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "shared", "capabilities"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := pluginResolveCapDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join(dir, "shared", "capabilities"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPluginResolveCapDir_Ambiguous(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "server", "capabilities"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "shared", "capabilities"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := pluginResolveCapDir(dir)
	if err == nil {
		t.Fatal("expected ambiguous error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q, want failed_precondition", p.Subtype)
	}
}

func TestPluginResolveCapDir_NeitherExists_DefaultsToServer(t *testing.T) {
	dir := t.TempDir()
	got, err := pluginResolveCapDir(dir)
	if err != nil {
		t.Fatalf("should default to server/capabilities, got error: %v", err)
	}
	if want := filepath.Join(dir, "server", "capabilities"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPluginResolveCapDir_AppType3_UsesServer(t *testing.T) {
	t.Setenv("MIAODA_APP_TYPE", "3")
	got, err := pluginResolveCapDir("/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := filepath.Join("/proj", "server", "capabilities"); got != want {
		t.Errorf("got %q, want %q (appType=3 should use server)", got, want)
	}
}

// --- pluginListCapabilities ---

func TestPluginListCapabilities_Empty(t *testing.T) {
	dir := t.TempDir()
	caps, err := pluginListCapabilities(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(caps) != 0 {
		t.Errorf("got %d caps, want 0", len(caps))
	}
}

func TestPluginListCapabilities_DirNotExist(t *testing.T) {
	caps, err := pluginListCapabilities("/nonexistent/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if caps != nil {
		t.Errorf("got %v, want nil", caps)
	}
}

func TestPluginListCapabilities_WithFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestCapJSON(t, dir, "cap1.json", map[string]interface{}{"id": "cap1", "name": "Cap One"})
	writeTestCapJSON(t, dir, "cap2.json", map[string]interface{}{"id": "cap2", "name": "Cap Two"})
	// non-JSON file should be skipped
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}

	caps, err := pluginListCapabilities(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(caps) != 2 {
		t.Fatalf("got %d caps, want 2", len(caps))
	}
}

func TestPluginListCapabilities_SkipsMalformed(t *testing.T) {
	dir := t.TempDir()
	writeTestCapJSON(t, dir, "good.json", map[string]interface{}{"id": "good"})
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	caps, err := pluginListCapabilities(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(caps) != 1 {
		t.Fatalf("got %d caps, want 1", len(caps))
	}
}

// --- helpers ---

func writeTestCapJSON(t *testing.T, dir, filename string, data map[string]interface{}) {
	t.Helper()
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), b, 0o644); err != nil {
		t.Fatal(err)
	}
}
