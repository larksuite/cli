package vfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOsFsImplementsFS(t *testing.T) {
	var _ FS = OsFs{}
}

func TestDefaultFSIsOsFs(t *testing.T) {
	if _, ok := DefaultFS.(OsFs); !ok {
		t.Fatal("DefaultFS should be OsFs")
	}
}

func TestOsFsBasicOperations(t *testing.T) {
	fs := OsFs{}
	dir := t.TempDir()

	// MkdirAll
	sub := filepath.Join(dir, "a", "b")
	if err := fs.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// WriteFile + ReadFile
	p := filepath.Join(sub, "test.txt")
	if err := fs.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, err := fs.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("ReadFile got %q, want %q", data, "hello")
	}

	// Stat
	info, err := fs.Stat(p)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Name() != "test.txt" {
		t.Fatalf("Stat name got %q", info.Name())
	}

	// Lstat
	info, err = fs.Lstat(p)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Name() != "test.txt" {
		t.Fatalf("Lstat name got %q", info.Name())
	}

	// Rename
	p2 := filepath.Join(sub, "test2.txt")
	if err := fs.Rename(p, p2); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	// Open
	f, err := fs.Open(p2)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	f.Close()

	// OpenFile
	f, err = fs.OpenFile(p2, os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	f.Close()

	// CreateTemp
	f, err = fs.CreateTemp(dir, "tmp-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	tmpName := f.Name()
	f.Close()

	// Remove
	if err := fs.Remove(tmpName); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Getwd
	if _, err := fs.Getwd(); err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	// UserHomeDir
	if _, err := fs.UserHomeDir(); err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
}
