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
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "dist"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dist", "index.html"), []byte("<html><body>hi</body></html>"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dist", "logo.svg"), []byte("<svg/>"), 0o644))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+html-publish",
				"--app-id", "app_x",
				"--path", "./dist",
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

	t.Run("HiddenFilesIncludedExceptGit", func(t *testing.T) {
		// The walker filters the .git directory (and a .git gitdir pointer file)
		// so a stray repo under --path doesn't ship its history / remote URL to a
		// public share URL. Generic hidden files like .DS_Store are NOT filtered —
		// only .git is — so users still see everything else they pointed --path at.
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "dist"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dist", "index.html"), []byte("<html/>"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dist", ".DS_Store"), []byte("noise"), 0o644))
		require.NoError(t, os.Mkdir(filepath.Join(dir, "dist", ".git"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dist", ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+html-publish",
				"--app-id", "app_x",
				"--path", "./dist",
				"--dry-run",
			},
			DefaultAs: "user",
			WorkDir:   dir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		// index.html + .DS_Store kept; .git/HEAD filtered out → 2 files.
		assert.Equal(t, int64(2), gjson.Get(result.Stdout, "file_count").Int(),
			"walker must keep non-.git hidden files but drop .git; got: %s", result.Stdout)
		names := gjson.Get(result.Stdout, "files").Array()
		var got []string
		for _, n := range names {
			got = append(got, n.String())
		}
		assert.Contains(t, got, "index.html")
		assert.Contains(t, got, ".DS_Store")
		assert.NotContains(t, got, ".git/HEAD", "walker must exclude .git contents")
	})

	t.Run("EmptyDir_ManifestEmpty", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "dist"), 0o755))
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+html-publish",
				"--app-id", "app_x",
				"--path", "./dist",
				"--dry-run",
			},
			DefaultAs: "user",
			WorkDir:   dir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, int64(0), gjson.Get(result.Stdout, "file_count").Int())
		assert.Equal(t, int64(0), gjson.Get(result.Stdout, "total_size_bytes").Int())
		assert.Contains(t, gjson.Get(result.Stdout, "validation_error").String(), "index.html",
			"empty dir should report index.html validation_error: %s", result.Stdout)
	})

	t.Run("MissingIndexHTML_SurfacesValidationError", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "dist"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dist", "page.html"), []byte("<html/>"), 0o644))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+html-publish",
				"--app-id", "app_x",
				"--path", "./dist",
				"--dry-run",
			},
			DefaultAs: "user",
			WorkDir:   dir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, int64(1), gjson.Get(result.Stdout, "file_count").Int())
		assert.Equal(t, "page.html", gjson.Get(result.Stdout, "files.0").String())
		assert.Contains(t, gjson.Get(result.Stdout, "validation_error").String(), "index.html")
	})

	t.Run("RejectsMissingAppID", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "dist"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dist", "index.html"), []byte("<html/>"), 0o644))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+html-publish",
				"--path", "./dist",
				"--dry-run",
			},
			DefaultAs: "user",
			WorkDir:   dir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		assert.Contains(t, validateErrorMessage(result), `required flag(s) "app-id" not set`)
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
		result.AssertExitCode(t, 2)
		assert.Contains(t, validateErrorMessage(result), `required flag(s) "path" not set`)
	})

	t.Run("RejectsSensitivePathsByDefault", func(t *testing.T) {
		// Validate scans candidates for well-known credential files and rejects
		// when any are found. Dry-run also fails (Validate runs before the
		// dry-run branch) — that's the point: dry-run preview matches Execute.
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "dist"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dist", "index.html"), []byte("<html/>"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dist", ".env"), []byte("SECRET=xxx\n"), 0o644))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+html-publish",
				"--app-id", "app_x",
				"--path", "./dist",
				"--dry-run",
			},
			DefaultAs: "user",
			WorkDir:   dir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		assert.Contains(t, validateErrorMessage(result), ".env",
			"validation error should name the offending file: %s", result.Stderr)
	})

	t.Run("AllowSensitiveOverride", func(t *testing.T) {
		// --allow-sensitive bypasses the credential-file gate (legitimate
		// cases: docs site shipping example .env files). Dry-run output
		// surfaces a sensitive_waived field so the caller still sees what
		// was let through.
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "dist"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dist", "index.html"), []byte("<html/>"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dist", ".env.example"), []byte("API_KEY=replace-me\n"), 0o644))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+html-publish",
				"--app-id", "app_x",
				"--path", "./dist",
				"--allow-sensitive",
				"--dry-run",
			},
			DefaultAs: "user",
			WorkDir:   dir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		waived := gjson.Get(result.Stdout, "sensitive_waived").Array()
		require.Len(t, waived, 1, "expected sensitive_waived to list the file, got: %s", result.Stdout)
		assert.Equal(t, ".env.example", waived[0].String())
	})

	t.Run("CleanCwdAllowed", func(t *testing.T) {
		// --path "." is no longer hard-rejected. A cwd that doesn't contain
		// well-known credential files is a valid publish target.
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html/>"), 0o644))

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
	})

	t.Run("TrimsAppIDAndPath", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "dist"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dist", "index.html"), []byte("<html/>"), 0o644))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+html-publish",
				"--app-id", "  app_x  ",
				"--path", "  ./dist  ",
				"--dry-run",
			},
			DefaultAs: "user",
			WorkDir:   dir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/upload_and_release_html_code",
			gjson.Get(result.Stdout, "api.0.url").String())
		assert.Equal(t, int64(1), gjson.Get(result.Stdout, "file_count").Int(),
			"path trimming must produce the same manifest as untrimmed input")
	})
}
