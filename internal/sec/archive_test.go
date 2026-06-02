// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// makeZip builds an in-memory zip with the given entries, writes it to path,
// and returns nothing — convenience for table-driven tests.
type zipEntry struct {
	name    string
	body    string
	mode    os.FileMode
	symlink string // when set, entry is a symlink with this target
}

func makeZip(t *testing.T, path string, entries []zipEntry) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.mode != 0 {
			hdr.SetMode(e.mode)
		}
		if e.symlink != "" {
			hdr.SetMode(os.ModeSymlink | 0o777)
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatalf("zip header %q: %v", e.name, err)
		}
		body := e.body
		if e.symlink != "" {
			body = e.symlink
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Fatalf("zip write %q: %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write zip: %v", err)
	}
}

func TestExtractZip_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "src.zip")
	makeZip(t, zipPath, []zipEntry{
		{name: "lark-sec-cli", body: "binary", mode: 0o755},
		{name: "ca.crt", body: "cert"},
		{name: "lib/libMetaSecML.dylib", body: "dylib", mode: 0o755},
	})
	dst := filepath.Join(tmp, "out")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ExtractZip(zipPath, dst); err != nil {
		t.Fatalf("ExtractZip: %v", err)
	}

	for name, want := range map[string]string{
		"lark-sec-cli":              "binary",
		"ca.crt":                    "cert",
		"lib/libMetaSecML.dylib":    "dylib",
	} {
		got, err := os.ReadFile(filepath.Join(dst, name))
		if err != nil {
			t.Errorf("read %s: %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s body = %q, want %q", name, got, want)
		}
	}
	if info, err := os.Stat(filepath.Join(dst, "lark-sec-cli")); err == nil {
		if info.Mode().Perm()&0o100 == 0 {
			t.Errorf("lark-sec-cli not executable: mode=%v", info.Mode())
		}
	}
}

func TestExtractZip_RejectsTraversal(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "evil.zip")
	makeZip(t, zipPath, []zipEntry{
		{name: "../../../etc/passwd", body: "pwned"},
	})
	dst := filepath.Join(tmp, "out")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ExtractZip(zipPath, dst); err == nil {
		t.Fatal("ExtractZip accepted zip-slip entry")
	}
}

func TestExtractZip_RejectsAbsolutePath(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "abs.zip")
	makeZip(t, zipPath, []zipEntry{
		{name: "/etc/passwd", body: "pwned"},
	})
	dst := filepath.Join(tmp, "out")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ExtractZip(zipPath, dst); err == nil {
		t.Fatal("ExtractZip accepted absolute-path entry")
	}
}
