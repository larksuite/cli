// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/qualitygate/manifest"
)

func TestConfigureQualityGateEnvironmentForcesDeterministicRegistry(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "on")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", "")

	configureQualityGateEnvironment()

	if got := os.Getenv("LARKSUITE_CLI_REMOTE_META"); got != "off" {
		t.Fatalf("LARKSUITE_CLI_REMOTE_META = %q, want off", got)
	}
	if got := os.Getenv("LARKSUITE_CLI_CONFIG_DIR"); got == "" {
		t.Fatal("LARKSUITE_CLI_CONFIG_DIR was not set")
	}
}

func TestCheckRequiresManifestInputs(t *testing.T) {
	code, stderr := runCheckCaptureStderr(t, []string{"--repo", t.TempDir(), "--cli-bin", "./lark-cli"})
	if code != 2 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "--manifest and --command-index are required") {
		t.Fatalf("stderr = %s", stderr)
	}
}

func TestCheckAcceptsPublicContentMetadataFlag(t *testing.T) {
	code, stderr := runCheckCaptureStderr(t, []string{
		"--repo", t.TempDir(),
		"--cli-bin", "./lark-cli",
		"--public-content-metadata", ".tmp/quality-gate/pr.json",
	})
	if code != 2 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr)
	}
	if strings.Contains(stderr, "flag provided but not defined") {
		t.Fatalf("public content metadata flag was not registered: %s", stderr)
	}
	if !strings.Contains(stderr, "--manifest and --command-index are required") {
		t.Fatalf("stderr = %s", stderr)
	}
}

func TestCheckRejectsUnsafePublicContentMetadataPath(t *testing.T) {
	code, stderr := runCheckCaptureStderr(t, []string{
		"--repo", t.TempDir(),
		"--cli-bin", "./lark-cli",
		"--public-content-metadata", filepath.Join(t.TempDir(), "pr.json"),
	})
	if code != 2 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "--public-content-metadata") || !strings.Contains(stderr, "--file") {
		t.Fatalf("stderr = %s, want unsafe public content metadata path error", stderr)
	}
}

func TestCheckReportsManifestReadErrorsWithFlagName(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "command-manifest.json")
	indexPath := filepath.Join(dir, "command-index.json")
	if err := os.WriteFile(manifestPath, []byte(`{"schema_version":999,"commands":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := manifest.WriteFile(indexPath, manifest.KindCommandIndex, manifest.Manifest{
		SchemaVersion: 1,
		Commands: []manifest.Command{{
			Path:          "drive file.comments create_v2",
			CanonicalPath: "drive file-comments create-v2",
			Source:        manifest.SourceService,
			Generated:     true,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	code, stderr := runCheckCaptureStderr(t, []string{
		"--repo", dir,
		"--cli-bin", "./lark-cli",
		"--manifest", manifestPath,
		"--command-index", indexPath,
	})
	if code != 2 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "--manifest:") {
		t.Fatalf("stderr = %s", stderr)
	}
}

func runCheckCaptureStderr(t *testing.T, args []string) (int, string) {
	t.Helper()
	original := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	code := runCheck(args)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stderr = original
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return code, string(data)
}
