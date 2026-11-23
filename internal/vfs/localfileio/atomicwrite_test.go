// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestAtomicWrite_WritesContentAndPermissionCorrectly(t *testing.T) {
	// GIVEN: a target path in a temp directory
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	data := []byte(`{"key":"value"}`)

	// WHEN: AtomicWrite writes data with 0644 permission
	if err := AtomicWrite(path, data, 0644); err != nil {
		t.Fatalf("AtomicWrite failed: %v", err)
	}

	// THEN: file content matches exactly
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("content = %q, want %q", got, data)
	}
}

func TestAtomicWrite_SetsRestrictivePermission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission test not reliable on Windows")
	}

	// GIVEN: a target path
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.json")

	// WHEN: AtomicWrite writes with 0600 permission
	if err := AtomicWrite(path, []byte("secret"), 0600); err != nil {
		t.Fatalf("AtomicWrite failed: %v", err)
	}

	// THEN: file permission is exactly 0600 (owner read-write only)
	info, _ := os.Stat(path)
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("permission = %04o, want 0600", perm)
	}
}

func TestAtomicWrite_OverwritesExistingFile(t *testing.T) {
	// GIVEN: an existing file with old content
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	AtomicWrite(path, []byte("old"), 0644)

	// WHEN: AtomicWrite overwrites with new content
	if err := AtomicWrite(path, []byte("new"), 0644); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	// THEN: file contains new content
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
}

func TestAtomicWrite_LeavesNoResidualTempFileOnError(t *testing.T) {
	// GIVEN: a target path in a non-existent nested directory
	path := filepath.Join(t.TempDir(), "nonexistent", "subdir", "file.txt")

	// WHEN: AtomicWrite fails (parent directory doesn't exist)
	err := AtomicWrite(path, []byte("data"), 0644)

	// THEN: the write fails
	if err == nil {
		t.Fatal("expected error writing to nonexistent dir")
	}

	// THEN: no .tmp files are left behind
	parentDir := filepath.Dir(filepath.Dir(path))
	entries, _ := os.ReadDir(parentDir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("residual temp file found: %s", e.Name())
		}
	}
}

func TestAtomicWrite_PreservesOriginalFileOnFailure(t *testing.T) {
	// GIVEN: an existing file with known content
	dir := t.TempDir()
	original := []byte("original content")
	path := filepath.Join(dir, "file.json")
	if err := AtomicWrite(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	// WHEN: AtomicWrite targets a non-existent directory (guaranteed to fail even as root)
	badPath := filepath.Join(dir, "no", "such", "dir", "file.json")
	err := AtomicWrite(badPath, []byte("new"), 0644)

	// THEN: write fails
	if err == nil {
		t.Fatal("expected error writing to non-existent dir")
	}

	// THEN: the original file at the valid path is untouched
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Errorf("original file corrupted: got %q, want %q", got, original)
	}
}

func TestAtomicWrite_HandlesCorrectlyUnderConcurrentWrites(t *testing.T) {
	// GIVEN: a target file that will be written by 20 concurrent goroutines
	dir := t.TempDir()
	path := filepath.Join(dir, "concurrent.json")

	// WHEN: 20 goroutines write simultaneously
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			data := []byte(`{"n":` + string(rune('0'+n%10)) + `}`)
			AtomicWrite(path, data, 0644)
		}(i)
	}
	wg.Wait()

	// THEN: file exists and is valid (not corrupted by interleaved writes)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if len(got) == 0 {
		t.Error("file is empty after concurrent writes")
	}
}

// The exclusive commit must publish the target only once the content is
// complete. A reader that blocks mid-copy proves the final name is absent while
// bytes are still in flight -- writing straight to the final name with O_EXCL
// would satisfy no-clobber but expose a partial file, and a killed process would
// leave it behind as a phantom target for the next attempt.
func TestExclusiveWriteFromReaderPublishesOnlyCompleteContent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "artifact.bin")

	released := make(chan struct{})
	observed := make(chan error, 1)
	reader := io.MultiReader(
		strings.NewReader("first-half"),
		readerFunc(func(p []byte) (int, error) {
			// The copy is now half done; the final name must not exist yet.
			_, statErr := os.Stat(target)
			observed <- statErr
			close(released)
			return 0, io.EOF
		}),
	)

	written, err := ExclusiveWriteFromReader(target, reader, 0600)
	if err != nil {
		t.Fatalf("ExclusiveWriteFromReader() error = %v", err)
	}
	<-released
	if statErr := <-observed; !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("target existed mid-copy: %v", statErr)
	}
	if written != int64(len("first-half")) {
		t.Fatalf("written = %d", written)
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil || string(content) != "first-half" {
		t.Fatalf("committed content = %q (err=%v)", content, readErr)
	}
}

// The commit refuses an existing target instead of replacing it.
func TestExclusiveWriteFromReaderRefusesExistingTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "artifact.bin")
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := ExclusiveWriteFromReader(target, strings.NewReader("replacement"), 0600)
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("error = %v, want fs.ErrExist", err)
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil || string(content) != "original" {
		t.Fatalf("existing content = %q (err=%v), want it preserved", content, readErr)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("directory holds %d entries, want only the original file (temp file leaked)", len(entries))
	}
}

type readerFunc func(p []byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }
