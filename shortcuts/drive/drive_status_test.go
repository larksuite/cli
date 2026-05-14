// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

// driveStatusScopedTokenResolver returns a token with caller-controlled scopes
// so tests can deterministically exercise the shortcut scope preflight.
type driveStatusScopedTokenResolver struct {
	scopes string
}

// ResolveToken satisfies credential.TokenProvider for scope-preflight tests.
func (r *driveStatusScopedTokenResolver) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.TokenResult, error) {
	return &credential.TokenResult{Token: "test-token", Scopes: r.scopes}, nil
}

// TestDriveStatusCategorizesByHash exercises the four-bucket classification
// against a real walk of the temp dir and a mocked Drive listing.
func TestDriveStatusCategorizesByHash(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	// Local layout:
	//   local/a.txt        — also on remote with different content → modified
	//   local/b.txt        — only local                            → new_local
	//   local/sub/c.txt    — also on remote with same content      → unchanged
	// Remote-only:
	//   d.txt                                                       → new_remote
	if err := os.MkdirAll("local/sub", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/a.txt", []byte("aaa"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}
	if err := os.WriteFile("local/b.txt", []byte("bbb"), 0o644); err != nil {
		t.Fatalf("WriteFile b.txt: %v", err)
	}
	if err := os.WriteFile("local/sub/c.txt", []byte("ccc"), 0o644); err != nil {
		t.Fatalf("WriteFile sub/c.txt: %v", err)
	}

	// Root folder list — order matters: stubs match in registration order.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_a", "name": "a.txt", "type": "file"},
					map[string]interface{}{"token": "tok_sub", "name": "sub", "type": "folder"},
					map[string]interface{}{"token": "tok_d", "name": "d.txt", "type": "file"},
					// noise: an online doc and a shortcut should be ignored
					map[string]interface{}{"token": "tok_doc", "name": "ignored.docx", "type": "docx"},
					map[string]interface{}{"token": "tok_sc", "name": "ignored.lnk", "type": "shortcut"},
				},
				"has_more": false,
			},
		},
	})

	// Subfolder list
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=tok_sub",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_c", "name": "c.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})

	// Download a.txt: remote content differs from local "aaa" → modified.
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_a/download",
		Status:  200,
		Body:    []byte("AAA"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	// Download c.txt: remote content matches local "ccc" → unchanged.
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_c/download",
		Status:  200,
		Body:    []byte("ccc"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"detection": "exact"`) {
		t.Fatalf("output missing detection=exact\noutput: %s", out)
	}
	checks := []struct {
		bucket string
		path   string
		token  string
	}{
		{"new_local", "b.txt", ""},
		{"new_remote", "d.txt", "tok_d"},
		{"modified", "a.txt", "tok_a"},
		{"unchanged", "sub/c.txt", "tok_c"},
	}
	for _, c := range checks {
		if !strings.Contains(out, `"`+c.bucket+`":`) {
			t.Errorf("output missing bucket %q\noutput: %s", c.bucket, out)
		}
		if !strings.Contains(out, `"rel_path": "`+c.path+`"`) {
			t.Errorf("output missing rel_path %q (expected in %s)\noutput: %s", c.path, c.bucket, out)
		}
		if c.token != "" && !strings.Contains(out, `"file_token": "`+c.token+`"`) {
			t.Errorf("output missing file_token %q (expected in %s)\noutput: %s", c.token, c.bucket, out)
		}
	}

	if strings.Contains(out, "ignored.docx") || strings.Contains(out, "ignored.lnk") {
		t.Errorf("output should skip docx/shortcut entries\noutput: %s", out)
	}

	reg.Verify(t)
}

func TestDriveStatusQuickCategorizesByModifiedTimeWithoutDownloads(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	if err := os.MkdirAll("local/sub", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/a.txt", []byte("local-a"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}
	if err := os.WriteFile("local/b.txt", []byte("local-b"), 0o644); err != nil {
		t.Fatalf("WriteFile b.txt: %v", err)
	}
	if err := os.WriteFile("local/sub/c.txt", []byte("local-c"), 0o644); err != nil {
		t.Fatalf("WriteFile sub/c.txt: %v", err)
	}

	matchTime := time.Unix(1715594880, 0)
	changedTime := time.Unix(1715594940, 0)
	if err := os.Chtimes("local/a.txt", matchTime, matchTime); err != nil {
		t.Fatalf("Chtimes a.txt: %v", err)
	}
	if err := os.Chtimes("local/sub/c.txt", changedTime, changedTime); err != nil {
		t.Fatalf("Chtimes sub/c.txt: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_a", "name": "a.txt", "type": "file", "modified_time": "1715594880"},
					map[string]interface{}{"token": "tok_sub", "name": "sub", "type": "folder"},
					map[string]interface{}{"token": "tok_d", "name": "d.txt", "type": "file", "modified_time": "1715595000"},
				},
				"has_more": false,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=tok_sub",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_c", "name": "c.txt", "type": "file", "modified_time": "1715594880"},
				},
				"has_more": false,
			},
		},
	})

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--quick",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"detection": "quick"`) {
		t.Fatalf("output missing detection=quick\noutput: %s", out)
	}
	checks := []struct {
		bucket string
		path   string
		token  string
	}{
		{"new_local", "b.txt", ""},
		{"new_remote", "d.txt", "tok_d"},
		{"modified", "sub/c.txt", "tok_c"},
		{"unchanged", "a.txt", "tok_a"},
	}
	for _, c := range checks {
		if !strings.Contains(out, `"`+c.bucket+`":`) {
			t.Errorf("output missing bucket %q\noutput: %s", c.bucket, out)
		}
		if !strings.Contains(out, `"rel_path": "`+c.path+`"`) {
			t.Errorf("output missing rel_path %q (expected in %s)\noutput: %s", c.path, c.bucket, out)
		}
		if c.token != "" && !strings.Contains(out, `"file_token": "`+c.token+`"`) {
			t.Errorf("output missing file_token %q (expected in %s)\noutput: %s", c.token, c.bucket, out)
		}
	}

	reg.Verify(t)
}

