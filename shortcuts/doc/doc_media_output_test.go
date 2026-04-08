// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDocMediaOutputPath_UsesContentDispositionExtension(t *testing.T) {
	headers := http.Header{
		"Content-Disposition": []string{`attachment; filename="drive_registry_config_addition.csv"; filename*=UTF-8''drive_registry_config_addition.csv`},
		"Content-Type":        []string{"application/octet-stream"},
	}

	got := resolveDocMediaOutputPath("preview", "tok_123", headers, "")
	if got != "preview.csv" {
		t.Fatalf("resolveDocMediaOutputPath() = %q, want %q", got, "preview.csv")
	}
}

func TestResolveDocMediaOutputPath_PreservesExplicitExtension(t *testing.T) {
	headers := http.Header{
		"Content-Disposition": []string{`attachment; filename="drive_registry_config_addition.csv"`},
		"Content-Type":        []string{"text/csv"},
	}

	got := resolveDocMediaOutputPath("preview.bin", "tok_123", headers, "")
	if got != "preview.bin" {
		t.Fatalf("resolveDocMediaOutputPath() = %q, want %q", got, "preview.bin")
	}
}

func TestResolveDocMediaOutputPath_UsesServerFilenameForDirectoryTarget(t *testing.T) {
	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	if err := os.Mkdir("downloads", 0755); err != nil {
		t.Fatalf("Mkdir() error: %v", err)
	}

	headers := http.Header{
		"Content-Disposition": []string{`attachment; filename="drive_registry_config_addition.csv"`},
		"Content-Type":        []string{"application/octet-stream"},
	}

	got := resolveDocMediaOutputPath("downloads", "tok_123", headers, "")
	want := filepath.Join("downloads", "drive_registry_config_addition.csv")
	if got != want {
		t.Fatalf("resolveDocMediaOutputPath() = %q, want %q", got, want)
	}
}

func TestResolveDocMediaOutputPath_UsesContentTypeFallback(t *testing.T) {
	headers := http.Header{
		"Content-Type": []string{"text/csv; charset=utf-8"},
	}

	got := resolveDocMediaOutputPath("preview", "tok_123", headers, "")
	if got != "preview.csv" {
		t.Fatalf("resolveDocMediaOutputPath() = %q, want %q", got, "preview.csv")
	}
}

func TestResolveDocMediaOutputPath_UsesTokenForDirectoryFallback(t *testing.T) {
	headers := http.Header{
		"Content-Type": []string{"image/png"},
	}

	got := resolveDocMediaOutputPath("downloads/", "tok_123", headers, "")
	want := filepath.Join("downloads", "tok_123.png")
	if got != want {
		t.Fatalf("resolveDocMediaOutputPath() = %q, want %q", got, want)
	}
}
