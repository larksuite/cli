// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"mime"
	"mime/multipart"
	"os"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// TestSheetMediaParentType pins the token→parent_type mapping that every
// sheets image-upload entry point funnels through. Native spreadsheet tokens
// use "sheet_image"; imported "office" spreadsheets carry a "fake_office_"
// synthetic token and must upload with "office_sheet_file".
func TestSheetMediaParentType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		token string
		want  string
	}{
		{"native spreadsheet token", "shtcnABC123", sheetImageParentType},
		{"empty token", "", sheetImageParentType},
		{"office imported token", "fake_office_abc123", officeSheetFileParentType},
		{"office token, only the prefix", fakeOfficeTokenPrefix, officeSheetFileParentType},
		{"prefix mid-string is not matched", "shtfake_office_abc", sheetImageParentType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := sheetMediaParentType(tc.token); got != tc.want {
				t.Fatalf("sheetMediaParentType(%q) = %q, want %q", tc.token, got, tc.want)
			}
		})
	}
}

// TestUploadSheetImage_ParentType exercises the uploadSheetImage collector end
// to end (the Execute path the dry-run tests don't reach), asserting the
// parent_type that actually goes out on the wire is derived from the token: a
// native spreadsheet uploads as sheet_image, an imported "office" spreadsheet
// (fake_office_-prefixed token) as office_sheet_file.
func TestUploadSheetImage_ParentType(t *testing.T) {
	cases := []struct {
		name           string
		token          string
		wantParentType string
	}{
		{"native spreadsheet", "shtcnTOK123", sheetImageParentType},
		{"office imported spreadsheet", "fake_office_abc123", officeSheetFileParentType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runtime, reg := newSheetMediaTestRuntime(t)
			// UploadDriveMediaAllTyped opens the file via the runtime's FileIO,
			// which sandboxes paths to the current working directory; chdir to a
			// temp dir and pass a relative name so the open is allowed.
			cmdutil.TestChdir(t, t.TempDir())
			if err := os.WriteFile("img.png", []byte("png-bytes"), 0o600); err != nil {
				t.Fatal(err)
			}

			stub := &httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/drive/v1/medias/upload_all",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"file_token": "boxTOK123"},
				},
			}
			reg.Register(stub)

			fileToken, err := uploadSheetImage(runtime, tc.token, "img.png", "img.png", 9)
			if err != nil {
				t.Fatalf("uploadSheetImage() error: %v", err)
			}
			if fileToken != "boxTOK123" {
				t.Fatalf("file_token = %q, want boxTOK123", fileToken)
			}

			body := decodeSheetMediaMultipartBody(t, stub)
			if got := body.Fields["parent_type"]; got != tc.wantParentType {
				t.Fatalf("parent_type = %q, want %q", got, tc.wantParentType)
			}
			if got := body.Fields["parent_node"]; got != tc.token {
				t.Fatalf("parent_node = %q, want %q", got, tc.token)
			}
			if got := body.Fields["file_name"]; got != "img.png" {
				t.Fatalf("file_name = %q, want img.png", got)
			}
		})
	}
}

// TestUploadSheetImage_FileOpenError confirms a missing image surfaces as a
// typed validation error (category=validation, subtype=invalid_argument) with
// the original os-level cause preserved for errors.Is, and proves the upload
// endpoint is never hit. No httpmock stub is registered, so if uploadSheetImage
// ever tried to POST upload_all the RoundTrip would return a
// "no stub for POST ..." network failure — that would surface as a
// non-validation category and fail the metadata assertion below. The
// category=validation + fs.ErrNotExist cause therefore strictly implies the
// short-circuit happened before the wire.
func TestUploadSheetImage_FileOpenError(t *testing.T) {
	runtime, _ := newSheetMediaTestRuntime(t)
	cmdutil.TestChdir(t, t.TempDir())

	_, err := uploadSheetImage(runtime, "shtcnTOK123", "missing.png", "missing.png", 1)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}

	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %v; want typed problem carrier", err)
	}
	if p.Category != errs.CategoryValidation {
		t.Fatalf("category = %q, want %q (non-validation implies the upload endpoint was reached)", p.Category, errs.CategoryValidation)
	}
	if p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidArgument)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v; want wrapped fs.ErrNotExist cause to be preserved", err)
	}
}

func newSheetMediaTestRuntime(t *testing.T) (*common.RuntimeContext, *httpmock.Registry) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := &core.CliConfig{
		AppID:     "test-sheets-media-" + t.Name(),
		AppSecret: "test-secret",
		Brand:     core.BrandFeishu,
	}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "sheets"}, cfg, f, core.AsBot)
	return runtime, reg
}

type sheetMediaCapturedMultipart struct {
	Fields map[string]string
	Files  map[string][]byte
}

func decodeSheetMediaMultipartBody(t *testing.T, stub *httpmock.Stub) sheetMediaCapturedMultipart {
	t.Helper()
	contentType := stub.CapturedHeaders.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("parse content-type %q: %v", contentType, err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("content type = %q, want multipart/form-data", mediaType)
	}
	reader := multipart.NewReader(bytes.NewReader(stub.CapturedBody), params["boundary"])
	body := sheetMediaCapturedMultipart{Fields: map[string]string{}, Files: map[string][]byte{}}
	for {
		part, err := reader.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("read multipart part: %v", err)
		}
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(part); err != nil {
			t.Fatalf("read multipart body for %q: %v", part.FormName(), err)
		}
		if part.FileName() != "" {
			body.Files[part.FormName()] = buf.Bytes()
			continue
		}
		body.Fields[part.FormName()] = buf.String()
	}
	return body
}
