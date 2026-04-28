// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"net/http"
	"os"
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
