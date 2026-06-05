// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDrive_StatusWorkflow exercises +status against a real Drive folder so
// the parts that dry-run can't reach — recursive listing pagination, the
// download+hash leg, scope handling, and the SHA-256 comparison itself —
// are covered against the real backend.
//
// Layout:
//
//	folder/                       (--folder-token target)
//	├── unchanged.txt   "match"   ↔ local: "match"     → unchanged
//	├── modified.txt    "remote"  ↔ local: "local"     → modified
//	└── remote-only.txt "remote"  ↔ (none)             → new_remote
//	local/                        (--local-dir target)
//	├── unchanged.txt   "match"
//	├── modified.txt    "local"
//	└── local-only.txt  "anything"                     → new_local
//
// Expected output: each of the four buckets contains exactly the file we
// expect, with file_token set for the three buckets that have a Drive side.
func TestDrive_StatusWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	folderName := "lark-cli-e2e-drive-status-" + suffix
	folderToken := createDriveFolder(t, parentT, ctx, folderName, "")

	// Local working directory. +status's --local-dir must be relative to
	// the binary's cwd, so each upload + the +status invocation share the
	// same WorkDir.
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "local"), 0o755); err != nil {
		t.Fatalf("mkdir local: %v", err)
	}

	// Helper: write a local file under workDir/<rel>.
	writeLocal := func(rel, content string) {
		t.Helper()
		full := filepath.Join(workDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir parent of %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	// Helper: stage <content> into a sibling temp file then upload it as
	// <name> under folderToken. +upload reads --file relative to its cwd.
	uploadDriveFile := func(name, content string) string {
		t.Helper()
		// Stage outside `local/` so the local-side tree only sees what
		// the test wants; +upload still reads relative to workDir.
		stage := "_upload_" + name
		writeLocal(stage, content)
		t.Cleanup(func() { _ = os.Remove(filepath.Join(workDir, stage)) })

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+upload",
				"--file", stage,
				"--folder-token", folderToken,
				"--name", name,
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		fileToken := gjson.Get(result.Stdout, "data.file_token").String()
		require.NotEmpty(t, fileToken, "uploaded file should have a token, stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			cleanupCtx, cleanupCancel := clie2e.CleanupContext()
			defer cleanupCancel()
			deleteResult, deleteErr := clie2e.RunCmdWithRetry(cleanupCtx, clie2e.Request{
				Args:      []string{"drive", "+delete", "--file-token", fileToken, "--type", "file", "--yes"},
				DefaultAs: "bot",
			}, clie2e.RetryOptions{})
			clie2e.ReportCleanupFailure(parentT, "delete drive file "+fileToken, deleteResult, deleteErr)
		})
		return fileToken
	}

	// Seed both sides. Order doesn't matter functionally, but doing the
	// uploads first lets the +status listing pick up everything in a
	// single pass.
	tokUnchanged := uploadDriveFile("unchanged.txt", "match")
	tokModified := uploadDriveFile("modified.txt", "remote")
	tokRemoteOnly := uploadDriveFile("remote-only.txt", "remote")

	writeLocal("local/unchanged.txt", "match")  // matches remote → unchanged
	writeLocal("local/modified.txt", "local")   // differs → modified
	writeLocal("local/local-only.txt", "extra") // only here → new_local

	// Run +status against the real folder.
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+status",
			"--local-dir", "local",
			"--folder-token", folderToken,
		},
		WorkDir:   workDir,
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	skipDriveStatusExactIfMissingDownloadScope(t, result)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	// Assert each bucket contains exactly the file we expect, with the
	// correct file_token for sides that have one.
	out := result.Stdout

	cases := []struct {
		bucket string
		path   string
		token  string // empty when the bucket has no Drive side
	}{
		{"unchanged", "unchanged.txt", tokUnchanged},
		{"modified", "modified.txt", tokModified},
		{"new_local", "local-only.txt", ""},
		{"new_remote", "remote-only.txt", tokRemoteOnly},
	}
	for _, c := range cases {
		bucket := gjson.Get(out, "data."+c.bucket)
		if !bucket.IsArray() {
			t.Fatalf("data.%s must be an array, stdout:\n%s", c.bucket, out)
		}
		var found bool
		bucket.ForEach(func(_, entry gjson.Result) bool {
			if entry.Get("rel_path").String() != c.path {
				return true // continue
			}
			found = true
			if c.token != "" {
				if got := entry.Get("file_token").String(); got != c.token {
					t.Errorf("%s entry %q: file_token=%q want %q", c.bucket, c.path, got, c.token)
				}
			} else if entry.Get("file_token").String() != "" {
				t.Errorf("%s entry %q must not carry file_token (local-only), stdout:\n%s", c.bucket, c.path, out)
			}
			return false // stop
		})
		if !found {
			t.Errorf("%s bucket missing %q\nstdout:\n%s", c.bucket, c.path, out)
		}
	}

	// Make sure each bucket is exactly the size we expect (4 files total,
	// no double-bucketing). +upload may attach extra metadata (e.g. a
	// folder type entry for `local/` itself) but the lister filters
	// type=file so the buckets should be clean.
	for _, b := range []struct {
		bucket string
		want   int
	}{
		{"unchanged", 1},
		{"modified", 1},
		{"new_local", 1},
		{"new_remote", 1},
	} {
		got := int(gjson.Get(out, "data."+b.bucket+".#").Int())
		if got != b.want {
			t.Errorf("data.%s length=%d want %d\nstdout:\n%s", b.bucket, got, b.want, out)
		}
	}

	quickResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+status",
			"--local-dir", "local",
			"--folder-token", folderToken,
			"--quick",
		},
		WorkDir:   workDir,
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	quickResult.AssertExitCode(t, 0)
	quickResult.AssertStdoutStatus(t, true)

	quickOut := quickResult.Stdout
	if got := gjson.Get(quickOut, "data.detection").String(); got != "quick" {
		t.Fatalf("quick detection=%q want quick\nstdout:\n%s", got, quickOut)
	}
	if got := int(gjson.Get(quickOut, "data.new_local.#").Int()); got != 1 {
		t.Fatalf("quick new_local length=%d want 1\nstdout:\n%s", got, quickOut)
	}
	if got := int(gjson.Get(quickOut, "data.new_remote.#").Int()); got != 1 {
		t.Fatalf("quick new_remote length=%d want 1\nstdout:\n%s", got, quickOut)
	}
	if got := gjson.Get(quickOut, "data.new_local.0.rel_path").String(); got != "local-only.txt" {
		t.Fatalf("quick new_local path=%q want local-only.txt\nstdout:\n%s", got, quickOut)
	}
	if got := gjson.Get(quickOut, "data.new_remote.0.rel_path").String(); got != "remote-only.txt" {
		t.Fatalf("quick new_remote path=%q want remote-only.txt\nstdout:\n%s", got, quickOut)
	}
	sharedCount := int(gjson.Get(quickOut, "data.modified.#").Int() + gjson.Get(quickOut, "data.unchanged.#").Int())
	if sharedCount != 2 {
		t.Fatalf("quick shared file count=%d want 2 across modified+unchanged\nstdout:\n%s", sharedCount, quickOut)
	}
	for _, path := range []string{"unchanged.txt", "modified.txt"} {
		if !gjson.Get(quickOut, `data.modified.#(rel_path="`+path+`")`).Exists() && !gjson.Get(quickOut, `data.unchanged.#(rel_path="`+path+`")`).Exists() {
			t.Fatalf("quick output missing shared path %q\nstdout:\n%s", path, quickOut)
		}
	}
}

