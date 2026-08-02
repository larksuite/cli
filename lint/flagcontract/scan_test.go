// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package flagcontract

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanRejectsLocalNormalizerAndIndependentAliasFlag(t *testing.T) {
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
	if len(got) != 2 {
		t.Fatalf("violations = %#v, want 2", got)
	}
	if got[0].Rule != "flag_alias_normalizer_owner" || got[1].Rule != "flag_alias_independent_flag" {
		t.Fatalf("rules = %q, %q", got[0].Rule, got[1].Rule)
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
