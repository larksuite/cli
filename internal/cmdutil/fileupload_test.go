// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs/localfileio"
)

func TestParseFileFlag(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		defaultField string
		wantField    string
		wantPath     string
		wantStdin    bool
	}{
		{
			name:         "simple filename uses default field",
			raw:          "photo.jpg",
			defaultField: "file",
			wantField:    "file",
			wantPath:     "photo.jpg",
			wantStdin:    false,
		},
		{
			name:         "simple filename with custom default",
			raw:          "photo.jpg",
			defaultField: "image",
			wantField:    "image",
			wantPath:     "photo.jpg",
			wantStdin:    false,
		},
		{
			name:         "explicit field prefix",
			raw:          "image=photo.jpg",
			defaultField: "file",
			wantField:    "image",
			wantPath:     "photo.jpg",
			wantStdin:    false,
		},
		{
			name:         "stdin bare",
			raw:          "-",
			defaultField: "file",
			wantField:    "file",
			wantPath:     "",
			wantStdin:    true,
		},
		{
			name:         "stdin with field prefix",
			raw:          "image=-",
			defaultField: "file",
			wantField:    "image",
			wantPath:     "",
			wantStdin:    true,
		},
		{
			name:         "path with equals sign (only first equals splits)",
			raw:          "field=path/to/file=1.jpg",
			defaultField: "file",
			wantField:    "field",
			wantPath:     "path/to/file=1.jpg",
			wantStdin:    false,
		},
		{
			name:         "absolute path no prefix",
			raw:          "/tmp/photo.jpg",
			defaultField: "file",
			wantField:    "file",
			wantPath:     "/tmp/photo.jpg",
			wantStdin:    false,
		},
		{
			name:         "absolute path with field prefix",
			raw:          "image=/tmp/photo.jpg",
			defaultField: "file",
			wantField:    "image",
			wantPath:     "/tmp/photo.jpg",
			wantStdin:    false,
		},
		{
			name:         "empty field prefix falls through to default",
			raw:          "=photo.jpg",
			defaultField: "file",
			wantField:    "file",
			wantPath:     "=photo.jpg",
			wantStdin:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, path, isStdin := ParseFileFlag(tt.raw, tt.defaultField)
			if field != tt.wantField {
				t.Errorf("field = %q, want %q", field, tt.wantField)
			}
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}
			if isStdin != tt.wantStdin {
				t.Errorf("isStdin = %v, want %v", isStdin, tt.wantStdin)
			}
		})
	}
}