// TestDrive_StatusQuickWorkflow proves that --quick really follows modified_time
// semantics on the live backend instead of silently behaving like the default
// exact hash mode.
//
// The fixture intentionally makes the two shared files diverge in opposite ways:
// - same-mtime.txt:   bytes differ, mtime matches remote   → quick=unchanged / exact=modified
// - remote-newer.txt: bytes match, local mtime is older    → quick=modified  / exact=unchanged
//
// This locks in the best-effort nature of quick mode with real Drive
// modified_time values fetched from the list API, plus the expected new_local /
// new_remote buckets.
func TestDrive_StatusQuickWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	folderName := "lark-cli-e2e-drive-status-quick-" + suffix
	folderToken := createDriveFolder(t, parentT, ctx, folderName, "")

	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "local"), 0o755); err != nil {
		t.Fatalf("mkdir local: %v", err)
	}

	writeLocal := func(rel, content string) {
		t.Helper()
		full := filepath.Join(workDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir parent of %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	uploadDriveFile := func(name, content string) string {
		t.Helper()
		stage := "_upload_" + name
		writeLocal(stage, content)
		t.Cleanup(func() { _ = os.Remove(filepath.Join(workDir, stage)) })

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+upload",
				"--file", stage,
				"--folder-token", folderToken,
				"--name", name,
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		fileToken := gjson.Get(result.Stdout, "data.file_token").String()
		require.NotEmpty(t, fileToken, "uploaded file should have a token, stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			cleanupCtx, cleanupCancel := clie2e.CleanupContext()
			defer cleanupCancel()
			deleteResult, deleteErr := clie2e.RunCmdWithRetry(cleanupCtx, clie2e.Request{
				Args:      []string{"drive", "+delete", "--file-token", fileToken, "--type", "file", "--yes"},
				DefaultAs: "bot",
			}, clie2e.RetryOptions{})
			clie2e.ReportCleanupFailure(parentT, "delete drive file "+fileToken, deleteResult, deleteErr)
		})
		return fileToken
	}

	tokSameMtime := uploadDriveFile("same-mtime.txt", "remote bytes A")
	tokRemoteNewer := uploadDriveFile("remote-newer.txt", "remote bytes B")
	tokRemoteOnly := uploadDriveFile("remote-only.txt", "remote only")

	remoteFiles := listDriveFolderFilesByName(t, ctx, folderToken)
	sameMtimeRemote := remoteFiles["same-mtime.txt"]
	remoteNewer := remoteFiles["remote-newer.txt"]
	if sameMtimeRemote.ModifiedTime == "" || remoteNewer.ModifiedTime == "" {
		t.Fatalf("expected modified_time for shared remote files, got: %#v", remoteFiles)
	}

	writeLocal("local/same-mtime.txt", "local bytes A")    // bytes differ from remote
	writeLocal("local/remote-newer.txt", "remote bytes B") // bytes match remote
	writeLocal("local/local-only.txt", "local only")       // local-only bucket

	sameMtimePath := filepath.Join(workDir, "local", "same-mtime.txt")
	remoteNewerPath := filepath.Join(workDir, "local", "remote-newer.txt")
	sameMtimeAt := mustParseDriveEpochForE2E(t, sameMtimeRemote.ModifiedTime)
	remoteNewerAt := mustParseDriveEpochForE2E(t, remoteNewer.ModifiedTime)
	if err := os.Chtimes(sameMtimePath, sameMtimeAt, sameMtimeAt); err != nil {
		t.Fatalf("chtimes same-mtime.txt: %v", err)
	}
	localOlder := remoteNewerAt.Add(-2 * time.Second)
	if err := os.Chtimes(remoteNewerPath, localOlder, localOlder); err != nil {
		t.Fatalf("chtimes remote-newer.txt: %v", err)
	}

	quickResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+status",
			"--local-dir", "local",
			"--folder-token", folderToken,
			"--quick",
		},
		WorkDir:   workDir,
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	quickResult.AssertExitCode(t, 0)
	quickResult.AssertStdoutStatus(t, true)

	quickOut := quickResult.Stdout
	if got := gjson.Get(quickOut, "data.detection").String(); got != "quick" {
		t.Fatalf("quick detection=%q want quick\nstdout:\n%s", got, quickOut)
	}
	assertStatusBucketEntry(t, quickOut, "unchanged", "same-mtime.txt", tokSameMtime)
	assertStatusBucketEntry(t, quickOut, "modified", "remote-newer.txt", tokRemoteNewer)
	assertStatusBucketEntry(t, quickOut, "new_local", "local-only.txt", "")
	assertStatusBucketEntry(t, quickOut, "new_remote", "remote-only.txt", tokRemoteOnly)
	assertStatusBucketLen(t, quickOut, "unchanged", 1)
	assertStatusBucketLen(t, quickOut, "modified", 1)
	assertStatusBucketLen(t, quickOut, "new_local", 1)
	assertStatusBucketLen(t, quickOut, "new_remote", 1)

	exactResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+status",
			"--local-dir", "local",
			"--folder-token", folderToken,
		},
		WorkDir:   workDir,
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	skipDriveStatusExactIfMissingDownloadScope(t, exactResult)
	exactResult.AssertExitCode(t, 0)
	exactResult.AssertStdoutStatus(t, true)

	exactOut := exactResult.Stdout
	if got := gjson.Get(exactOut, "data.detection").String(); got != "exact" {
		t.Fatalf("exact detection=%q want exact\nstdout:\n%s", got, exactOut)
	}
	assertStatusBucketEntry(t, exactOut, "modified", "same-mtime.txt", tokSameMtime)
	assertStatusBucketEntry(t, exactOut, "unchanged", "remote-newer.txt", tokRemoteNewer)
	assertStatusBucketEntry(t, exactOut, "new_local", "local-only.txt", "")
	assertStatusBucketEntry(t, exactOut, "new_remote", "remote-only.txt", tokRemoteOnly)
	assertStatusBucketLen(t, exactOut, "unchanged", 1)
	assertStatusBucketLen(t, exactOut, "modified", 1)
	assertStatusBucketLen(t, exactOut, "new_local", 1)
	assertStatusBucketLen(t, exactOut, "new_remote", 1)
}

