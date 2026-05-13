// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

const (
	duplicateRemoteFileIDFirst  = "example-file-token-first"
	duplicateRemoteFileIDSecond = "example-file-token-second"
	duplicateRemoteFileIDThird  = "example-file-token-third"
	duplicateRemoteFolderID     = "example-folder-token"
)

func TestDriveStatusFailsOnDuplicateRemoteFiles(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	registerDuplicateRemoteFiles(reg)

	err := mountAndRunDrive(t, DriveStatus, []string{
		"+status",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	assertDuplicateRemotePathError(t, err, "dup.txt", duplicateRemoteFileIDFirst, duplicateRemoteFileIDSecond)
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty on duplicate_remote_path, got: %s", stdout.String())
	}

	reg.Verify(t)
}

func TestDrivePullFailsOnDuplicateRemoteFilesBeforeWriting(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	registerDuplicateRemoteFiles(reg)

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	assertDuplicateRemotePathError(t, err, "dup.txt", duplicateRemoteFileIDFirst, duplicateRemoteFileIDSecond)
	if _, statErr := os.Stat(filepath.Join("local", "dup.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("duplicate default failure must not write local dup.txt; stat err=%v", statErr)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty on duplicate_remote_path, got: %s", stdout.String())
	}

	reg.Verify(t)
}

func TestDrivePullRenameDownloadsDuplicateRemoteFilesWithStableHashSuffix(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	registerDuplicateRemoteFiles(reg)
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/" + duplicateRemoteFileIDFirst + "/download",
		Status:  200,
		Body:    []byte("FIRST"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/" + duplicateRemoteFileIDSecond + "/download",
		Status:  200,
		Body:    []byte("SECOND"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

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
	mustReadFile(t, filepath.Join("local", "dup.txt"), "FIRST")
	mustReadFile(t, filepath.Join("local", renamedRelPath), "SECOND")
	if strings.Contains(renamedRelPath, duplicateRemoteFileIDSecond) {
		t.Fatalf("renamed rel_path should not expose raw file token: %s", renamedRelPath)
	}
	payload := decodeDrivePullStdout(t, stdout.Bytes())
	if got := payload.Data.Summary.Downloaded; got != 2 {
		t.Fatalf("summary.downloaded = %d, want 2", got)
	}
	if item := findPullItem(payload.Data.Items, renamedRelPath); item.SourceID == "" || item.FileToken != "" {
		t.Fatalf("rename item should emit source_id without file_token, got: %#v", item)
	}
	assertPullItemAction(t, stdout.Bytes(), renamedRelPath, "downloaded")

	reg.Verify(t)
}

func TestDrivePullRenameStrengthensSuffixWhenShortHashTargetAlreadyExists(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	shortHashRelPath := expectedRenamedRelPath("dup.txt", duplicateRemoteFileIDSecond, 12, 0)
	registerRemoteListing(reg, "folder_root", []map[string]interface{}{
		{"token": duplicateRemoteFileIDFirst, "name": "dup.txt", "type": "file", "size": 5, "created_time": "1", "modified_time": "1"},
		{"token": duplicateRemoteFileIDSecond, "name": "dup.txt", "type": "file", "size": 6, "created_time": "2", "modified_time": "2"},
		{"token": duplicateRemoteFileIDThird, "name": shortHashRelPath, "type": "file", "size": 7, "created_time": "3", "modified_time": "3"},
	})
	registerDownload(reg, duplicateRemoteFileIDFirst, "FIRST")
	registerDownload(reg, duplicateRemoteFileIDSecond, "SECOND")
	registerDownload(reg, duplicateRemoteFileIDThird, "THIRD")

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

	occupied := occupiedRemotePaths([]driveRemoteEntry{
		{RelPath: "dup.txt"},
		{RelPath: "dup.txt"},
		{RelPath: shortHashRelPath},
	})
	strongerRelPath, err := relPathWithUniqueFileTokenSuffix("dup.txt", duplicateRemoteFileIDSecond, occupied)
	if err != nil {
		t.Fatalf("relPathWithUniqueFileTokenSuffix: %v", err)
	}
	if strongerRelPath == shortHashRelPath {
		t.Fatalf("expected stronger unique suffix when %q is already occupied", shortHashRelPath)
	}
	mustReadFile(t, filepath.Join("local", shortHashRelPath), "THIRD")
	mustReadFile(t, filepath.Join("local", strongerRelPath), "SECOND")
	payload := decodeDrivePullStdout(t, stdout.Bytes())
	if got := payload.Data.Summary.Downloaded; got != 3 {
		t.Fatalf("summary.downloaded = %d, want 3", got)
	}
	if item := findPullItem(payload.Data.Items, strongerRelPath); item.SourceID == "" || item.FileToken != "" {
		t.Fatalf("rename item should emit source_id without file_token, got: %#v", item)
	}
	assertPullItemAction(t, stdout.Bytes(), strongerRelPath, "downloaded")

	reg.Verify(t)
}

func TestDrivePullRenameAppendsSequenceWhenAllHashSuffixTargetsAreOccupied(t *testing.T) {
	fileToken := duplicateRemoteFileIDSecond
	tokenHash := stableTokenHash(fileToken)
	occupied := map[string]struct{}{
		"dup.txt": {},
		relPathWithSuffix("dup.txt", "__lark_"+tokenHash[:12]): {},
		relPathWithSuffix("dup.txt", "__lark_"+tokenHash[:24]): {},
		relPathWithSuffix("dup.txt", "__lark_"+tokenHash):      {},
		relPathWithSuffix("dup.txt", "__lark_"+tokenHash+"_2"): {},
	}

	got, err := relPathWithUniqueFileTokenSuffix("dup.txt", fileToken, occupied)
	if err != nil {
		t.Fatalf("relPathWithUniqueFileTokenSuffix: %v", err)
	}
	want := relPathWithSuffix("dup.txt", "__lark_"+tokenHash+"_3")
	if got != want {
		t.Fatalf("unique rel_path = %q, want %q", got, want)
	}
}

func TestRelPathWithUniqueFileTokenSuffixReturnsErrorAfterMaxAttempts(t *testing.T) {
	fileToken := duplicateRemoteFileIDSecond
	tokenHash := stableTokenHash(fileToken)
	occupied := map[string]struct{}{
		"dup.txt": {},
	}
	for _, suffix := range []string{
		"__lark_" + tokenHash[:12],
		"__lark_" + tokenHash[:24],
		"__lark_" + tokenHash,
	} {
		occupied[relPathWithSuffix("dup.txt", suffix)] = struct{}{}
	}
	for attempt := 2; attempt <= driveUniqueSuffixMaxSeq; attempt++ {
		occupied[relPathWithSuffix("dup.txt", "__lark_"+tokenHash+"_"+strconv.Itoa(attempt))] = struct{}{}
	}

	_, err := relPathWithUniqueFileTokenSuffix("dup.txt", fileToken, occupied)
	if err == nil {
		t.Fatal("expected relPathWithUniqueFileTokenSuffix to fail after exhausting all suffix attempts")
	}
}

func TestDrivePullNewestChoosesMostRecentDuplicateRemoteFile(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	registerDuplicateRemoteFiles(reg)
	registerDownload(reg, duplicateRemoteFileIDSecond, "SECOND")

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--on-duplicate-remote", "newest",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	mustReadFile(t, filepath.Join("local", "dup.txt"), "SECOND")
	assertPullItemAction(t, stdout.Bytes(), "dup.txt", "downloaded")
	payload := decodeDrivePullStdout(t, stdout.Bytes())
	if got := payload.Data.Summary.Downloaded; got != 1 {
		t.Fatalf("summary.downloaded = %d, want 1", got)
	}
	if item := findPullItem(payload.Data.Items, "dup.txt"); item.FileToken != duplicateRemoteFileIDSecond {
		t.Fatalf("stdout should surface the chosen newest file token, got: %#v", item)
	}

	reg.Verify(t)
}

func TestDrivePullOldestChoosesOldestDuplicateRemoteFile(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	registerDuplicateRemoteFiles(reg)
	registerDownload(reg, duplicateRemoteFileIDFirst, "FIRST")

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--on-duplicate-remote", "oldest",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	mustReadFile(t, filepath.Join("local", "dup.txt"), "FIRST")
	assertPullItemAction(t, stdout.Bytes(), "dup.txt", "downloaded")
	payload := decodeDrivePullStdout(t, stdout.Bytes())
	if got := payload.Data.Summary.Downloaded; got != 1 {
		t.Fatalf("summary.downloaded = %d, want 1", got)
	}
	if item := findPullItem(payload.Data.Items, "dup.txt"); item.FileToken != duplicateRemoteFileIDFirst {
		t.Fatalf("stdout should surface the chosen oldest file token, got: %#v", item)
	}

	reg.Verify(t)
}

func TestDrivePullRenameHandlesNestedDuplicateRemoteFilesEndToEnd(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	registerRemoteListing(reg, "folder_root", []map[string]interface{}{
		{"token": duplicateRemoteFolderID, "name": "sub", "type": "folder", "created_time": "1", "modified_time": "1"},
	})
	registerRemoteListing(reg, duplicateRemoteFolderID, []map[string]interface{}{
		{"token": duplicateRemoteFileIDFirst, "name": "dup.txt", "type": "file", "size": 5, "created_time": "1", "modified_time": "1"},
		{"token": duplicateRemoteFileIDSecond, "name": "dup.txt", "type": "file", "size": 6, "created_time": "2", "modified_time": "2"},
	})
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

	renamedRelPath := expectedRenamedRelPath("sub/dup.txt", duplicateRemoteFileIDSecond, 12, 0)
	mustReadFile(t, filepath.Join("local", "sub", "dup.txt"), "FIRST")
	mustReadFile(t, filepath.Join("local", filepath.FromSlash(renamedRelPath)), "SECOND")
	assertPullItemAction(t, stdout.Bytes(), "sub/dup.txt", "downloaded")
	assertPullItemAction(t, stdout.Bytes(), renamedRelPath, "downloaded")

	reg.Verify(t)
}

func TestDrivePushFailsOnDuplicateRemoteFilesBeforeUpload(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "dup.txt"), []byte("LOCAL"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	registerDuplicateRemoteFiles(reg)

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--if-exists", "overwrite",
		"--delete-remote",
		"--yes",
		"--as", "bot",
	}, f, stdout)
	assertDuplicateRemotePathError(t, err, "dup.txt", duplicateRemoteFileIDFirst, duplicateRemoteFileIDSecond)
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty on duplicate_remote_path, got: %s", stdout.String())
	}

	reg.Verify(t)
}

