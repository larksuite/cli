// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package flagcontract

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanRejectsLocalNormalizer(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "shortcuts/demo/demo.go", `package demo
func mount(cmd interface{ SetNormalizeFunc(any) }) { cmd.SetNormalizeFunc(nil) }
var flags = []struct { Name, Desc string; Hidden bool }{
    {Name: "sort-order", Hidden: true, Desc: "hidden alias for --order"},
    {Name: "legacy-sort", Hidden: true, Desc: "legacy vocabulary normalized to --order"},
}
`)
	writeFixture(t, root, "shortcuts/demo/demo_test.go", `package demo
func ignored(cmd interface{ SetNormalizeFunc(any) }) { cmd.SetNormalizeFunc(nil) }
`)
	writeFixture(t, root, aliasOwnerPath, `package flagalias
func bind(cmd interface{ SetNormalizeFunc(any) }) { cmd.SetNormalizeFunc(nil) }
`)

	got, err := ScanRepoWithOptions(root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("violations = %#v, want 1", got)
	}
	if got[0].Rule != "flag_alias_normalizer_owner" {
		t.Fatalf("rule = %q", got[0].Rule)
	}
}

func TestScanDoesNotInferAliasSemanticsFromHelpText(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "shortcuts/demo/demo.go", `package demo
var flags = []struct { Name, Desc string; Hidden bool }{
    {Name: "range", Hidden: true, Desc: "alias for --start-cell; ranges collapse to their top-left cell"},
}
`)

	got, err := ScanRepoWithOptions(root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("help prose must not define alias structure: %#v", got)
	}
}

func TestScanCurrentRepositoryHasNoViolations(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ScanRepoWithOptions(root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("flag contract violations: %#v", got)
	}
}

func writeFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
