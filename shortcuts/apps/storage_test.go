// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

// storageTempDir points GetConfigDir at an isolated temp dir for the test.
func storageTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	return dir
}

func TestStorageWriteReadRoundTrip(t *testing.T) {
	storageTempDir(t)
	want := []byte(`{"username":"u","token":"t"}`)
	if err := Write("app_a", "git.json", want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Read("app_a", "git.json")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStorageReadMissingReturnsNil(t *testing.T) {
	storageTempDir(t)
	got, err := Read("app_a", "nope")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Fatalf("want nil, got %q", got)
	}
}

func TestStorageEmptyArgsRejected(t *testing.T) {
	storageTempDir(t)
	if _, err := Read("", "k"); err == nil {
		t.Error("Read empty appID should error")
	}
	if _, err := Read("a", ""); err == nil {
		t.Error("Read empty key should error")
	}
	if err := Write("", "k", nil); err == nil {
		t.Error("Write empty appID should error")
	}
	if err := Write("a", "", nil); err == nil {
		t.Error("Write empty key should error")
	}
	if err := Delete("", "k"); err == nil {
		t.Error("Delete empty appID should error")
	}
	if _, err := List(""); err == nil {
		t.Error("List empty appID should error")
	}
}

func TestStorageOverwrite(t *testing.T) {
	storageTempDir(t)
	if err := Write("app_a", "git.json", []byte("v1")); err != nil {
		t.Fatalf("Write1: %v", err)
	}
	if err := Write("app_a", "git.json", []byte("v2")); err != nil {
		t.Fatalf("Write2: %v", err)
	}
	got, _ := Read("app_a", "git.json")
	if string(got) != "v2" {
		t.Errorf("want v2, got %q", got)
	}
}

func TestStorageDeleteIdempotent(t *testing.T) {
	storageTempDir(t)
	if err := Write("app_a", "git.json", []byte("x")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := Delete("app_a", "git.json"); err != nil {
		t.Fatalf("first Delete: %v", err)
	}
	if got, _ := Read("app_a", "git.json"); got != nil {
		t.Error("file should be gone after Delete")
	}
	if err := Delete("app_a", "git.json"); err != nil {
		t.Errorf("second Delete should be nil (idempotent), got %v", err)
	}
}

func TestStorageListKeys(t *testing.T) {
	storageTempDir(t)
	for _, k := range []string{"git.json", "meta.json", "notes"} {
		if err := Write("app_a", k, []byte("x")); err != nil {
			t.Fatalf("Write %s: %v", k, err)
		}
	}
	got, err := List("app_a")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	sort.Strings(got)
	want := []string{"git.json", "meta.json", "notes"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestStorageListMissingAppDir(t *testing.T) {
	storageTempDir(t)
	got, err := List("never_written")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty, got %v", got)
	}
}

func TestStorageListSkipsSubdirs(t *testing.T) {
	dir := storageTempDir(t)
	if err := Write("app_a", "git.json", []byte("x")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "spark", "app_a", "sub"), 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got, err := List("app_a")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0] != "git.json" {
		t.Errorf("want [git.json], got %v", got)
	}
}

func TestStorageListSkipsInvalidDecodedKeys(t *testing.T) {
	dir := storageTempDir(t)
	if err := Write("app_a", "git.json", []byte("x")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	for _, name := range []string{"%zz", "%2E", "%2E%2E", "bad%2F..%2Fkey"} {
		if err := os.WriteFile(filepath.Join(dir, "spark", "app_a", name), []byte("x"), 0600); err != nil {
			t.Fatalf("write polluted key %s: %v", name, err)
		}
	}
	got, err := List("app_a")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0] != "git.json" {
		t.Errorf("want [git.json], got %v", got)
	}
}

func TestStorageEscapesAppIDAndKey(t *testing.T) {
	dir := storageTempDir(t)
	const appID, key = "a/b", "x/y"
	if err := Write(appID, key, []byte("v")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// no path traversal: spark/ has exactly one (escaped) app dir, no nested a/b tree
	entries, _ := os.ReadDir(filepath.Join(dir, "spark"))
	if len(entries) != 1 {
		t.Fatalf("expected 1 escaped app dir under spark/, got %v", entries)
	}
	got, err := Read(appID, key)
	if err != nil || string(got) != "v" {
		t.Fatalf("Read escaped: got %q err %v", got, err)
	}
	keys, err := List(appID)
	if err != nil || len(keys) != 1 || keys[0] != key {
		t.Fatalf("List escaped: got %v err %v", keys, err)
	}
}

func TestStorageRejectsTraversal(t *testing.T) {
	dir := storageTempDir(t)
	for _, bad := range []string{"..", ".", "../x", "a/../b"} {
		if err := Write(bad, "k", []byte("x")); err == nil {
			t.Errorf("Write appID=%q should error", bad)
		}
		if err := Write("app", bad, []byte("x")); err == nil {
			t.Errorf("Write key=%q should error", bad)
		}
		if _, err := Read(bad, "k"); err == nil {
			t.Errorf("Read appID=%q should error", bad)
		}
		if err := Delete(bad, "k"); err == nil {
			t.Errorf("Delete appID=%q should error", bad)
		}
		if _, err := List(bad); err == nil {
			t.Errorf("List appID=%q should error", bad)
		}
	}
	// nothing escaped out of spark/ into ~/.lark-cli
	if _, err := os.Stat(filepath.Join(dir, "k")); !os.IsNotExist(err) {
		t.Error("traversal must not create files outside spark/")
	}
}

func TestStorageReadNonNotExistError(t *testing.T) {
	dir := storageTempDir(t)
	// A directory at the key path makes ReadFile fail with a non-ErrNotExist error.
	if err := os.MkdirAll(filepath.Join(dir, "spark", "app_a", "git.json"), 0700); err != nil {
		t.Fatalf("mkdir key path: %v", err)
	}
	if _, err := Read("app_a", "git.json"); err == nil {
		t.Fatal("expected error reading a directory key path")
	}
}

func TestStorageWriteMkdirError(t *testing.T) {
	dir := storageTempDir(t)
	// A file at spark/ makes creating the per-app directory under it fail.
	if err := os.WriteFile(filepath.Join(dir, "spark"), []byte("x"), 0600); err != nil {
		t.Fatalf("write spark file: %v", err)
	}
	if err := Write("app_a", "git.json", []byte("x")); err == nil {
		t.Fatal("expected mkdir error when spark/ is a file")
	}
}

func TestStorageWriteAtomicError(t *testing.T) {
	dir := storageTempDir(t)
	// A directory at the key path makes the atomic write/rename over it fail.
	if err := os.MkdirAll(filepath.Join(dir, "spark", "app_a", "git.json"), 0700); err != nil {
		t.Fatalf("mkdir key path: %v", err)
	}
	if err := Write("app_a", "git.json", []byte("x")); err == nil {
		t.Fatal("expected atomic write error when key path is a directory")
	}
}

func TestStorageDeleteInvalidKey(t *testing.T) {
	storageTempDir(t)
	if err := Delete("app_a", ".."); err == nil {
		t.Fatal("expected error deleting an invalid key")
	}
}

func TestStorageDeleteRemoveError(t *testing.T) {
	dir := storageTempDir(t)
	// A non-empty directory at the key path makes Remove fail (non-ErrNotExist).
	if err := os.MkdirAll(filepath.Join(dir, "spark", "app_a", "git.json", "child"), 0700); err != nil {
		t.Fatalf("mkdir key path: %v", err)
	}
	if err := Delete("app_a", "git.json"); err == nil {
		t.Fatal("expected error removing a non-empty directory key path")
	}
}

func TestStorageListReadDirError(t *testing.T) {
	dir := storageTempDir(t)
	// A file at the per-app directory path makes ReadDir fail (non-ErrNotExist).
	if err := os.MkdirAll(filepath.Join(dir, "spark"), 0700); err != nil {
		t.Fatalf("mkdir spark: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spark", "app_a"), []byte("x"), 0600); err != nil {
		t.Fatalf("write app file: %v", err)
	}
	if _, err := List("app_a"); err == nil {
		t.Fatal("expected error listing a file app directory")
	}
}

func TestStoragePermsAndDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("perm bits not meaningful on windows")
	}
	dir := storageTempDir(t)
	if err := Write("app_a", "git.json", []byte("x")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	fi, err := os.Stat(filepath.Join(dir, "spark", "app_a", "git.json"))
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("file perm = %o, want 0600", fi.Mode().Perm())
	}
	di, err := os.Stat(filepath.Join(dir, "spark", "app_a"))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if di.Mode().Perm() != 0700 {
		t.Errorf("dir perm = %o, want 0700", di.Mode().Perm())
	}
}
