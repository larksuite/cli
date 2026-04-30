// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// countingOpenProvider wraps a fileio.Provider and counts FileIO.Open
// calls. Used by the multipart test to pin the single-shared-fd
// optimization: a regression to reopen-per-block would push the
// counter above 1.
type countingOpenProvider struct {
	inner fileio.Provider
	opens *atomic.Int32
}

func (p *countingOpenProvider) Name() string { return "counting-open" }

func (p *countingOpenProvider) ResolveFileIO(ctx context.Context) fileio.FileIO {
	return &countingOpenFileIO{inner: p.inner.ResolveFileIO(ctx), opens: p.opens}
}

type countingOpenFileIO struct {
	inner fileio.FileIO
	opens *atomic.Int32
}

func (c *countingOpenFileIO) Open(name string) (fileio.File, error) {
	c.opens.Add(1)
	return c.inner.Open(name)
}
func (c *countingOpenFileIO) Stat(name string) (fileio.FileInfo, error) {
	return c.inner.Stat(name)
}
func (c *countingOpenFileIO) ResolvePath(p string) (string, error) { return c.inner.ResolvePath(p) }
func (c *countingOpenFileIO) Save(p string, opts fileio.SaveOptions, body io.Reader) (fileio.SaveResult, error) {
	return c.inner.Save(p, opts, body)
}

// TestDrivePushUploadsAndCreatesParents verifies the happy path: a local
// tree with a top-level file and a subdirectory is mirrored onto a Drive
// folder, with create_folder called once for the missing sub-folder.
func TestDrivePushUploadsAndCreatesParents(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll(filepath.Join("local", "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "a.txt"), []byte("AAA"), 0o644); err != nil {
		t.Fatalf("WriteFile a: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "sub", "b.txt"), []byte("BBB"), 0o644); err != nil {
		t.Fatalf("WriteFile b: %v", err)
	}

	// Empty remote: no files, no folders.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"files": []interface{}{}, "has_more": false},
		},
	})

	// Upload a.txt into the root folder.
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"file_token": "tok_a"},
		},
	})

	// create_folder for "sub", returns a fresh token.
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/create_folder",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"token": "fld_sub"},
		},
	})

	// Upload b.txt into the freshly created sub-folder.
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"file_token": "tok_b"},
		},
	})

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"uploaded": 2`) {
		t.Errorf("expected uploaded=2, got: %s", out)
	}
	if !strings.Contains(out, `"failed": 0`) {
		t.Errorf("expected failed=0, got: %s", out)
	}
	if !strings.Contains(out, `"deleted_remote": 0`) {
		t.Errorf("expected deleted_remote=0 (flag not set), got: %s", out)
	}
}

// TestDrivePushOverwritesWhenIfExistsOverwrite verifies that a local file
// whose rel_path already maps to a type=file on Drive is sent through
// upload_all with the existing file_token in the form body, and that the
// returned version is propagated to items[].version.
func TestDrivePushOverwritesWhenIfExistsOverwrite(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "keep.txt"), []byte("local-new"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_keep_old", "name": "keep.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})

	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "tok_keep_new",
				"version":    "v42",
			},
		},
	}
	reg.Register(uploadStub)

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		// Default --if-exists is now "skip"; opt into overwrite explicitly
		// to exercise the file_token-on-upload_all path this test pins.
		"--if-exists", "overwrite",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"uploaded": 1`) {
		t.Errorf("expected uploaded=1, got: %s", out)
	}
	if !strings.Contains(out, `"action": "overwritten"`) {
		t.Errorf("expected action=overwritten, got: %s", out)
	}
	if !strings.Contains(out, `"version": "v42"`) {
		t.Errorf("expected version propagated, got: %s", out)
	}

	// The overwrite request must carry the existing file_token in the
	// form body so the backend knows to bump the existing file's version
	// rather than create a sibling. Decode the captured multipart body
	// using the existing decoder helper.
	body := decodeDriveMultipartBody(t, uploadStub)
	if got := body.Fields["file_token"]; got != "tok_keep_old" {
		t.Fatalf("upload_all form file_token = %q, want tok_keep_old", got)
	}
	if got := body.Fields["file_name"]; got != "keep.txt" {
		t.Fatalf("upload_all form file_name = %q, want keep.txt", got)
	}
}

