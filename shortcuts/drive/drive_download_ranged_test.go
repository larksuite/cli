// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

// withDriveDownloadPartSize shrinks ranged parts so tests can span several.
func withDriveDownloadPartSize(t *testing.T, size int64) {
	t.Helper()
	prev := driveDownloadPartSize
	driveDownloadPartSize = size
	t.Cleanup(func() { driveDownloadPartSize = prev })
}

// registerDriveDownloadRevision answers one metadata lookup with a revision;
// a nil revision omits latest_modify_time.
func registerDriveDownloadRevision(reg *httpmock.Registry, fileToken string, revision interface{}) *httpmock.Stub {
	meta := map[string]interface{}{"doc_token": fileToken, "doc_type": "file", "title": "remote.bin"}
	if revision != nil {
		meta["latest_modify_time"] = revision
	}
	stub := &httpmock.Stub{
		Method: http.MethodPost,
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{meta},
			},
		},
	}
	reg.Register(stub)
	return stub
}

// registerDriveDownloadPart answers one Range request with bytes [start, end].
func registerDriveDownloadPart(reg *httpmock.Registry, fileToken string, content []byte, start, end int) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method:  http.MethodGet,
		URL:     "/open-apis/drive/v1/files/" + fileToken + "/download",
		Status:  http.StatusPartialContent,
		RawBody: content[start : end+1],
		Headers: http.Header{
			"Content-Type":  []string{"application/octet-stream"},
			"Content-Range": []string{fmt.Sprintf("bytes %d-%d/%d", start, end, len(content))},
		},
	}
	reg.Register(stub)
	return stub
}

// TestDriveDownloadCombinesRangedParts verifies large files are fetched in
// bounded Range requests and re-check the revision before committing.
func TestDriveDownloadCombinesRangedParts(t *testing.T) {
	withDriveDownloadPartSize(t, 4)
	content := []byte("abcdefghij")
	f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveDownloadExportAuth(reg, "file_ranged", true)
	before := registerDriveDownloadRevision(reg, "file_ranged", "1792558577")
	parts := []*httpmock.Stub{
		registerDriveDownloadPart(reg, "file_ranged", content, 0, 3),
		registerDriveDownloadPart(reg, "file_ranged", content, 4, 7),
		registerDriveDownloadPart(reg, "file_ranged", content, 8, 9),
	}
	after := registerDriveDownloadRevision(reg, "file_ranged", "1792558577")

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_ranged",
		"--output", "ranged.bin",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr=%s", err, stderr.String())
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "ranged.bin"))
	if err != nil || !bytes.Equal(data, content) {
		t.Fatalf("downloaded content = %q, err=%v; want %q", data, err, content)
	}
	for i, want := range []string{"bytes=0-3", "bytes=4-7", "bytes=8-9"} {
		if got := parts[i].CapturedHeaders.Get("Range"); got != want {
			t.Fatalf("part %d Range = %q, want %q", i, got, want)
		}
	}
	if len(before.CapturedBodies) != 1 || len(after.CapturedBodies) != 1 {
		t.Fatalf("revision reads before=%d after=%d, want 1 each", len(before.CapturedBodies), len(after.CapturedBodies))
	}
	if !strings.Contains(stdout.String(), `"size_bytes": 10`) {
		t.Fatalf("stdout missing size: %s", stdout.String())
	}
	if strings.Contains(stderr.String(), "revision unavailable") {
		t.Fatalf("unexpected single-request warning: %s", stderr.String())
	}
}

// TestDriveDownloadRejectsFileReplacedDuringParts verifies an in-place
// replacement between parts fails without committing mixed bytes.
func TestDriveDownloadRejectsFileReplacedDuringParts(t *testing.T) {
	withDriveDownloadPartSize(t, 4)
	content := []byte("abcdefghij")
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveDownloadExportAuth(reg, "file_replaced", true)
	registerDriveDownloadRevision(reg, "file_replaced", "1792558577")
	registerDriveDownloadPart(reg, "file_replaced", content, 0, 3)
	registerDriveDownloadPart(reg, "file_replaced", content, 4, 7)
	registerDriveDownloadPart(reg, "file_replaced", content, 8, 9)
	registerDriveDownloadRevision(reg, "file_replaced", "1792558600")

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_replaced",
		"--output", "replaced.bin",
		"--as", "bot",
	}, f, stdout)

	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkRepresentationChanged || !problem.Retryable {
		t.Fatalf("err=%v problem=%+v, want retryable representation_changed", err, problem)
	}
	entries, readErr := os.ReadDir(tmpDir)
	if readErr != nil {
		t.Fatalf("ReadDir() error: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("output directory = %v, want nothing committed", entries)
	}
}

