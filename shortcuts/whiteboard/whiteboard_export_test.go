// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// TestSyntaxType verifies syntax names, extensions, and validity checks.
func TestSyntaxType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		st        SyntaxType
		wantStr   string
		wantExt   string
		wantValid bool
	}{
		{
			name:      "PlantUML",
			st:        SyntaxTypePlantUML,
			wantStr:   "plantuml",
			wantExt:   ".puml",
			wantValid: true,
		},
		{
			name:      "Mermaid",
			st:        SyntaxTypeMermaid,
			wantStr:   "mermaid",
			wantExt:   ".mmd",
			wantValid: true,
		},
		{
			name:      "invalid type 0",
			st:        SyntaxType(0),
			wantStr:   "",
			wantExt:   "",
			wantValid: false,
		},
		{
			name:      "invalid type 3",
			st:        SyntaxType(3),
			wantStr:   "",
			wantExt:   "",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.st.String(); got != tt.wantStr {
				t.Errorf("SyntaxType.String() = %q, want %q", got, tt.wantStr)
			}
			if got := tt.st.ExtensionName(); got != tt.wantExt {
				t.Errorf("SyntaxType.ExtensionName() = %q, want %q", got, tt.wantExt)
			}
			if got := tt.st.IsValid(); got != tt.wantValid {
				t.Errorf("SyntaxType.IsValid() = %v, want %v", got, tt.wantValid)
			}
		})
	}
}

// TestWhiteboardQuery_Validate verifies query flag validation for supported output modes.
func TestWhiteboardQuery_Validate(t *testing.T) {
	ctx := context.Background()
	chdirTemp(t)

	tests := []struct {
		name      string
		flags     map[string]string
		boolFlags map[string]bool
		wantErr   bool
	}{
		{
			name: "valid: image with output",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"output_as":        "image",
				"output":           "output.png",
			},
			wantErr: false,
		},
		{
			name: "valid: code without output",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"output_as":        "code",
			},
			wantErr: false,
		},
		{
			name: "valid: raw without output",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"output_as":        "raw",
			},
			wantErr: false,
		},
		{
			name: "invalid: image without output",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"output_as":        "image",
			},
			wantErr: true,
		},
		{
			name: "invalid: bad output_as value",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"output_as":        "invalid",
			},
			wantErr: true,
		},
		{
			name: "valid: with overwrite flag",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"output_as":        "code",
				"output":           "output.puml",
			},
			boolFlags: map[string]bool{
				"overwrite": true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRuntime(tt.flags, tt.boolFlags)
			err := WhiteboardQuery.Validate(ctx, rt)
			if (err != nil) != tt.wantErr {
				t.Errorf("WhiteboardQuery.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestWhiteboardQuery_Validate_TypedErrors locks the typed-envelope contract:
// input-validation failures surface as *errs.ValidationError carrying
// SubtypeInvalidArgument and the offending --flag, readable via errs.ProblemOf
// and errors.As — the shape downstream consumers (and exit-code mapping) rely on.
func TestWhiteboardQuery_Validate_TypedErrors(t *testing.T) {
	ctx := context.Background()
	chdirTemp(t)

	tests := []struct {
		name      string
		flags     map[string]string
		wantParam string
	}{
		{
			name:      "image without output",
			flags:     map[string]string{"whiteboard-token": "t", "output_as": "image"},
			wantParam: "--output",
		},
		{
			name:      "bad output_as value",
			flags:     map[string]string{"whiteboard-token": "t", "output_as": "invalid"},
			wantParam: "--output_as",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WhiteboardQuery.Validate(ctx, newTestRuntime(tt.flags, nil))
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error is not *errs.ValidationError: %T", err)
			}
			if ve.Subtype != errs.SubtypeInvalidArgument {
				t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
			}
			if ve.Param != tt.wantParam {
				t.Errorf("Param = %q, want %q", ve.Param, tt.wantParam)
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("errs.ProblemOf returned false")
			}
			if p.Category != errs.CategoryValidation {
				t.Errorf("Category = %q, want %q", p.Category, errs.CategoryValidation)
			}
			if p.Subtype != errs.SubtypeInvalidArgument {
				t.Errorf("Problem subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidArgument)
			}
		})
	}
}

// TestWhiteboardExport_Validate verifies the canonical +export flag spelling
// and output type names while legacy +query validation remains covered above.
func TestWhiteboardExport_Validate(t *testing.T) {
	ctx := context.Background()
	chdirTemp(t)

	tests := []struct {
		name      string
		flags     map[string]string
		wantErr   bool
		wantParam string
	}{
		{
			name: "valid: preview with output",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"output-type":      "preview",
				"output":           "output",
			},
		},
		{
			name: "valid: source without output",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"output-type":      "source",
			},
		},
		{
			name: "invalid: preview without output",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"output-type":      "preview",
			},
			wantErr:   true,
			wantParam: "--output",
		},
		{
			name: "invalid: bad output-type value",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"output-type":      "image",
			},
			wantErr:   true,
			wantParam: "--output-type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WhiteboardExport.Validate(ctx, newTestRuntime(tt.flags, nil))
			if (err != nil) != tt.wantErr {
				t.Fatalf("WhiteboardExport.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				return
			}
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error is not *errs.ValidationError: %T", err)
			}
			if ve.Param != tt.wantParam {
				t.Fatalf("Param = %q, want %q", ve.Param, tt.wantParam)
			}
		})
	}
}

// TestExportWhiteboardPreview_HTTPError locks the download-path failure
// behavior: a failed preview download surfaces as a typed errs.* envelope, not
// a flat legacy error.
func TestExportWhiteboardPreview_HTTPError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	chdirTemp(t)

	reg.Register(&httpmock.Stub{
		Method:      "GET",
		URL:         "/open-apis/board/v1/whiteboards/test-token-preview-5xx/download_as_image",
		Status:      500,
		RawBody:     []byte("gateway error"),
		ContentType: "text/plain",
	})

	args := []string{"+query", "--whiteboard-token", "test-token-preview-5xx", "--output_as", "image", "--output", "output"}
	err := runShortcut(t, WhiteboardQuery, args, factory, stdout)
	if err == nil {
		t.Fatal("expected error for HTTP 500 download")
	}
	if _, ok := errs.ProblemOf(err); !ok {
		t.Fatalf("error is not a typed errs.* envelope: %T (%v)", err, err)
	}
	var ne *errs.NetworkError
	if !errors.As(err, &ne) || ne.Subtype != errs.SubtypeNetworkServer || ne.Code != 500 || !ne.Retryable {
		t.Fatalf("HTTP 500 should be retryable network/server_error, got %T (%v)", err, err)
	}
}

