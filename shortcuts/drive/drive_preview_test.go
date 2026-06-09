// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// TestDrivePreviewListOnlyNormalizesCandidates verifies list mode output is
// normalized from preview_result payloads.
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

// TestDrivePreviewDownloadUsesResolvedTypeCodeAndRenamePolicy verifies preview
// downloads use the resolved type and rename collision handling.
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

// TestDrivePreviewRejectsUnavailableType verifies unavailable preview types
// return an actionable validation error.
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

// TestSelectDrivePreviewCandidatePrefersDownloadableAliasMatch verifies alias
// selection prefers a downloadable candidate over an earlier unavailable one.
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

// TestDriveCoverListOnlyUsesStaticSpecs verifies cover list mode returns the
// built-in spec catalog without calling APIs.
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

// TestDriveCoverDownloadUsesMappedCoverOptionAndPreviewType verifies cover
// downloads send the expected preview_download query mapping.
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

// TestDriveCoverDownload404ReturnsFailedPrecondition verifies the +cover path
// reclassifies preview_download HTTP 404 as a non-retryable spec/state issue.
func TestDriveCoverDownload404ReturnsFailedPrecondition(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/medias/file_cover/preview_download",
		Status: http.StatusNotFound,
		Body:   []byte(`{"code":404,"msg":"no artifact"}`),
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
		},
	})

	err := mountAndRunDrive(t, DriveCover, []string{
		"+cover",
		"--file-token", "file_cover",
		"--spec", "square",
		"--output", "cover",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected cover 404 error, got nil")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if validationErr.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("subtype=%q, want %q", validationErr.Subtype, errs.SubtypeFailedPrecondition)
	}
	if validationErr.Param != "--spec" {
		t.Fatalf("param=%q, want --spec", validationErr.Param)
	}
	if validationErr.Code != http.StatusNotFound {
		t.Fatalf("code=%d, want %d", validationErr.Code, http.StatusNotFound)
	}
	if !strings.Contains(validationErr.Hint, "--list-only") {
		t.Fatalf("hint=%q, want --list-only guidance", validationErr.Hint)
	}
	if !strings.Contains(validationErr.Hint, "file token/version is invalid") {
		t.Fatalf("hint=%q, want invalid file token/version guidance", validationErr.Hint)
	}
	if !strings.Contains(validationErr.Hint, "available cover specs") && !strings.Contains(validationErr.Hint, "default, icon, grid") {
		t.Fatalf("hint=%q, want available cover specs guidance", validationErr.Hint)
	}
	if !strings.Contains(validationErr.Error(), `preview_download returned HTTP 404 for --spec "square"`) {
		t.Fatalf("message=%q, want neutral 404 message", validationErr.Error())
	}
}

// newDrivePreviewRuntime builds a shortcut runtime with preconfigured preview
// and cover flags for DryRun and helper tests.
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

// decodeDryRunOutput marshals a DryRunAPI helper into a generic map for test
// assertions.
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

// TestDrivePreviewDryRunIncludesVersionAndMode verifies preview DryRun records
// versioned request metadata in download mode.
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
	if len(api) != 2 {
		t.Fatalf("len(api)=%d, want 2", len(api))
	}
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
	downloadCall, _ := api[1].(map[string]interface{})
	if got := downloadCall["method"]; got != "GET" {
		t.Fatalf("download method=%v, want GET", got)
	}
	if got := downloadCall["url"]; got != "/open-apis/drive/v1/medias/file_preview/preview_download" {
		t.Fatalf("download url=%v, want preview_download", got)
	}
	params, _ := downloadCall["params"].(map[string]interface{})
	if got := params["preview_type"]; got != "<selected type_code from preview_result>" {
		t.Fatalf("download params.preview_type=%v, want placeholder", got)
	}
	if got := params["version"]; got != "7" {
		t.Fatalf("download params.version=%v, want 7", got)
	}
}

// TestDrivePreviewDryRunListOmitsBodyWithoutVersion verifies list-mode DryRun
// omits the request body when no version is supplied.
func TestDrivePreviewDryRunListOmitsBodyWithoutVersion(t *testing.T) {
	runtime := newDrivePreviewRuntime(t, "drive +preview", map[string]string{
		"file-token": "file_preview",
	}, map[string]bool{"list-only": true})

	data := decodeDryRunOutput(t, DrivePreview.DryRun(context.Background(), runtime))
	if got := data["mode"]; got != "list" {
		t.Fatalf("mode=%v, want list", got)
	}
	api, _ := data["api"].([]interface{})
	call, _ := api[0].(map[string]interface{})
	if _, ok := call["body"]; ok {
		t.Fatalf("dry-run body should be omitted when version is empty: %#v", call)
	}
}

