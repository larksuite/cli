// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

// bytesRctx bundles a RuntimeContext with its captured stderr for tests that
// exercise best-effort side effects announced on stderr.
type bytesRctx struct {
	rctx   *common.RuntimeContext
	stderr *bytes.Buffer
}

func newBytesRctx(t *testing.T) *bytesRctx {
	t.Helper()
	cfg := &core.CliConfig{AppID: "test-app", AppSecret: "s", Brand: core.BrandFeishu, UserOpenId: "ou_t"}
	factory, _, stderrBuf, _ := cmdutil.TestFactory(t, cfg)
	cmd := &cobra.Command{Use: "test-sync"}
	cmd.SetContext(context.Background())
	return &bytesRctx{
		rctx:   common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser),
		stderr: stderrBuf,
	}
}

func readJSONFile(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := map[string]interface{}{}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("%s is not valid JSON: %v", path, err)
	}
	return doc
}

func TestWriteSparkAppSection(t *testing.T) {
	t.Run("creates the file when missing", func(t *testing.T) {
		dir := t.TempDir()
		if err := writeSparkAppSection(dir, "app_new", "https://apps.example/app/app_new"); err != nil {
			t.Fatal(err)
		}
		doc := readJSONFile(t, filepath.Join(dir, "spark.json"))
		app, _ := doc["app"].(map[string]interface{})
		if app["id"] != "app_new" || app["online_url"] != "https://apps.example/app/app_new" {
			t.Errorf("app section = %v", app)
		}
	})
	t.Run("empty url omits the key", func(t *testing.T) {
		dir := t.TempDir()
		if err := writeSparkAppSection(dir, "app_x", ""); err != nil {
			t.Fatal(err)
		}
		app, _ := readJSONFile(t, filepath.Join(dir, "spark.json"))["app"].(map[string]interface{})
		if _, has := app["online_url"]; has || app["id"] != "app_x" {
			t.Errorf("app section = %v, want id only", app)
		}
	})
	t.Run("preserves declaration and unknown fields", func(t *testing.T) {
		dir := t.TempDir()
		seed := `{"stack":"custom-webapp","dev":{"port":5173},"future_field":42,"app":{"id":"app_old","online_url":"https://apps.example/old"}}`
		if err := os.WriteFile(filepath.Join(dir, "spark.json"), []byte(seed), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeSparkAppSection(dir, "app_new", "https://apps.example/new"); err != nil {
			t.Fatal(err)
		}
		doc := readJSONFile(t, filepath.Join(dir, "spark.json"))
		if doc["stack"] != "custom-webapp" || doc["future_field"] != float64(42) {
			t.Errorf("declaration/unknown fields must survive: %v", doc)
		}
		app, _ := doc["app"].(map[string]interface{})
		if app["id"] != "app_new" || app["online_url"] != "https://apps.example/new" {
			t.Errorf("app section must be replaced wholesale: %v", app)
		}
	})
	t.Run("broken json is an error", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "spark.json"), []byte("{broken"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeSparkAppSection(dir, "app_x", "u"); err == nil || !strings.Contains(err.Error(), "parse") {
			t.Errorf("want parse error, got %v", err)
		}
	})
}

func TestWriteSparkScaffoldFields(t *testing.T) {
	t.Run("seed stack wins, version always stamped", func(t *testing.T) {
		dir := t.TempDir()
		seed := `{"stack":"seed-webapp","version":"0.0.1","dev":{"port":5173}}`
		if err := os.WriteFile(filepath.Join(dir, "spark.json"), []byte(seed), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeSparkScaffoldFields(dir, "cli-derived-webapp", "1.2.3"); err != nil {
			t.Fatal(err)
		}
		doc := readJSONFile(t, filepath.Join(dir, "spark.json"))
		if doc["stack"] != "seed-webapp" {
			t.Errorf("seed-declared stack must not be overwritten: %v", doc["stack"])
		}
		if doc["version"] != "1.2.3" {
			t.Errorf("version must be stamped with the rendered package version: %v", doc["version"])
		}
	})
	t.Run("broken json is an error", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "spark.json"), []byte("["), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeSparkScaffoldFields(dir, "s", "1.0.0"); err == nil {
			t.Error("want parse error")
		}
	})
}