type driveStatusListedFile struct {
	Token        string
	ModifiedTime string
}

func listDriveFolderFilesByName(t *testing.T, ctx context.Context, folderToken string) map[string]driveStatusListedFile {
	t.Helper()
	params := fmt.Sprintf(`{"folder_token":"%s","page_size":200}`, folderToken)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"drive", "files", "list", "--params", params},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	files := make(map[string]driveStatusListedFile)
	gjson.Get(result.Stdout, "data.files").ForEach(func(_, entry gjson.Result) bool {
		name := entry.Get("name").String()
		if name == "" {
			return true
		}
		files[name] = driveStatusListedFile{
			Token:        entry.Get("token").String(),
			ModifiedTime: entry.Get("modified_time").String(),
		}
		return true
	})
	return files
}

func mustParseDriveEpochForE2E(t *testing.T, raw string) time.Time {
	t.Helper()
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatalf("parse Drive epoch %q: %v", raw, err)
	}
	switch {
	case v > 1e14 || v < -1e14:
		return time.UnixMicro(v)
	case v > 1e11 || v < -1e11:
		return time.UnixMilli(v)
	default:
		return time.Unix(v, 0)
	}
}

func assertStatusBucketEntry(t *testing.T, stdout, bucket, relPath, fileToken string) {
	t.Helper()
	entry := gjson.Get(stdout, `data.`+bucket+`.#(rel_path="`+relPath+`")`)
	if !entry.Exists() {
		t.Fatalf("bucket %s missing rel_path %q\nstdout:\n%s", bucket, relPath, stdout)
	}
	if fileToken == "" {
		if got := entry.Get("file_token").String(); got != "" {
			t.Fatalf("bucket %s rel_path %q unexpectedly carried file_token=%q\nstdout:\n%s", bucket, relPath, got, stdout)
		}
		return
	}
	if got := entry.Get("file_token").String(); got != fileToken {
		t.Fatalf("bucket %s rel_path %q file_token=%q want %q\nstdout:\n%s", bucket, relPath, got, fileToken, stdout)
	}
}

func assertStatusBucketLen(t *testing.T, stdout, bucket string, want int) {
	t.Helper()
	if got := int(gjson.Get(stdout, "data."+bucket+".#").Int()); got != want {
		t.Fatalf("bucket %s length=%d want %d\nstdout:\n%s", bucket, got, want, stdout)
	}
}

func skipDriveStatusExactIfMissingDownloadScope(t *testing.T, result *clie2e.Result) {
	t.Helper()
	if result == nil || result.ExitCode == 0 {
		return
	}
	combinedLower := strings.ToLower(result.Stdout + "\n" + result.Stderr)
	if strings.Contains(combinedLower, "missing_scope") && strings.Contains(combinedLower, "drive:file:download") {
		t.Skipf("skip drive +status exact live workflow due to missing drive:file:download scope: stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
}
