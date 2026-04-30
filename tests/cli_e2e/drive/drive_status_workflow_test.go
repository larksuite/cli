// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"os"
	"path/filepath"
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
}
