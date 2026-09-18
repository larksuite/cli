// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"bytes"
	"context"
	"encoding/json"
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
// use "sheet_image"; a spreadsheet backed by an imported office file must upload
// with "office_sheet_file".
//
// The token shape itself is common.IsLocalOfficeToken's contract and is pinned
// exhaustively in TestIsLocalOfficeToken. What this test owns is the sheets
// half: that the shared answer selects office_sheet_file and nothing else does.
// The shape cases are kept here anyway, deliberately duplicated, because they
// are what would catch the mapping being wired to a different predicate — or to
// none — and a table holding only native tokens could not tell a working mapping
// from a hardcoded sheetImageParentType.
//
// Four labels here used to be wrong: the row called "25 char, at boundary" held
// a 27-character token and the three "new 27-char" rows held 28-character ones,
// so the length floor the labels claimed to cover was never actually tested. The
// 25/24 rows below are the real boundary.
func TestSheetMediaParentType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		token string
		want  string
	}{
		{"native spreadsheet token", "shtcnABC123", sheetImageParentType},
		{"empty token", "", sheetImageParentType},
		{"fake_office imported token", "fake_office_abc123", officeSheetFileParentType},
		{"fake_office token, only the prefix", common.FakeOfficeTokenPrefix, officeSheetFileParentType},
		{"local_office imported token", "local_office_abc123", officeSheetFileParentType},
		{"local_office token, only the prefix", common.LocalOfficeTokenPrefix, officeSheetFileParentType},
		{"interleaved OFL0X office token", "aaaaOaaaaFaaaaLaaaa0aaaaXaaa", officeSheetFileParentType},
		{"interleaved exlcn token", "abcdeefghxijkllmnopcqrstnuv", sheetImageParentType},
		{"interleaved shtcn native token", "abcdsefghhijkltmnopcqrstnuv", sheetImageParentType},
		{"interleaved pptcn token", "abcdpefghpijkltmnopcqrstnuv", sheetImageParentType},
		{"interleaved wodcn token", "abcdwefghoijkldmnopcqrstnuv", sheetImageParentType},
		{"interleaved OFL0X, 25 chars (marker exactly fills the token)", "aaaaOaaaaFaaaaLaaaa0aaaaX", officeSheetFileParentType},
		{"interleaved OFL0X, 24 chars (one short of holding the marker)", "aaaaOaaaaFaaaaLaaaa0aaaa", sheetImageParentType},
		{"interleaved OFL0X, 27 chars (current local-office format)", "aaaaOaaaaFaaaaLaaaa0aaaaXaa", officeSheetFileParentType},
		{"interleaved OFL0X, 29 chars (longer than any known format)", "aaaaOaaaaFaaaaLaaaa0aaaaXaaaa", officeSheetFileParentType},
		{"interleaved OFL0X, 28 chars with xlsx office-type enum", "bbbbObbbbFbbbbLbbbb0bbbbXbbE", officeSheetFileParentType},
		{"interleaved OFL0X, 28 chars with ppt office-type enum", "ccccOccccFccccLcccc0ccccXccP", officeSheetFileParentType},
		{"interleaved OFL0X, 28 chars with word office-type enum", "ddddOddddFddddLdddd0ddddXddW", officeSheetFileParentType},
		{"fake_office prefix mid-string is not matched", "shtfake_office_abc", sheetImageParentType},
		{"local_office prefix mid-string is not matched", "shtlocal_office_abc", sheetImageParentType},
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
// (legacy prefix or interleaved OFL0X marker) as office_sheet_file.
func TestUploadSheetImage_ParentType(t *testing.T) {
	cases := []struct {
		name           string
		token          string
		wantParentType string
	}{
		{"native spreadsheet", "shtcnTOK123", sheetImageParentType},
		{"fake_office imported spreadsheet", "fake_office_abc123", officeSheetFileParentType},
		{"local_office imported spreadsheet", "local_office_abc123", officeSheetFileParentType},
		{"interleaved OFL0X imported spreadsheet", "aaaaOaaaaFaaaaLaaaa0aaaaXaaa", officeSheetFileParentType},
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

// officeShapedWikiNodeToken is a wiki node_token deliberately shaped like an
// imported office token. No real wiki node looks like this — that is the point.
// A dry-run that derives its parent_type from the unresolved wiki token instead
// of from the ref's kind previews office_sheet_file for it, which is the bug
// these cases pin shut.
const officeShapedWikiNodeToken = "aaaaOaaaaFaaaaLaaaa0aaaaXaa"

// TestSheetsDryRunParentType pins the parent_type a preview shows for each ref
// kind. A sheet ref derives it from the token it already holds; a wiki ref
// cannot, because the token it holds is the node_token, not the spreadsheet one
// Execute will upload against.
//
// The wiki rows are the ones with teeth: they carry office-shaped node tokens,
// so a helper that forwarded ref.Token to sheetMediaParentType would answer
// office_sheet_file and fail here.
func TestSheetsDryRunParentType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ref  spreadsheetRef
		want string
	}{
		{"native sheet ref", spreadsheetRef{Kind: spreadsheetRefSheet, Token: "shtcnABC123"}, sheetImageParentType},
		{"office sheet ref", spreadsheetRef{Kind: spreadsheetRefSheet, Token: "fake_office_abc123"}, officeSheetFileParentType},
		{"office-marker sheet ref", spreadsheetRef{Kind: spreadsheetRefSheet, Token: officeShapedWikiNodeToken}, officeSheetFileParentType},
		{"wiki ref", spreadsheetRef{Kind: spreadsheetRefWiki, Token: "wikcnABC123"}, sheetImageParentType},
		{"wiki ref, office-shaped node token", spreadsheetRef{Kind: spreadsheetRefWiki, Token: officeShapedWikiNodeToken}, sheetImageParentType},
		{"wiki ref, office-prefixed node token", spreadsheetRef{Kind: spreadsheetRefWiki, Token: "fake_office_abc123"}, sheetImageParentType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := sheetsDryRunParentType(tc.ref); got != tc.want {
				t.Fatalf("sheetsDryRunParentType(%+v) = %q, want %q", tc.ref, got, tc.want)
			}
		})
	}
}