func TestDrivePullFailsOnRemoteFileFolderConflictEvenWithRename(t *testing.T) {
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
	registerRemoteListing(reg, duplicateRemoteFolderID, nil)

	err := mountAndRunDrive(t, DrivePull, []string{
		"+pull",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--on-duplicate-remote", "rename",
		"--as", "bot",
	}, f, stdout)
	assertDuplicateRemotePathError(t, err, "dup", duplicateRemoteFileIDFirst, duplicateRemoteFolderID)
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty on duplicate_remote_path, got: %s", stdout.String())
	}

	reg.Verify(t)
}

func TestDrivePushFailsOnRemoteFileFolderConflictEvenWithNewest(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "dup"), []byte("LOCAL"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	registerRemoteListing(reg, "folder_root", []map[string]interface{}{
		{"token": duplicateRemoteFileIDFirst, "name": "dup", "type": "file", "size": 5, "created_time": "1", "modified_time": "1"},
		{"token": duplicateRemoteFolderID, "name": "dup", "type": "folder", "created_time": "2", "modified_time": "2"},
	})
	registerRemoteListing(reg, duplicateRemoteFolderID, nil)

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--on-duplicate-remote", "newest",
		"--if-exists", "skip",
		"--as", "bot",
	}, f, stdout)
	assertDuplicateRemotePathError(t, err, "dup", duplicateRemoteFileIDFirst, duplicateRemoteFolderID)
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty on duplicate_remote_path, got: %s", stdout.String())
	}

	reg.Verify(t)
}