// TestDriveStatusQuickMarksUntrustedTimestampAsModified locks in the
// conservative fallback for malformed remote modified_time values.
func TestDriveStatusQuickMarksUntrustedTimestampAsModified(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/a.txt", []byte("local"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_a", "name": "a.txt", "type": "file", "modified_time": "not-a-timestamp"},
				},
				"has_more": false,
			},
		},
	})

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--quick",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"detection": "quick"`) {
		t.Fatalf("output missing detection=quick\noutput: %s", out)
	}
	if !strings.Contains(out, `"modified":`) || !strings.Contains(out, `"rel_path": "a.txt"`) {
		t.Fatalf("invalid remote modified_time must fall back to modified\noutput: %s", out)
	}

	reg.Verify(t)
}

// TestDriveStatusExactRejectsMissingDownloadScope proves that exact mode keeps
// requiring drive:file:download even after quick mode made download optional.
func TestDriveStatusExactRejectsMissingDownloadScope(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	f.Credential = credential.NewCredentialProvider(nil, nil, &driveStatusScopedTokenResolver{scopes: "drive:drive.metadata:readonly"}, nil)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/a.txt", []byte("local"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected missing_scope error for exact mode without drive:file:download")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected structured exit error, got %T", err)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "missing_scope" {
		t.Fatalf("expected missing_scope detail, got %#v", exitErr.Detail)
	}
	if !strings.Contains(err.Error(), "missing required scope(s): drive:file:download") {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitErr.Detail == nil || !strings.Contains(exitErr.Detail.Hint, "auth login --scope") {
		t.Fatalf("missing scope hint not found in detail: %#v", exitErr.Detail)
	}
	if !strings.Contains(err.Error(), "drive:file:download") {
		t.Fatalf("error should mention drive:file:download: %v", err)
	}
}

