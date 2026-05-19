// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildHTMLPublishTarball_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	candidates, err := walkHTMLPublishCandidates(dir)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	tarball, err := buildHTMLPublishTarball(candidates)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(tarball.Path) })

	if len(tarball.SHA256) != 64 {
		t.Fatalf("SHA256 wrong len: %d", len(tarball.SHA256))
	}
	if tarball.Size <= 0 {
		t.Fatalf("size=%d", tarball.Size)
	}

	f, err := os.Open(tarball.Path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	tr := tar.NewReader(gz)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("tar.Next: %v", err)
	}
	if hdr.Name != "index.html" {
		t.Fatalf("entry name = %q, want index.html", hdr.Name)
	}
	body, err := io.ReadAll(tr)
	if err != nil || string(body) != "<html></html>" {
		t.Fatalf("body=%q err=%v", body, err)
	}
}

func TestBuildHTMLPublishTarball_EmptyCandidates(t *testing.T) {
	if _, err := buildHTMLPublishTarball(nil); err == nil {
		t.Fatalf("expected error")
	}
}
