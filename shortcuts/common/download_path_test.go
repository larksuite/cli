// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"net/http"
	"testing"
)

func TestResolveDownloadFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		header   http.Header
		fallback string
		want     string
	}{
		{
			name: "content disposition filename wins",
			header: http.Header{
				"Content-Disposition": []string{`attachment; filename="report-v7.md"`},
			},
			fallback: "boxcn123",
			want:     "report-v7.md",
		},
		{
			name: "path traversal in header is stripped",
			header: http.Header{
				"Content-Disposition": []string{`attachment; filename="../nested/report-v7.md"`},
			},
			fallback: "boxcn123",
			want:     "report-v7.md",
		},
		{
			name:     "fallback when header missing",
			header:   http.Header{},
			fallback: "boxcn123",
			want:     "boxcn123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolveDownloadFileName(tt.header, tt.fallback); got != tt.want {
				t.Fatalf("ResolveDownloadFileName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAutoAppendDownloadExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		path   string
		header http.Header
		want   string
	}{
		{
			name: "explicit extension is preserved",
			path: "artifact.bin",
			header: http.Header{
				"Content-Type": []string{"text/csv; charset=utf-8"},
			},
			want: "artifact.bin",
		},
		{
			name: "appends extension from content type",
			path: "artifact",
			header: http.Header{
				"Content-Type": []string{"text/csv; charset=utf-8"},
			},
			want: "artifact.csv",
		},
		{
			name: "appends extension from content disposition when content type is generic",
			path: "artifact",
			header: http.Header{
				"Content-Type":        []string{"application/octet-stream"},
				"Content-Disposition": []string{`attachment; filename="report-v7.md"`},
			},
			want: "artifact.md",
		},
		{
			name: "trailing dot is normalized before append",
			path: "artifact.",
			header: http.Header{
				"Content-Type": []string{"text/plain; charset=utf-8"},
			},
			want: "artifact.txt",
		},
		{
			name: "unknown type keeps suffixless path",
			path: "artifact.",
			header: http.Header{
				"Content-Type": []string{"application/octet-stream"},
			},
			want: "artifact",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, _ := AutoAppendDownloadExtension(tt.path, tt.header, "")
			if got != tt.want {
				t.Fatalf("AutoAppendDownloadExtension() = %q, want %q", got, tt.want)
			}
		})
	}
}