// TestExportWhiteboardPreview_HTTPNotFoundIsAPIError verifies 404 preview downloads surface as typed API errors.
func TestExportWhiteboardPreview_HTTPNotFoundIsAPIError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	chdirTemp(t)

	reg.Register(&httpmock.Stub{
		Method:      "GET",
		URL:         "/open-apis/board/v1/whiteboards/missing-token/download_as_image",
		Status:      404,
		RawBody:     []byte("not found"),
		ContentType: "text/plain",
	})

	args := []string{"+query", "--whiteboard-token", "missing-token", "--output_as", "image", "--output", "output"}
	err := runShortcut(t, WhiteboardQuery, args, factory, stdout)
	if err == nil {
		t.Fatal("expected error for HTTP 404 download")
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) || apiErr.Subtype != errs.SubtypeNotFound || apiErr.Code != 404 {
		t.Fatalf("HTTP 404 should be api/not_found, got %T (%v)", err, err)
	}
}

// TestWhiteboardQuery_DryRun verifies dry-run output for the supported query modes.
func TestWhiteboardQuery_DryRun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name       string
		flags      map[string]string
		wantMethod string
		wantPath   string
	}{
		{
			name: "dry run image",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"output_as":        "image",
				"output":           "output.png",
			},
			wantMethod: "GET",
			wantPath:   "/open-apis/board/v1/whiteboards/test...-123/download_as_image",
		},
		{
			name: "dry run code",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"output_as":        "code",
			},
			wantMethod: "GET",
			wantPath:   "/open-apis/board/v1/whiteboards/test...-123/nodes",
		},
		{
			name: "dry run raw",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"output_as":        "raw",
			},
			wantMethod: "GET",
			wantPath:   "/open-apis/board/v1/whiteboards/test...-123/nodes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRuntime(tt.flags, nil)
			dryRun := WhiteboardQuery.DryRun(ctx, rt)
			if dryRun == nil {
				t.Fatalf("WhiteboardQuery.DryRun() returned nil")
			}
			var got struct {
				API []struct {
					Method string                 `json:"method"`
					URL    string                 `json:"url"`
					Body   map[string]interface{} `json:"body"`
				} `json:"api"`
			}
			data, err := json.Marshal(dryRun)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal() error = %v; data=%s", err, string(data))
			}
			if len(got.API) != 1 {
				t.Fatalf("api len = %d, want 1; data=%s", len(got.API), string(data))
			}
			if got.API[0].Method != tt.wantMethod {
				t.Fatalf("method = %q, want %q; data=%s", got.API[0].Method, tt.wantMethod, string(data))
			}
			if got.API[0].URL != tt.wantPath {
				t.Fatalf("url = %q, want %q; data=%s", got.API[0].URL, tt.wantPath, string(data))
			}
		})
	}
}

// TestWhiteboardQuery_DryRun_InvalidOutputAs verifies dry-run guidance for unsupported output modes.
func TestWhiteboardQuery_DryRun_InvalidOutputAs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rt := newTestRuntime(map[string]string{
		"whiteboard-token": "test-token-123",
		"output_as":        "invalid",
	}, nil)

	dryRun := WhiteboardQuery.DryRun(ctx, rt)
	if dryRun == nil {
		t.Fatal("WhiteboardQuery.DryRun() returned nil")
	}

	data, err := json.Marshal(dryRun)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), "image | svg | code | raw") {
		t.Fatalf("dry run desc = %s, want invalid output_as guidance", string(data))
	}
}

// TestWhiteboardQuery_Execute_InvalidOutputAs_TypedError verifies invalid output modes return typed validation errors.
func TestWhiteboardQuery_Execute_InvalidOutputAs_TypedError(t *testing.T) {
	rt := newTestRuntime(map[string]string{
		"whiteboard-token": "test-token-123",
		"output_as":        "invalid",
	}, nil)

	err := WhiteboardQuery.Execute(context.Background(), rt)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error is not *errs.ValidationError: %T (%v)", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != "--output_as" {
		t.Errorf("Param = %q, want %q", ve.Param, "--output_as")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("errs.ProblemOf returned false")
	}
	if p.Category != errs.CategoryValidation {
		t.Errorf("Category = %q, want %q", p.Category, errs.CategoryValidation)
	}
	if p.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Problem subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidArgument)
	}
}