func TestValidateFileFlag(t *testing.T) {
	tests := []struct {
		name       string
		file       string
		params     string
		data       string
		outputPath string
		pageAll    bool
		httpMethod string
		wantErr    string // empty means no error
	}{
		{
			name:       "empty file is valid",
			file:       "",
			httpMethod: "GET",
			wantErr:    "",
		},
		{
			name:       "empty file path",
			file:       "field=",
			httpMethod: "POST",
			wantErr:    "--file: empty file path",
		},
		{
			name:       "file with output",
			file:       "photo.jpg",
			outputPath: "out.json",
			httpMethod: "POST",
			wantErr:    "--file and --output are mutually exclusive",
		},
		{
			name:       "file with page-all",
			file:       "photo.jpg",
			pageAll:    true,
			httpMethod: "POST",
			wantErr:    "--file and --page-all are mutually exclusive",
		},
		{
			name:       "stdin file with stdin data",
			file:       "-",
			data:       "-",
			httpMethod: "POST",
			wantErr:    "--file and --data cannot both read from stdin",
		},
		{
			name:       "stdin file with stdin params",
			file:       "-",
			params:     "-",
			httpMethod: "POST",
			wantErr:    "--file and --params cannot both read from stdin",
		},
		{
			name:       "file with GET method",
			file:       "photo.jpg",
			httpMethod: "GET",
			wantErr:    "--file requires POST, PUT, PATCH, or DELETE method",
		},
		{
			name:       "file with POST method",
			file:       "photo.jpg",
			httpMethod: "POST",
			wantErr:    "",
		},
		{
			name:       "file with PUT method",
			file:       "photo.jpg",
			httpMethod: "PUT",
			wantErr:    "",
		},
		{
			name:       "file with PATCH method",
			file:       "photo.jpg",
			httpMethod: "PATCH",
			wantErr:    "",
		},
		{
			name:       "file with DELETE method",
			file:       "photo.jpg",
			httpMethod: "DELETE",
			wantErr:    "",
		},
		{
			name:       "stdin with field prefix and data stdin",
			file:       "image=-",
			data:       "-",
			httpMethod: "POST",
			wantErr:    "--file and --data cannot both read from stdin",
		},
		{
			name:       "stdin with field prefix and params stdin",
			file:       "image=-",
			params:     "-",
			httpMethod: "POST",
			wantErr:    "--file and --params cannot both read from stdin",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFileFlag(tt.file, tt.params, tt.data, tt.outputPath, tt.pageAll, tt.httpMethod)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestBuildFormdata(t *testing.T) {
	fio := &localfileio.LocalFileIO{}

	t.Run("stdin success", func(t *testing.T) {
		stdin := bytes.NewReader([]byte("file-content-here"))
		fd, err := BuildFormdata(fio, "file", "", true, stdin, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fd == nil {
			t.Fatal("expected non-nil Formdata")
		}
	})

	t.Run("stdin nil reader", func(t *testing.T) {
		_, err := BuildFormdata(fio, "file", "", true, nil, nil)
		if err == nil {
			t.Fatal("expected error for nil stdin")
		}
		if !strings.Contains(err.Error(), "stdin is not available") {
			t.Errorf("error = %q, want containing %q", err.Error(), "stdin is not available")
		}
	})

	t.Run("stdin empty", func(t *testing.T) {
		stdin := bytes.NewReader([]byte{})
		_, err := BuildFormdata(fio, "file", "", true, stdin, nil)
		if err == nil {
			t.Fatal("expected error for empty stdin")
		}
		if !strings.Contains(err.Error(), "stdin is empty") {
			t.Errorf("error = %q, want containing %q", err.Error(), "stdin is empty")
		}
	})

	t.Run("file open success", func(t *testing.T) {
		dir := t.TempDir()
		TestChdir(t, dir)

		if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0600); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		fd, err := BuildFormdata(fio, "photo", "test.txt", false, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fd == nil {
			t.Fatal("expected non-nil Formdata")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		dir := t.TempDir()
		TestChdir(t, dir)

		_, err := BuildFormdata(fio, "file", "nonexistent.txt", false, nil, nil)
		if err == nil {
			t.Fatal("expected error for missing file")
		}
		if !strings.Contains(err.Error(), "cannot open file:") {
			t.Errorf("error = %q, want containing %q", err.Error(), "cannot open file:")
		}
	})

	t.Run("dataJSON fields added", func(t *testing.T) {
		dir := t.TempDir()
		TestChdir(t, dir)

		if err := os.WriteFile(filepath.Join(dir, "upload.bin"), []byte("data"), 0600); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		dataJSON := map[string]any{
			"file_name":   "report.pdf",
			"parent_type": "doc_image",
			"size":        1024,
		}

		fd, err := BuildFormdata(fio, "file", "upload.bin", false, nil, dataJSON)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fd == nil {
			t.Fatal("expected non-nil Formdata")
		}
	})

	t.Run("dataJSON nil is fine", func(t *testing.T) {
		stdin := bytes.NewReader([]byte("content"))
		fd, err := BuildFormdata(fio, "file", "", true, stdin, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fd == nil {
			t.Fatal("expected non-nil Formdata")
		}
	})

	t.Run("dataJSON non-map is ignored", func(t *testing.T) {
		stdin := bytes.NewReader([]byte("content"))
		fd, err := BuildFormdata(fio, "file", "", true, stdin, "not-a-map")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fd == nil {
			t.Fatal("expected non-nil Formdata")
		}
	})
}
