// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
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

// TestDrivePullSkipsExistingWhenSmartPolicyAndLocalIsUpToDate verifies the
// smart fast path for Drive → local mirrors: when the local copy is already
// at least as new as the remote file, +pull skips the download.
func TestDrivePullSkipsExistingWhenSmartPolicyAndLocalIsUpToDate(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	localPath := filepath.Join("local", "keep.txt")
	if err := os.WriteFile(localPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	localMTime := time.Unix(200, 500*int64(time.Millisecond))
	if err := os.Chtimes(localPath, localMTime, localMTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_keep", "name": "keep.txt", "type": "file", "size": 5, "modified_time": "100"},
				},
				"has_more": false,
			},
		},
	})

	// Intentionally NO download stub: smart mode should skip the transfer.
	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--if-exists", "smart",
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
		t.Errorf("expected downloaded=0, got: %s", out)
	}
	mustReadFile(t, localPath, "hello")
}

// TestDrivePullDownloadsWhenSmartPolicyAndRemoteIsNewer verifies the smart
// policy still downloads when the remote file is newer than the local copy.
func TestDrivePullDownloadsWhenSmartPolicyAndRemoteIsNewer(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	localPath := filepath.Join("local", "keep.txt")
	if err := os.WriteFile(localPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	localMTime := time.Unix(100, 500*int64(time.Millisecond))
	if err := os.Chtimes(localPath, localMTime, localMTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_keep", "name": "keep.txt", "type": "file", "size": 5, "modified_time": "200"},
				},
				"has_more": false,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_keep/download",
		Status:  200,
		Body:    []byte("WORLD"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--if-exists", "smart",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"downloaded": 1`) {
		t.Errorf("expected downloaded=1, got: %s", out)
	}
	mustReadFile(t, localPath, "WORLD")
	info, err := os.Stat(localPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got, want := info.ModTime(), time.Unix(200, 0); !got.Equal(want) {
		t.Fatalf("local mtime = %v, want %v", got, want)
	}
}

// TestDrivePullTreatsModifiedTimePreservationFailureAsNotice verifies a local
// write that succeeds but cannot preserve remote modified_time still reports a
// successful download and only emits an operator-facing notice on stderr.
func TestDrivePullTreatsModifiedTimePreservationFailureAsNotice(t *testing.T) {
	f, stdout, stderrBuf, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	prevChtimes := drivePullChtimes
	drivePullChtimes = func(string, time.Time, time.Time) error {
		return fmt.Errorf("mtime mutation unsupported")
	}
	t.Cleanup(func() {
		drivePullChtimes = prevChtimes
	})

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_keep", "name": "keep.txt", "type": "file", "size": 5, "modified_time": "200"},
				},
				"has_more": false,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_keep/download",
		Status:  200,
		Body:    []byte("WORLD"),
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
		t.Fatalf("unexpected error: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderrBuf.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"downloaded": 1`) {
		t.Errorf("expected downloaded=1, got: %s", out)
	}
	if !strings.Contains(out, `"failed": 0`) {
		t.Errorf("expected failed=0, got: %s", out)
	}
	mustReadFile(t, filepath.Join("local", "keep.txt"), "WORLD")
	if !strings.Contains(stderrBuf.String(), "could not preserve remote modified_time") {
		t.Errorf("expected stderr notice about modified_time preservation failure, got: %s", stderrBuf.String())
	}

	reg.Verify(t)
}

func TestDrivePullShouldSkipSmartFallsBackWhenMetadataCannotBeTrusted(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	localPath := filepath.Join("local", "keep.txt")
	if err := os.WriteFile(localPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	localMTime := time.Unix(100, 500*int64(time.Millisecond))
	if err := os.Chtimes(localPath, localMTime, localMTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, driveTestConfig(), f, core.AsBot)

	for _, tt := range []struct {
		name       string
		ifExists   string
		remoteFile drivePullTarget
	}{
		{
			name:       "non-smart policy",
			ifExists:   drivePullIfExistsOverwrite,
			remoteFile: drivePullTarget{ModifiedTime: "100"},
		},
		{
			name:       "missing remote timestamp",
			ifExists:   drivePullIfExistsSmart,
			remoteFile: drivePullTarget{ModifiedTime: ""},
		},
		{
			name:       "invalid remote timestamp",
			ifExists:   drivePullIfExistsSmart,
			remoteFile: drivePullTarget{ModifiedTime: "not-a-time"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := drivePullShouldSkipSmart(localPath, tt.remoteFile, tt.ifExists, runtime); got {
				t.Fatalf("drivePullShouldSkipSmart() = true, want false for %s", tt.name)
			}
		})
	}
}

func TestDrivePullShouldSkipSmartFallsBackWhenPathCannotBeResolved(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, driveTestConfig(), f, core.AsBot)

	if got := drivePullShouldSkipSmart("../escape.txt", drivePullTarget{ModifiedTime: "100"}, drivePullIfExistsSmart, runtime); got {
		t.Fatal("drivePullShouldSkipSmart() = true, want false when ResolvePath rejects the target")
	}
}

func TestDrivePullShouldSkipSmartFallsBackWhenLocalFileDisappeared(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, driveTestConfig(), f, core.AsBot)

	if got := drivePullShouldSkipSmart(filepath.Join("local", "missing.txt"), drivePullTarget{ModifiedTime: "100"}, drivePullIfExistsSmart, runtime); got {
		t.Fatal("drivePullShouldSkipSmart() = true, want false when os.Stat cannot find the local file")
	}
}

