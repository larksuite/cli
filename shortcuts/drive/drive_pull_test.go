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

// TestDrivePullDownloadsAndCreatesParents verifies the happy path: a remote
// folder with a top-level file plus a subfolder is fully reproduced under
// --local-dir, including auto-created parent directories.
func TestDrivePullDownloadsAndCreatesParents(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
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
					// noise: an online doc must be skipped
					map[string]interface{}{"token": "tok_doc", "name": "ignored.docx", "type": "docx"},
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
					map[string]interface{}{"token": "tok_b", "name": "b.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})

	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_a/download",
		Status:  200,
		Body:    []byte("AAA"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_b/download",
		Status:  200,
		Body:    []byte("BBB"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"downloaded": 2`) {
		t.Errorf("expected downloaded=2, got: %s", out)
	}
	if strings.Contains(out, "ignored.docx") {
		t.Errorf("docx entries must be skipped, got: %s", out)
	}

	// File contents must reach disk under the right paths.
	mustReadFile(t, filepath.Join("local", "a.txt"), "AAA")
	mustReadFile(t, filepath.Join("local", "sub", "b.txt"), "BBB")
}

// TestDrivePullSkipsExistingWhenSkipPolicy verifies --if-exists=skip leaves
// existing local files untouched and counts them under summary.skipped.
func TestDrivePullSkipsExistingWhenSkipPolicy(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "keep.txt"), []byte("local-original"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_keep", "name": "keep.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--if-exists", "skip",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"skipped": 1`) {
		t.Errorf("expected skipped=1, got: %s", out)
	}
	if !strings.Contains(out, `"downloaded": 0`) {
		t.Errorf("expected downloaded=0 with --if-exists=skip, got: %s", out)
	}

	// Existing local content must be preserved verbatim.
	mustReadFile(t, filepath.Join("local", "keep.txt"), "local-original")
}

// TestDrivePullDeleteLocalRequiresYes verifies the upfront safety guard:
// --delete-local without --yes must be rejected before any API call.
func TestDrivePullDeleteLocalRequiresYes(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--delete-local",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for --delete-local without --yes, got nil")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error must reference --yes, got: %v", err)
	}
}