func TestDrivePushDeleteRemoteDeletesUnchosenDuplicateSibling(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "dup.txt"), []byte("LOCAL"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	registerDuplicateRemoteFiles(reg)
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/drive/v1/files/" + duplicateRemoteFileIDFirst,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--if-exists", "skip",
		"--on-duplicate-remote", "newest",
		"--delete-remote",
		"--yes",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}
	assertPushItemAction(t, stdout.Bytes(), "dup.txt", "deleted_remote", duplicateRemoteFileIDFirst)

	reg.Verify(t)
}

func TestDrivePushOldestOverwritesChosenDuplicateAndDeletesSibling(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "dup.txt"), []byte("LOCAL"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	registerDuplicateRemoteFiles(reg)
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "dup-oldest-new-token",
				"version":    "v11",
			},
		},
	}
	reg.Register(uploadStub)
	deleteStub := &httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/drive/v1/files/" + duplicateRemoteFileIDSecond,
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	}
	reg.Register(deleteStub)

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--if-exists", "overwrite",
		"--on-duplicate-remote", "oldest",
		"--delete-remote",
		"--yes",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	body := decodeDriveMultipartBody(t, uploadStub)
	if got := body.Fields["file_token"]; got != duplicateRemoteFileIDFirst {
		t.Fatalf("upload_all form file_token = %q, want %q", got, duplicateRemoteFileIDFirst)
	}
	assertPushItemAction(t, stdout.Bytes(), "dup.txt", "deleted_remote", duplicateRemoteFileIDSecond)
	if deleteStub.CapturedHeaders == nil {
		t.Fatal("DELETE for the newer duplicate sibling was never issued")
	}

	reg.Verify(t)
}

