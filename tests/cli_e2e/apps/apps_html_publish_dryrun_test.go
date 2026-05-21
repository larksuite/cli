// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestAppsHTMLPublishDryRun exercises the walker / manifest layer without
// packing or uploading. --path goes through LocalFileIO which bounds reads to
// the runtime cwd, so each sub-test seeds fixtures in a t.TempDir and runs
// the binary with WorkDir set to that dir + relative --path.
//
// Hidden files are intentionally included — the walker is deliberately not
// filtering, so the manifest must reflect everything the user pointed --path
// at. Users are documented to pass clean build output directories (e.g.
// ./dist), not source trees.
func TestAppsHTMLPublishDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("Directory_ReportsManifest", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html><body>hi</body></html>"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "logo.svg"), []byte("<svg/>"), 0o644))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+html-publish",
				"--app-id", "app_x",
				"--path", ".",
				"--dry-run",
			},
			DefaultAs: "user",
			WorkDir:   dir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "POST", gjson.Get(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/upload_and_release_html_code", gjson.Get(result.Stdout, "api.0.url").String())
		// file_count / files / total_size_bytes sit at envelope top level
		// (not under api.0.body — manifest is dry-run metadata, not the HTTP body).
		assert.Equal(t, int64(2), gjson.Get(result.Stdout, "file_count").Int())
		assert.Greater(t, gjson.Get(result.Stdout, "total_size_bytes").Int(), int64(0))
		files := gjson.Get(result.Stdout, "files").Array()
		require.Len(t, files, 2)
		names := []string{files[0].String(), files[1].String()}
		assert.Contains(t, names, "index.html")
		assert.Contains(t, names, "logo.svg")
	})

	t.Run("SingleFile_OneEntry", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "page.html"), []byte("<html></html>"), 0o644))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+html-publish",
				"--app-id", "app_x",
				"--path", "page.html",
				"--dry-run",
			},
			DefaultAs: "user",
			WorkDir:   dir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, int64(1), gjson.Get(result.Stdout, "file_count").Int())
		assert.Equal(t, "page.html", gjson.Get(result.Stdout, "files.0").String())
	})

	t.Run("HiddenFilesIncluded", func(t *testing.T) {
		// Walker MUST NOT silently filter .git / .DS_Store — that's an explicit
		// design decision so users pass clean ./dist trees, not source repos.
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html/>"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("noise"), 0o644))
		require.NoError(t, os.Mkdir(filepath.Join(dir, ".git"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+html-publish",
				"--app-id", "app_x",
				"--path", ".",
				"--dry-run",
			},
			DefaultAs: "user",
			WorkDir:   dir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, int64(3), gjson.Get(result.Stdout, "file_count").Int(),
			"walker must include hidden files; got: %s", result.Stdout)
	})

	t.Run("EmptyDir_ManifestEmpty", func(t *testing.T) {
		// Dry-run only builds the manifest; an empty dir produces file_count=0
		// without erroring. The index.html / no-files check fires in Execute,
		// after the tarball stage — out of dry-run's scope.
		dir := t.TempDir()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+html-publish",
				"--app-id", "app_x",
				"--path", ".",
				"--dry-run",
			},
			DefaultAs: "user",
			WorkDir:   dir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, int64(0), gjson.Get(result.Stdout, "file_count").Int())
		assert.Equal(t, int64(0), gjson.Get(result.Stdout, "total_size_bytes").Int())
	})

	t.Run("MissingIndexHTML_PassesDryRun", func(t *testing.T) {
		// Same as EmptyDir: index.html requirement is enforced in Execute, not
		// at dry-run. Dry-run reports the manifest as-is.
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "page.html"), []byte("<html/>"), 0o644))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+html-publish",
				"--app-id", "app_x",
				"--path", ".",
				"--dry-run",
			},
			DefaultAs: "user",
			WorkDir:   dir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, int64(1), gjson.Get(result.Stdout, "file_count").Int())
		assert.Equal(t, "page.html", gjson.Get(result.Stdout, "files.0").String())
	})

	t.Run("RejectsMissingAppID", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html/>"), 0o644))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+html-publish",
				"--path", ".",
				"--dry-run",
			},
			DefaultAs: "user",
			WorkDir:   dir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 1)
		assert.Contains(t, result.Stdout+result.Stderr, `required flag(s) "app-id" not set`)
	})

	t.Run("RejectsMissingPath", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+html-publish",
				"--app-id", "app_x",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 1)
		assert.Contains(t, result.Stdout+result.Stderr, `required flag(s) "path" not set`)
	})
}