// TestDrivePushDefaultsToSkipForExistingRemote pins the new default. Until
// the upload_all overwrite/version protocol field is fully rolled out, the
// default --if-exists is "skip" so a first-time push against a non-empty
// folder never trips on the missing-version branch by surprise. A test
// that registers no upload_all stub would fail with "unmatched stub" if
// the default were still "overwrite".
func TestDrivePushDefaultsToSkipForExistingRemote(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "keep.txt"), []byte("local"), 0o644); err != nil {
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

	// Intentionally NO upload_all stub: with the new default, the file
	// is silently skipped — any upload_all call would trip the registry's
	// strict no-stub-or-die contract.

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"skipped": 1`) {
		t.Errorf("expected default behavior to skip existing remote files, got: %s", out)
	}
	if !strings.Contains(out, `"uploaded": 0`) {
		t.Errorf("expected uploaded=0 under default --if-exists=skip, got: %s", out)
	}
	if !strings.Contains(out, `"failed": 0`) {
		t.Errorf("expected failed=0, got: %s", out)
	}
}

// TestDrivePushSkipsWhenIfExistsSkip verifies --if-exists=skip leaves Drive
// content untouched and counts the file under summary.skipped without
// hitting upload_all.
func TestDrivePushSkipsWhenIfExistsSkip(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "keep.txt"), []byte("local"), 0o644); err != nil {
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

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
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
	if !strings.Contains(out, `"uploaded": 0`) {
		t.Errorf("expected uploaded=0 with --if-exists=skip, got: %s", out)
	}

	// reg.Verify is implicit via the registry's no-stub-or-die contract:
	// if --if-exists=skip had triggered an upload_all anyway, the request
	// would 404 against the registry and the run would have errored above.
}

// TestDrivePushDeleteRemoteRequiresYes locks in the upfront safety guard:
// --delete-remote without --yes must be refused before any list / upload
// happens, so a stray flag never silently deletes anything.
func TestDrivePushDeleteRemoteRequiresYes(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--delete-remote",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for --delete-remote without --yes, got nil")
	}
	if !strings.Contains(err.Error(), "--delete-remote") || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error must mention --delete-remote and --yes, got: %v", err)
	}
}

// TestDrivePushDeleteRemoteSkipsOnlineDocs is the load-bearing safety
// regression: with --delete-remote --yes, the command must only DELETE
// type=file Drive entries that have no local counterpart. Online docs
// (docx/sheet/bitable/...), shortcuts and folders that share a rel_path
// with a missing local file must be left alone.
func TestDrivePushDeleteRemoteSkipsOnlineDocs(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// One local file that already exists remotely and matches by rel_path.
	if err := os.WriteFile(filepath.Join("local", "kept.txt"), []byte("kept"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Remote contains:
	//   - kept.txt: a type=file with a local counterpart -> overwritten
	//   - orphan.txt: a type=file without local counterpart -> deleted_remote
	//   - notes.docx: an online doc -> must be left alone (not deleted)
	//   - link: a shortcut -> must be left alone
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_kept", "name": "kept.txt", "type": "file"},
					map[string]interface{}{"token": "tok_orphan", "name": "orphan.txt", "type": "file"},
					map[string]interface{}{"token": "tok_doc", "name": "notes.docx", "type": "docx"},
					map[string]interface{}{"token": "tok_link", "name": "link", "type": "shortcut"},
				},
				"has_more": false,
			},
		},
	})

	// Overwrite kept.txt.
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "tok_kept",
				"version":    "v2",
			},
		},
	})

	deleteStub := &httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/drive/v1/files/tok_orphan",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	}
	reg.Register(deleteStub)

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		// This test exercises the overwrite path against kept.txt; opt
		// into overwrite explicitly now that the default is "skip".
		"--if-exists", "overwrite",
		"--delete-remote",
		"--yes",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"uploaded": 1`) {
		t.Errorf("expected uploaded=1 (overwrite of kept.txt), got: %s", out)
	}
	if !strings.Contains(out, `"deleted_remote": 1`) {
		t.Errorf("expected deleted_remote=1 (just orphan.txt), got: %s", out)
	}
	if strings.Contains(out, "notes.docx") {
		t.Errorf("notes.docx must not appear in items (online docs are protected): %s", out)
	}
	// Bare-substring check (matches the notes.docx assertion above):
	// drivePushItem serializes the field as "file_token", so a check
	// against `"token": "tok_link"` would never fire — silent test
	// failure to catch a regression. A bare substring is strictly
	// stronger: it catches leaks into file_token, log lines, anywhere.
	if strings.Contains(out, "tok_link") {
		t.Errorf("shortcut tok_link must not appear in items: %s", out)
	}

	// Registry.RoundTrip sets CapturedHeaders unconditionally on a match,
	// so a non-nil value proves the DELETE for tok_orphan actually went
	// through. If the test reached this point with no DELETE issued the
	// registry would already have errored on the first uncovered request.
	if deleteStub.CapturedHeaders == nil {
		t.Fatal("DELETE for tok_orphan was never issued; --delete-remote did not run")
	}
}