func TestDrivePushNewestResolvesNestedDuplicateRemoteFilesEndToEnd(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll(filepath.Join("local", "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "sub", "dup.txt"), []byte("LOCAL"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	registerRemoteListing(reg, "folder_root", []map[string]interface{}{
		{"token": duplicateRemoteFolderID, "name": "sub", "type": "folder", "created_time": "1", "modified_time": "1"},
	})
	registerRemoteListing(reg, duplicateRemoteFolderID, []map[string]interface{}{
		{"token": duplicateRemoteFileIDFirst, "name": "dup.txt", "type": "file", "size": 5, "created_time": "1", "modified_time": "1"},
		{"token": duplicateRemoteFileIDSecond, "name": "dup.txt", "type": "file", "size": 6, "created_time": "2", "modified_time": "2"},
	})
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "nested-dup-new-token",
				"version":    "v7",
			},
		},
	}
	reg.Register(uploadStub)
	deleteStub := &httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/drive/v1/files/" + duplicateRemoteFileIDFirst,
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	}
	reg.Register(deleteStub)

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--if-exists", "overwrite",
		"--on-duplicate-remote", "newest",
		"--delete-remote",
		"--yes",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	body := decodeDriveMultipartBody(t, uploadStub)
	if got := body.Fields["file_token"]; got != duplicateRemoteFileIDSecond {
		t.Fatalf("upload_all form file_token = %q, want %q", got, duplicateRemoteFileIDSecond)
	}
	assertPushItemAction(t, stdout.Bytes(), "sub/dup.txt", "deleted_remote", duplicateRemoteFileIDFirst)
	if deleteStub.CapturedHeaders == nil {
		t.Fatal("DELETE for nested duplicate sibling was never issued")
	}

	reg.Verify(t)
}

func TestChooseRemoteFileSortsByParsedTimes(t *testing.T) {
	files := []driveRemoteEntry{
		{FileToken: "token_b", CreatedTime: "9", ModifiedTime: "9"},
		{FileToken: "token_a", CreatedTime: "10", ModifiedTime: "10"},
	}
	gotNewest, err := chooseRemoteFile(files, driveDuplicateRemoteNewest)
	if err != nil {
		t.Fatalf("chooseRemoteFile newest: %v", err)
	}
	if gotNewest.FileToken != "token_a" {
		t.Fatalf("newest token = %q, want token_a", gotNewest.FileToken)
	}
	gotOldest, err := chooseRemoteFile(files, driveDuplicateRemoteOldest)
	if err != nil {
		t.Fatalf("chooseRemoteFile oldest: %v", err)
	}
	if gotOldest.FileToken != "token_b" {
		t.Fatalf("oldest token = %q, want token_b", gotOldest.FileToken)
	}
}