// TestImageUploadDryRun_WikiRefStaysNative walks the same rule through both
// shortcuts that preview an upload_all, using the public flags rather than a
// hand-built ref. Each is checked against a /sheets/ URL carrying the identical
// office-shaped token, so the two rows differ only in the ref's kind — which is
// what proves the preview reads the kind and not the token's shape.
func TestImageUploadDryRun_WikiRefStaysNative(t *testing.T) {
	t.Parallel()
	shortcuts := []struct {
		name string
		sc   common.Shortcut
		args func(urlFlag string) []string
	}{
		{"cells-set-image", CellsSetImage, func(u string) []string {
			return []string{"--url", u, "--sheet-id", testSheetID, "--range", "A1", "--image", "./README.md"}
		}},
		{"float-image-create", FloatImageCreate, func(u string) []string {
			return []string{
				"--url", u, "--sheet-id", testSheetID,
				"--image", "./README.md", "--image-name", "logo.png",
				"--position-row", "0", "--position-col", "A",
				"--size-width", "100", "--size-height", "50",
			}
		}},
	}
	for _, sc := range shortcuts {
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()
			for _, tc := range []struct {
				kind string
				url  string
				want string
			}{
				{"wiki", "https://example.feishu.cn/wiki/" + officeShapedWikiNodeToken, sheetImageParentType},
				{"sheets", "https://example.feishu.cn/sheets/" + officeShapedWikiNodeToken, officeSheetFileParentType},
			} {
				t.Run(tc.kind, func(t *testing.T) {
					calls := parseDryRunAPI(t, sc.sc, sc.args(tc.url))
					upload, _ := calls[0].(map[string]interface{})
					if upload["url"] != "/open-apis/drive/v1/medias/upload_all" {
						t.Fatalf("first call = %v, want upload_all", upload["url"])
					}
					body, _ := upload["body"].(map[string]interface{})
					if body["parent_type"] != tc.want {
						t.Fatalf("parent_type = %v, want %q", body["parent_type"], tc.want)
					}
				})
			}
		})
	}
}

// largeImageSize is one byte past the single-part ceiling: the smallest input
// that must take the chunked branch, so the test pins the boundary rather than
// some comfortably large number that would still pass a wrong comparison.
const largeImageSize = common.MaxDriveMediaUploadSinglePartSize + 1