// TestDrivePushRejectsAbsoluteLocalDir confirms SafeLocalFlagPath surfaces
// the proper flag name in the error message.
func TestDrivePushRejectsAbsoluteLocalDir(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
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

// TestDrivePushRejectsBadIfExistsEnum verifies the framework's enum guard.
func TestDrivePushRejectsBadIfExistsEnum(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
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

// TestDrivePushOverwriteWithoutVersionFails surfaces the protocol contract:
// when the upload_all response for an overwrite call lacks both `version`
// and `data_version`, the shortcut must report a structured api_error
// rather than silently report success. Important during the transitional
// period where the deployed backend may not yet expose the field.
func TestDrivePushOverwriteWithoutVersionFails(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "keep.txt"), []byte("local"), 0o644); err != nil {
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
	// upload_all returns a NEW file_token but no version. Using a distinct
	// value (tok_keep_new vs the entry's tok_keep) is what makes the test
	// able to distinguish "items[].file_token comes from the upload_all
	// response" from "fall back to entry.FileToken" — if the assertion
	// stubbed the same token as the entry, a regression to the fallback
	// branch would silently still pass.
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"file_token": "tok_keep_new"},
		},
	})

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		// Default is "skip" now; this test specifically exercises the
		// overwrite-failure branch, so opt into overwrite explicitly.
		"--if-exists", "overwrite",
		"--as", "bot",
	}, f, stdout)
	// Item-level failures bump the exit code via output.ErrBare(ExitAPI),
	// preserving the structured items[] envelope on stdout. Older behavior
	// was to silently return nil; the assertion below pins the new contract.
	if err == nil {
		t.Fatalf("expected non-zero exit on item-level failure, got nil\nstdout: %s", stdout.String())
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitAPI {
		t.Errorf("expected ExitAPI (%d), got code=%d", output.ExitAPI, exitErr.Code)
	}
	if exitErr.Detail != nil {
		t.Errorf("ErrBare should carry no Detail (the items[] envelope already covered the per-item error), got: %#v", exitErr.Detail)
	}

	out := stdout.String()
	// summary.failed should reflect the missing version; summary.uploaded
	// should not pretend the overwrite succeeded.
	if !strings.Contains(out, `"failed": 1`) {
		t.Errorf("expected failed=1, got: %s", out)
	}
	if !strings.Contains(out, "no version") {
		t.Errorf("expected error about missing version in items[].error, got: %s", out)
	}
	// Pin the token-stability contract: the failed item must surface the
	// token returned by upload_all (tok_keep_new), NOT the fallback
	// entry.FileToken (tok_keep). Without this, a regression that always
	// uses entry.FileToken on failure would slip through.
	if !strings.Contains(out, `"file_token": "tok_keep_new"`) {
		t.Errorf("expected failed item to surface upload_all's returned file_token (tok_keep_new), got: %s", out)
	}
}

