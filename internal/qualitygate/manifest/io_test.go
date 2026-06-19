// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRejectsDuplicateCommandPaths(t *testing.T) {
	m := Manifest{SchemaVersion: 1, Commands: []Command{
		{Path: "docs +fetch", CanonicalPath: "docs +fetch", Source: SourceShortcut},
		{Path: "docs +fetch", CanonicalPath: "docs +fetch", Source: SourceShortcut},
	}}
	if err := m.Validate(KindCommandManifest); err == nil {
		t.Fatal("expected duplicate command path to fail")
	}
}

func TestValidateRejectsInvalidSource(t *testing.T) {
	m := Manifest{SchemaVersion: 1, Commands: []Command{
		{Path: "docs +fetch", CanonicalPath: "docs +fetch", Source: Source("invalid")},
	}}
	if err := m.Validate(KindCommandManifest); err == nil {
		t.Fatal("expected invalid source to fail")
	}
}

func TestReadFileValidatesInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":999,"commands":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(path, KindCommandManifest); err == nil {
		t.Fatal("expected invalid schema_version to fail")
	}
}

func TestReadBytesValidatesInput(t *testing.T) {
	if _, err := ReadBytes([]byte(`{"schema_version":1,"commands":[{"path":"drive file.comments create_v2","source":"service"}]}`), KindCommandIndex); err == nil {
		t.Fatal("expected service command without generated=true to fail")
	}
}

func TestValidateRejectsSwappedManifestKinds(t *testing.T) {
	serviceOnly := Manifest{SchemaVersion: 1, Commands: []Command{{
		Path:          "drive file.comments create_v2",
		CanonicalPath: "drive file-comments create-v2",
		Source:        SourceService,
		Generated:     true,
	}}}
	if err := serviceOnly.Validate(KindCommandManifest); err == nil {
		t.Fatal("command-manifest should not accept a service-only manifest")
	}

	handAuthoredOnly := Manifest{SchemaVersion: 1, Commands: []Command{{
		Path:          "docs +fetch",
		CanonicalPath: "docs +fetch",
		Source:        SourceShortcut,
	}}}
	if err := handAuthoredOnly.Validate(KindCommandIndex); err == nil {
		t.Fatal("command-index should require at least one service command")
	}
}

func TestWriteFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "command-index.json")
	want := Manifest{SchemaVersion: 1, Commands: []Command{{
		Path:          "drive file.comments create_v2",
		CanonicalPath: "drive file-comments create-v2",
		Source:        SourceService,
		Generated:     true,
		Flags:         []Flag{{Name: "file-token", TakesValue: true}},
	}}}
	if err := WriteFile(path, KindCommandIndex, want); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	got, err := ReadFile(path, KindCommandIndex)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got.Commands[0].Path != want.Commands[0].Path {
		t.Fatalf("path = %q, want %q", got.Commands[0].Path, want.Commands[0].Path)
	}
}
