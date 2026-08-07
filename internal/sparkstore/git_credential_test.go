// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sparkstore

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestAppStorageRoundTrip exercises the adapter the Git credential helper is
// handed: it must delegate to the package functions, so a value written through
// it reads back through it and disappears on Delete.
func TestAppStorageRoundTrip(t *testing.T) {
	storageTempDir(t)
	var store AppStorage

	want := []byte(`{"username":"u","token":"t"}`)
	if err := store.Write("app_a", "git.json", want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := store.Read("app_a", "git.json")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("Read = %q, want %q", got, want)
	}

	if err := store.Delete("app_a", "git.json"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, err := store.Read("app_a", "git.json"); err != nil || got != nil {
		t.Fatalf("Read after Delete = (%q, %v), want (nil, nil)", got, err)
	}
	// Deleting what is already gone is not an error.
	if err := store.Delete("app_a", "git.json"); err != nil {
		t.Fatalf("Delete (missing): %v", err)
	}
}

// TestAppStorageListAppIDs covers what ListAppIDs adds over the package
// functions: it reads the storage root, decodes the escaped directory names back
// into app ids, and ignores anything that is not a valid app directory.
func TestAppStorageListAppIDs(t *testing.T) {
	storageTempDir(t)
	var store AppStorage

	// An id needing escaping proves the listing decodes rather than reporting
	// the on-disk name.
	for _, appID := range []string{"app_a", "app b/c"} {
		if err := store.Write(appID, "git.json", []byte("x")); err != nil {
			t.Fatalf("Write(%q): %v", appID, err)
		}
	}
	// A stray file at the root is not an app.
	if err := os.WriteFile(filepath.Join(Root(), "loose.json"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write stray file: %v", err)
	}

	got, err := store.ListAppIDs()
	if err != nil {
		t.Fatalf("ListAppIDs: %v", err)
	}
	sort.Strings(got)
	want := []string{"app b/c", "app_a"}
	if len(got) != len(want) {
		t.Fatalf("ListAppIDs() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ListAppIDs() = %#v, want %#v", got, want)
		}
	}
}

// TestAppStorageListAppIDsWithoutRoot pins the empty-not-error contract: a fresh
// install has no storage root, and the credential helper must be able to list
// zero apps rather than fail.
func TestAppStorageListAppIDsWithoutRoot(t *testing.T) {
	storageTempDir(t)
	var store AppStorage

	if _, err := os.Stat(Root()); !os.IsNotExist(err) {
		t.Fatalf("storage root should not exist yet: stat error = %v", err)
	}
	got, err := store.ListAppIDs()
	if err != nil {
		t.Fatalf("ListAppIDs: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListAppIDs() = %#v, want empty", got)
	}
}