// TestDrivePushOverwritePartialSuccessSurfacesReturnedToken pins the
// partial-success contract for upload_all: when the backend returns a
// non-zero `code` AND a non-empty `data.file_token`, the bytes have
// already landed under that returned token. The shortcut must surface
// THAT token in items[].file_token (not silently fall back to
// entry.FileToken via the empty-string sentinel), so callers always
// have a usable handle to whatever state Drive ended up in.
func TestDrivePushOverwritePartialSuccessSurfacesReturnedToken(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "keep.txt"), []byte("local"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_keep_old", "name": "keep.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})
	// upload_all returns a partial-success: non-zero code + a brand-new
	// file_token. The shortcut must not drop that token.
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 1234, "msg": "partial-success simulated",
			"data": map[string]interface{}{"file_token": "tok_keep_partial"},
		},
	})

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--if-exists", "overwrite",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatalf("expected non-zero exit on item-level failure, got nil\nstdout: %s", stdout.String())
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitAPI {
		t.Fatalf("expected ExitAPI from output.ExitError, got %T %v", err, err)
	}

	out := stdout.String()
	if !strings.Contains(out, `"failed": 1`) {
		t.Errorf("expected failed=1, got: %s", out)
	}
	// The freshly returned token must be the one in items[].file_token,
	// not the stale entry.FileToken (tok_keep_old).
	if !strings.Contains(out, `"file_token": "tok_keep_partial"`) {
		t.Errorf("expected items[].file_token to surface upload_all's returned token (tok_keep_partial), got: %s", out)
	}
	if strings.Contains(out, `"file_token": "tok_keep_old"`) {
		t.Errorf("must NOT fall back to entry.FileToken when upload_all returned a token; got: %s", out)
	}
}

// TestDrivePushSkipsDeleteAfterUploadFailure pins the half-sync safety
// guard: when any upload / overwrite / folder step fails, the
// --delete-remote phase must be skipped entirely. Otherwise a partial
// upload would proceed to delete remote orphans, leaving the tenant in
// a worse state than where it started (files missing locally and now
// also gone from Drive).
//
// Test setup mirrors the missing-version overwrite failure: one local
// file with a same-name remote that overwrites with no version field,
// plus one orphan remote file that --delete-remote --yes would
// otherwise delete. With the fix in place the orphan must NOT be
// reached.
func TestDrivePushSkipsDeleteAfterUploadFailure(t *testing.T) {
	f, stdout, stderrBuf, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "keep.txt"), []byte("local"), 0o644); err != nil {
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
					map[string]interface{}{"token": "tok_orphan", "name": "orphan.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})
	// Overwrite returns no version → fails.
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"file_token": "tok_keep"},
		},
	})
	// Crucially: do NOT register a DELETE stub for tok_orphan. If the
	// delete phase were still triggered after the upload failure, the
	// registry's no-stub-or-die contract would surface an unmatched
	// request and the test would catch the regression.

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--if-exists", "overwrite",
		"--delete-remote",
		"--yes",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatalf("expected non-zero exit on overwrite failure, got nil\nstdout: %s", stdout.String())
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitAPI {
		t.Fatalf("expected ExitAPI ExitError, got %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, `"failed": 1`) {
		t.Errorf("expected failed=1 (overwrite missing-version), got: %s", out)
	}
	if !strings.Contains(out, `"deleted_remote": 0`) {
		t.Errorf("expected deleted_remote=0 (delete phase must skip after upload failure), got: %s", out)
	}
	if strings.Contains(out, "tok_orphan") {
		t.Errorf("orphan file token must not appear in items (delete phase skipped), got: %s", out)
	}
	// Operator-facing hint that cleanup was skipped lives on stderr.
	if !strings.Contains(stderrBuf.String(), "Skipping --delete-remote") {
		t.Errorf("expected stderr hint about skipped delete phase, got: %s", stderrBuf.String())
	}
}