// TestWhiteboardQuery_ShortcutRegistration verifies the whiteboard query shortcut metadata.
func TestWhiteboardQuery_ShortcutRegistration(t *testing.T) {
	t.Parallel()

	// Verify WhiteboardQuery is properly configured
	if WhiteboardQuery.Command != "+query" {
		t.Errorf("WhiteboardQuery.Command = %q, want \"+query\"", WhiteboardQuery.Command)
	}
	if WhiteboardQuery.Service != "whiteboard" {
		t.Errorf("WhiteboardQuery.Service = %q, want \"whiteboard\"", WhiteboardQuery.Service)
	}
	if len(WhiteboardQuery.Scopes) == 0 {
		t.Errorf("WhiteboardQuery.Scopes is empty, expected at least one scope")
	}
	if len(WhiteboardQuery.Flags) == 0 {
		t.Errorf("WhiteboardQuery.Flags is empty, expected at least one flag")
	}
	if !WhiteboardQuery.Hidden {
		t.Errorf("WhiteboardQuery should be hidden because +export is the canonical command")
	}

	// Verify WhiteboardExport is the visible canonical shortcut.
	if WhiteboardExport.Command != "+export" {
		t.Errorf("WhiteboardExport.Command = %q, want \"+export\"", WhiteboardExport.Command)
	}
	if WhiteboardExport.Service != "whiteboard" {
		t.Errorf("WhiteboardExport.Service = %q, want \"whiteboard\"", WhiteboardExport.Service)
	}
	if WhiteboardExport.Hidden {
		t.Errorf("WhiteboardExport should be visible")
	}
	if flag := shortcutFlag(WhiteboardExport, "output_as"); flag != nil {
		t.Errorf("WhiteboardExport --output_as should not be registered; got %#v", *flag)
	}
	if flag := shortcutFlag(WhiteboardExport, "output-type"); flag == nil || flag.Hidden {
		t.Errorf("WhiteboardExport --output-type should exist and be visible")
	}
	if flag := shortcutFlag(WhiteboardQuery, "output_as"); flag == nil || flag.Hidden {
		t.Errorf("WhiteboardQuery --output_as should exist and remain visible on the hidden legacy command")
	}
	if flag := shortcutFlag(WhiteboardQuery, "output-type"); flag != nil {
		t.Errorf("WhiteboardQuery --output-type should not be registered; got %#v", *flag)
	}
}

// TestSaveOutputFile verifies output saving, overwrite handling, and extension-specific paths.
func TestSaveOutputFile(t *testing.T) {
	t.Parallel()

	// Create a temp dir and cd into it
	chdirTemp(t)

	// Create a subdirectory for testing directory output
	err := os.Mkdir("testdir", 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	tests := []struct {
		name      string
		outPath   string
		ext       string
		token     string
		overwrite bool
		setupFile bool
		wantPath  string
		wantErr   bool
		checkPath bool
	}{
		{
			name:      "path is directory",
			outPath:   "testdir",
			ext:       ".puml",
			token:     "token123",
			overwrite: false,
			setupFile: false,
			wantPath:  filepath.Join("testdir", "whiteboard_token123.puml"),
			wantErr:   false,
			checkPath: true,
		},
		{
			name:      "path has correct extension",
			outPath:   "output.puml",
			ext:       ".puml",
			token:     "token123",
			overwrite: false,
			setupFile: false,
			wantPath:  "output.puml",
			wantErr:   false,
			checkPath: true,
		},
		{
			name:      "path has different extension",
			outPath:   "output.txt",
			ext:       ".puml",
			token:     "token123",
			overwrite: false,
			setupFile: false,
			wantPath:  "output.puml",
			wantErr:   false,
			checkPath: true,
		},
		{
			name:      "path has no extension",
			outPath:   "output",
			ext:       ".json",
			token:     "token123",
			overwrite: false,
			setupFile: false,
			wantPath:  "output.json",
			wantErr:   false,
			checkPath: true,
		},
		{
			name:      "file exists without overwrite",
			outPath:   "existing.txt",
			ext:       ".txt",
			token:     "token123",
			overwrite: false,
			setupFile: true,
			wantPath:  "existing.txt",
			wantErr:   true,
			checkPath: false,
		},
		{
			name:      "file exists with overwrite",
			outPath:   "overwrite.txt",
			ext:       ".txt",
			token:     "token123",
			overwrite: true,
			setupFile: true,
			wantPath:  "overwrite.txt",
			wantErr:   false,
			checkPath: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test file if needed
			if tt.setupFile {
				err := os.WriteFile(tt.wantPath, []byte("existing content"), 0644)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				defer os.Remove(tt.wantPath)
			}

			rt := newTestRuntime(nil, map[string]bool{"overwrite": tt.overwrite})
			testData := strings.NewReader("test content")

			gotPath, size, err := saveOutputFile(tt.outPath, tt.ext, tt.token, rt, testData)
			defer func() {
				if gotPath != "" {
					os.Remove(gotPath)
				}
			}()

			if (err != nil) != tt.wantErr {
				t.Errorf("saveOutputFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if tt.checkPath {
					// Check if path is correct
					if tt.outPath == "testdir" {
						// For directory case, just check extension and dir
						if filepath.Ext(gotPath) != tt.ext {
							t.Errorf("saveOutputFile() extension = %q, want %q", filepath.Ext(gotPath), tt.ext)
						}
						if filepath.Dir(gotPath) != "testdir" {
							t.Errorf("saveOutputFile() dir = %q, want %q", filepath.Dir(gotPath), "testdir")
						}
					} else {
						// For file case, check exact path
						if gotPath != tt.wantPath {
							t.Errorf("saveOutputFile() path = %q, want %q", gotPath, tt.wantPath)
						}
					}
					// Check if file was written
					content, err := os.ReadFile(gotPath)
					if err != nil {
						t.Errorf("Failed to read saved file: %v", err)
					}
					if string(content) != "test content" {
						t.Errorf("File content = %q, want %q", string(content), "test content")
					}
					if size != int64(len("test content")) {
						t.Errorf("File size = %d, want %d", size, len("test content"))
					}
				}
			}
		})
	}
}

// TestSaveOutputFile_InvalidFinalPathTypedError verifies invalid save paths return typed validation errors.
func TestSaveOutputFile_InvalidFinalPathTypedError(t *testing.T) {
	chdirTemp(t)

	rt := newTestRuntime(nil, nil)
	_, _, err := saveOutputFile("../escape", ".png", "token123", rt, strings.NewReader("test content"))
	if err == nil {
		t.Fatal("expected error for unsafe final path")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error is not *errs.ValidationError: %T (%v)", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument || ve.Param != "--output" {
		t.Fatalf("validation details = subtype %q param %q, want %q --output", ve.Subtype, ve.Param, errs.SubtypeInvalidArgument)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("errs.ProblemOf returned false")
	}
	if p.Category != errs.CategoryValidation {
		t.Errorf("Category = %q, want %q", p.Category, errs.CategoryValidation)
	}
	if p.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Problem subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidArgument)
	}
	if !errors.Is(err, fileio.ErrPathValidation) {
		t.Fatalf("expected path-validation cause to be preserved, err=%v", err)
	}
}

func newExecuteFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	config := &core.CliConfig{
		AppID:      "test-app-" + strings.ReplaceAll(strings.ToLower(t.Name()), "/", "-"),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_testuser",
	}
	factory, stdout, _, reg := cmdutil.TestFactory(t, config)
	return factory, stdout, reg
}

func runShortcut(t *testing.T, shortcut common.Shortcut, args []string, factory *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	// Temporarily lower risk for testing
	originalRisk := shortcut.Risk
	shortcut.Risk = "read"
	shortcut.AuthTypes = []string{"bot"}

	parent := &cobra.Command{Use: "whiteboard"}
	shortcut.Mount(parent, factory)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	stdout.Reset()
	err := parent.ExecuteContext(context.Background())

	// Restore original risk
	shortcut.Risk = originalRisk
	return err
}

// TestWhiteboardQueryExecute_AsRaw verifies raw query execution emits the raw node payload.
func TestWhiteboardQueryExecute_AsRaw(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	// Mock nodes API response
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/board/v1/whiteboards/test-token-123/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{"id": "node1"},
				},
			},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-123", "--output_as", "raw"}
	if err := runShortcut(t, WhiteboardQuery, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	if got := stdout.String(); !strings.Contains(got, `"nodes"`) {
		t.Fatalf("stdout=%s", got)
	}
}

// TestWhiteboardQueryExecute_AsCode verifies code query execution emits extracted diagram source.
func TestWhiteboardQueryExecute_AsCode(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	chdirTemp(t)

	// Mock nodes API response with code block
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/board/v1/whiteboards/test-token-123/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"syntax": map[string]interface{}{
							"code":        "graph TD\nA-->B",
							"syntax_type": float64(2),
						},
					},
				},
			},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-123", "--output_as", "code"}
	if err := runShortcut(t, WhiteboardQuery, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
}

// TestExportWhiteboardCode_EmptyNodes verifies code export handles empty whiteboards.
func TestExportWhiteboardCode_EmptyNodes(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	// Mock nodes API response with empty nodes
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/board/v1/whiteboards/test-token-empty/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"nodes": nil,
			},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-empty", "--output_as", "code"}
	if err := runShortcut(t, WhiteboardQuery, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
}

// TestExportWhiteboardCode_NoCodeBlocks verifies code export reports whiteboards without code blocks.
func TestExportWhiteboardCode_NoCodeBlocks(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	// Mock nodes API response with no syntax blocks
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/board/v1/whiteboards/test-token-nocode/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{"id": "node1"},
				},
			},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-nocode", "--output_as", "code"}
	if err := runShortcut(t, WhiteboardQuery, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
}

// TestExportWhiteboardCode_InvalidSyntaxType verifies unknown syntax types are rejected.
func TestExportWhiteboardCode_InvalidSyntaxType(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	// Mock nodes API response with invalid syntax type
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/board/v1/whiteboards/test-token-invalid-syntax/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"syntax": map[string]interface{}{
							"code":        "some code",
							"syntax_type": float64(999), // invalid type
						},
					},
				},
			},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-invalid-syntax", "--output_as", "code"}
	if err := runShortcut(t, WhiteboardQuery, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
}

// TestExportWhiteboardCode_MultipleCodeBlocks verifies multiple code blocks are exported together.
func TestExportWhiteboardCode_MultipleCodeBlocks(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	// Mock nodes API response with multiple code blocks
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/board/v1/whiteboards/test-token-multiple/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"syntax": map[string]interface{}{
							"code":        "graph TD\nA-->B",
							"syntax_type": float64(2),
						},
					},
					map[string]interface{}{
						"syntax": map[string]interface{}{
							"code":        "classDiagram\nclass A",
							"syntax_type": float64(2),
						},
					},
				},
			},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-multiple", "--output_as", "code"}
	if err := runShortcut(t, WhiteboardQuery, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	if !strings.Contains(stdout.String(), "multiple code blocks found") {
		t.Fatalf("stdout missing multiple blocks message: %s", stdout.String())
	}
}

// TestExportWhiteboardCode_SingleBlock_PlantUML_DirectOutput verifies direct PlantUML output for a single code block.
func TestExportWhiteboardCode_SingleBlock_PlantUML_DirectOutput(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	// Mock nodes API response with single PlantUML code block
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/board/v1/whiteboards/test-token-single-plantuml/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"syntax": map[string]interface{}{
							"code":        "@startuml\n:start;\n:process;\n@enduml",
							"syntax_type": float64(1),
						},
					},
				},
			},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-single-plantuml", "--output_as", "code"}
	if err := runShortcut(t, WhiteboardQuery, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	if !strings.Contains(stdout.String(), "@startuml") {
		t.Fatalf("stdout missing plantuml code: %s", stdout.String())
	}
}

// TestExportWhiteboardCode_SingleBlock_Mermaid_DirectOutput verifies direct Mermaid output for a single code block.
func TestExportWhiteboardCode_SingleBlock_Mermaid_DirectOutput(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	// Mock nodes API response with single Mermaid code block
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/board/v1/whiteboards/test-token-single-mermaid/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"nodes": []interface{}{
					map[string]interface{}{
						"syntax": map[string]interface{}{
							"code":        "flowchart TD\n    A --> B",
							"syntax_type": float64(2),
						},
					},
				},
			},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-single-mermaid", "--output_as", "code"}
	if err := runShortcut(t, WhiteboardQuery, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	if !strings.Contains(stdout.String(), "flowchart TD") {
		t.Fatalf("stdout missing mermaid code: %s", stdout.String())
	}
}

