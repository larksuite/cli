// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestPluginInstall_SinglePlugin(t *testing.T) {
	dir := t.TempDir()
	writeTestPkgJSON(t, dir, map[string]interface{}{})
	oldPluginDir := filepath.Join(dir, "node_modules", "@test", "my-plugin")
	if err := os.MkdirAll(oldPluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldPluginDir, "package.json"), []byte(`{"version":"0.9.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	oldMarker := filepath.Join(oldPluginDir, "old.txt")
	if err := os.WriteFile(oldMarker, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdirTest(t, dir)

	factory, stdout, reg := newAppsExecuteFactory(t)

	// Mock batch_query API (new protocol: plugin_keys array, response data.items flat list)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/plugin/versions/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"key":               "@test/my-plugin",
						"version":           "1.0.0",
						"download_approach": "inner",
						"status":            "active",
					},
				},
			},
		},
	})

	// Mock download API (POST with JSON body, returns binary tgz)
	tgzData := buildTestTGZ(t, map[string]string{
		"manifest.json": `{"actions":[]}`,
		"package.json":  `{"name":"@test/my-plugin","version":"1.0.0"}`,
	})
	reg.Register(&httpmock.Stub{
		Method:      "POST",
		URL:         "/open-apis/spark/v1/plugin/versions/download_package",
		RawBody:     tgzData,
		ContentType: "application/octet-stream",
	})

	err := runAppsShortcut(t, AppsPluginInstall, []string{
		"+plugin-install", "--name", "@test/my-plugin", "--version", "1.0.0",
		"--format", "json", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file extracted
	manifestPath := filepath.Join(dir, "node_modules", "@test/my-plugin", "manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("manifest.json not extracted: %v", err)
	}
	if _, err := os.Stat(oldMarker); !os.IsNotExist(err) {
		t.Fatalf("old plugin content must be replaced after successful extraction, stat error = %v", err)
	}

	// Verify package.json updated
	pkg, _ := pluginReadPackageJSON(dir)
	ap := pluginGetActionPlugins(pkg)
	if v := ap["@test/my-plugin"]; v != "1.0.0" {
		t.Errorf("actionPlugins[@test/my-plugin] = %q, want 1.0.0", v)
	}

	// Verify output
	var env map[string]interface{}
	json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]interface{})
	if data["status"] != "installed" {
		t.Errorf("status = %v, want installed", data["status"])
	}
}

func TestPluginInstall_AlreadyInstalled(t *testing.T) {
	dir := t.TempDir()
	writeTestPkgJSON(t, dir, map[string]interface{}{
		"actionPlugins": map[string]interface{}{
			"@test/my-plugin": "1.0.0",
		},
	})
	// Create an existing installed plugin with package.json containing version
	pkgDir := filepath.Join(dir, "node_modules", "@test/my-plugin")
	os.MkdirAll(pkgDir, 0o755)
	os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"version":"1.0.0"}`), 0o644)
	chdirTest(t, dir)

	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsPluginInstall, []string{
		"+plugin-install", "--name", "@test/my-plugin", "--version", "1.0.0",
		"--format", "json", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var env map[string]interface{}
	json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]interface{})
	if data["status"] != "already_installed" {
		t.Errorf("status = %v, want already_installed", data["status"])
	}
}

// --- tgz helpers ---