// TestChooseRemoteFileSortsMixedUnitEpochsByActualTime verifies duplicate
// resolution compares actual timestamps rather than raw integer magnitudes when
// Drive mixes second- and millisecond-resolution epoch strings.
func TestChooseRemoteFileSortsMixedUnitEpochsByActualTime(t *testing.T) {
	files := []driveRemoteEntry{
		{FileToken: "token_seconds", CreatedTime: "1715594881", ModifiedTime: "1715594881"},
		{FileToken: "token_millis", CreatedTime: "1715594880123", ModifiedTime: "1715594880123"},
	}
	gotNewest, err := chooseRemoteFile(files, driveDuplicateRemoteNewest)
	if err != nil {
		t.Fatalf("chooseRemoteFile newest: %v", err)
	}
	if gotNewest.FileToken != "token_seconds" {
		t.Fatalf("newest token = %q, want token_seconds", gotNewest.FileToken)
	}
	gotOldest, err := chooseRemoteFile(files, driveDuplicateRemoteOldest)
	if err != nil {
		t.Fatalf("chooseRemoteFile oldest: %v", err)
	}
	if gotOldest.FileToken != "token_millis" {
		t.Fatalf("oldest token = %q, want token_millis", gotOldest.FileToken)
	}
}

// TestDrivePushDeleteRemoteKeepsActualNewestDuplicateAcrossMixedEpochUnits
// proves the duplicate selector and delete pass agree on the true newest file
// even when remote timestamps use mixed epoch units.
func TestDrivePushDeleteRemoteKeepsActualNewestDuplicateAcrossMixedEpochUnits(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "dup.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	registerRemoteListing(reg, "folder_root", []map[string]interface{}{
		{"token": duplicateRemoteFileIDFirst, "name": "dup.txt", "type": "file", "size": 5, "created_time": "1715594880123", "modified_time": "1715594880123"},
		{"token": duplicateRemoteFileIDSecond, "name": "dup.txt", "type": "file", "size": 6, "created_time": "1715594881", "modified_time": "1715594881"},
	})
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "dup-new-token",
				"version":    "v7",
			},
		},
	}
	reg.Register(uploadStub)
	deleteStub := &httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/drive/v1/files/" + duplicateRemoteFileIDFirst,
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	}
	reg.Register(deleteStub)

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--if-exists", "overwrite",
		"--on-duplicate-remote", "newest",
		"--delete-remote",
		"--yes",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	body := decodeDriveMultipartBody(t, uploadStub)
	if got := body.Fields["file_token"]; got != duplicateRemoteFileIDSecond {
		t.Fatalf("upload_all form file_token = %q, want %q", got, duplicateRemoteFileIDSecond)
	}
	assertPushItemAction(t, stdout.Bytes(), "dup.txt", "deleted_remote", duplicateRemoteFileIDFirst)
	if deleteStub.CapturedHeaders == nil {
		t.Fatal("DELETE for older mixed-unit duplicate sibling was never issued")
	}

	reg.Verify(t)
}

func TestChooseRemoteFileFallsBackToFileTokenOnTimeParseFailure(t *testing.T) {
	files := []driveRemoteEntry{
		{FileToken: "token_a", CreatedTime: "bad", ModifiedTime: "bad"},
		{FileToken: "token_b", CreatedTime: "10", ModifiedTime: "10"},
	}
	got, err := chooseRemoteFile(files, driveDuplicateRemoteNewest)
	if err != nil {
		t.Fatalf("chooseRemoteFile: %v", err)
	}
	if got.FileToken != "token_a" {
		t.Fatalf("fallback token = %q, want token_a", got.FileToken)
	}
}

func TestChooseRemoteFileRejectsEmptyCandidates(t *testing.T) {
	_, err := chooseRemoteFile(nil, driveDuplicateRemoteNewest)
	if err == nil {
		t.Fatal("expected chooseRemoteFile to reject empty candidates")
	}
}

