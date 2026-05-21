// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadStamp_Missing(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	got, err := ReadStamp()
	if err != nil {
		t.Fatalf("ReadStamp() err = %v, want nil for ENOENT", err)
	}
	if got != "" {
		t.Errorf("ReadStamp() = %q, want \"\" for missing file", got)
	}
}

func TestReadStamp_Normal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "skills.stamp"), []byte("1.0.21"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadStamp()
	if err != nil || got != "1.0.21" {
		t.Errorf("ReadStamp() = (%q, %v), want (\"1.0.21\", nil)", got, err)
	}
}

func TestReadStamp_TrailingNewlineTolerated(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "skills.stamp"), []byte("1.0.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := ReadStamp()
	if got != "1.0.21" {
		t.Errorf("ReadStamp() = %q, want \"1.0.21\" (newline trimmed)", got)
	}
}

func TestReadStamp_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "skills.stamp"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadStamp()
	if err != nil || got != "" {
		t.Errorf("ReadStamp() = (%q, %v), want (\"\", nil)", got, err)
	}
}

func TestWriteStamp_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	if err := WriteStamp("1.0.21"); err != nil {
		t.Fatalf("WriteStamp() = %v, want nil", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "skills.stamp"))
	if string(got) != "1.0.21" {
		t.Errorf("file content = %q, want \"1.0.21\"", string(got))
	}
}

func TestWriteStamp_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	if err := WriteStamp("1.0.20"); err != nil {
		t.Fatal(err)
	}
	if err := WriteStamp("1.0.21"); err != nil {
		t.Fatal(err)
	}
	got, _ := ReadStamp()
	if got != "1.0.21" {
		t.Errorf("ReadStamp() after overwrite = %q, want \"1.0.21\"", got)
	}
}

func TestWriteStamp_NoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	if err := WriteStamp("1.0.21"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "skills.stamp"))
	if string(raw) != "1.0.21" {
		t.Errorf("raw file = %q, want exactly \"1.0.21\" (no newline)", string(raw))
	}
}

// TestWriteStamp_MkdirAllFailure verifies WriteStamp returns the mkdir error
// when the base config dir cannot be created (parent path is a regular file).
func TestWriteStamp_MkdirAllFailure(t *testing.T) {
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	// Create a regular file where MkdirAll wants to create a directory.
	if err := os.WriteFile(blocker, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Point the config dir at a path UNDER the regular file — MkdirAll must fail.
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(blocker, "child"))

	if err := WriteStamp("1.0.21"); err == nil {
		t.Fatal("WriteStamp() = nil, want non-nil error from MkdirAll failure")
	}
}