// TestDriveDownloadWithoutRevisionUsesOneFullResponse verifies that, when the
// revision cannot be read, parts are never combined from an unpinned file.
func TestDriveDownloadWithoutRevisionUsesOneFullResponse(t *testing.T) {
	withDriveDownloadPartSize(t, 4)
	content := []byte("abcdefghij")
	f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveDownloadExportAuth(reg, "file_unpinned", true)
	reg.Register(&httpmock.Stub{
		Method: http.MethodPost,
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Status: http.StatusInternalServerError,
		Body:   map[string]interface{}{"code": 1, "msg": "metadata unavailable"},
	})
	registerDriveDownloadPart(reg, "file_unpinned", content, 0, 3)
	full := &httpmock.Stub{
		Method:  http.MethodGet,
		URL:     "/open-apis/drive/v1/files/file_unpinned/download",
		RawBody: content,
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	}
	reg.Register(full)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_unpinned",
		"--output", "unpinned.bin",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr=%s", err, stderr.String())
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "unpinned.bin"))
	if err != nil || !bytes.Equal(data, content) {
		t.Fatalf("downloaded content = %q, err=%v; want %q", data, err, content)
	}
	if got := full.CapturedHeaders.Get("Range"); got != "" {
		t.Fatalf("fallback Range = %q, want one full response", got)
	}
}

// TestDriveDownloadSinglePartSkipsRevisionRecheck verifies a file that fits in
// the first response needs no second metadata read.
func TestDriveDownloadSinglePartSkipsRevisionRecheck(t *testing.T) {
	content := []byte("small")
	f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveDownloadExportAuth(reg, "file_small", true)
	registerDriveDownloadRevision(reg, "file_small", "1792558577")
	part := registerDriveDownloadPart(reg, "file_small", content, 0, len(content)-1)
	extra := &httpmock.Stub{
		Method:   http.MethodPost,
		URL:      "/open-apis/drive/v1/metas/batch_query",
		Body:     map[string]interface{}{"code": 0, "data": map[string]interface{}{"metas": []interface{}{}}},
		Optional: true,
	}
	reg.Register(extra)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_small",
		"--output", "small.bin",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr=%s", err, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(tmpDir, "small.bin"))
	if err != nil || !bytes.Equal(data, content) {
		t.Fatalf("downloaded content = %q, err=%v; want %q", data, err, content)
	}
	if got := part.CapturedHeaders.Get("Range"); got != fmt.Sprintf("bytes=0-%d", driveDownloadPartSize-1) {
		t.Fatalf("first Range = %q, want one part-sized probe", got)
	}
	if len(extra.CapturedBodies) != 0 {
		t.Fatalf("revision re-check ran %d times for a single-part file, want 0", len(extra.CapturedBodies))
	}
}

// TestDriveDownloadNumericRevisionCombinesParts verifies a latest_modify_time
// emitted as a JSON number still pins the revision.
func TestDriveDownloadNumericRevisionCombinesParts(t *testing.T) {
	withDriveDownloadPartSize(t, 4)
	content := []byte("abcdefghij")
	f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveDownloadExportAuth(reg, "file_numeric", true)
	registerDriveDownloadRevision(reg, "file_numeric", 1792558577)
	registerDriveDownloadPart(reg, "file_numeric", content, 0, 3)
	registerDriveDownloadPart(reg, "file_numeric", content, 4, 7)
	last := registerDriveDownloadPart(reg, "file_numeric", content, 8, 9)
	registerDriveDownloadRevision(reg, "file_numeric", 1792558577)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_numeric",
		"--output", "numeric.bin",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr=%s", err, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(tmpDir, "numeric.bin"))
	if err != nil || !bytes.Equal(data, content) {
		t.Fatalf("downloaded content = %q, err=%v; want %q", data, err, content)
	}
	if got := last.CapturedHeaders.Get("Range"); got != "bytes=8-9" {
		t.Fatalf("last Range = %q, want bytes=8-9", got)
	}
}

// TestDriveDownloadMissingRevisionUsesOneFullResponse verifies metadata
// without latest_modify_time never lets parts of an unpinned file combine.
func TestDriveDownloadMissingRevisionUsesOneFullResponse(t *testing.T) {
	withDriveDownloadPartSize(t, 4)
	content := []byte("abcdefghij")
	f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())
	registerDriveDownloadExportAuth(reg, "file_no_revision", true)
	registerDriveDownloadRevision(reg, "file_no_revision", nil)
	registerDriveDownloadPart(reg, "file_no_revision", content, 0, 3)
	full := &httpmock.Stub{
		Method:  http.MethodGet,
		URL:     "/open-apis/drive/v1/files/file_no_revision/download",
		RawBody: content,
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	}
	reg.Register(full)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	err := mountAndRunDrive(t, DriveDownload, []string{
		"+download",
		"--file-token", "file_no_revision",
		"--output", "no_revision.bin",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr=%s", err, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(tmpDir, "no_revision.bin"))
	if err != nil || !bytes.Equal(data, content) {
		t.Fatalf("downloaded content = %q, err=%v; want %q", data, err, content)
	}
	if got := full.CapturedHeaders.Get("Range"); got != "" {
		t.Fatalf("fallback Range = %q, want one full response", got)
	}
}
