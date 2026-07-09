// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/qualitygate/manifest"
)

func TestManifestExportWritesManifestAndCommandIndex(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "command-manifest.json")
	indexPath := filepath.Join(dir, "command-index.json")

	code := runManifestExport([]string{
		"--manifest-out", manifestPath,
		"--command-index-out", indexPath,
	}, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}

	m, err := manifest.ReadFile(manifestPath, manifest.KindCommandManifest)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := manifest.ReadFile(indexPath, manifest.KindCommandIndex)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Commands) == 0 || len(idx.Commands) == 0 {
		t.Fatalf("empty export: manifest=%d index=%d", len(m.Commands), len(idx.Commands))
	}
	if hasServiceCommand(m) {
		t.Fatal("command-manifest should not include service commands")
	}
	if !hasServiceCommand(idx) {
		t.Fatal("command-index should include service commands")
	}
}

func TestManifestExportRequiresOutputPaths(t *testing.T) {
	var stderr bytes.Buffer
	code := runManifestExport(nil, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d", code)
	}
	if got := stderr.String(); !bytes.Contains([]byte(got), []byte("--manifest-out and --command-index-out are required")) {
		t.Fatalf("stderr = %s", got)
	}
}

func TestConfigureManifestExportEnvironmentForcesDeterministicRegistry(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "on")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", "")

	configureManifestExportEnvironment()

	if got := os.Getenv("LARKSUITE_CLI_REMOTE_META"); got != "off" {
		t.Fatalf("LARKSUITE_CLI_REMOTE_META = %q, want off", got)
	}
	if got := os.Getenv("LARKSUITE_CLI_CONFIG_DIR"); got == "" {
		t.Fatal("LARKSUITE_CLI_CONFIG_DIR was not set")
	}
}

func TestCollectContainsDocsFetchAndDryRunFlag(t *testing.T) {
	got, err := collectHandAuthored(context.Background())
	if err != nil {
		t.Fatalf("collectHandAuthored() error = %v", err)
	}
	cmd := findManifestCommand(&got, "docs +fetch")
	if cmd == nil {
		t.Fatalf("docs +fetch not found")
	}
	if !cmd.Runnable {
		t.Fatalf("docs +fetch should be runnable")
	}
	if findManifestFlag(cmd, "dry-run") == nil {
		t.Fatalf("docs +fetch should expose --dry-run")
	}
	if cmd.Source != manifest.SourceShortcut {
		t.Fatalf("docs +fetch source = %q, want shortcut", cmd.Source)
	}
}

func TestCollectExcludesGeneratedServiceCommands(t *testing.T) {
	got, err := collectHandAuthored(context.Background())
	if err != nil {
		t.Fatalf("collectHandAuthored() error = %v", err)
	}
	for _, cmd := range got.Commands {
		if cmd.Source == manifest.SourceService || cmd.Generated {
			t.Fatalf("quality-gate manifest should not include generated service command: %#v", cmd)
		}
	}
}

func TestCollectCommandIndexIncludesEmbeddedServiceCommand(t *testing.T) {
	got, err := collectCommandIndex(context.Background())
	if err != nil {
		t.Fatalf("collectCommandIndex() error = %v", err)
	}
	cmd := findManifestCommand(&got, "drive file.comments create_v2")
	if cmd == nil {
		t.Fatalf("drive file.comments create_v2 not found")
	}
	if cmd.Source != manifest.SourceService {
		t.Fatalf("source = %q, want service", cmd.Source)
	}
	if !cmd.Generated {
		t.Fatalf("service command should be marked generated")
	}
	if !cmd.Runnable {
		t.Fatalf("service method command should be runnable")
	}
	for _, name := range []string{"file-token", "params", "data", "dry-run"} {
		if findManifestFlag(cmd, name) == nil {
			t.Fatalf("drive file.comments create_v2 should expose --%s", name)
		}
	}
}

func TestCollectCommandIndexDoesNotInheritGeneratedFromServiceParentForShortcut(t *testing.T) {
	got, err := collectCommandIndex(context.Background())
	if err != nil {
		t.Fatalf("collectCommandIndex() error = %v", err)
	}
	cmd := findManifestCommand(&got, "docs +fetch")
	if cmd == nil {
		t.Fatalf("docs +fetch not found")
	}
	if cmd.Source != manifest.SourceShortcut {
		t.Fatalf("docs +fetch source = %q, want shortcut", cmd.Source)
	}
	if cmd.Generated {
		t.Fatalf("shortcut under service parent must not inherit generated=true")
	}
}

func TestCollectCommandIndexPreservesHandAuthoredMetadataForOverlappingCommands(t *testing.T) {
	handAuthored, err := collectHandAuthored(context.Background())
	if err != nil {
		t.Fatalf("collectHandAuthored() error = %v", err)
	}
	idx, err := collectCommandIndex(context.Background())
	if err != nil {
		t.Fatalf("collectCommandIndex() error = %v", err)
	}
	for _, handCmd := range handAuthored.Commands {
		indexCmd := findManifestCommand(&idx, handCmd.Path)
		if indexCmd == nil {
			t.Fatalf("command-index missing hand-authored command %q", handCmd.Path)
		}
		if indexCmd.Source != handCmd.Source || indexCmd.Generated != handCmd.Generated {
			t.Fatalf("command-index metadata for %q = source:%s generated:%v, want source:%s generated:%v", handCmd.Path, indexCmd.Source, indexCmd.Generated, handCmd.Source, handCmd.Generated)
		}
	}
}

func TestCollectDoesNotInheritGeneratedFromServiceParentForShortcut(t *testing.T) {
	got, err := collectHandAuthored(context.Background())
	if err != nil {
		t.Fatalf("collectHandAuthored() error = %v", err)
	}
	cmd := findManifestCommand(&got, "docs +fetch")
	if cmd == nil {
		t.Fatalf("docs +fetch not found")
	}
	if cmd.Source != manifest.SourceShortcut {
		t.Fatalf("docs +fetch source = %q, want shortcut", cmd.Source)
	}
	if cmd.Generated {
		t.Fatalf("shortcut under service parent must not inherit generated=true")
	}
}

func TestCollectIgnoresRuntimeStrictMode(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "dry-run")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "dry-run")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	got, err := collectHandAuthored(context.Background())
	if err != nil {
		t.Fatalf("collectHandAuthored() error = %v", err)
	}
	if cmd := findManifestCommand(&got, "contact +search-user"); cmd == nil {
		t.Fatal("user-only shortcut missing; manifest collection should not apply runtime strict mode")
	}
}

func hasServiceCommand(m manifest.Manifest) bool {
	for _, cmd := range m.Commands {
		if cmd.Source == manifest.SourceService {
			return true
		}
	}
	return false
}

func findManifestCommand(m *manifest.Manifest, path string) *manifest.Command {
	for i := range m.Commands {
		if m.Commands[i].Path == path {
			return &m.Commands[i]
		}
	}
	return nil
}

func findManifestFlag(cmd *manifest.Command, name string) *manifest.Flag {
	for i := range cmd.Flags {
		if cmd.Flags[i].Name == name {
			return &cmd.Flags[i]
		}
	}
	return nil
}
