// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

func TestExtractArchive(t *testing.T) {
	for format, build := range map[string]func(*testing.T, string, string){
		"tar.gz": writeTestTarGzipEntry, "zip": writeTestZipEntry,
	} {
		for _, tc := range []struct {
			name, entry string
			limit       int64
		}{
			{"valid", "skill/SKILL.md", 7},
			{"path traversal", "../escape", 7},
			{"expanded size", "skill/SKILL.md", 6},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				root := t.TempDir()
				archive, destination := filepath.Join(root, "artifact"), filepath.Join(root, "out")
				build(t, archive, tc.entry)
				err := extractArchiveWithLimit(archive, destination, tc.limit)
				switch tc.name {
				case "valid":
					if err != nil {
						t.Fatal(err)
					}
					assertFile(t, filepath.Join(destination, tc.entry), "content")
				case "path traversal":
					if err == nil {
						t.Fatal("extractArchive succeeded")
					}
					if _, err := vfs.Stat(filepath.Join(root, "escape")); !errors.Is(err, fs.ErrNotExist) {
						t.Fatalf("archive wrote outside destination: %v", err)
					}
				case "expanded size":
					if err == nil || !strings.Contains(err.Error(), "exceeds 6 bytes") {
						t.Fatalf("err = %v", err)
					}
				}
			})
		}
	}
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

func writeTestZipEntry(t *testing.T, path, name string) {
	t.Helper()
	if err := vfs.WriteFile(path, buildTestZip(t, map[string]testZipFile{name: {content: "content"}}), 0o600); err != nil {
		t.Fatal(err)
	}
}
