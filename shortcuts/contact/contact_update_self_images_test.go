// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

type selfAvatarFixtureFile struct {
	data                                []byte
	size                                int64
	mode                                fs.FileMode
	statErr, openErr, readErr, closeErr error
	closes                              int
}

type selfAvatarFixtureFiles struct {
	fileio.FileIO
	file         *selfAvatarFixtureFile
	stats, opens int
}

func (*selfAvatarFixtureFiles) Name() string                                  { return "self-profile-files" }
func (f *selfAvatarFixtureFiles) ResolveFileIO(context.Context) fileio.FileIO { return f }
func (f *selfAvatarFixtureFiles) Stat(name string) (fileio.FileInfo, error) {
	f.stats++
	if name != "avatar.png" {
		return nil, fs.ErrNotExist
	}
	if f.file.statErr != nil {
		return nil, f.file.statErr
	}
	return f.file, nil
}
func (f *selfAvatarFixtureFiles) Open(name string) (fileio.File, error) {
	f.opens++
	if name != "avatar.png" {
		return nil, fs.ErrNotExist
	}
	if f.file.openErr != nil {
		return nil, f.file.openErr
	}
	return &selfAvatarFixtureReader{Reader: bytes.NewReader(f.file.data), file: f.file}, nil
}

func (f *selfAvatarFixtureFile) Size() int64       { return f.size }
func (f *selfAvatarFixtureFile) Mode() fs.FileMode { return f.mode }
func (f *selfAvatarFixtureFile) IsDir() bool       { return f.mode.IsDir() }

type selfAvatarFixtureReader struct {
	*bytes.Reader
	file *selfAvatarFixtureFile
}

func (r *selfAvatarFixtureReader) Read(p []byte) (int, error) {
	if r.file.readErr != nil {
		return 0, r.file.readErr
	}
	return r.Reader.Read(p)
}
func (r *selfAvatarFixtureReader) Close() error { r.file.closes++; return r.file.closeErr }

func installSelfAvatarFiles(t *testing.T, wire *selfAvatarWire) *selfAvatarFixtureFiles {
	t.Helper()
	files := &selfAvatarFixtureFiles{file: &selfAvatarFixtureFile{data: []byte("avatar fixture bytes"), size: 20}}
	wire.factory.FileIOProvider = files
	return files
}

