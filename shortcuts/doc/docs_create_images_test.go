// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseImageRefs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		markdown string
		want     []imageRef
	}{
		{
			name:     "no images",
			markdown: "hello world",
			want:     nil,
		},
		{
			name:     "single local image",
			markdown: "text ![alt](images/photo.png) more",
			want:     []imageRef{{fullMatch: "![alt](images/photo.png)", path: "images/photo.png"}},
		},
		{
			name:     "single URL image",
			markdown: "![logo](https://example.com/logo.png)",
			want:     []imageRef{{fullMatch: "![logo](https://example.com/logo.png)", path: "https://example.com/logo.png"}},
		},
		{
			name:     "multiple images",
			markdown: "![a](a.png) text ![b](https://x.com/b.jpg) ![c](./c.gif)",
			want: []imageRef{
				{fullMatch: "![a](a.png)", path: "a.png"},
				{fullMatch: "![b](https://x.com/b.jpg)", path: "https://x.com/b.jpg"},
				{fullMatch: "![c](./c.gif)", path: "./c.gif"},
			},
		},
		{
			name:     "empty alt text",
			markdown: "![](image.png)",
			want:     []imageRef{{fullMatch: "![](image.png)", path: "image.png"}},
		},
		{
			name:     "path with subdirectory",
			markdown: "![screenshot](case_images/shot1.jpg)",
			want:     []imageRef{{fullMatch: "![screenshot](case_images/shot1.jpg)", path: "case_images/shot1.jpg"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseImageRefs(tt.markdown)
			if len(got) != len(tt.want) {
				t.Fatalf("parseImageRefs() returned %d refs, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i].fullMatch != tt.want[i].fullMatch {
					t.Errorf("[%d] fullMatch = %q, want %q", i, got[i].fullMatch, tt.want[i].fullMatch)
				}
				if got[i].path != tt.want[i].path {
					t.Errorf("[%d] path = %q, want %q", i, got[i].path, tt.want[i].path)
				}
			}
		})
	}
}

func TestIsLocalPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		{"images/photo.png", true},
		{"./photo.png", true},
		{"photo.png", true},
		{"https://example.com/photo.png", false},
		{"http://example.com/photo.png", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			if got := isLocalPath(tt.path); got != tt.want {
				t.Errorf("isLocalPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestHasLocalImages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		markdown string
		want     bool
	}{
		{"no images", "just text", false},
		{"only URL images", "![a](https://example.com/a.png)", false},
		{"local image", "![a](photo.png)", true},
		{"mixed", "![a](https://x.com/a.png) ![b](local.jpg)", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := hasLocalImages(tt.markdown); got != tt.want {
				t.Errorf("hasLocalImages() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSafeImagePath(t *testing.T) {
	t.Parallel()

	// Create a temp directory with a test image
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "images")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	testFile := filepath.Join(subDir, "test.png")
	if err := os.WriteFile(testFile, []byte("fake-png"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("valid relative path", func(t *testing.T) {
		t.Parallel()
		got, err := safeImagePath("images/test.png", tmpDir)
		if err != nil {
			t.Fatalf("safeImagePath() error: %v", err)
		}
		// Resolve symlinks for comparison (macOS /var -> /private/var)
		wantResolved, _ := filepath.EvalSymlinks(testFile)
		if got != wantResolved {
			t.Errorf("safeImagePath() = %q, want %q", got, wantResolved)
		}
	})

	t.Run("rejects absolute path", func(t *testing.T) {
		t.Parallel()
		_, err := safeImagePath("/etc/passwd", tmpDir)
		if err == nil {
			t.Fatal("safeImagePath() expected error for absolute path")
		}
	})

	t.Run("rejects traversal outside base", func(t *testing.T) {
		t.Parallel()
		_, err := safeImagePath("../../etc/passwd", tmpDir)
		if err == nil {
			t.Fatal("safeImagePath() expected error for path traversal")
		}
	})

	t.Run("rejects non-existent file", func(t *testing.T) {
		t.Parallel()
		_, err := safeImagePath("images/nonexistent.png", tmpDir)
		if err == nil {
			t.Fatal("safeImagePath() expected error for non-existent file")
		}
	})
}

func TestValidateImageFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	t.Run("valid png file", func(t *testing.T) {
		t.Parallel()
		f := filepath.Join(tmpDir, "test.png")
		if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := validateImageFile(f)
		if err != nil {
			t.Fatalf("validateImageFile() unexpected error: %v", err)
		}
	})

	t.Run("rejects unsupported extension", func(t *testing.T) {
		t.Parallel()
		f := filepath.Join(tmpDir, "test.svg")
		if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := validateImageFile(f)
		if err == nil {
			t.Fatal("validateImageFile() expected error for unsupported format")
		}
	})

	t.Run("valid jpg file", func(t *testing.T) {
		t.Parallel()
		f := filepath.Join(tmpDir, "test.jpg")
		if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := validateImageFile(f)
		if err != nil {
			t.Fatalf("validateImageFile() unexpected error: %v", err)
		}
	})

	t.Run("valid webp file", func(t *testing.T) {
		t.Parallel()
		f := filepath.Join(tmpDir, "test.webp")
		if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := validateImageFile(f)
		if err != nil {
			t.Fatalf("validateImageFile() unexpected error: %v", err)
		}
	})
}

func TestExtractDocumentID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result map[string]interface{}
		want   string
	}{
		{
			name:   "direct document_id",
			result: map[string]interface{}{"document_id": "doc123"},
			want:   "doc123",
		},
		{
			name:   "from URL",
			result: map[string]interface{}{"url": "https://example.com/docx/abc456"},
			want:   "abc456",
		},
		{
			name:   "empty result",
			result: map[string]interface{}{},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := extractDocumentID(tt.result); got != tt.want {
				t.Errorf("extractDocumentID() = %q, want %q", got, tt.want)
			}
		})
	}
}