// TestDriveStatusQuickAcceptsMissingDownloadScope ensures quick mode is not
// blocked on the exact-mode download scope precheck.
func TestDriveStatusQuickAcceptsMissingDownloadScope(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	f.Credential = credential.NewCredentialProvider(nil, nil, &driveStatusScopedTokenResolver{scopes: "drive:drive.metadata:readonly"}, nil)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/a.txt", []byte("local"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_a", "name": "a.txt", "type": "file", "modified_time": "not-a-timestamp"},
				},
				"has_more": false,
			},
		},
	})

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--quick",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("quick mode should not require drive:file:download: %v\nstdout: %s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"detection": "quick"`) {
		t.Fatalf("output missing detection=quick\noutput: %s", stdout.String())
	}

	reg.Verify(t)
}

// TestDriveStatusShouldTreatAsUnchangedQuick exercises the tiny quick helper
// directly so Codecov also sees coverage on the helper body itself.
func TestDriveStatusShouldTreatAsUnchangedQuick(t *testing.T) {
	t.Run("matching timestamp returns true", func(t *testing.T) {
		if !driveStatusShouldTreatAsUnchangedQuick("1715594880", time.Unix(1715594880, 500)) {
			t.Fatal("expected matching second-resolution timestamps to be unchanged")
		}
	})

	t.Run("different timestamp returns false", func(t *testing.T) {
		if driveStatusShouldTreatAsUnchangedQuick("1715594881", time.Unix(1715594880, 0)) {
			t.Fatal("expected different timestamps to be treated as modified")
		}
	})

	t.Run("invalid timestamp returns false", func(t *testing.T) {
		if driveStatusShouldTreatAsUnchangedQuick("not-a-timestamp", time.Unix(1715594880, 0)) {
			t.Fatal("expected invalid timestamp to be treated as modified")
		}
	})
}

// TestDriveStatusPaginatesRemoteListing pins multi-page handling end-to-end
// AND the dual-field tolerance of common.PaginationMeta. Page 1 surfaces
// `next_page_token` (Drive's historical name); page 2 surfaces `page_token`
// (what the shared helper also accepts). If the shortcut had hard-coded
// either field name, one of the two pages' files would be silently dropped
// from the comparison and would land in the wrong bucket. Stub order is
// significant: httpmock matches in registration order, and both stubs key on
// the GET .../files URL — they pop in turn, so page 1's response (with the
// continuation token) must be registered before page 2's terminator.
func TestDriveStatusPaginatesRemoteListing(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Page 1: returns one file plus a continuation token via
	// next_page_token (the field Drive currently emits).
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_p1", "name": "page1.txt", "type": "file"},
				},
				"has_more":        true,
				"next_page_token": "cursor-page-2",
			},
		},
	})

	// Page 2: returns the second file with has_more=false. This stub uses
	// page_token (the alternate spelling) to lock in that the shared
	// PaginationMeta helper accepts BOTH field names.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_p2", "name": "page2.txt", "type": "file"},
				},
				"has_more":   false,
				"page_token": "",
			},
		},
	})

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	// Both pages contributed to new_remote (local is empty).
	for _, want := range []string{
		`"rel_path": "page1.txt"`,
		`"file_token": "tok_p1"`,
		`"rel_path": "page2.txt"`,
		`"file_token": "tok_p2"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q (a page must have been silently dropped)\noutput: %s", want, out)
		}
	}

	reg.Verify(t)
}

func TestDriveStatusFailsOnRemoteFileFolderConflict(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	registerRemoteListing(reg, "folder_root", []map[string]interface{}{
		{"token": duplicateRemoteFileIDFirst, "name": "dup", "type": "file", "size": 5, "created_time": "1", "modified_time": "1"},
		{"token": duplicateRemoteFolderID, "name": "dup", "type": "folder", "created_time": "2", "modified_time": "2"},
	})
	registerRemoteListing(reg, duplicateRemoteFolderID, []map[string]interface{}{
		{"token": "nested-file-token", "name": "child.txt", "type": "file", "size": 1, "created_time": "3", "modified_time": "3"},
	})

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	assertDuplicateRemotePathError(t, err, "dup", duplicateRemoteFileIDFirst, duplicateRemoteFolderID)
	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty on duplicate_remote_path, got: %s", stdout.String())
	}

	reg.Verify(t)
}

func TestDriveStatusRejectsMissingLocalDir(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "does-not-exist",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for missing local dir, got nil")
	}
}

func TestDriveStatusRejectsLocalFile(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.WriteFile("not-a-dir.txt", []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "not-a-dir.txt",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error when --local-dir is a file, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestDriveStatusRejectsAbsoluteLocalDir(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "/etc",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for absolute --local-dir, got nil")
	}
}

// TestDriveStatusRejectsEmptyFolderToken covers the Validate-stage required
// check that runs before ResourceName: an empty --folder-token must surface
// a structured FlagError referencing the flag name.
func TestDriveStatusRejectsEmptyFolderToken(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "local",
		"--folder-token", "",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for empty --folder-token, got nil")
	}
	if !strings.Contains(err.Error(), "--folder-token") {
		t.Fatalf("error must reference --folder-token, got: %v", err)
	}
}

