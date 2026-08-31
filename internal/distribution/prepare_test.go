// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestListSkillsIgnoresRootFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "lark-example"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("metadata"), 0o644); err != nil {
		t.Fatal(err)
	}
	names, err := listSkills(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"lark-example"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
}