func TestDrivePullSkipsWhenSmartIgnoresRemoteSize(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	localPath := filepath.Join("local", "keep.txt")
	if err := os.WriteFile(localPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	localMTime := time.Unix(200, 500*int64(time.Millisecond))
	if err := os.Chtimes(localPath, localMTime, localMTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_keep", "name": "keep.txt", "type": "file", "size": 999, "modified_time": "100"},
				},
				"has_more": false,
			},
		},
	})

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--if-exists", "smart",
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
		t.Errorf("expected downloaded=0, got: %s", out)
	}
	mustReadFile(t, localPath, "hello")
}

// TestDrivePullSurfacesDirectoryFileMirrorConflict pins the contract
// for the case where Drive ships a regular file at a rel_path that is
// already a directory locally. SafeOutputPath would refuse to overwrite
// the directory at write time, but if --if-exists=skip silently swallows
// the collision the caller sees "skipped" and assumes the mirror is
// in sync. The fix surfaces it as a structured `partial_failure`
// ExitError (non-zero exit + items[] in error.detail) under both skip
// and overwrite policies so callers can react via exit code.
func TestDrivePullSurfacesDirectoryFileMirrorConflict(t *testing.T) {
	for _, policy := range []string{"overwrite", "skip"} {
		t.Run(policy, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

			tmpDir := t.TempDir()
			withDriveWorkingDir(t, tmpDir)
			// Local has a directory at "shadow" — Drive says it's a
			// regular file at the same rel_path. This is the conflict.
			if err := os.MkdirAll(filepath.Join("local", "shadow"), 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}

			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "folder_token=folder_root",
				Body: map[string]interface{}{
					"code": 0, "msg": "ok",
					"data": map[string]interface{}{
						"files": []interface{}{
							map[string]interface{}{"token": "tok_shadow", "name": "shadow", "type": "file"},
						},
						"has_more": false,
					},
				},
			})

			err := mountAndRunDrive(t, DrivePull, []string{
				"+pull",
				"--local-dir", "local",
				"--folder-token", "folder_root",
				"--if-exists", policy,
				"--as", "bot",
			}, f, stdout)
			detail := assertDrivePullPartialFailure(t, err)
			summary, items := splitDrivePullDetail(t, detail)
			if got := summary["failed"]; got != float64(1) {
				t.Errorf("[%s] summary.failed = %v, want 1", policy, got)
			}
			if got := summary["skipped"]; got != float64(0) {
				t.Errorf("[%s] mirror conflict must NOT be swallowed as skipped (skipped=%v)", policy, got)
			}
			if len(items) != 1 || items[0]["action"] != "failed" {
				t.Errorf("[%s] expected one items[] entry with action=failed, got: %#v", policy, items)
			}
			if msg, _ := items[0]["error"].(string); !strings.Contains(msg, "is a directory") {
				t.Errorf("[%s] error message should mention the directory conflict, got: %q", policy, msg)
			}
			if stdout.Len() != 0 {
				t.Errorf("[%s] stdout should be empty on partial_failure, got: %s", policy, stdout.String())
			}
		})
	}
}