// writeSparseImage creates a file of exactly size bytes without writing them.
// A real 20 MB fixture would cost the same in the repo and in every CI run for
// no extra signal — only the stat'd size matters to the branch under test.
func writeSparseImage(t *testing.T, name string, size int64) {
	t.Helper()
	f, err := os.Create(name)
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	defer f.Close()
	if err := f.Truncate(size); err != nil {
		t.Fatalf("truncate %s: %v", name, err)
	}
}

// TestUploadSheetImage_LargeFileUsesMultipart proves an image past the 20 MB
// ceiling takes the chunked endpoints instead of failing on upload_all, and
// that the parent_type it carries there is still the office/native answer.
//
// upload_all is deliberately left unstubbed: the registry answers an unstubbed
// call with a transport error, so a regression that sent the oversized file
// single-part fails here rather than silently uploading. That is the shape the
// backend used to punish with a bare 1061002 "params error" naming neither the
// size nor the limit.
func TestUploadSheetImage_LargeFileUsesMultipart(t *testing.T) {
	cases := []struct {
		name           string
		token          string
		wantParentType string
	}{
		{"native spreadsheet", "shtcnTOK123", sheetImageParentType},
		{"imported office spreadsheet", "aaaaOaaaaFaaaaLaaaa0aaaaXaaa", officeSheetFileParentType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runtime, reg := newSheetMediaTestRuntime(t)
			cmdutil.TestChdir(t, t.TempDir())
			writeSparseImage(t, "big.png", largeImageSize)

			prepare := &httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/drive/v1/medias/upload_prepare",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"upload_id":  "up_123",
						"block_size": float64(common.MaxDriveMediaUploadSinglePartSize),
						"block_num":  float64(2),
					},
				},
			}
			reg.Register(prepare)
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/drive/v1/medias/upload_part",
				Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
				// block_num=2 above, so the part endpoint is hit twice.
				Reusable: true,
			})
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/drive/v1/medias/upload_finish",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"file_token": "boxBIG123"},
				},
			})

			fileToken, err := uploadSheetImage(runtime, tc.token, "big.png", "big.png", largeImageSize)
			if err != nil {
				t.Fatalf("uploadSheetImage() error: %v", err)
			}
			if fileToken != "boxBIG123" {
				t.Fatalf("file_token = %q, want boxBIG123", fileToken)
			}

			var prepareBody map[string]interface{}
			if err := json.Unmarshal(prepare.CapturedBody, &prepareBody); err != nil {
				t.Fatalf("decode upload_prepare body: %v", err)
			}
			if got := prepareBody["parent_type"]; got != tc.wantParentType {
				t.Fatalf("prepare parent_type = %v, want %q", got, tc.wantParentType)
			}
			if got := prepareBody["parent_node"]; got != tc.token {
				t.Fatalf("prepare parent_node = %v, want %q", got, tc.token)
			}
		})
	}
}

// TestImageUploadDryRun_LargeFilePreviewsChunks pins that the preview follows
// the same size branch Execute takes. A preview that promised one upload_all
// for a file the CLI will send in chunks is a preview of a different request —
// the failure mode this domain has been closing everywhere else.
func TestImageUploadDryRun_LargeFilePreviewsChunks(t *testing.T) {
	cases := []struct {
		name      string
		size      int64
		wantSteps []string
	}{
		{"at the ceiling stays single-part", common.MaxDriveMediaUploadSinglePartSize, []string{
			"/open-apis/drive/v1/medias/upload_all",
		}},
		{"one byte past it goes chunked", largeImageSize, []string{
			"/open-apis/drive/v1/medias/upload_prepare",
			"/open-apis/drive/v1/medias/upload_part",
			"/open-apis/drive/v1/medias/upload_finish",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmdutil.TestChdir(t, t.TempDir())
			writeSparseImage(t, "img.png", tc.size)

			calls := parseDryRunAPI(t, CellsSetImage, []string{
				"--spreadsheet-token", testToken, "--sheet-id", testSheetID,
				"--range", "A1", "--image", "./img.png",
			})
			// The upload steps, then the set_cell_range tool call.
			if len(calls) != len(tc.wantSteps)+1 {
				t.Fatalf("api calls = %d, want %d", len(calls), len(tc.wantSteps)+1)
			}
			for i, want := range tc.wantSteps {
				call, _ := calls[i].(map[string]interface{})
				if call["url"] != want {
					t.Fatalf("call %d url = %v, want %q", i, call["url"], want)
				}
			}
		})
	}
}