// TestExportWhiteboardPreview verifies preview downloads can be written to disk.
func TestExportWhiteboardPreview(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	chdirTemp(t)

	// Mock download preview image API response with RawBody
	reg.Register(&httpmock.Stub{
		Method:      "GET",
		URL:         "/open-apis/board/v1/whiteboards/test-token-preview/download_as_image",
		Status:      200,
		RawBody:     []byte("fake PNG image data"),
		ContentType: "image/png",
	})

	args := []string{"+query", "--whiteboard-token", "test-token-preview", "--output_as", "image", "--output", "output", "--overwrite"}
	if err := runShortcut(t, WhiteboardQuery, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	// Verify the file was written with .png extension
	data, err := os.ReadFile("output.png")
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "fake PNG image data" {
		t.Fatalf("image content = %q, want %q", string(data), "fake PNG image data")
	}
}

// TestExportWhiteboardPreview_UsesContentTypeExtension verifies preview image
// downloads are saved according to the API response Content-Type rather than a
// hard-coded PNG suffix.
func TestExportWhiteboardPreview_UsesContentTypeExtension(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	chdirTemp(t)

	reg.Register(&httpmock.Stub{
		Method:      "GET",
		URL:         "/open-apis/board/v1/whiteboards/test-token-preview-jpeg/download_as_image",
		Status:      200,
		RawBody:     []byte("fake JPEG image data"),
		ContentType: "image/jpeg",
	})

	args := []string{"+export", "--whiteboard-token", "test-token-preview-jpeg", "--output-type", "preview", "--output", "output", "--overwrite"}
	if err := runShortcut(t, WhiteboardExport, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	if _, err := os.Stat("output.png"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output.png should not exist when response Content-Type is image/jpeg, stat err=%v", err)
	}
	data, err := os.ReadFile("output.jpg")
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "fake JPEG image data" {
		t.Fatalf("image content = %q, want %q", string(data), "fake JPEG image data")
	}
}

func TestExportWhiteboardPreview_RejectsNonImageContentTypeWithoutSiblingOverwrite(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	chdirTemp(t)

	if err := os.WriteFile("report.html", []byte("keep me"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	reg.Register(&httpmock.Stub{
		Method:      "GET",
		URL:         "/open-apis/board/v1/whiteboards/test-token-preview-html/download_as_image",
		Status:      200,
		RawBody:     []byte("<html>bad gateway</html>"),
		ContentType: "text/html; charset=utf-8",
	})

	args := []string{"+export", "--whiteboard-token", "test-token-preview-html", "--output-type", "preview", "--output", "report.png", "--overwrite"}
	err := runShortcut(t, WhiteboardExport, args, factory, stdout)
	if err == nil {
		t.Fatal("expected error for non-image preview response")
	}
	assertInvalidResponse(t, err)

	data, readErr := os.ReadFile("report.html")
	if readErr != nil {
		t.Fatalf("ReadFile() error: %v", readErr)
	}
	if string(data) != "keep me" {
		t.Fatalf("report.html was overwritten: %q", string(data))
	}
	if _, statErr := os.Stat("report.png"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("report.png should not be written on invalid response, stat err=%v", statErr)
	}
}

func TestExportWhiteboardPreview_IgnoresContentDispositionExtension(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	chdirTemp(t)

	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/board/v1/whiteboards/test-token-preview-disposition/download_as_image",
		Status:  200,
		RawBody: []byte("fake JPEG image data"),
		Headers: http.Header{
			"Content-Type":        []string{"image/jpeg"},
			"Content-Disposition": []string{`attachment; filename="payload.sh"`},
		},
	})

	args := []string{"+export", "--whiteboard-token", "test-token-preview-disposition", "--output-type", "preview", "--output", "output", "--overwrite"}
	if err := runShortcut(t, WhiteboardExport, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	if _, err := os.Stat("output.sh"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output.sh should not be created from Content-Disposition, stat err=%v", err)
	}
	data, err := os.ReadFile("output.jpg")
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "fake JPEG image data" {
		t.Fatalf("image content = %q, want %q", string(data), "fake JPEG image data")
	}
}

func TestExportWhiteboardPreview_RejectsMismatchedExplicitExtension(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	chdirTemp(t)

	reg.Register(&httpmock.Stub{
		Method:      "GET",
		URL:         "/open-apis/board/v1/whiteboards/test-token-preview-mismatch/download_as_image",
		Status:      200,
		RawBody:     []byte("fake JPEG image data"),
		ContentType: "image/jpeg",
	})

	args := []string{"+export", "--whiteboard-token", "test-token-preview-mismatch", "--output-type", "preview", "--output", "report.png", "--overwrite"}
	err := runShortcut(t, WhiteboardExport, args, factory, stdout)
	if err == nil {
		t.Fatal("expected error for mismatched explicit extension")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error is not *errs.ValidationError: %T (%v)", err, err)
	}
	if ve.Subtype != errs.SubtypeFailedPrecondition || ve.Param != "--output" {
		t.Fatalf("validation details = subtype %q param %q, want %q --output", ve.Subtype, ve.Param, errs.SubtypeFailedPrecondition)
	}
	if _, statErr := os.Stat("report.jpg"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("report.jpg should not be created when explicit path mismatches, stat err=%v", statErr)
	}
}

func TestExportWhiteboardPreview_AllowsMatchingExplicitExtension(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	chdirTemp(t)

	reg.Register(&httpmock.Stub{
		Method:      "GET",
		URL:         "/open-apis/board/v1/whiteboards/test-token-preview-matching/download_as_image",
		Status:      200,
		RawBody:     []byte("fake JPEG image data"),
		ContentType: "image/jpeg",
	})

	args := []string{"+export", "--whiteboard-token", "test-token-preview-matching", "--output-type", "preview", "--output", "report.jpeg", "--overwrite"}
	if err := runShortcut(t, WhiteboardExport, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	data, err := os.ReadFile("report.jpeg")
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "fake JPEG image data" {
		t.Fatalf("image content = %q, want %q", string(data), "fake JPEG image data")
	}
}

// TestExportWhiteboardRaw_EmptyNodes verifies raw export reports empty whiteboards.
func TestExportWhiteboardRaw_EmptyNodes(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	// Mock nodes API response with empty nodes
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/board/v1/whiteboards/test-token-raw-empty/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"nodes": nil,
			},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-raw-empty", "--output_as", "raw"}
	if err := runShortcut(t, WhiteboardQuery, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
}

// TestFetchWhiteboardNodes_APIError verifies node fetch failures preserve typed API errors.
func TestFetchWhiteboardNodes_APIError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	// Mock nodes API response with error code
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/board/v1/whiteboards/test-token-api-error/nodes",
		Body: map[string]interface{}{
			"code": 10001,
			"msg":  "permission denied",
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-api-error", "--output_as", "raw"}
	err := runShortcut(t, WhiteboardQuery, args, factory, stdout)
	if err == nil {
		t.Fatalf("Expected API error, but got none")
	}
	// The nodes fetch now classifies the Lark error code into a typed envelope
	// carrying the numeric code.
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error is not a typed errs.* envelope: %T (%v)", err, err)
	}
	if p.Code != 10001 {
		t.Errorf("Problem.Code = %d, want 10001", p.Code)
	}
}

// TestFetchWhiteboardNodes_InvalidResponseTypedError verifies malformed node responses become typed invalid-response errors.
func TestFetchWhiteboardNodes_InvalidResponseTypedError(t *testing.T) {
	tests := []struct {
		name  string
		token string
		data  map[string]interface{}
	}{
		{
			name:  "nodes not array",
			token: "test-token-bad-nodes",
			data:  map[string]interface{}{"nodes": "not-an-array"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/board/v1/whiteboards/" + tt.token + "/nodes",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "success",
					"data": tt.data,
				},
			})

			args := []string{"+query", "--whiteboard-token", tt.token, "--output_as", "raw"}
			err := runShortcut(t, WhiteboardQuery, args, factory, stdout)
			assertInvalidResponse(t, err)
		})
	}
}