// TestDrivePullPaginationHandlesPageTokenField pins the cross-API
// pagination contract: Drive list paginates by page_token / next_page_token,
// and the shared common.PaginationMeta helper accepts both. If +pull
// only honored next_page_token it would silently stop after the first
// page when the backend returns the (newer) page_token field, so any
// remote files past page one would never be downloaded and would be
// invisible to --delete-local.
func TestDrivePullPaginationHandlesPageTokenField(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// First page: returns has_more + page_token (NOT next_page_token).
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_a", "name": "a.txt", "type": "file"},
				},
				"has_more":   true,
				"page_token": "page2",
			},
		},
	})
	// Second page: terminator.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "page_token=page2",
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
		t.Errorf("expected both pages to be fetched (downloaded=2), got: %s", out)
	}
	mustReadFile(t, filepath.Join("local", "a.txt"), "AAA")
	mustReadFile(t, filepath.Join("local", "b.txt"), "BBB")
	reg.Verify(t)
}

func TestDrivePullRenameSummarizesDuplicateDownloadsAndAvoidsRawTokenInRelPath(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	registerDuplicateRemoteFiles(reg)
	registerDownload(reg, duplicateRemoteFileIDFirst, "FIRST")
	registerDownload(reg, duplicateRemoteFileIDSecond, "SECOND")

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--on-duplicate-remote", "rename",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	renamedRelPath := expectedRenamedRelPath("dup.txt", duplicateRemoteFileIDSecond, 12, 0)
	payload := decodeDrivePullStdout(t, stdout.Bytes())
	if got := payload.Data.Summary.Downloaded; got != 2 {
		t.Fatalf("summary.downloaded = %d, want 2", got)
	}
	if out := stdout.String(); strings.Contains(out, duplicateRemoteFileIDSecond) {
		t.Fatalf("stdout should not expose the raw duplicate file token in rename mode, got: %s", out)
	}
	if item := findPullItem(payload.Data.Items, renamedRelPath); item.SourceID == "" || item.FileToken != "" {
		t.Fatalf("rename item should emit source_id without file_token, got: %#v", item)
	}
	mustReadFile(t, filepath.Join("local", "dup.txt"), "FIRST")
	mustReadFile(t, filepath.Join("local", renamedRelPath), "SECOND")
	assertPullItemAction(t, stdout.Bytes(), "dup.txt", "downloaded")
	assertPullItemAction(t, stdout.Bytes(), renamedRelPath, "downloaded")

	reg.Verify(t)
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

// TestDrivePullDeleteLocalPreservesLocalFileShadowedByOnlineDoc is the
// regression for the case where Drive holds an online doc (docx, sheet,
// shortcut, …) at the same rel_path as a local file. The online doc is
// NOT in the downloadable set (type≠file) but Drive still owns that path,
// so --delete-local must not treat the local file as orphaned. Before the
// fix, the delete pass consulted only the type=file map and would unlink
// the local file every time it shared a name with an online doc.
func TestDrivePullDeleteLocalPreservesLocalFileShadowedByOnlineDoc(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// User keeps a local copy at the same path Drive serves an online
	// doc — should survive --delete-local.
	if err := os.WriteFile(filepath.Join("local", "notes.docx"), []byte("LOCAL-DOCX"), 0o644); err != nil {
		t.Fatalf("WriteFile shadow: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_keep", "name": "keep.txt", "type": "file"},
					// Same name as the local file — must be tracked by
					// the lister even though it is not downloadable.
					map[string]interface{}{"token": "tok_doc", "name": "notes.docx", "type": "docx"},
				},
				"has_more": false,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_keep/download",
		Status:  200,
		Body:    []byte("KEEP"),
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
	if !strings.Contains(out, `"deleted_local": 0`) {
		t.Errorf("expected deleted_local=0 (online doc shadows local file), got: %s", out)
	}
	mustReadFile(t, filepath.Join("local", "notes.docx"), "LOCAL-DOCX")
	mustReadFile(t, filepath.Join("local", "keep.txt"), "KEEP")
}

// TestDrivePullDeleteLocalPreservesLocalFileShadowedByRemoteFolder pins
// the same allPaths contract for the folder branch: a remote folder
// occupies its own rel_path (not just the rel_paths of its children),
// so a local regular file at the same name must NOT be treated as
// orphaned. Before the folder-branch fix, listRemote only added the
// folder's children to allPaths, leaving the folder name itself
// missing.
func TestDrivePullDeleteLocalPreservesLocalFileShadowedByRemoteFolder(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Local has a regular file named "shadow"; Drive has a folder of
	// the same name. The local file must survive --delete-local.
	if err := os.WriteFile(filepath.Join("local", "shadow"), []byte("LOCAL-FILE"), 0o644); err != nil {
		t.Fatalf("WriteFile shadow: %v", err)
	}

	// Root folder list: one folder named "shadow", and an unrelated
	// downloadable file so we can assert the download path still
	// works alongside the protected name.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_shadow_dir", "name": "shadow", "type": "folder"},
					map[string]interface{}{"token": "tok_keep", "name": "keep.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})
	// Subfolder is empty — keeps the folder branch active without
	// adding extra files to the assertions.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=tok_shadow_dir",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files":    []interface{}{},
				"has_more": false,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_keep/download",
		Status:  200,
		Body:    []byte("KEEP"),
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
	if !strings.Contains(out, `"deleted_local": 0`) {
		t.Errorf("expected deleted_local=0 (remote folder shadows local file), got: %s", out)
	}
	mustReadFile(t, filepath.Join("local", "shadow"), "LOCAL-FILE")
	mustReadFile(t, filepath.Join("local", "keep.txt"), "KEEP")
}

// TestDrivePullDeleteLocalCountsFailureInSummary pins the contract that
// a failed delete shows up in summary.failed (not just in items[]) AND
// surfaces as a partial_failure ExitError so callers can detect the
// half-synced state via exit code. Before the fix, the delete_failed
// branches appended an item but left `failed` at zero AND returned nil,
// so the JSON envelope reported `ok=true`+`exit=0` even when the mirror
// was incomplete. Setup forces os.Remove to fail by making the file's
// containing directory read-only (chmod 0o555) right before the run;
// cleanup restores 0o755 so t.TempDir teardown succeeds.
func TestDrivePullDeleteLocalCountsFailureInSummary(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	stale := filepath.Join("local", "stale.txt")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile stale: %v", err)
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

	// Lock the parent directory so the delete fails. Restore in a
	// cleanup so t.TempDir's RemoveAll can succeed.
	if err := os.Chmod("local", 0o555); err != nil {
		t.Fatalf("Chmod 555: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod("local", 0o755) })

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--delete-local",
		"--yes",
		"--as", "bot",
	}, f, stdout)
	detail := assertDrivePullPartialFailure(t, err)
	summary, items := splitDrivePullDetail(t, detail)
	if got := summary["failed"]; got != float64(1) {
		t.Errorf("summary.failed = %v, want 1 (delete_failed must increment failed)", got)
	}
	if got := summary["deleted_local"]; got != float64(0) {
		t.Errorf("summary.deleted_local = %v, want 0", got)
	}
	if len(items) != 1 || items[0]["action"] != "delete_failed" {
		t.Errorf("expected one items[] entry with action=delete_failed, got: %#v", items)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty on partial_failure, got: %s", stdout.String())
	}
}

// TestDrivePullDownloadFailureSkipsDeleteLocalAndExitsNonZero pins the
// gating contract for --delete-local: when the download pass produced
// any failure, the delete walk MUST be skipped entirely and the command
// MUST exit non-zero with type=partial_failure. The half-synced state
// where some Drive files are missing locally AND some local-only files
// have been removed is never observable.
func TestDrivePullDownloadFailureSkipsDeleteLocalAndExitsNonZero(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// stale.txt only exists locally — without the gate, --delete-local
	// would unlink it. The gate must prevent that when downloads fail.
	stale := filepath.Join("local", "stale.txt")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile stale: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_broken", "name": "broken.bin", "type": "file"},
				},
				"has_more": false,
			},
		},
	})
	// Download stub returns 500 so the download fails.
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_broken/download",
		Status:  500,
		Body:    []byte(`{"code":99999,"msg":"backend boom"}`),
		Headers: http.Header{"Content-Type": []string{"application/json"}},
	})

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--delete-local",
		"--yes",
		"--as", "bot",
	}, f, stdout)
	exitErr := assertDrivePullPartialFailure(t, err)
	if !strings.Contains(exitErr.Detail.Message, "--delete-local was skipped") {
		t.Errorf("expected message to mention --delete-local skip, got: %q", exitErr.Detail.Message)
	}

	summary, items := splitDrivePullDetail(t, exitErr)
	if got := summary["failed"]; got != float64(1) {
		t.Errorf("summary.failed = %v, want 1", got)
	}
	if got := summary["deleted_local"]; got != float64(0) {
		t.Errorf("summary.deleted_local = %v, want 0 (delete pass must be skipped on download failure)", got)
	}
	// The download failure is the only items[] entry — no delete_local /
	// delete_failed entries because the delete pass was skipped entirely.
	if len(items) != 1 || items[0]["action"] != "failed" {
		t.Errorf("expected one items[] entry with action=failed, got: %#v", items)
	}

	// stale.txt MUST still exist on disk.
	if _, statErr := os.Stat(stale); statErr != nil {
		t.Fatalf("stale.txt must survive when --delete-local is skipped after a download failure; stat err=%v", statErr)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty on partial_failure, got: %s", stdout.String())
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

// assertDrivePullPartialFailure asserts that err is the structured
// partial_failure ExitError +pull returns when any item-level failure
// happens, and returns the unwrapped *ExitError so the caller can drill
// into Detail.Detail without re-doing the type assertion.
func assertDrivePullPartialFailure(t *testing.T, err error) *output.ExitError {
	t.Helper()
	if err == nil {
		t.Fatal("expected partial_failure ExitError, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitAPI {
		t.Errorf("exit code = %d, want %d (ExitAPI)", exitErr.Code, output.ExitAPI)
	}
	if exitErr.Detail == nil {
		t.Fatalf("ExitError.Detail must be set on partial_failure")
	}
	if exitErr.Detail.Type != "partial_failure" {
		t.Errorf("error.type = %q, want partial_failure", exitErr.Detail.Type)
	}
	return exitErr
}

// splitDrivePullDetail extracts the {summary, items[]} payload from the
// ExitError detail. We round-trip through JSON so test assertions don't
// depend on the concrete map types the production code happens to use.
func splitDrivePullDetail(t *testing.T, exitErr *output.ExitError) (map[string]interface{}, []map[string]interface{}) {
	t.Helper()
	raw, err := json.Marshal(exitErr.Detail.Detail)
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}
	var got struct {
		Summary map[string]interface{}   `json:"summary"`
		Items   []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal detail: %v\nraw=%s", err, string(raw))
	}
	if got.Summary == nil {
		t.Fatalf("error.detail missing summary; raw=%s", string(raw))
	}
	return got.Summary, got.Items
}