// TestDriveStatusDoesNotEscapeViaSymlinkParentRef is the regression for the
// "link/.." escape: filepath.Clean string-shrinks "link/.." to ".", so a
// raw walk on the user-supplied input can land on the kernel-resolved
// path through link's target's parent — outside cwd. The fix is to walk
// SafeInputPath's canonical absolute root instead of the raw input.
//
// Setup: an "escape" sibling directory contains a sentinel file; cwd
// contains a "link" symlink pointing into that escape directory.
// Calling +status with --local-dir "link/.." must not surface the
// sentinel — the walk must stay inside cwd.
func TestDriveStatusDoesNotEscapeViaSymlinkParentRef(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	// Sentinel lives outside cwd; the agent must never see it.
	escapeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(escapeDir, "secret.txt"), []byte("S3CRET"), 0o644); err != nil {
		t.Fatalf("WriteFile secret: %v", err)
	}

	// cwd has a symlink that points into the sentinel's parent.
	cwdDir := t.TempDir()
	withDriveWorkingDir(t, cwdDir)
	if err := os.Symlink(escapeDir, filepath.Join(cwdDir, "link")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	// A normal file inside cwd just to make the walk non-trivial.
	if err := os.WriteFile(filepath.Join(cwdDir, "ok.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("WriteFile ok: %v", err)
	}

	// Empty remote folder so any path that surfaces in the output
	// must have come from the local walk.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files":    []interface{}{},
				"has_more": false,
			},
		},
	})

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "link/..",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if strings.Contains(out, "secret.txt") || strings.Contains(out, "S3CRET") {
		t.Fatalf("walk escaped via link/..: secret.txt leaked into output\noutput:\n%s", out)
	}
	// ok.txt is in cwd and must classify as new_local (no remote stub for it).
	if !strings.Contains(out, `"rel_path": "ok.txt"`) {
		t.Fatalf("expected ok.txt in new_local, got:\n%s", out)
	}
}

// TestDriveStatusSkipsSymlinkInsideRoot pins down WalkDir's default policy
// for symlinks discovered as child entries: they are reported with a
// non-regular file mode and the callback skips them, so a symlink inside
// the validated root pointing into an out-of-tree directory cannot leak
// the target's contents.
func TestDriveStatusSkipsSymlinkInsideRoot(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	// Sentinel sits outside cwd; a child symlink inside the walked root
	// points there. If the walker followed child symlinks (it must not),
	// the sentinel's name would surface in new_local.
	escapeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(escapeDir, "secret.txt"), []byte("S3CRET"), 0o644); err != nil {
		t.Fatalf("WriteFile secret: %v", err)
	}

	cwdDir := t.TempDir()
	withDriveWorkingDir(t, cwdDir)
	if err := os.MkdirAll(filepath.Join("local", "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "ok.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("WriteFile ok: %v", err)
	}
	// Child-of-root symlink that resolves out of the validated subtree.
	if err := os.Symlink(escapeDir, filepath.Join("local", "sub", "escape")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files":    []interface{}{},
				"has_more": false,
			},
		},
	})

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}
	out := stdout.String()
	if strings.Contains(out, "secret.txt") || strings.Contains(out, "S3CRET") {
		t.Fatalf("walk followed child symlink and leaked sentinel:\n%s", out)
	}
	if !strings.Contains(out, `"rel_path": "ok.txt"`) {
		t.Fatalf("expected ok.txt in new_local; got:\n%s", out)
	}
}

// TestDriveStatusSurvivesCircularSymlinkInsideRoot makes sure WalkDir
// terminates even when a child symlink points back at one of its
// ancestors. WalkDir's default policy already declines to follow child
// symlinks; this test pins that contract for our caller.
func TestDriveStatusSurvivesCircularSymlinkInsideRoot(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	cwdDir := t.TempDir()
	withDriveWorkingDir(t, cwdDir)
	if err := os.MkdirAll(filepath.Join("local", "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "sub", "real.txt"), []byte("real"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// loop symlink: cwd/local/sub/loop -> cwd/local (an ancestor).
	loopTarget, err := filepath.Abs(filepath.Join("local"))
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	if err := os.Symlink(loopTarget, filepath.Join("local", "sub", "loop")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files":    []interface{}{},
				"has_more": false,
			},
		},
	})

	// If WalkDir followed the loop, this test would never finish; the
	// test runner's per-test timeout would surface that as a failure.
	err = mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"rel_path": "sub/real.txt"`) {
		t.Fatalf("expected sub/real.txt in new_local; got:\n%s", stdout.String())
	}
}

// TestDriveStatusRejectsMalformedFolderToken covers the ResourceName format
// guard: a token with control characters (newline) must be rejected before
// any API call is made.
func TestDriveStatusRejectsMalformedFolderToken(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "local",
		"--folder-token", "tok\nwithnewline",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for malformed --folder-token, got nil")
	}
	if !strings.Contains(err.Error(), "--folder-token") {
		t.Fatalf("error must reference --folder-token, got: %v", err)
	}
}