func TestUpdateSelfImageFiles(t *testing.T) {
	wire := setupSelfAvatarWire(t, "directory:employee.update:write im:resource")
	files := installSelfAvatarFiles(t, wire)
	files.file.size = 1 // The file may grow between Stat and Open; upload the complete bounded read.
	data, _, err := executeSelfAvatar(t, wire, "--avatar-file", "avatar.png")
	if err != nil {
		t.Fatalf("image update failed: %v", err)
	}
	uploads := wire.calls("POST")
	if len(uploads) != 1 || uploads[0].ImageType != "avatar" || uploads[0].ImageName != "avatar.png" || !bytes.Equal(uploads[0].ImageBytes, []byte("avatar fixture bytes")) {
		t.Fatalf("unexpected avatar upload: %+v", uploads)
	}
	if files.file.closes != 1 {
		t.Fatalf("avatar closes=%d", files.file.closes)
	}
	patches := wire.calls("PATCH")
	if len(patches) != 1 {
		t.Fatalf("patches=%d", len(patches))
	}
	var body map[string]map[string]string
	if err := json.Unmarshal(patches[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(body, map[string]map[string]string{"employee": {"avatar_key": "img_fixture_avatar"}}) {
		t.Fatalf("PATCH=%s", patches[0].Body)
	}
	if !reflect.DeepEqual(data.UpdatedFields, []string{"avatar_key"}) {
		t.Fatalf("fields=%v", data.UpdatedFields)
	}
	if len(wire.requests) != 3 || wire.requests[0].Method != "GET" || wire.requests[1].Method != "POST" || wire.requests[2].Method != "PATCH" {
		t.Fatalf("expected user_info, avatar upload, then PATCH: %+v", wire.requests)
	}
}

func TestUpdateSelfImageFailures(t *testing.T) {
	readFailure := errors.New("fixture read failure")
	closeFailure := errors.New("fixture close failure")
	for _, tc := range []struct {
		name        string
		file        *selfAvatarFixtureFile
		uploadReply string
		wantUploads int
	}{
		{"missing file", &selfAvatarFixtureFile{statErr: fs.ErrNotExist}, "", 0},
		{"empty file", &selfAvatarFixtureFile{}, "", 0},
		{"too large", &selfAvatarFixtureFile{size: 10*1024*1024 + 1}, "", 0},
		{"not regular", &selfAvatarFixtureFile{size: 1, mode: fs.ModeNamedPipe}, "", 0},
		{"open denied", &selfAvatarFixtureFile{size: 1, openErr: fs.ErrPermission}, "", 0},
		{"read fails", &selfAvatarFixtureFile{size: 1, readErr: readFailure}, "", 0},
		{"close fails", &selfAvatarFixtureFile{size: 1, data: []byte("x"), closeErr: closeFailure}, "", 0},
		{"became empty", &selfAvatarFixtureFile{size: 1}, "", 0},
		{"grew past limit", &selfAvatarFixtureFile{size: 1, data: bytes.Repeat([]byte("x"), 10*1024*1024+1)}, "", 0},
		{"upload API rejects", &selfAvatarFixtureFile{size: 1, data: []byte("x")}, `{"code":234007,"msg":"fixture upload rejected"}`, 1},
		{"upload has no key", &selfAvatarFixtureFile{size: 1, data: []byte("x")}, `{"code":0,"data":{}}`, 1},
		{"upload has blank key", &selfAvatarFixtureFile{size: 1, data: []byte("x")}, `{"code":0,"data":{"image_key":" "}}`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := setupSelfAvatarWire(t, "directory:employee.update:write im:resource")
			files := installSelfAvatarFiles(t, wire)
			files.file = tc.file
			if tc.uploadReply != "" {
				wire.uploadResponse = tc.uploadReply
			}
			_, output, err := executeSelfAvatar(t, wire, "--avatar-file", "avatar.png")
			if err == nil || output != "" || len(wire.calls("POST")) != tc.wantUploads || len(wire.calls("PATCH")) != 0 {
				t.Fatalf("err=%v output=%s uploads=%d patches=%d", err, output, len(wire.calls("POST")), len(wire.calls("PATCH")))
			}
		})
	}
}

func TestUpdateSelfImageGrantRequiredBeforeUpload(t *testing.T) {
	wire := setupSelfAvatarWire(t, "directory:employee.update:write")
	files := installSelfAvatarFiles(t, wire)
	_, _, err := executeSelfAvatar(t, wire, "--avatar-file", "avatar.png")
	var permission *errs.PermissionError
	if !errors.As(err, &permission) || permission.Subtype != errs.SubtypeMissingScope ||
		!reflect.DeepEqual(permission.MissingScopes, []string{"im:resource"}) ||
		len(wire.calls("POST")) != 0 || len(wire.calls("PATCH")) != 0 || files.opens != 0 {
		t.Fatalf("err=%v uploads=%d patches=%d opens=%d", err, len(wire.calls("POST")), len(wire.calls("PATCH")), files.opens)
	}
}

func TestUpdateSelfFilePreviewDoesNotRead(t *testing.T) {
	wire := setupSelfAvatarWire(t, "")
	files := installSelfAvatarFiles(t, wire)
	_, output, err := executeSelfAvatar(t, wire, "--avatar-file", "missing.png", "--dry-run")
	if err != nil || len(wire.requests) != 0 || files.stats != 0 || files.opens != 0 || wire.credentials.tokenValues != 0 {
		t.Fatalf("err=%v requests=%d stat=%d open=%d", err, len(wire.requests), files.stats, files.opens)
	}
	if !strings.Contains(output, `"avatar"`) || !strings.Contains(output, "avatar_key") {
		t.Fatalf("incomplete preview: %s", output)
	}
}

func TestUpdateSelfImageLimitIncludesTenMiB(t *testing.T) {
	wire := setupSelfAvatarWire(t, "directory:employee.update:write im:resource")
	files := installSelfAvatarFiles(t, wire)
	files.file = &selfAvatarFixtureFile{data: bytes.Repeat([]byte("x"), 10*1024*1024), size: 10 * 1024 * 1024}
	_, _, err := executeSelfAvatar(t, wire, "--avatar-file", "avatar.png")
	if err != nil || len(wire.calls("POST")) != 1 || len(wire.calls("PATCH")) != 1 {
		t.Fatalf("err=%v uploads=%d patches=%d", err, len(wire.calls("POST")), len(wire.calls("PATCH")))
	}
	if len(wire.calls("POST")[0].ImageBytes) != 10*1024*1024 {
		t.Fatal("image bytes were truncated at the size limit")
	}
}

func TestUpdateSelfUploadSuccessIsNotProfileSuccess(t *testing.T) {
	wire := setupSelfAvatarWire(t, "directory:employee.update:write im:resource")
	installSelfAvatarFiles(t, wire)
	wire.patchResponse = `{"code":2221156,"msg":"fixture profile update rejected"}`
	_, output, err := executeSelfAvatar(t, wire, "--avatar-file", "avatar.png")
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Code != 2221156 || problem.LogID != "self-profile-fixture-log" || !strings.Contains(problem.Message, "fixture profile update rejected") || output != "" || len(wire.calls("POST")) != 1 || len(wire.calls("PATCH")) != 1 {
		t.Fatalf("err=%v output=%s uploads=%d patches=%d", err, output, len(wire.calls("POST")), len(wire.calls("PATCH")))
	}
}

func TestUpdateSelfPreservesTypedFileIOErrors(t *testing.T) {
	for _, stage := range []string{"stat", "open", "read", "close"} {
		t.Run(stage, func(t *testing.T) {
			wire := setupSelfAvatarWire(t, "directory:employee.update:write im:resource")
			files := installSelfAvatarFiles(t, wire)
			failure := errs.NewNetworkError(errs.SubtypeNetworkTimeout, "fixture storage timeout").WithRetryable().WithCause(context.DeadlineExceeded)
			image := files.file
			switch stage {
			case "stat":
				image.statErr = failure
			case "open":
				image.openErr = failure
			case "read":
				image.readErr = failure
			case "close":
				image.closeErr = failure
			}
			_, output, err := executeSelfAvatar(t, wire, "--avatar-file", "avatar.png")
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkTimeout || !problem.Retryable || !errors.Is(err, failure) {
				t.Fatalf("FileIO error semantics lost: err=%v problem=%+v", err, problem)
			}
			if output != "" || len(wire.calls("POST")) != 0 || len(wire.calls("PATCH")) != 0 {
				t.Fatalf("storage failure continued writes: output=%s requests=%+v", output, wire.requests)
			}
		})
	}
}
