// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestSlidesMediaDownloadDirectDownload(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/medias/media_direct/download",
		Status: http.StatusOK,
		Body:   []byte("jpeg-bytes"),
		Headers: http.Header{
			"Content-Type": []string{"image/jpeg"},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesMediaDownload, []string{
		"+media-download",
		"--file-token", "media_direct",
		"--output", "assets/cover",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	wantPath := canonicalSlidesMediaDownloadTestPath(t, filepath.Join(dir, "assets", "cover.jpg"))
	if got := data["path"]; got != wantPath {
		t.Fatalf("path = %v, want %s", got, wantPath)
	}
	if got := data["source"]; got != "download" {
		t.Fatalf("source = %v, want download", got)
	}
	if got := data["size"]; got != float64(len("jpeg-bytes")) {
		t.Fatalf("size = %v, want %d", got, len("jpeg-bytes"))
	}
	body, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read downloaded image: %v", err)
	}
	if string(body) != "jpeg-bytes" {
		t.Fatalf("downloaded image = %q, want jpeg-bytes", body)
	}
}

func TestSlidesMediaDownloadFallsBackToSourceFilePreviewOnForbidden(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method:  http.MethodGet,
		URL:     "/open-apis/drive/v1/medias/media_forbidden/download",
		Status:  http.StatusForbidden,
		RawBody: []byte("permission denied"),
	})
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/medias/media_forbidden/preview_download?preview_type=16",
		Status: http.StatusOK,
		Body:   []byte("preview-jpeg"),
		Headers: http.Header{
			"Content-Type": []string{"image/jpeg"},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesMediaDownload, []string{
		"+media-download",
		"--file-token", "media_forbidden",
		"--output", "assets/cover",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected fallback error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	if got := data["source"]; got != "preview" {
		t.Fatalf("source = %v, want preview", got)
	}
	wantPath := canonicalSlidesMediaDownloadTestPath(t, filepath.Join(dir, "assets", "cover.jpg"))
	if got := data["path"]; got != wantPath {
		t.Fatalf("path = %v, want %s", got, wantPath)
	}
	body, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read preview image: %v", err)
	}
	if string(body) != "preview-jpeg" {
		t.Fatalf("preview image = %q, want preview-jpeg", body)
	}
}

func canonicalSlidesMediaDownloadTestPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve path %q: %v", path, err)
	}
	return canonical
}

func TestSlidesMediaDownloadDoesNotFallbackOnNonPermissionError(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method:  http.MethodGet,
		URL:     "/open-apis/drive/v1/medias/media_missing/download",
		Status:  http.StatusNotFound,
		RawBody: []byte("not found"),
	})
	reg.Register(&httpmock.Stub{
		Method:   http.MethodGet,
		URL:      "/open-apis/drive/v1/medias/media_missing/preview_download?preview_type=16",
		Optional: true,
	})

	err := runSlidesShortcut(t, f, stdout, SlidesMediaDownload, []string{
		"+media-download",
		"--file-token", "media_missing",
		"--output", "missing.jpg",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected direct download error")
	}
	if strings.Contains(err.Error(), "preview") {
		t.Fatalf("non-permission error should not fall back: %v", err)
	}
}

func TestSlidesMediaDownloadDryRunIncludesFallback(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	err := runSlidesShortcut(t, f, stdout, SlidesMediaDownload, []string{
		"+media-download",
		"--file-token", "media_dry_run",
		"--output", "assets/cover.jpg",
		"--dry-run",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected dry-run error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"/open-apis/drive/v1/medias/media_dry_run/download",
		"/open-apis/drive/v1/medias/media_dry_run/preview_download",
		"preview_type",
		"assets/cover.jpg",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q: %s", want, out)
		}
	}
}