// TestDrivePushExitsZeroOnCleanRun pins the inverse: a successful run
// with no failures must NOT bump the exit code. Without this the
// ErrBare-on-failure path could regress to "always non-zero" silently.
func TestDrivePushExitsZeroOnCleanRun(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "a.txt"), []byte("AAA"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"files": []interface{}{}, "has_more": false},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"file_token": "tok_a"},
		},
	})

	if err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout); err != nil {
		t.Fatalf("expected nil error on clean run, got: %v\nstdout: %s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"failed": 0`) {
		t.Errorf("expected failed=0, got: %s", stdout.String())
	}
}

// TestDrivePushUploadsSiblingWhenRemoteSameNameIsNativeDoc pins the
// behavior when a local regular file shares its rel_path with a Lark
// native cloud document on Drive (sheet/docx/bitable/...).
//
// The contract:
//   - The native doc is NOT a candidate for overwrite — the remoteFiles
//     view collapsed off listRemoteFolder only keeps type=file entries,
//     so the local "report" is treated as new and the upload_all body
//     must NOT carry the sheet's token in the file_token form field
//     (that would mean overwriting the sheet's bytes, not creating a
//     sibling).
//   - The native doc must NOT be deleted by --delete-remote --yes either.
//     The orphan check iterates remoteFiles only; a sheet at the same
//     rel_path as a missing local file would otherwise look orphaned.
//
// Both invariants together: native cloud docs are never reached by either
// the upload or the delete path, regardless of whether a same-named
// local file exists.
func TestDrivePushUploadsSiblingWhenRemoteSameNameIsNativeDoc(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "report"), []byte("local-csv-bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Remote contains:
	//   - "report" as a Lark native sheet  (type=sheet, tok_sheet) — must be
	//     left strictly alone
	//   - "minutes" as a native docx       (type=docx,  tok_docx)  — no
	//     local counterpart, --delete-remote must skip it
	// No type=file entries at all — every local rel_path is a "new upload"
	// from the remoteFiles view's perspective (listRemoteFolder returns the
	// sheet/docx in the unified entry map but they're filtered out when
	// collapsing to the type=file view).
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_sheet", "name": "report", "type": "sheet"},
					map[string]interface{}{"token": "tok_docx", "name": "minutes", "type": "docx"},
				},
				"has_more": false,
			},
		},
	})

	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"file_token": "tok_report_file"},
		},
	}
	reg.Register(uploadStub)

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--delete-remote",
		"--yes",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"uploaded": 1`) {
		t.Errorf("expected uploaded=1 (sibling create, not overwrite), got: %s", out)
	}
	// Native docs are never iterated for delete; --delete-remote must be 0.
	if !strings.Contains(out, `"deleted_remote": 0`) {
		t.Errorf("native docs must not be deleted, got: %s", out)
	}
	// Items must show the upload as a fresh "uploaded", not "overwritten",
	// and must point at the new token, never at the sheet's token.
	if !strings.Contains(out, `"action": "uploaded"`) {
		t.Errorf("expected action=uploaded for sibling create, got: %s", out)
	}
	if !strings.Contains(out, `"file_token": "tok_report_file"`) {
		t.Errorf("expected new file_token in items, got: %s", out)
	}
	if strings.Contains(out, "tok_sheet") {
		t.Errorf("sheet token must not appear anywhere in output, got: %s", out)
	}
	if strings.Contains(out, "tok_docx") {
		t.Errorf("docx token must not appear anywhere in output, got: %s", out)
	}

	// The upload_all multipart body must NOT carry file_token. Carrying
	// the sheet's token would mean asking the backend to overwrite the
	// sheet's bytes with our local file's bytes — the exact bug this
	// test guards against. file_name is "report" (no extension, matching
	// the basename) and parent_node is the root folder.
	body := decodeDriveMultipartBody(t, uploadStub)
	if got, present := body.Fields["file_token"]; present {
		t.Fatalf("upload_all body must NOT include file_token (would overwrite the sheet); got %q", got)
	}
	if got := body.Fields["file_name"]; got != "report" {
		t.Fatalf("upload_all file_name = %q, want \"report\"", got)
	}
	if got := body.Fields["parent_node"]; got != "folder_root" {
		t.Fatalf("upload_all parent_node = %q, want folder_root", got)
	}
}

// TestDrivePushReusesExistingRemoteFolder verifies that when a remote
// folder already exists at the target rel_path, the upload uses its
// pre-existing token instead of calling create_folder. Together with the
// "creates parents" test this pins the folderCache contract end-to-end.
func TestDrivePushReusesExistingRemoteFolder(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll(filepath.Join("local", "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join("local", "sub", "b.txt"), []byte("BBB"), 0o644); err != nil {
		t.Fatalf("WriteFile b: %v", err)
	}

	// Root contains the folder "sub" already; recurse returns no files.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "fld_existing_sub", "name": "sub", "type": "folder"},
				},
				"has_more": false,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=fld_existing_sub",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"files": []interface{}{}, "has_more": false},
		},
	})

	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"file_token": "tok_b"},
		},
	}
	reg.Register(uploadStub)

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	if !strings.Contains(stdout.String(), `"uploaded": 1`) {
		t.Errorf("expected uploaded=1, got: %s", stdout.String())
	}

	// Upload must have been routed under the pre-existing sub-folder
	// rather than re-creating it; the form's parent_node should be the
	// remote folder token discovered in the listing.
	body := decodeDriveMultipartBody(t, uploadStub)
	if got := body.Fields["parent_node"]; got != "fld_existing_sub" {
		t.Fatalf("upload parent_node = %q, want fld_existing_sub (folderCache miss?)", got)
	}
}

// TestDrivePushMirrorsEmptyDirectories confirms the gap codex review
// flagged: a local directory with no files inside must still surface on
// Drive as a created sub-folder, not be silently dropped because the
// upload loop only iterates regular files.
func TestDrivePushMirrorsEmptyDirectories(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll(filepath.Join("local", "empty"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Empty remote.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"files": []interface{}{}, "has_more": false},
		},
	})

	// create_folder for "empty" — must be issued even though no file
	// will land underneath it.
	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/create_folder",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"token": "fld_empty"},
		},
	}
	reg.Register(createStub)

	err := mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"action": "folder_created"`) {
		t.Errorf("expected folder_created action for empty dir, got: %s", out)
	}
	if !strings.Contains(out, `"rel_path": "empty"`) {
		t.Errorf("expected rel_path=empty in items, got: %s", out)
	}
	// Empty dirs do NOT bump summary.uploaded — that field is for files.
	if !strings.Contains(out, `"uploaded": 0`) {
		t.Errorf("uploaded should remain 0 for an empty-dir-only push, got: %s", out)
	}
	if createStub.CapturedHeaders == nil {
		t.Fatal("create_folder for empty/ was never issued")
	}
}