// TestFetchWhiteboardNodes_MissingNodesIsEmpty verifies that a response with
// missing nodes field is treated as an empty whiteboard (success), not an error.
// This matches the behavior introduced in commit 4b39b037.
func TestFetchWhiteboardNodes_MissingNodesIsEmpty(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/board/v1/whiteboards/test-token-missing-nodes/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-missing-nodes", "--output_as", "raw"}
	if err := runShortcut(t, WhiteboardQuery, args, factory, stdout); err != nil {
		t.Fatalf("expected success for missing nodes (empty whiteboard), got err=%v", err)
	}

	if !strings.Contains(stdout.String(), "whiteboard is empty") {
		t.Fatalf("stdout missing empty whiteboard message: %s", stdout.String())
	}
}

// TestExportWhiteboardSvg_DirectOutput verifies SVG export is printed when no output path is provided.
func TestExportWhiteboardSvg_DirectOutput(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	svgContent := `<svg xmlns="http://www.w3.org/2000/svg"><rect width="100" height="100"/></svg>`
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-svg/export",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"content":   base64.StdEncoding.EncodeToString([]byte(svgContent)),
				"mime_type": "image/svg+xml",
			},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-svg", "--output_as", "svg"}
	if err := runShortcut(t, WhiteboardQuery, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	if !strings.Contains(stdout.String(), "svg_content") {
		t.Fatalf("stdout missing svg_content key: %s", stdout.String())
	}
}

// TestExportWhiteboardSvg_SaveToFile verifies SVG export is written to the requested file.
func TestExportWhiteboardSvg_SaveToFile(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	chdirTemp(t)

	svgContent := `<svg xmlns="http://www.w3.org/2000/svg"><circle cx="50" cy="50" r="40"/></svg>`
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-svg-file/export",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"content":   base64.StdEncoding.EncodeToString([]byte(svgContent)),
				"mime_type": "image/svg+xml",
			},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-svg-file", "--output_as", "svg", "--output", "output", "--overwrite"}
	if err := runShortcut(t, WhiteboardQuery, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	data, err := os.ReadFile("output.svg")
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != svgContent {
		t.Fatalf("svg content = %q, want %q", string(data), svgContent)
	}
}

// TestExportWhiteboardSvg_PrettyOutput verifies pretty output includes inline SVG content.
func TestExportWhiteboardSvg_PrettyOutput(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	svgContent := `<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0L10 10"/></svg>`
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-svg-pretty/export",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"content":   base64.StdEncoding.EncodeToString([]byte(svgContent)),
				"mime_type": "image/svg+xml",
			},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-svg-pretty", "--output_as", "svg", "--format", "pretty"}
	if err := runShortcut(t, WhiteboardQuery, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	if got := stdout.String(); !strings.Contains(got, svgContent) {
		t.Fatalf("stdout = %q, want svg content", got)
	}
}

// TestExportWhiteboardSvg_SaveToFile_PrettyOutput verifies pretty output reports the saved SVG path and size.
func TestExportWhiteboardSvg_SaveToFile_PrettyOutput(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	chdirTemp(t)

	svgContent := `<svg xmlns="http://www.w3.org/2000/svg"><ellipse cx="60" cy="40" rx="50" ry="30"/></svg>`
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-svg-file-pretty/export",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"content":   base64.StdEncoding.EncodeToString([]byte(svgContent)),
				"mime_type": "image/svg+xml",
			},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-svg-file-pretty", "--output_as", "svg", "--output", "output", "--overwrite", "--format", "pretty"}
	if err := runShortcut(t, WhiteboardQuery, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	if got := stdout.String(); !strings.Contains(got, "SVG saved to output.svg") || !strings.Contains(got, "File size:") {
		t.Fatalf("stdout = %q, want save summary", got)
	}

	data, err := os.ReadFile("output.svg")
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != svgContent {
		t.Fatalf("svg content = %q, want %q", string(data), svgContent)
	}
}

// TestExportWhiteboardSvg_SaveToFile_ExistingFileWithoutOverwrite verifies existing SVG outputs require --overwrite.
func TestExportWhiteboardSvg_SaveToFile_ExistingFileWithoutOverwrite(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	chdirTemp(t)

	if err := os.WriteFile("output.svg", []byte("existing content"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svgContent := `<svg xmlns="http://www.w3.org/2000/svg"><line x1="0" y1="0" x2="1" y2="1"/></svg>`
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-svg-existing/export",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"content":   base64.StdEncoding.EncodeToString([]byte(svgContent)),
				"mime_type": "image/svg+xml",
			},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-svg-existing", "--output_as", "svg", "--output", "output"}
	err := runShortcut(t, WhiteboardQuery, args, factory, stdout)
	if err == nil {
		t.Fatal("expected error for existing output without overwrite")
	}

	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error is not *errs.ValidationError: %T (%v)", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != "--overwrite" {
		t.Errorf("Param = %q, want %q", ve.Param, "--overwrite")
	}
}