func TestReadAppDevProjectConfig_BrokenJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spark.json"), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, found, err := readAppDevProjectConfig(dir)
	if !found || err == nil {
		t.Errorf("broken file must report found=true with a parse error, got found=%v err=%v", found, err)
	}
}

func TestSyncSparkAppURL(t *testing.T) {
	chdir := func(t *testing.T, dir string) {
		t.Helper()
		orig, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chdir(orig) })
	}
	newRctx := func(t *testing.T) *bytesRctx { return newBytesRctx(t) }

	t.Run("matching project syncs and is idempotent", func(t *testing.T) {
		dir := t.TempDir()
		seed := `{"stack":"custom-webapp","app":{"id":"app_m"}}`
		if err := os.WriteFile(filepath.Join(dir, "spark.json"), []byte(seed), 0o644); err != nil {
			t.Fatal(err)
		}
		chdir(t, dir)
		r := newRctx(t)
		syncSparkAppURL(r.rctx, "app_m", "https://apps.example/app/app_m")
		doc := readJSONFile(t, filepath.Join(dir, "spark.json"))
		app, _ := doc["app"].(map[string]interface{})
		if app["online_url"] != "https://apps.example/app/app_m" {
			t.Fatalf("url must be synced, got %v", app)
		}
		if !strings.Contains(r.stderr.String(), "synced into") {
			t.Errorf("stderr must announce the sync, got %q", r.stderr.String())
		}
		// Second call with the same url must be a silent no-op.
		r.stderr.Reset()
		syncSparkAppURL(r.rctx, "app_m", "https://apps.example/app/app_m")
		if r.stderr.Len() != 0 {
			t.Errorf("already-synced url must skip silently, stderr=%q", r.stderr.String())
		}
	})
	t.Run("skips silently outside a project and on id mismatch", func(t *testing.T) {
		empty := t.TempDir()
		chdir(t, empty)
		r := newRctx(t)
		syncSparkAppURL(r.rctx, "app_x", "u") // no spark.json
		if _, err := os.Stat(filepath.Join(empty, "spark.json")); !os.IsNotExist(err) {
			t.Error("no file must be created outside a project")
		}

		other := t.TempDir()
		if err := os.WriteFile(filepath.Join(other, "spark.json"), []byte(`{"app":{"id":"app_other"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		chdir(t, other)
		syncSparkAppURL(r.rctx, "app_x", "https://apps.example/u")
		app, _ := readJSONFile(t, filepath.Join(other, "spark.json"))["app"].(map[string]interface{})
		if _, has := app["online_url"]; has {
			t.Error("a different recorded app id must not be touched")
		}
	})
	t.Run("write failure only warns on stderr", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "spark.json"), []byte(`{"app":{"id":"app_ro"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(dir, "spark.json"), 0o444); err != nil {
			t.Fatal(err)
		}
		chdir(t, dir)
		r := newRctx(t)
		syncSparkAppURL(r.rctx, "app_ro", "https://apps.example/app/app_ro")
		if !strings.Contains(r.stderr.String(), "warning: failed to sync") {
			t.Errorf("write failure must warn on stderr, got %q", r.stderr.String())
		}
	})
	t.Run("skips silently on a broken declaration", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "spark.json"), []byte("{bad"), 0o644); err != nil {
			t.Fatal(err)
		}
		chdir(t, dir)
		r := newRctx(t)
		syncSparkAppURL(r.rctx, "app_x", "u")
		if b, _ := os.ReadFile(filepath.Join(dir, "spark.json")); string(b) != "{bad" {
			t.Error("a broken file must be left untouched")
		}
	})
}

func TestSparkWritersTolerateNullRoot(t *testing.T) {
	for name, write := range map[string]func(dir string) error{
		"app section":     func(dir string) error { return writeSparkAppSection(dir, "app_x", "https://apps.example/x") },
		"scaffold fields": func(dir string) error { return writeSparkScaffoldFields(dir, "s-webapp", "1.0.0") },
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "spark.json"), []byte("null"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := write(dir); err != nil {
				t.Fatalf("a literal JSON null root must not fail the write: %v", err)
			}
			readJSONFile(t, filepath.Join(dir, "spark.json"))
		})
	}
}