func TestSlidesMediaDownloadCorrectsExtensionByContentType(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/medias/media_png_ext_jpeg/download",
		Status: http.StatusOK,
		Body:   []byte("jpeg-data"),
		Headers: http.Header{
			"Content-Type": []string{"image/jpeg"},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesMediaDownload, []string{
		"+media-download",
		"--file-token", "media_png_ext_jpeg",
		"--output", "assets/cover.png",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	wantPath := canonicalSlidesMediaDownloadTestPath(t, filepath.Join(dir, "assets", "cover.jpg"))
	if got := data["path"]; got != wantPath {
		t.Fatalf("path = %v, want %s", got, wantPath)
	}
	if got := data["output_adjusted"]; got != true {
		t.Fatalf("output_adjusted = %v, want true", got)
	}
	if got := data["requested_output"]; got != "assets/cover.png" {
		t.Fatalf("requested_output = %v, want assets/cover.png", got)
	}
	body, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read corrected file: %v", err)
	}
	if string(body) != "jpeg-data" {
		t.Fatalf("file content = %q, want jpeg-data", body)
	}
}

func TestSlidesMediaDownloadPreservesMatchingJpegExt(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/medias/media_jpeg_ext/download",
		Status: http.StatusOK,
		Body:   []byte("jpeg-data"),
		Headers: http.Header{
			"Content-Type": []string{"image/jpeg"},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesMediaDownload, []string{
		"+media-download",
		"--file-token", "media_jpeg_ext",
		"--output", "assets/cover.jpeg",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	wantPath := canonicalSlidesMediaDownloadTestPath(t, filepath.Join(dir, "assets", "cover.jpeg"))
	if got := data["path"]; got != wantPath {
		t.Fatalf("path = %v, want %s", got, wantPath)
	}
	if _, adjusted := data["output_adjusted"]; adjusted {
		t.Fatalf("output_adjusted should be omitted for matching .jpeg extension, got %v", data["output_adjusted"])
	}
	body, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(body) != "jpeg-data" {
		t.Fatalf("file content = %q, want jpeg-data", body)
	}
}

func TestSlidesMediaDownloadCorrectsJpgExtToPngForPngContent(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/medias/media_jpg_ext_png/download",
		Status: http.StatusOK,
		Body:   []byte("png-data"),
		Headers: http.Header{
			"Content-Type": []string{"image/png"},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesMediaDownload, []string{
		"+media-download",
		"--file-token", "media_jpg_ext_png",
		"--output", "assets/cover.jpg",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	wantPath := canonicalSlidesMediaDownloadTestPath(t, filepath.Join(dir, "assets", "cover.png"))
	if got := data["path"]; got != wantPath {
		t.Fatalf("path = %v, want %s", got, wantPath)
	}
	if got := data["output_adjusted"]; got != true {
		t.Fatalf("output_adjusted = %v, want true", got)
	}
	if got := data["requested_output"]; got != "assets/cover.jpg" {
		t.Fatalf("requested_output = %v, want assets/cover.jpg", got)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "assets", "cover.jpg")); !os.IsNotExist(statErr) {
		t.Fatalf("requested .jpg path unexpectedly exists, stat error = %v", statErr)
	}
	body, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read corrected file: %v", err)
	}
	if string(body) != "png-data" {
		t.Fatalf("file content = %q, want png-data", body)
	}
}

func TestSlidesMediaDownloadCorrectsExtWithContentTypeParameters(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/medias/media_content_type_params/download",
		Status: http.StatusOK,
		Body:   []byte("jpeg-data"),
		Headers: http.Header{
			"Content-Type": []string{"image/jpeg; charset=binary"},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesMediaDownload, []string{
		"+media-download",
		"--file-token", "media_content_type_params",
		"--output", "assets/cover.png",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	wantPath := canonicalSlidesMediaDownloadTestPath(t, filepath.Join(dir, "assets", "cover.jpg"))
	if got := data["path"]; got != wantPath {
		t.Fatalf("path = %v, want %s", got, wantPath)
	}
	if got := data["output_adjusted"]; got != true {
		t.Fatalf("output_adjusted = %v, want true", got)
	}
}

func TestSlidesMediaDownloadPreservesExtForUnknownContentType(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/medias/media_unknown_type/download",
		Status: http.StatusOK,
		Body:   []byte("unknown-bytes"),
		Headers: http.Header{
			"Content-Type": []string{"application/octet-stream"},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesMediaDownload, []string{
		"+media-download",
		"--file-token", "media_unknown_type",
		"--output", "assets/cover.png",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	wantPath := canonicalSlidesMediaDownloadTestPath(t, filepath.Join(dir, "assets", "cover.png"))
	if got := data["path"]; got != wantPath {
		t.Fatalf("path = %v, want %s (unknown Content-Type should not change user-specified ext)", got, wantPath)
	}
	if _, adjusted := data["output_adjusted"]; adjusted {
		t.Fatalf("output_adjusted should be omitted for unknown Content-Type, got %v", data["output_adjusted"])
	}
}

func TestSlidesMediaDownloadPreservesExtForEmptyContentType(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method:  http.MethodGet,
		URL:     "/open-apis/drive/v1/medias/media_no_ct/download",
		Status:  http.StatusOK,
		Body:    []byte("no-ct-bytes"),
		Headers: http.Header{},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesMediaDownload, []string{
		"+media-download",
		"--file-token", "media_no_ct",
		"--output", "assets/cover.png",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	wantPath := canonicalSlidesMediaDownloadTestPath(t, filepath.Join(dir, "assets", "cover.png"))
	if got := data["path"]; got != wantPath {
		t.Fatalf("path = %v, want %s (empty Content-Type should not change user-specified ext)", got, wantPath)
	}
	if _, adjusted := data["output_adjusted"]; adjusted {
		t.Fatalf("output_adjusted should be omitted for empty Content-Type, got %v", data["output_adjusted"])
	}
}

func TestSlidesMediaDownloadWebpContentCorrection(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/medias/media_webp/download",
		Status: http.StatusOK,
		Body:   []byte("webp-bytes"),
		Headers: http.Header{
			"Content-Type": []string{"image/webp"},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesMediaDownload, []string{
		"+media-download",
		"--file-token", "media_webp",
		"--output", "assets/cover.png",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	wantPath := canonicalSlidesMediaDownloadTestPath(t, filepath.Join(dir, "assets", "cover.webp"))
	if got := data["path"]; got != wantPath {
		t.Fatalf("path = %v, want %s", got, wantPath)
	}
	if got := data["output_adjusted"]; got != true {
		t.Fatalf("output_adjusted = %v, want true", got)
	}
}

func TestSlidesMediaDownloadCaseInsensitiveExtMatch(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/medias/media_upper_ext/download",
		Status: http.StatusOK,
		Body:   []byte("jpeg-data"),
		Headers: http.Header{
			"Content-Type": []string{"image/jpeg"},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesMediaDownload, []string{
		"+media-download",
		"--file-token", "media_upper_ext",
		"--output", "assets/cover.JPG",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	wantPath := canonicalSlidesMediaDownloadTestPath(t, filepath.Join(dir, "assets", "cover.JPG"))
	if got := data["path"]; got != wantPath {
		t.Fatalf("path = %v, want %s", got, wantPath)
	}
	if _, adjusted := data["output_adjusted"]; adjusted {
		t.Fatalf("output_adjusted should be omitted for case-insensitive .JPG match, got %v", data["output_adjusted"])
	}
}

func TestSlidesMediaDownloadAdjustsExtBeforeAvoidingExistingPath(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatalf("create output dir: %v", err)
	}
	existingPath := filepath.Join(dir, "assets", "cover.png")
	if err := os.WriteFile(existingPath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write existing output: %v", err)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/medias/media_ext_dedup/download",
		Status: http.StatusOK,
		Body:   []byte("new-jpeg"),
		Headers: http.Header{
			"Content-Type": []string{"image/jpeg"},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesMediaDownload, []string{
		"+media-download",
		"--file-token", "media_ext_dedup",
		"--output", "assets/cover.png",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, err := os.ReadFile(existingPath); err != nil || string(got) != "existing" {
		t.Fatalf("existing .png output overwritten: content=%q, err=%v", got, err)
	}
	actualPath := filepath.Join(dir, "assets", "cover.jpg")
	if got, readErr := os.ReadFile(actualPath); readErr != nil || string(got) != "new-jpeg" {
		t.Fatalf("corrected output = %q, err=%v", got, readErr)
	}
	actualPath = canonicalSlidesMediaDownloadTestPath(t, actualPath)
	data := decodeShortcutData(t, stdout)
	if data["requested_output"] != "assets/cover.png" || data["output"] != actualPath || data["output_adjusted"] != true {
		t.Fatalf("adjusted output metadata = %#v", data)
	}
}

func TestSlidesMediaDownloadContentDispositionExtOverriddenByContentType(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/medias/media_cd_wrong_ext/download",
		Status: http.StatusOK,
		Body:   []byte("png-data"),
		Headers: http.Header{
			"Content-Type":        []string{"image/png"},
			"Content-Disposition": []string{`attachment; filename="image.jpg"`},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesMediaDownload, []string{
		"+media-download",
		"--file-token", "media_cd_wrong_ext",
		"--output-dir", "downloads",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	wantPath := canonicalSlidesMediaDownloadTestPath(t, filepath.Join(dir, "downloads", "image.png"))
	if got := data["path"]; got != wantPath {
		t.Fatalf("path = %v, want %s (Content-Disposition .jpg should be overridden by Content-Type image/png)", got, wantPath)
	}
	body, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(body) != "png-data" {
		t.Fatalf("file content = %q, want png-data", body)
	}
}