func TestCompareDriveRemoteModifiedToLocalSupportsSecondAndMillisecondEpochs(t *testing.T) {
	t.Run("second resolution truncates local mtime", func(t *testing.T) {
		cmp, ok := compareDriveRemoteModifiedToLocal("100", time.Unix(100, 900*int64(time.Millisecond)))
		if !ok {
			t.Fatal("expected second-resolution timestamp to parse")
		}
		if cmp != 0 {
			t.Fatalf("cmp = %d, want 0 when local only differs below second resolution", cmp)
		}
	})

	t.Run("millisecond resolution stays precise", func(t *testing.T) {
		const remoteMillis = int64(1715594880123)
		cmp, ok := compareDriveRemoteModifiedToLocal(strconv.FormatInt(remoteMillis, 10), time.UnixMilli(remoteMillis))
		if !ok {
			t.Fatal("expected millisecond-resolution timestamp to parse")
		}
		if cmp != 0 {
			t.Fatalf("cmp = %d, want 0 for equal millisecond timestamps", cmp)
		}
	})

	t.Run("microsecond resolution stays precise", func(t *testing.T) {
		const remoteMicros = int64(1715594880123456)
		cmp, ok := compareDriveRemoteModifiedToLocal(strconv.FormatInt(remoteMicros, 10), time.UnixMicro(remoteMicros))
		if !ok {
			t.Fatal("expected microsecond-resolution timestamp to parse")
		}
		if cmp != 0 {
			t.Fatalf("cmp = %d, want 0 for equal microsecond timestamps", cmp)
		}
	})

	t.Run("invalid timestamp is rejected", func(t *testing.T) {
		if _, ok := compareDriveRemoteModifiedToLocal("not-a-time", time.Now()); ok {
			t.Fatal("expected invalid remote timestamp to be rejected")
		}
	})
}

func TestDrivePullRemoteViewsRejectsUnknownStrategy(t *testing.T) {
	_, _, err := drivePullRemoteViews([]driveRemoteEntry{
		{RelPath: "dup.txt", Type: driveTypeFile, FileToken: duplicateRemoteFileIDFirst},
		{RelPath: "dup.txt", Type: driveTypeFile, FileToken: duplicateRemoteFileIDSecond},
	}, "mystery")
	if err == nil {
		t.Fatal("expected drivePullRemoteViews to reject an unknown duplicate strategy")
	}
}

func registerDuplicateRemoteFiles(reg *httpmock.Registry) {
	registerRemoteListing(reg, "folder_root", []map[string]interface{}{
		{"token": duplicateRemoteFileIDFirst, "name": "dup.txt", "type": "file", "size": 5, "created_time": "1", "modified_time": "1"},
		{"token": duplicateRemoteFileIDSecond, "name": "dup.txt", "type": "file", "size": 6, "created_time": "2", "modified_time": "2"},
	})
}

func registerRemoteListing(reg *httpmock.Registry, folderToken string, files []map[string]interface{}) {
	items := make([]interface{}, 0, len(files))
	for _, file := range files {
		items = append(items, file)
	}
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=" + folderToken,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files":    items,
				"has_more": false,
			},
		},
	})
}

func registerDownload(reg *httpmock.Registry, fileToken, body string) {
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/" + fileToken + "/download",
		Status:  200,
		Body:    []byte(body),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})
}