// TestDrivePreviewDryRunDownloadWithoutVersionShowsResolvedVersion verifies
// download-mode DryRun documents the second preview_download step even when the
// final version is only known after preview_result resolves candidates.
func TestDrivePreviewDryRunDownloadWithoutVersionShowsResolvedVersion(t *testing.T) {
	runtime := newDrivePreviewRuntime(t, "drive +preview", map[string]string{
		"file-token": "file_preview",
		"type":       "pdf",
		"output":     "preview",
	}, nil)

	data := decodeDryRunOutput(t, DrivePreview.DryRun(context.Background(), runtime))
	api, _ := data["api"].([]interface{})
	if len(api) != 2 {
		t.Fatalf("len(api)=%d, want 2", len(api))
	}
	downloadCall, _ := api[1].(map[string]interface{})
	params, _ := downloadCall["params"].(map[string]interface{})
	if got := params["version"]; got != "<resolved version from preview_result>" {
		t.Fatalf("download params.version=%v, want resolved-version placeholder", got)
	}
}

// TestDriveCoverDryRunListAndDownload verifies cover DryRun output for both
// list and download modes.
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

// TestDriveCoverDryRunDefaultSpecIncludesVersionAndPlatform verifies DryRun
// params include version and built-in platform metadata for default covers.
func TestDriveCoverDryRunDefaultSpecIncludesVersionAndPlatform(t *testing.T) {
	runtime := newDrivePreviewRuntime(t, "drive +cover", map[string]string{
		"file-token": "file_cover",
		"spec":       "default",
		"version":    "5",
		"output":     "cover",
	}, nil)

	data := decodeDryRunOutput(t, DriveCover.DryRun(context.Background(), runtime))
	api, _ := data["api"].([]interface{})
	call, _ := api[0].(map[string]interface{})
	params, _ := call["params"].(map[string]interface{})
	if got := params["bus_type"]; got != "cover" {
		t.Fatalf("params.bus_type=%v, want cover", got)
	}
	if got := params["platform"]; got != "pc" {
		t.Fatalf("params.platform=%v, want pc", got)
	}
	if got := params["version"]; got != "5" {
		t.Fatalf("params.version=%v, want 5", got)
	}
}

// TestDrivePreviewValidationErrors verifies preview flag validation rejects
// incomplete and conflicting argument combinations.
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

	err = mountAndRunDrive(t, DrivePreview, []string{
		"+preview",
		"--file-token", "file_preview",
		"--type", "pdf",
		"--as", "bot",
	}, f, nil)
	if err == nil || !strings.Contains(err.Error(), "--output is required when --type is set") {
		t.Fatalf("unexpected missing output error: %v", err)
	}
}

// TestDrivePreviewNotReadyReturnsFailedPrecondition verifies a known but
// unready preview candidate returns a failed-precondition error.
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

// TestDriveCoverRejectsUnknownSpec verifies unsupported cover specs produce a
// validation error with available alternatives.
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

// TestDriveCoverValidationErrors verifies cover flag validation rejects
// incomplete and conflicting argument combinations.
func TestDriveCoverValidationErrors(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	err := mountAndRunDrive(t, DriveCover, []string{
		"+cover",
		"--file-token", "file_cover",
		"--spec", "default",
		"--as", "bot",
	}, f, nil)
	if err == nil || !strings.Contains(err.Error(), "--output is required when --spec is set") {
		t.Fatalf("unexpected missing output error: %v", err)
	}

	err = mountAndRunDrive(t, DriveCover, []string{
		"+cover",
		"--file-token", "file_cover",
		"--list-only",
		"--spec", "default",
		"--as", "bot",
	}, f, nil)
	if err == nil || !strings.Contains(err.Error(), "--spec cannot be combined with --list-only") {
		t.Fatalf("unexpected list-only conflict: %v", err)
	}
}