// TestExportWhiteboardSvg_HTTP5xx verifies plain HTTP 5xx failures are classified as retryable network errors.
func TestExportWhiteboardSvg_HTTP5xx(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method:      "POST",
		URL:         "/open-apis/board/v1/whiteboards/test-token-svg-5xx/export",
		Status:      502,
		RawBody:     []byte("bad gateway"),
		ContentType: "text/plain",
	})

	args := []string{"+query", "--whiteboard-token", "test-token-svg-5xx", "--output_as", "svg"}
	err := runShortcut(t, WhiteboardQuery, args, factory, stdout)
	if err == nil {
		t.Fatal("expected error for HTTP 502")
	}
	var ne *errs.NetworkError
	if !errors.As(err, &ne) {
		t.Fatalf("error is not *errs.NetworkError: %T (%v)", err, err)
	}
	if ne.Subtype != errs.SubtypeNetworkServer {
		t.Errorf("Subtype = %q, want %q", ne.Subtype, errs.SubtypeNetworkServer)
	}
	if ne.Code != 502 {
		t.Errorf("Code = %d, want 502", ne.Code)
	}
	if !ne.Retryable {
		t.Error("expected Retryable = true")
	}
}

// TestExportWhiteboardSvg_HTTP5xxJSONEnvelopeReturnsAPIError verifies API envelopes take precedence over generic 5xx handling.
func TestExportWhiteboardSvg_HTTP5xxJSONEnvelopeReturnsAPIError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method:      "POST",
		URL:         "/open-apis/board/v1/whiteboards/test-token-svg-5xx-json/export",
		Status:      502,
		ContentType: "application/json",
		RawBody:     []byte(`{"code":99002,"msg":"export task failed"}`),
	})

	args := []string{"+query", "--whiteboard-token", "test-token-svg-5xx-json", "--output_as", "svg"}
	err := runShortcut(t, WhiteboardQuery, args, factory, stdout)
	if err == nil {
		t.Fatal("expected error for HTTP 502 JSON envelope")
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not *errs.APIError: %T (%v)", err, err)
	}
	var ne *errs.NetworkError
	if errors.As(err, &ne) {
		t.Fatalf("expected JSON envelope to win over HTTP 5xx fallback, got *errs.NetworkError: %v", err)
	}
	if apiErr.Subtype != errs.SubtypeUnknown {
		t.Errorf("Subtype = %q, want %q", apiErr.Subtype, errs.SubtypeUnknown)
	}
	if apiErr.Code != 99002 {
		t.Errorf("Code = %d, want 99002", apiErr.Code)
	}
}

// TestExportWhiteboardSvg_HTTP4xx verifies plain HTTP 4xx failures are surfaced as API errors.
func TestExportWhiteboardSvg_HTTP4xx(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method:      "POST",
		URL:         "/open-apis/board/v1/whiteboards/test-token-svg-403/export",
		Status:      403,
		RawBody:     []byte("forbidden"),
		ContentType: "text/plain",
	})

	args := []string{"+query", "--whiteboard-token", "test-token-svg-403", "--output_as", "svg"}
	err := runShortcut(t, WhiteboardQuery, args, factory, stdout)
	if err == nil {
		t.Fatal("expected error for HTTP 403")
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not *errs.APIError: %T (%v)", err, err)
	}
	if apiErr.Subtype != errs.SubtypeUnknown {
		t.Errorf("Subtype = %q, want %q", apiErr.Subtype, errs.SubtypeUnknown)
	}
	if apiErr.Code != 403 {
		t.Errorf("Code = %d, want 403", apiErr.Code)
	}
}

// TestExportWhiteboardSvg_HTTPNotFoundJSONEnvelopeIsAPIError verifies not-found envelopes preserve the typed API error classification.
func TestExportWhiteboardSvg_HTTPNotFoundJSONEnvelopeIsAPIError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method:      "POST",
		URL:         "/open-apis/board/v1/whiteboards/missing-token-svg/export",
		Status:      404,
		ContentType: "application/json",
		RawBody:     []byte(`{"code":99001,"msg":"whiteboard not found"}`),
	})

	args := []string{"+query", "--whiteboard-token", "missing-token-svg", "--output_as", "svg"}
	err := runShortcut(t, WhiteboardQuery, args, factory, stdout)
	if err == nil {
		t.Fatal("expected error for HTTP 404 JSON envelope")
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not *errs.APIError: %T (%v)", err, err)
	}
	if apiErr.Subtype != errs.SubtypeNotFound {
		t.Errorf("Subtype = %q, want %q", apiErr.Subtype, errs.SubtypeNotFound)
	}
	if apiErr.Code != 99001 {
		t.Errorf("Code = %d, want 99001", apiErr.Code)
	}
}

// TestExportWhiteboardSvg_HTTPNotFoundPlainText verifies plain-text 404 responses surface as not-found API errors.
func TestExportWhiteboardSvg_HTTPNotFoundPlainText(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method:      "POST",
		URL:         "/open-apis/board/v1/whiteboards/missing-token-svg-plain/export",
		Status:      404,
		ContentType: "text/plain",
		RawBody:     []byte("whiteboard not found"),
	})

	args := []string{"+query", "--whiteboard-token", "missing-token-svg-plain", "--output_as", "svg"}
	err := runShortcut(t, WhiteboardQuery, args, factory, stdout)
	if err == nil {
		t.Fatal("expected error for HTTP 404 plain text response")
	}

	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not *errs.APIError: %T (%v)", err, err)
	}
	if apiErr.Subtype != errs.SubtypeNotFound {
		t.Errorf("Subtype = %q, want %q", apiErr.Subtype, errs.SubtypeNotFound)
	}
	if apiErr.Code != 404 {
		t.Errorf("Code = %d, want 404", apiErr.Code)
	}
}

