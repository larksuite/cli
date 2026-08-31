// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
			got, err := os.ReadFile(filepath.Join(destination, "skill", "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "content" {
				t.Fatalf("content = %q", got)
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
	t.Helper()
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	tw := tar.NewWriter(gz)
	content := []byte("content")
	if err := tw.WriteHeader(&tar.Header{Name: "skill/SKILL.md", Mode: 0o644, Size: int64(len(content))}); err != nil {
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
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeTestZip(t *testing.T, path string) {
	t.Helper()
	var data bytes.Buffer
	zw := zip.NewWriter(&data)
	entry, err := zw.Create("skill/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("content")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