// TestDrivePullDeletesLocalOnlyFilesWhenYes verifies that --delete-local
// --yes removes local files absent from Drive after downloading the new
// content.
func TestDrivePullDeletesLocalOnlyFilesWhenYes(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll(filepath.Join("local", "subdir"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// stale.txt only exists locally → must be deleted.
	if err := os.WriteFile(filepath.Join("local", "stale.txt"), []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile stale: %v", err)
	}
	// orphan in a subdir → must also be deleted.
	if err := os.WriteFile(filepath.Join("local", "subdir", "orphan.txt"), []byte("orphan"), 0o644); err != nil {
		t.Fatalf("WriteFile orphan: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_new", "name": "fresh.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_new/download",
		Status:  200,
		Body:    []byte("FRESH"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--delete-local",
		"--yes",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"downloaded": 1`) {
		t.Errorf("expected downloaded=1, got: %s", out)
	}
	if !strings.Contains(out, `"deleted_local": 2`) {
		t.Errorf("expected deleted_local=2, got: %s", out)
	}

	mustReadFile(t, filepath.Join("local", "fresh.txt"), "FRESH")
	if _, err := os.Stat(filepath.Join("local", "stale.txt")); !os.IsNotExist(err) {
		t.Errorf("stale.txt should have been removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join("local", "subdir", "orphan.txt")); !os.IsNotExist(err) {
		t.Errorf("subdir/orphan.txt should have been removed, stat err=%v", err)
	}
}

// TestDrivePullDeleteLocalDoesNotEscapeViaSymlinkParentRef is the
// regression for the "link/.." escape applied to --delete-local — the
// most dangerous variant, since the bug would otherwise let the kernel
// walk through the symlink target's parent and delete files outside
// cwd.
//
// Setup: an "escape" sibling directory contains a sentinel file; cwd
// has a "link" symlink pointing into that escape directory. Running
// +pull with --local-dir "link/.." --delete-local --yes against an
// empty remote folder must NOT delete the sentinel.
func TestDrivePullDeleteLocalDoesNotEscapeViaSymlinkParentRef(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	// Sentinel sits outside cwd; if the bug existed, --delete-local
	// would unlink it.
	escapeDir := t.TempDir()
	sentinel := filepath.Join(escapeDir, "secret.txt")
	if err := os.WriteFile(sentinel, []byte("S3CRET"), 0o644); err != nil {
		t.Fatalf("WriteFile sentinel: %v", err)
	}

	cwdDir := t.TempDir()
	withDriveWorkingDir(t, cwdDir)
	if err := os.Symlink(escapeDir, filepath.Join(cwdDir, "link")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	// One file inside cwd to confirm the walk did run.
	cwdLocal := filepath.Join(cwdDir, "ok.txt")
	if err := os.WriteFile(cwdLocal, []byte("ok"), 0o644); err != nil {
		t.Fatalf("WriteFile cwd: %v", err)
	}

	// Remote is empty — so under --delete-local --yes the only files
	// the walk identifies as "local-only" are inside the canonical
	// walk root.
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

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "link/..",
		"--folder-token", "folder_root",
		"--delete-local",
		"--yes",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	// Must-haves:
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("sentinel %q must still exist after +pull --delete-local; stat err=%v", sentinel, err)
	}
	// And the cwd-local file should have been deleted (it is local-only
	// and remote is empty), proving the walk DID run, just not into
	// the escape directory.
	if _, err := os.Stat(cwdLocal); !os.IsNotExist(err) {
		t.Fatalf("ok.txt should have been deleted (local-only with empty remote); stat err=%v", err)
	}
	out := stdout.String()
	if strings.Contains(out, "S3CRET") || strings.Contains(out, escapeDir) {
		t.Fatalf("escape directory leaked into output:\n%s", out)
	}
}

// TestDrivePullSkipsSymlinkInsideRoot pins WalkDir's default symlink
// behavior in the +pull --delete-local path. A child symlink under the
// validated root pointing into an out-of-tree directory must NOT be
// followed: WalkDir surfaces it as a non-regular entry, our callback
// skips it, and the sentinel inside the target survives the delete pass.
func TestDrivePullSkipsSymlinkInsideRoot(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	escapeDir := t.TempDir()
	sentinel := filepath.Join(escapeDir, "secret.txt")
	if err := os.WriteFile(sentinel, []byte("S3CRET"), 0o644); err != nil {
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
	if err := os.Symlink(escapeDir, filepath.Join("local", "sub", "escape")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// Empty remote so --delete-local would target every regular file
	// the walker can reach.
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

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--delete-local",
		"--yes",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("sentinel %q must survive (walker followed child symlink): %v", sentinel, err)
	}
	if _, err := os.Stat(filepath.Join("local", "ok.txt")); !os.IsNotExist(err) {
		t.Fatalf("local/ok.txt should have been deleted (proves walk ran), got: %v", err)
	}
}

// TestDrivePullSurvivesCircularSymlinkInsideRoot ensures the walker
// terminates even when the validated root contains a child symlink
// pointing back at one of its ancestors.
func TestDrivePullSurvivesCircularSymlinkInsideRoot(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	cwdDir := t.TempDir()
	withDriveWorkingDir(t, cwdDir)
	if err := os.MkdirAll(filepath.Join("local", "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "sub", "real.txt"), []byte("real"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
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

	err = mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--delete-local",
		"--yes",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}
	if _, err := os.Stat(filepath.Join("local", "sub", "real.txt")); !os.IsNotExist(err) {
		t.Fatalf("real.txt should be deleted (proves walk completed)")
	}
}

// TestDrivePullDownloadDoesNotEscapeViaSymlinkParentRef pins the second
// half of the canonical-root fix: with --local-dir "link/..", which
// SafeInputPath happily accepts (filepath.Clean shrinks "link/.." to
// "."), download targets must land inside the canonical cwd, never
// inside the symlink target's parent. Without the fix the download
// would write into a sibling directory.
func TestDrivePullDownloadDoesNotEscapeViaSymlinkParentRef(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	// escapeDir is a sibling temp dir; nothing should ever land here.
	escapeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(escapeDir, "preexisting.txt"), []byte("DO-NOT-TOUCH"), 0o644); err != nil {
		t.Fatalf("WriteFile preexisting: %v", err)
	}

	cwdDir := t.TempDir()
	withDriveWorkingDir(t, cwdDir)
	if err := os.Symlink(escapeDir, filepath.Join(cwdDir, "link")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_x", "name": "downloaded.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_x/download",
		Status:  200,
		Body:    []byte("REMOTE-BODY"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "link/..",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	mustReadFile(t, filepath.Join(cwdDir, "downloaded.txt"), "REMOTE-BODY")
	if _, err := os.Stat(filepath.Join(escapeDir, "downloaded.txt")); !os.IsNotExist(err) {
		t.Fatalf("downloaded.txt must NOT land in escape dir; stat err=%v", err)
	}
	mustReadFile(t, filepath.Join(escapeDir, "preexisting.txt"), "DO-NOT-TOUCH")
}

// TestDrivePullRejectsAbsoluteLocalDir confirms SafeLocalFlagPath surfaces
// the proper flag name in the error message.
func TestDrivePullRejectsAbsoluteLocalDir(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "/etc",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for absolute --local-dir, got nil")
	}
	if !strings.Contains(err.Error(), "--local-dir") {
		t.Fatalf("error must reference --local-dir, got: %v", err)
	}
}

// TestDrivePullRejectsBadIfExistsEnum verifies the framework's enum guard.
func TestDrivePullRejectsBadIfExistsEnum(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--if-exists", "fail-and-die",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected enum validation error, got nil")
	}
	if !strings.Contains(err.Error(), "if-exists") {
		t.Fatalf("error must reference --if-exists, got: %v", err)
	}
}

func mustReadFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("file %s content = %q, want %q", path, string(data), want)
	}
}