// TestDrivePreviewCommonHelpers exercises helper branches for extension
// inference and fallback extension mapping.
func TestDrivePreviewCommonHelpers(t *testing.T) {
	if got := drivePreviewFallbackExt("pdf"); got != ".pdf" {
		t.Fatalf("fallbackExt(pdf)=%q, want .pdf", got)
	}
	if got := drivePreviewFallbackExt("html"); got != ".html" {
		t.Fatalf("fallbackExt(html)=%q, want .html", got)
	}
	if got := drivePreviewFallbackExt("text"); got != ".txt" {
		t.Fatalf("fallbackExt(text)=%q, want .txt", got)
	}
	if got := drivePreviewFallbackExt("jpg"); got != ".jpg" {
		t.Fatalf("fallbackExt(jpg)=%q, want .jpg", got)
	}
	if got := drivePreviewFallbackExt("jpg_lin"); got != ".jpg" {
		t.Fatalf("fallbackExt(jpg_lin)=%q, want .jpg", got)
	}
	if got := drivePreviewFallbackExt("split_png"); got != ".png" {
		t.Fatalf("fallbackExt(split_png)=%q, want .png", got)
	}
	if got := drivePreviewFallbackExt("source"); got != "" {
		t.Fatalf("fallbackExt(source)=%q, want empty", got)
	}
	if got := drivePreviewFallbackExt("unknown"); got != "" {
		t.Fatalf("fallbackExt(unknown)=%q, want empty", got)
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
	header.Set("Content-Disposition", `attachment; filename="preview"`)
	if resolution := drivePreviewExtensionByContentDisposition(header); resolution != nil {
		t.Fatalf("content disposition without ext should be nil: %+v", resolution)
	}

	path, fallback := autoAppendDrivePreviewExtension("cover", http.Header{}, ".png")
	if path != "cover.png" || fallback == nil || fallback.Source != "fallback" {
		t.Fatalf("fallback append = (%q, %+v), want cover.png with fallback source", path, fallback)
	}
	path, fallback = autoAppendDrivePreviewExtension("cover.", http.Header{}, ".png")
	if path != "cover.png" || fallback == nil {
		t.Fatalf("trailing-dot append = (%q, %+v), want cover.png", path, fallback)
	}
	path, fallback = autoAppendDrivePreviewExtension("cover.pdf", http.Header{}, ".png")
	if path != "cover.pdf" || fallback != nil {
		t.Fatalf("explicit ext append = (%q, %+v), want unchanged path", path, fallback)
	}
}

// TestDrivePreviewMetadataAndPathResolution verifies metadata normalization
// and output path resolution helpers across rename and overwrite flows.
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

	unusedPath, err := nextAvailableDrivePreviewPath(runtime.FileIO(), "fresh.pdf")
	if err != nil {
		t.Fatalf("nextAvailableDrivePreviewPath(unused) error: %v", err)
	}
	if unusedPath != "fresh.pdf" {
		t.Fatalf("unusedPath=%q, want fresh.pdf", unusedPath)
	}

	overwritten, _, err := resolveDrivePreviewOutputPath(runtime, "preview.pdf", header, ".pdf", drivePreviewIfExistsOverwrite)
	if err != nil {
		t.Fatalf("resolveDrivePreviewOutputPath(overwrite) error: %v", err)
	}
	if !strings.HasSuffix(overwritten, "preview.pdf") {
		t.Fatalf("overwritten=%q, want preview.pdf suffix", overwritten)
	}

	f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	f.FileIOProvider = &statErrorProvider{inner: f.FileIOProvider, err: fs.ErrPermission}
	runtimeWithStatErr := newDrivePreviewRuntime(t, "drive +preview", nil, nil)
	runtimeWithStatErr.Factory = f
	_, _, err = resolveDrivePreviewOutputPath(runtimeWithStatErr, "blocked.pdf", header, ".pdf", drivePreviewIfExistsError)
	if err == nil {
		t.Fatal("expected stat permission error, got nil")
	}
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("expected *errs.InternalError, got %T: %v", err, err)
	}
	if internalErr.Subtype != errs.SubtypeFileIO {
		t.Fatalf("Subtype=%q, want %q", internalErr.Subtype, errs.SubtypeFileIO)
	}
}

type drivePreviewTestStringer string

type statErrorProvider struct {
	inner fileio.Provider
	err   error
}

func (p *statErrorProvider) Name() string { return "stat-error" }

func (p *statErrorProvider) ResolveFileIO(ctx context.Context) fileio.FileIO {
	return &statErrorFileIO{inner: p.inner.ResolveFileIO(ctx), err: p.err}
}

type statErrorFileIO struct {
	inner fileio.FileIO
	err   error
}

func (f *statErrorFileIO) Open(name string) (fileio.File, error) { return f.inner.Open(name) }

func (f *statErrorFileIO) Stat(string) (fileio.FileInfo, error) { return nil, f.err }

func (f *statErrorFileIO) ResolvePath(path string) (string, error) { return f.inner.ResolvePath(path) }

func (f *statErrorFileIO) Save(path string, opts fileio.SaveOptions, body io.Reader) (fileio.SaveResult, error) {
	return f.inner.Save(path, opts, body)
}

// String implements fmt.Stringer for scalar helper tests.
func (s drivePreviewTestStringer) String() string { return string(s) }

// TestDrivePreviewScalarHelpers verifies scalar coercion helpers normalize
// mixed API field types into strings.
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

// TestDrivePreviewAliasAndAvailabilityHelpers verifies alias lookup,
// normalization, and available-type de-duplication helpers.
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

// TestDrivePreviewUnavailableHintAndContentTypeFallback verifies unavailable
// preview errors and content-type fallback extension inference.
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