func assertDuplicateRemotePathError(t *testing.T, err error, relPath string, tokens ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected duplicate_remote_path error, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitAPI {
		t.Fatalf("exit code = %d, want %d", exitErr.Code, output.ExitAPI)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "duplicate_remote_path" {
		t.Fatalf("error detail = %#v, want duplicate_remote_path", exitErr.Detail)
	}
	detailMap, ok := exitErr.Detail.Detail.(map[string]interface{})
	if !ok {
		t.Fatalf("duplicate detail type = %T, want map[string]interface{}", exitErr.Detail.Detail)
	}
	duplicates, ok := detailMap["duplicates_remote"].([]driveDuplicateRemotePath)
	if !ok {
		t.Fatalf("duplicate detail duplicates_remote type = %T, want []driveDuplicateRemotePath", detailMap["duplicates_remote"])
	}
	if len(duplicates) == 0 {
		t.Fatal("duplicate detail should include at least one rel_path group")
	}
	if _, hasLegacyFilesKey := detailMap["files"]; hasLegacyFilesKey {
		t.Fatalf("duplicate detail should not expose legacy files key: %#v", detailMap)
	}
	var matched bool
	for _, duplicate := range duplicates {
		if duplicate.RelPath != relPath {
			continue
		}
		matched = true
		if len(duplicate.Entries) != len(tokens) {
			t.Fatalf("duplicate entry count = %d, want %d for rel_path %q", len(duplicate.Entries), len(tokens), relPath)
		}
		for i, token := range tokens {
			if duplicate.Entries[i].FileToken != token {
				t.Fatalf("duplicate entry %d file_token = %q, want %q", i, duplicate.Entries[i].FileToken, token)
			}
			if duplicate.Entries[i].Type == "" {
				t.Fatalf("duplicate entry %d missing type for rel_path %q", i, relPath)
			}
		}
	}
	if !matched {
		t.Fatalf("duplicate detail missing rel_path group %q: %#v", relPath, duplicates)
	}
	raw, marshalErr := json.Marshal(exitErr.Detail.Detail)
	if marshalErr != nil {
		t.Fatalf("marshal detail: %v", marshalErr)
	}
	text := string(raw)
	if !strings.Contains(text, relPath) {
		t.Fatalf("duplicate detail missing rel_path %q: %s", relPath, text)
	}
	for _, token := range tokens {
		if !strings.Contains(text, token) {
			t.Fatalf("duplicate detail missing token %q: %s", token, text)
		}
	}
}

type drivePullStdoutPayload struct {
	Data struct {
		Summary struct {
			Downloaded int `json:"downloaded"`
			Skipped    int `json:"skipped"`
			Failed     int `json:"failed"`
		} `json:"summary"`
		Items []struct {
			RelPath   string `json:"rel_path"`
			FileToken string `json:"file_token,omitempty"`
			SourceID  string `json:"source_id,omitempty"`
			Action    string `json:"action"`
		} `json:"items"`
	} `json:"data"`
}

func decodeDrivePullStdout(t *testing.T, raw []byte) drivePullStdoutPayload {
	t.Helper()
	var payload drivePullStdoutPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode pull stdout: %v\n%s", err, string(raw))
	}
	return payload
}

func findPullItem(items []struct {
	RelPath   string `json:"rel_path"`
	FileToken string `json:"file_token,omitempty"`
	SourceID  string `json:"source_id,omitempty"`
	Action    string `json:"action"`
}, relPath string) struct {
	RelPath   string `json:"rel_path"`
	FileToken string `json:"file_token,omitempty"`
	SourceID  string `json:"source_id,omitempty"`
	Action    string `json:"action"`
} {
	for _, item := range items {
		if item.RelPath == relPath {
			return item
		}
	}
	return struct {
		RelPath   string `json:"rel_path"`
		FileToken string `json:"file_token,omitempty"`
		SourceID  string `json:"source_id,omitempty"`
		Action    string `json:"action"`
	}{}
}

func expectedRenamedRelPath(relPath, fileToken string, hashLen, attempt int) string {
	sum := sha256.Sum256([]byte(fileToken))
	hash := hex.EncodeToString(sum[:])
	suffix := "__lark_" + hash[:hashLen]
	if attempt > 0 {
		suffix = "__lark_" + hash + "_" + strconv.Itoa(attempt)
	}
	dir, base := path.Split(relPath)
	ext := path.Ext(base)
	if ext == base {
		return dir + base + suffix
	}
	stem := base[:len(base)-len(ext)]
	return dir + stem + suffix + ext
}

func assertPullItemAction(t *testing.T, raw []byte, relPath, action string) {
	t.Helper()
	var payload struct {
		Data struct {
			Items []struct {
				RelPath string `json:"rel_path"`
				Action  string `json:"action"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode pull stdout: %v\n%s", err, string(raw))
	}
	for _, item := range payload.Data.Items {
		if item.RelPath == relPath && item.Action == action {
			return
		}
	}
	t.Fatalf("missing pull item %q/%q in stdout: %s", relPath, action, string(raw))
}

func assertPushItemAction(t *testing.T, raw []byte, relPath, action, fileToken string) {
	t.Helper()
	var payload struct {
		Data struct {
			Items []struct {
				RelPath   string `json:"rel_path"`
				Action    string `json:"action"`
				FileToken string `json:"file_token"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode push stdout: %v\n%s", err, string(raw))
	}
	for _, item := range payload.Data.Items {
		if item.RelPath == relPath && item.Action == action && item.FileToken == fileToken {
			return
		}
	}
	t.Fatalf("missing push item %q/%q/%q in stdout: %s", relPath, action, fileToken, string(raw))
}
