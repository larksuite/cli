// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

func TestExtractArchiveFormats(t *testing.T) {
	tests := []struct {
		name  string
		build func(*testing.T, string)
	}{
		{"tar.gz", writeTestTarGzip},
		{"zip", writeTestZip},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			archive := filepath.Join(root, "artifact")
			tt.build(t, archive)
			destination := filepath.Join(root, "out")
			if err := extractArchive(archive, destination); err != nil {
				t.Fatal(err)
			}
			got, err := vfs.ReadFile(filepath.Join(destination, "skill", "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "content" {
				t.Fatalf("content = %q", got)
			}
		})
	}
}

func TestExtractArchiveRejectsEntriesOutsideDestination(t *testing.T) {
	for _, tt := range []struct {
		name  string
		build func(*testing.T, string, string)
	}{
		{"tar.gz", writeTestTarGzipEntry},
		{"zip", writeTestZipEntry},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			archive := filepath.Join(root, "artifact")
			tt.build(t, archive, "../escape")
			if err := extractArchive(archive, filepath.Join(root, "out")); err == nil {
				t.Fatal("extractArchive succeeded")
			}
			if _, err := vfs.Stat(filepath.Join(root, "escape")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("archive wrote outside destination: %v", err)
			}
		})
	}
}

func TestExtractArchiveRejectsExcessiveExpandedSize(t *testing.T) {
	for _, tt := range []struct {
		name  string
		build func(*testing.T, string)
	}{
		{"tar.gz", writeTestTarGzip},
		{"zip", writeTestZip},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			archive := filepath.Join(root, "artifact")
			tt.build(t, archive)
			err := extractArchiveWithLimit(archive, filepath.Join(root, "out"), 6)
			if err == nil || !strings.Contains(err.Error(), "exceeds 6 bytes") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func writeTestTarGzip(t *testing.T, path string) {
	writeTestTarGzipEntry(t, path, "skill/SKILL.md")
}

func writeTestTarGzipEntry(t *testing.T, path, name string) {
	t.Helper()
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	tw := tar.NewWriter(gz)
	content := []byte("content")
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := vfs.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeTestZip(t *testing.T, path string) {
	writeTestZipEntry(t, path, "skill/SKILL.md")
}

func writeTestZipEntry(t *testing.T, path, name string) {
	t.Helper()
	var data bytes.Buffer
	zw := zip.NewWriter(&data)
	entry, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("content")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := vfs.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