func TestPluginExtractTGZ(t *testing.T) {
	tgzData := buildTestTGZ(t, map[string]string{
		"manifest.json": `{"actions":[]}`,
		"README.md":     "# Hello",
	})

	destDir := t.TempDir()
	if err := pluginExtractTGZ(bytes.NewReader(tgzData), destDir); err != nil {
		t.Fatalf("extract error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "manifest.json"))
	if err != nil {
		t.Fatalf("manifest.json not extracted: %v", err)
	}
	if string(data) != `{"actions":[]}` {
		t.Errorf("manifest.json content = %q", string(data))
	}
}

func TestPluginExtractTGZ_PathTraversal(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{
		Name:     "package/../../../etc/passwd",
		Size:     5,
		Mode:     0o644,
		Typeflag: tar.TypeReg,
	})
	tw.Write([]byte("evil!"))
	tw.Close()
	gz.Close()

	destDir := t.TempDir()
	if err := pluginExtractTGZ(&buf, destDir); err != nil {
		t.Fatalf("extract should not error, but skip bad entries: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "..", "..", "etc", "passwd")); err == nil {
		t.Error("path traversal should have been blocked")
	}
}

func TestPluginExtractTGZ_RejectsAggregateExpandedSize(t *testing.T) {
	tgzData := buildTestTGZ(t, map[string]string{
		"first.txt":  "123456",
		"second.txt": "12345",
	})
	destDir := t.TempDir()
	err := pluginExtractTGZWithLimits(bytes.NewReader(tgzData), destDir, pluginExtractLimits{
		maxEntryBytes: 10,
		maxTotalBytes: 10,
		maxEntries:    10,
	})
	if err == nil || !strings.Contains(err.Error(), "expanded size limit") {
		t.Fatalf("extract error = %v, want expanded size limit", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "second.txt")); !os.IsNotExist(err) {
		t.Fatalf("second entry must not be created after aggregate limit failure, stat error = %v", err)
	}
}

func TestPluginExtractTGZ_RejectsOversizedEntryWithoutTruncation(t *testing.T) {
	tgzData := buildTestTGZ(t, map[string]string{"large.txt": "12345"})
	destDir := t.TempDir()
	err := pluginExtractTGZWithLimits(bytes.NewReader(tgzData), destDir, pluginExtractLimits{
		maxEntryBytes: 4,
		maxTotalBytes: 10,
		maxEntries:    10,
	})
	if err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("extract error = %v, want entry size limit", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "large.txt")); !os.IsNotExist(err) {
		t.Fatalf("oversized entry must not be truncated to disk, stat error = %v", err)
	}
}

func TestPluginExtractTGZ_RejectsTooManyEntries(t *testing.T) {
	tgzData := buildTestTGZ(t, map[string]string{
		"first.txt":  "1",
		"second.txt": "2",
		"third.txt":  "3",
	})
	destDir := t.TempDir()
	err := pluginExtractTGZWithLimits(bytes.NewReader(tgzData), destDir, pluginExtractLimits{
		maxEntryBytes: 10,
		maxTotalBytes: 10,
		maxEntries:    2,
	})
	if err == nil || !strings.Contains(err.Error(), "entry limit") {
		t.Fatalf("extract error = %v, want entry limit", err)
	}
}

func TestPluginInstall_RejectedArchivePreservesExistingPlugin(t *testing.T) {
	dir := t.TempDir()
	writeTestPkgJSON(t, dir, map[string]interface{}{
		"actionPlugins": map[string]interface{}{"@test/my-plugin": "0.9.0"},
	})
	pluginDir := filepath.Join(dir, "node_modules", "@test", "my-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(pluginDir, "keep.txt")
	if err := os.WriteFile(filepath.Join(pluginDir, "package.json"), []byte(`{"version":"0.9.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, []byte("old plugin"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdirTest(t, dir)

	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/plugin/versions/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"items": []interface{}{
				map[string]interface{}{"key": "@test/my-plugin", "version": "1.0.0"},
			}},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:      "POST",
		URL:         "/open-apis/spark/v1/plugin/versions/download_package",
		RawBody:     []byte("not a gzip stream"),
		ContentType: "application/octet-stream",
	})

	err := runAppsShortcut(t, AppsPluginInstall, []string{
		"+plugin-install", "--name", "@test/my-plugin", "--version", "1.0.0",
		"--format", "json", "--as", "user",
	}, factory, stdout)
	if err == nil {
		t.Fatal("install should reject malformed plugin archive")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("install error is not structured: %v", err)
	}
	if problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeFileIO {
		t.Fatalf("error classification = %s/%s, want %s/%s", problem.Category, problem.Subtype, errs.CategoryInternal, errs.SubtypeFileIO)
	}
	if !errors.Is(err, gzip.ErrHeader) {
		t.Fatalf("malformed gzip cause not preserved: %v", err)
	}
	data, readErr := os.ReadFile(markerPath)
	if readErr != nil {
		t.Fatalf("existing plugin was not preserved: %v", readErr)
	}
	if string(data) != "old plugin" {
		t.Fatalf("existing marker = %q, want old plugin", data)
	}
	staged, globErr := filepath.Glob(filepath.Join(filepath.Dir(pluginDir), ".plugin-install-*"))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(staged) != 0 {
		t.Fatalf("rejected staging directories were not cleaned up: %v", staged)
	}
}

func TestPluginReplaceDirectory_RollbackFailurePreservesBackup(t *testing.T) {
	root := t.TempDir()
	stagedDir := filepath.Join(root, "staged")
	destDir := filepath.Join(root, "plugin")
	if err := os.MkdirAll(stagedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "keep.txt"), []byte("old plugin"), 0o644); err != nil {
		t.Fatal(err)
	}

	replaceErr := errors.New("replace failed")
	restoreErr := errors.New("restore failed")
	renameCalls := 0
	err := pluginReplaceDirectoryWithRename(stagedDir, destDir, func(oldPath, newPath string) error {
		renameCalls++
		switch renameCalls {
		case 1:
			return os.Rename(oldPath, newPath)
		case 2:
			return replaceErr
		case 3:
			return restoreErr
		default:
			t.Fatalf("unexpected rename call %d", renameCalls)
			return nil
		}
	})
	if !errors.Is(err, replaceErr) {
		t.Fatalf("replace error = %v, want preserved replacement cause", err)
	}
	if !strings.Contains(err.Error(), "previous plugin preserved at") {
		t.Fatalf("replace error = %v, want backup recovery path", err)
	}
	backups, globErr := filepath.Glob(filepath.Join(root, ".plugin-backup-*", "previous", "keep.txt"))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(backups) != 1 {
		t.Fatalf("preserved backup files = %v, want one", backups)
	}
	data, readErr := os.ReadFile(backups[0])
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "old plugin" {
		t.Fatalf("preserved backup = %q, want old plugin", data)
	}
}

// buildTestTGZ creates a .tgz in memory with files under a "package/" prefix.
func buildTestTGZ(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		content := files[name]
		tw.WriteHeader(&tar.Header{
			Name:     "package/" + name,
			Size:     int64(len(content)),
			Mode:     0o644,
			Typeflag: tar.TypeReg,
		})
		tw.Write([]byte(content))
	}

	tw.Close()
	gz.Close()
	return buf.Bytes()
}
