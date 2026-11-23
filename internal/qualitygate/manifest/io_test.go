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

func TestValidateAcceptsDistinctFlagAliases(t *testing.T) {
	m := Manifest{SchemaVersion: 1, Commands: []Command{{
		Path:          "im +messages",
		CanonicalPath: "im +messages",
		Source:        SourceShortcut,
		Flags: []Flag{
			{Name: "order", Aliases: []string{"sort", "sort-order"}},
			{Name: "query", Aliases: []string{"keyword"}},
		},
	}}}
	if err := m.Validate(KindCommandManifest); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsFlagAliasCollisions(t *testing.T) {
	tests := []struct {
		name  string
		flags []Flag
	}{
		{
			name: "alias and canonical",
			flags: []Flag{
				{Name: "order", Aliases: []string{"query"}},
				{Name: "query"},
			},
		},
		{
			name: "alias and alias",
			flags: []Flag{
				{Name: "order", Aliases: []string{"sort"}},
				{Name: "field", Aliases: []string{"sort"}},
			},
		},
		{
			name:  "alias self reference",
			flags: []Flag{{Name: "order", Aliases: []string{"order"}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := Manifest{SchemaVersion: 1, Commands: []Command{{
				Path: "im +messages", CanonicalPath: "im +messages", Source: SourceShortcut, Flags: test.flags,
			}}}
			if err := m.Validate(KindCommandManifest); err == nil {
				t.Fatal("expected alias collision to fail")
			}
		})
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
