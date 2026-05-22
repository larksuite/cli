// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestDrivePreviewListOnlyNormalizesCandidates(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/file_preview/preview_result",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"preview_results": []map[string]interface{}{
					{"preview_type": 0, "preview_status": 0},
					{"preview_type": 14, "preview_status": 1},
					{"preview_type": 16, "preview_status": 7},
				},
			},
		},
	})

	err := mountAndRunDrive(t, DrivePreview, []string{
		"+preview",
		"--file-token", "file_preview",
		"--list-only",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if got := data["mode"]; got != "list" {
		t.Fatalf("mode=%v, want list", got)
	}
	candidates, _ := data["candidates"].([]interface{})
	if len(candidates) != 3 {
		t.Fatalf("len(candidates)=%d, want 3", len(candidates))
	}
	first, _ := candidates[0].(map[string]interface{})
	if got := first["type"]; got != "pdf" {
		t.Fatalf("candidate[0].type=%v, want pdf", got)
	}
	if got := first["type_code"]; got != "0" {
		t.Fatalf("candidate[0].type_code=%v, want 0", got)
	}
	if got := first["status"]; got != "READY" {
		t.Fatalf("candidate[0].status=%v, want READY", got)
	}
	if got := first["downloadable"]; got != true {
		t.Fatalf("candidate[0].downloadable=%v, want true", got)
	}
	second, _ := candidates[1].(map[string]interface{})
	if got := second["status_code"]; got != "1" {
		t.Fatalf("candidate[1].status_code=%v, want 1", got)
	}
	if got := second["reason"]; got != "Preview is still processing." {
		t.Fatalf("candidate[1].reason=%v, want processing reason", got)
	}
}

func TestDrivePreviewDownloadUsesResolvedTypeCodeAndRenamePolicy(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/file_preview/preview_result",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"version": 7,
				"preview_results": []map[string]interface{}{
					{"preview_type": 0, "preview_status": 0},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/medias/file_preview/preview_download?preview_type=0",
		Status: 200,
		Body:   []byte("%PDF-1.7"),
		Headers: http.Header{
			"Content-Type": []string{"application/pdf"},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.WriteFile(filepath.Join(tmpDir, "report.pdf"), []byte("old"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunDrive(t, DrivePreview, []string{
		"+preview",
		"--file-token", "file_preview",
		"--type", "pdf",
		"--output", "report",
		"--if-exists", "rename",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if got := data["selected_type"]; got != "pdf" {
		t.Fatalf("selected_type=%v, want pdf", got)
	}
	resolvedTmpDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("EvalSymlinks() error: %v", err)
	}
	wantPath := filepath.Join(resolvedTmpDir, "report (1).pdf")
	if got := data["output_path"]; got != wantPath {
		t.Fatalf("output_path=%v, want %s", got, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected preview artifact at %q: %v", wantPath, err)
	}
}

func TestDrivePreviewRejectsUnavailableType(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/file_preview/preview_result",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"preview_results": []map[string]interface{}{
					{"preview_type": 8, "preview_status": 0},
				},
			},
		},
	})

	err := mountAndRunDrive(t, DrivePreview, []string{
		"+preview",
		"--file-token", "file_preview",
		"--type", "pdf",
		"--output", "report",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected unavailable type error, got nil")
	}
	if !strings.Contains(err.Error(), `requested preview type "pdf" is not available`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSelectDrivePreviewCandidatePrefersDownloadableAliasMatch(t *testing.T) {
	candidate, ok := selectDrivePreviewCandidate([]drivePreviewCandidate{
		{Type: "png", TypeCode: "1", Downloadable: false, Status: "PROCESSING"},
		{Type: "jpg", TypeCode: "7", Downloadable: true, Status: "READY"},
	}, "image")
	if !ok {
		t.Fatal("expected alias match, got none")
	}
	if candidate.Type != "jpg" {
		t.Fatalf("selected candidate=%q, want jpg", candidate.Type)
	}
	if !candidate.Downloadable {
		t.Fatalf("selected candidate should be downloadable: %+v", candidate)
	}
}

func TestDriveCoverListOnlyUsesStaticSpecs(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	err := mountAndRunDrive(t, DriveCover, []string{
		"+cover",
		"--file-token", "file_cover",
		"--list-only",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	candidates, _ := data["candidates"].([]interface{})
	if len(candidates) != len(driveCoverSpecs) {
		t.Fatalf("len(candidates)=%d, want %d", len(candidates), len(driveCoverSpecs))
	}
	last, _ := candidates[len(candidates)-1].(map[string]interface{})
	if got := last["spec"]; got != "square" {
		t.Fatalf("last spec=%v, want square", got)
	}
}

func TestDriveCoverDownloadUsesMappedCoverOptionAndPreviewType(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	var capturedQuery url.Values
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/medias/file_cover/preview_download",
		Status: 200,
		Body:   []byte("png-data"),
		Headers: http.Header{
			"Content-Type": []string{"image/png"},
		},
		OnMatch: func(req *http.Request) {
			capturedQuery = req.URL.Query()
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DriveCover, []string{
		"+cover",
		"--file-token", "file_cover",
		"--spec", "square",
		"--output", "cover",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if got := data["selected_spec"]; got != "square" {
		t.Fatalf("selected_spec=%v, want square", got)
	}
	resolvedTmpDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("EvalSymlinks() error: %v", err)
	}
	wantPath := filepath.Join(resolvedTmpDir, "cover.png")
	if got := data["output_path"]; got != wantPath {
		t.Fatalf("output_path=%v, want %s", got, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected cover file at %q: %v", wantPath, err)
	}
	if got := capturedQuery.Get("preview_type"); got != "1" {
		t.Fatalf("preview_type=%q, want 1", got)
	}
	if got := capturedQuery.Get("bus_type"); got != "" {
		t.Fatalf("bus_type=%q, want empty for square crop flow", got)
	}
	if got := capturedQuery.Get("platform"); got != "" {
		t.Fatalf("platform=%q, want empty when using default platform", got)
	}
	if got := capturedQuery.Get("width"); got != "360" {
		t.Fatalf("width=%q, want 360", got)
	}
	if got := capturedQuery.Get("height"); got != "360" {
		t.Fatalf("height=%q, want 360", got)
	}
	if got := capturedQuery.Get("policy"); got != "near" {
		t.Fatalf("policy=%q, want near", got)
	}
}

func newDrivePreviewRuntime(t *testing.T, use string, stringFlags map[string]string, boolFlags map[string]bool) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: use}
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("spec", "", "")
	cmd.Flags().String("version", "", "")
	cmd.Flags().String("output", "", "")
	cmd.Flags().String("if-exists", drivePreviewIfExistsError, "")
	cmd.Flags().Bool("list-only", false, "")
	for name, value := range stringFlags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	for name, value := range boolFlags {
		if !value {
			continue
		}
		if err := cmd.Flags().Set(name, "true"); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	return common.TestNewRuntimeContextWithCtx(context.Background(), cmd, driveTestConfig())
}

func decodeDryRunOutput(t *testing.T, dry *common.DryRunAPI) map[string]interface{} {
	t.Helper()

	raw, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal dry run: %v", err)
	}
	return out
}

func TestDrivePreviewDryRunIncludesVersionAndMode(t *testing.T) {
	runtime := newDrivePreviewRuntime(t, "drive +preview", map[string]string{
		"file-token": "file_preview",
		"type":       "image",
		"version":    "7",
		"output":     "preview",
	}, nil)

	data := decodeDryRunOutput(t, DrivePreview.DryRun(context.Background(), runtime))
	if got := data["mode"]; got != "download" {
		t.Fatalf("mode=%v, want download", got)
	}
	if got := data["requested_type"]; got != "image" {
		t.Fatalf("requested_type=%v, want image", got)
	}
	api, _ := data["api"].([]interface{})
	call, _ := api[0].(map[string]interface{})
	if got := call["method"]; got != "POST" {
		t.Fatalf("method=%v, want POST", got)
	}
	if got := call["url"]; got != "/open-apis/drive/v1/medias/file_preview/preview_result" {
		t.Fatalf("url=%v, want preview_result", got)
	}
	body, _ := call["body"].(map[string]interface{})
	if got := body["version"]; got != "7" {
		t.Fatalf("body.version=%v, want 7", got)
	}
}

func TestDriveCoverDryRunListAndDownload(t *testing.T) {
	listRuntime := newDrivePreviewRuntime(t, "drive +cover", map[string]string{
		"file-token": "file_cover",
	}, map[string]bool{"list-only": true})
	listData := decodeDryRunOutput(t, DriveCover.DryRun(context.Background(), listRuntime))
	if got := listData["mode"]; got != "list" {
		t.Fatalf("list mode=%v, want list", got)
	}
	if _, ok := listData["candidates"].([]interface{}); !ok {
		t.Fatalf("list candidates missing: %#v", listData)
	}

	downloadRuntime := newDrivePreviewRuntime(t, "drive +cover", map[string]string{
		"file-token": "file_cover",
		"spec":       "square",
		"version":    "3",
		"output":     "cover",
	}, nil)
	downloadData := decodeDryRunOutput(t, DriveCover.DryRun(context.Background(), downloadRuntime))
	if got := downloadData["selected_spec"]; got != "square" {
		t.Fatalf("selected_spec=%v, want square", got)
	}
	api, _ := downloadData["api"].([]interface{})
	call, _ := api[0].(map[string]interface{})
	params, _ := call["params"].(map[string]interface{})
	if got := params["width"]; got != float64(360) {
		t.Fatalf("params.width=%v, want 360", got)
	}
	if got := params["policy"]; got != "near" {
		t.Fatalf("params.policy=%v, want near", got)
	}
}

func TestDrivePreviewValidationErrors(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	err := mountAndRunDrive(t, DrivePreview, []string{
		"+preview",
		"--file-token", "file_preview",
		"--as", "bot",
	}, f, nil)
	if err == nil || !strings.Contains(err.Error(), "either --list-only or --type is required") {
		t.Fatalf("unexpected missing type error: %v", err)
	}

	err = mountAndRunDrive(t, DrivePreview, []string{
		"+preview",
		"--file-token", "file_preview",
		"--list-only",
		"--type", "pdf",
		"--as", "bot",
	}, f, nil)
	if err == nil || !strings.Contains(err.Error(), "--type cannot be combined with --list-only") {
		t.Fatalf("unexpected list-only conflict: %v", err)
	}
}

func TestDrivePreviewNotReadyReturnsFailedPrecondition(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/file_preview/preview_result",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"preview_results": []map[string]interface{}{
					{"preview_type": 1, "preview_status": 1},
				},
			},
		},
	})

	err := mountAndRunDrive(t, DrivePreview, []string{
		"+preview",
		"--file-token", "file_preview",
		"--type", "image",
		"--output", "preview",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected not-ready error, got nil")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if validationErr.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("subtype=%q, want %q", validationErr.Subtype, errs.SubtypeFailedPrecondition)
	}
	if validationErr.Param != "--type" {
		t.Fatalf("param=%q, want --type", validationErr.Param)
	}
	if !strings.Contains(validationErr.Hint, "--list-only") {
		t.Fatalf("hint=%q, want list-only guidance", validationErr.Hint)
	}
}

func TestDriveCoverRejectsUnknownSpec(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	err := mountAndRunDrive(t, DriveCover, []string{
		"+cover",
		"--file-token", "file_cover",
		"--spec", "poster",
		"--output", "cover",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected invalid spec error, got nil")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if validationErr.Param != "--spec" {
		t.Fatalf("param=%q, want --spec", validationErr.Param)
	}
	if !strings.Contains(validationErr.Hint, "available cover specs") {
		t.Fatalf("hint=%q, want available specs", validationErr.Hint)
	}
}

func TestDrivePreviewCommonHelpers(t *testing.T) {
	if got := drivePreviewFallbackExt("pdf"); got != ".pdf" {
		t.Fatalf("fallbackExt(pdf)=%q, want .pdf", got)
	}
	if got := drivePreviewFallbackExt("source"); got != "" {
		t.Fatalf("fallbackExt(source)=%q, want empty", got)
	}
	specs := availableDriveCoverSpecs()
	if len(specs) == 0 || specs[len(specs)-1] != "square" {
		t.Fatalf("availableDriveCoverSpecs()=%v, want square included", specs)
	}

	header := http.Header{}
	header.Set("Content-Disposition", `attachment; filename="preview.pdf"`)
	resolution := drivePreviewExtensionByContentDisposition(header)
	if resolution == nil || resolution.Ext != ".pdf" {
		t.Fatalf("content disposition resolution=%+v, want .pdf", resolution)
	}

	path, fallback := autoAppendDrivePreviewExtension("cover", http.Header{}, ".png")
	if path != "cover.png" || fallback == nil || fallback.Source != "fallback" {
		t.Fatalf("fallback append = (%q, %+v), want cover.png with fallback source", path, fallback)
	}
}

func TestDrivePreviewMetadataAndPathResolution(t *testing.T) {
	candidate := drivePreviewCandidate{TypeCode: "999", StatusCode: "", Reason: ""}
	applyDrivePreviewTypeMeta(&candidate)
	applyDrivePreviewStatusMeta(&candidate)
	if candidate.Type != "unknown_999" {
		t.Fatalf("candidate.Type=%q, want unknown_999", candidate.Type)
	}
	if candidate.Reason != "Preview status is missing." {
		t.Fatalf("candidate.Reason=%q, want missing-status reason", candidate.Reason)
	}

	ready := drivePreviewCandidate{TypeCode: "1", StatusCode: "0"}
	applyDrivePreviewTypeMeta(&ready)
	applyDrivePreviewStatusMeta(&ready)
	if ready.Type != "png" || !ready.Downloadable {
		t.Fatalf("ready candidate=%+v, want downloadable png", ready)
	}

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.WriteFile(filepath.Join(tmpDir, "preview.pdf"), []byte("old"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	runtime := newDrivePreviewRuntime(t, "drive +preview", nil, nil)
	header := http.Header{}
	header.Set("Content-Type", "application/pdf")
	renamed, _, err := resolveDrivePreviewOutputPath(runtime, "preview", header, ".pdf", drivePreviewIfExistsRename)
	if err != nil {
		t.Fatalf("resolveDrivePreviewOutputPath(rename) error: %v", err)
	}
	if !strings.HasSuffix(renamed, "preview (1).pdf") {
		t.Fatalf("renamed=%q, want preview (1).pdf suffix", renamed)
	}

	_, _, err = resolveDrivePreviewOutputPath(runtime, "preview", header, ".pdf", "keep")
	if err == nil {
		t.Fatal("expected invalid if-exists error, got nil")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if validationErr.Param != "--if-exists" {
		t.Fatalf("param=%q, want --if-exists", validationErr.Param)
	}
}

type drivePreviewTestStringer string

func (s drivePreviewTestStringer) String() string { return string(s) }

func TestDrivePreviewScalarHelpers(t *testing.T) {
	got := firstString(map[string]interface{}{
		"blank":   "   ",
		"number":  float64(7),
		"flag":    true,
		"named":   drivePreviewTestStringer(" named "),
		"integer": int64(9),
	}, "blank", "named", "number")
	if got != "named" {
		t.Fatalf("firstString()=%q, want named", got)
	}

	if got := firstString(map[string]interface{}{"flag": true}, "flag"); got != "true" {
		t.Fatalf("firstString(bool)=%q, want true", got)
	}
	if got := firstString(map[string]interface{}{"integer": int64(9)}, "integer"); got != "9" {
		t.Fatalf("firstString(int64)=%q, want 9", got)
	}

	if got := versionString(" 42 "); got != "42" {
		t.Fatalf("versionString(string)=%q, want 42", got)
	}
	if got := versionString(float64(8)); got != "8" {
		t.Fatalf("versionString(float64)=%q, want 8", got)
	}
	if got := versionString(int64(11)); got != "11" {
		t.Fatalf("versionString(int64)=%q, want 11", got)
	}
	if got := versionString(struct{}{}); got != "" {
		t.Fatalf("versionString(struct)=%q, want empty", got)
	}
}

func TestDrivePreviewAliasAndAvailabilityHelpers(t *testing.T) {
	if got := normalizeDrivePreviewRequest(" Source File "); got != "source_file" {
		t.Fatalf("normalizeDrivePreviewRequest()=%q, want source_file", got)
	}

	aliases := previewAliasesForCandidate(drivePreviewCandidate{TypeCode: "1"})
	if len(aliases) == 0 || aliases[0] != "image" {
		t.Fatalf("previewAliasesForCandidate()=%v, want image alias", aliases)
	}
	if got := previewAliasesForCandidate(drivePreviewCandidate{TypeCode: "999"}); got != nil {
		t.Fatalf("previewAliasesForCandidate(unknown)=%v, want nil", got)
	}

	types := availableDrivePreviewTypes([]drivePreviewCandidate{
		{Type: "pdf"},
		{Type: "pdf"},
		{Type: " jpg "},
		{Type: ""},
	})
	if len(types) != 2 || types[0] != "pdf" || types[1] != "jpg" {
		t.Fatalf("availableDrivePreviewTypes()=%v, want [pdf jpg]", types)
	}
}

func TestDrivePreviewUnavailableHintAndContentTypeFallback(t *testing.T) {
	err := wrapDrivePreviewUnavailable("file_preview", "html", []drivePreviewCandidate{
		{Type: "pdf"},
		{Type: "jpg"},
	}, "")
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if !strings.Contains(validationErr.Hint, "available preview types: pdf, jpg") {
		t.Fatalf("hint=%q, want available preview types", validationErr.Hint)
	}

	err = wrapDrivePreviewUnavailable("file_preview", "html", nil, fmt.Sprintf("custom reason for %s", "html"))
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if !strings.Contains(validationErr.Hint, "--list-only") {
		t.Fatalf("hint=%q, want list-only guidance", validationErr.Hint)
	}

	resolution := drivePreviewExtensionByContentType("text/plain; charset=utf-8")
	if resolution == nil || resolution.Ext != ".txt" {
		t.Fatalf("drivePreviewExtensionByContentType()=%+v, want .txt", resolution)
	}
}