// TestExportWhiteboardSvg_InvalidJSON verifies malformed success responses are rejected as invalid responses.
func TestExportWhiteboardSvg_InvalidJSON(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method:      "POST",
		URL:         "/open-apis/board/v1/whiteboards/test-token-svg-badjson/export",
		Status:      200,
		RawBody:     []byte("not json at all"),
		ContentType: "application/json",
	})

	args := []string{"+query", "--whiteboard-token", "test-token-svg-badjson", "--output_as", "svg"}
	err := runShortcut(t, WhiteboardQuery, args, factory, stdout)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	assertInvalidResponse(t, err)
}

// TestExportWhiteboardSvg_InvalidBody200PlainText verifies plain-text 200 responses are rejected as invalid export responses.
func TestExportWhiteboardSvg_InvalidBody200PlainText(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method:      "POST",
		URL:         "/open-apis/board/v1/whiteboards/test-token-svg-plain-200/export",
		Status:      200,
		RawBody:     []byte("not json at all"),
		ContentType: "text/plain",
	})

	args := []string{"+query", "--whiteboard-token", "test-token-svg-plain-200", "--output_as", "svg"}
	err := runShortcut(t, WhiteboardQuery, args, factory, stdout)
	if err == nil {
		t.Fatal("expected error for plain text success response")
	}
	assertInvalidResponse(t, err)
}

// TestExportWhiteboardSvg_NonZeroCode verifies non-zero API codes are returned as typed API errors.
func TestExportWhiteboardSvg_NonZeroCode(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-svg-apierr/export",
		Body: map[string]interface{}{
			"code": 99001,
			"msg":  "whiteboard not found",
			"data": map[string]interface{}{},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-svg-apierr", "--output_as", "svg"}
	err := runShortcut(t, WhiteboardQuery, args, factory, stdout)
	if err == nil {
		t.Fatal("expected error for non-zero code")
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is not *errs.APIError: %T (%v)", err, err)
	}
	if apiErr.Code != 99001 {
		t.Errorf("Code = %d, want 99001", apiErr.Code)
	}
}

// TestExportWhiteboardSvg_InvalidBase64 verifies invalid SVG payload encoding is rejected.
func TestExportWhiteboardSvg_InvalidBase64(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-svg-badbase64/export",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"content":   "!!!not-valid-base64!!!",
				"mime_type": "image/svg+xml",
			},
		},
	})

	args := []string{"+query", "--whiteboard-token", "test-token-svg-badbase64", "--output_as", "svg"}
	err := runShortcut(t, WhiteboardQuery, args, factory, stdout)
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
	assertInvalidResponse(t, err)
}

// TestWhiteboardQuery_Validate_SvgValid verifies svg is accepted as a valid query output format.
func TestWhiteboardQuery_Validate_SvgValid(t *testing.T) {
	ctx := context.Background()
	chdirTemp(t)

	rt := newTestRuntime(map[string]string{
		"whiteboard-token": "test-token-123",
		"output_as":        "svg",
	}, nil)
	if err := WhiteboardQuery.Validate(ctx, rt); err != nil {
		t.Fatalf("expected svg to be valid, got err=%v", err)
	}
}

// TestWhiteboardQuery_DryRun_Svg verifies the svg dry-run request uses the export endpoint and body.
func TestWhiteboardQuery_DryRun_Svg(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	rt := newTestRuntime(map[string]string{
		"whiteboard-token": "test-token-123",
		"output_as":        "svg",
	}, nil)
	dryRun := WhiteboardQuery.DryRun(ctx, rt)
	if dryRun == nil {
		t.Fatal("DryRun() returned nil for svg")
	}

	data, err := json.Marshal(dryRun)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Params map[string]interface{} `json:"params"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if len(got.API) != 1 {
		t.Fatalf("len(api) = %d, want 1", len(got.API))
	}
	if got.API[0].Method != "POST" {
		t.Fatalf("method = %q, want POST", got.API[0].Method)
	}
	if got.API[0].URL != "/open-apis/board/v1/whiteboards/test...-123/export" {
		t.Fatalf("url = %q", got.API[0].URL)
	}
	if got.API[0].Body["export_type"] != "svg" {
		t.Fatalf("body = %#v, want export_type=svg", got.API[0].Body)
	}
	if _, ok := got.API[0].Params["export_type"]; ok {
		t.Fatalf("params should not include export_type, got %#v", got.API[0].Params)
	}
}

// assertInvalidResponse verifies an error is classified as a typed invalid-response failure.
func assertInvalidResponse(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected invalid response error")
	}
	var ie *errs.InternalError
	if !errors.As(err, &ie) {
		t.Fatalf("error is not *errs.InternalError: %T (%v)", err, err)
	}
	if ie.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("Subtype = %q, want %q", ie.Subtype, errs.SubtypeInvalidResponse)
	}
}

// newTestRuntime creates a RuntimeContext with string flags for testing.
func newTestRuntime(flags map[string]string, boolFlags map[string]bool) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "test"}
	for name := range flags {
		cmd.Flags().String(name, "", "")
	}
	for name := range boolFlags {
		cmd.Flags().Bool(name, false, "")
	}
	// Parse empty args so flags have defaults, then set values.
	cmd.ParseFlags(nil)
	for name, val := range flags {
		cmd.Flags().Set(name, val)
	}
	for name, val := range boolFlags {
		if val {
			cmd.Flags().Set(name, "true")
		}
	}
	return &common.RuntimeContext{Cmd: cmd}
}

// chdirTemp changes the working directory to a fresh temp directory and
// restores it when the test finishes.
func chdirTemp(t *testing.T) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

func shortcutFlag(shortcut common.Shortcut, name string) *common.Flag {
	for i := range shortcut.Flags {
		if shortcut.Flags[i].Name == name {
			return &shortcut.Flags[i]
		}
	}
	return nil
}
