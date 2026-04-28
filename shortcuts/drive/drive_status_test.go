// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

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