// TestDrivePushUploadsLargeFileViaMultipart locks in the 3-step
// upload_prepare/upload_part/upload_finish flow used for files larger
// than common.MaxDriveMediaUploadSinglePartSize. The single shared file
// handle (opened once outside the part loop, reused via SectionReader)
// is implicitly exercised: the test asserts upload_part is invoked
// twice in a row before upload_finish, and would deadlock or surface a
// fileio open error if the helper still re-opened per chunk and the
// httpmock registry didn't have enough Open-permission stubs.
//
// The local file is created with Truncate to size+1 so it crosses the
// single-part threshold by one byte, exercising the "remaining < block
// size" tail path on the second chunk.
//
// Use a distinct AppID to avoid SDK token cache collisions with other
// tests (mirrors the existing TestDriveUploadLargeFileUsesMultipart
// pattern).
func TestDrivePushUploadsLargeFileViaMultipart(t *testing.T) {
	pushTestConfig := &core.CliConfig{
		AppID: "drive-push-multipart-test", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, pushTestConfig)

	// Wrap the default FileIO provider to count Open calls. The
	// shared-fd refactor opens the local file exactly once and feeds
	// each block through io.NewSectionReader at distinct offsets; a
	// regression back to "open per block" would bump this counter to
	// block_num (2 in this test).
	opens := &atomic.Int32{}
	f.FileIOProvider = &countingOpenProvider{inner: f.FileIOProvider, opens: opens}

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	largePath := filepath.Join("local", "large.bin")
	fh, err := os.Create(largePath)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := fh.Truncate(common.MaxDriveMediaUploadSinglePartSize + 1); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Empty remote — large.bin is a fresh upload, not an overwrite.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"files": []interface{}{}, "has_more": false},
		},
	})

	prepareStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_prepare",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"upload_id":  "upid-large",
				"block_size": float64(common.MaxDriveMediaUploadSinglePartSize),
				"block_num":  float64(2),
			},
		},
	}
	reg.Register(prepareStub)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_part",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_part",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_finish",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"file_token": "tok_large"},
		},
	})

	err = mountAndRunDrive(t, DrivePush, []string{
		"+push",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"uploaded": 1`) {
		t.Errorf("expected uploaded=1, got: %s", out)
	}
	if !strings.Contains(out, `"file_token": "tok_large"`) {
		t.Errorf("expected file_token=tok_large in items, got: %s", out)
	}

	// upload_prepare's body must NOT include file_token (this was a
	// fresh upload, not an overwrite). Decoding the body proves both
	// that the request was issued and that the conditional file_token
	// branch in drivePushUploadMultipart was skipped correctly.
	body := decodeCapturedJSONBody(t, prepareStub)
	if _, present := body["file_token"]; present {
		t.Fatalf("upload_prepare body must omit file_token for fresh uploads, got: %#v", body)
	}
	if got := body["parent_type"]; got != driveUploadParentTypeExplorer {
		t.Fatalf("parent_type = %#v, want %q", got, driveUploadParentTypeExplorer)
	}
	if got := body["parent_node"]; got != "folder_root" {
		t.Fatalf("parent_node = %#v, want folder_root", got)
	}

	// One Open across the entire multipart upload — the load-bearing
	// pin for the shared-fd refactor. block_num=2 above means a
	// reopen-per-block regression would land at 2.
	if got := opens.Load(); got != 1 {
		t.Fatalf("FileIO.Open invocations = %d, want exactly 1 (single shared fd across all blocks)", got)
	}
}

// TestDrivePushHelpersRelPath pins the path utilities since the upload
// loop relies on slash-only rel_paths even on Windows.
func TestDrivePushHelpersRelPath(t *testing.T) {
	t.Parallel()

	if got := drivePushParentRel("a.txt"); got != "" {
		t.Fatalf("parent of root file should be empty, got %q", got)
	}
	if got := drivePushParentRel("a/b/c.txt"); got != "a/b" {
		t.Fatalf("parent of a/b/c.txt = %q, want a/b", got)
	}
	if parent, name := drivePushSplitRel("a/b/c"); parent != "a/b" || name != "c" {
		t.Fatalf("split a/b/c = (%q,%q), want (a/b,c)", parent, name)
	}
	if parent, name := drivePushSplitRel("solo"); parent != "" || name != "solo" {
		t.Fatalf("split solo = (%q,%q), want (\"\",solo)", parent, name)
	}
	if got := joinRelDrive("", "x"); got != "x" {
		t.Fatalf("joinRelDrive(\"\", x) = %q, want x", got)
	}
	if got := joinRelDrive("a", "x"); got != "a/x" {
		t.Fatalf("joinRelDrive(a, x) = %q, want a/x", got)
	}
}
