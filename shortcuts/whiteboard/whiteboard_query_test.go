// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

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
			wantPath:   "/open-apis/board/v1/whiteboards/test-token-123/download_as_image",
		},
		{
			name: "dry run code",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"output_as":        "code",
			},
			wantMethod: "GET",
			wantPath:   "/open-apis/board/v1/whiteboards/test-token-123/nodes",
		},
		{
			name: "dry run raw",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"output_as":        "raw",
			},
			wantMethod: "GET",
			wantPath:   "/open-apis/board/v1/whiteboards/test-token-123/nodes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRuntime(tt.flags, nil)
			dryRun := WhiteboardQuery.DryRun(ctx, rt)
			if dryRun == nil {
				t.Fatalf("WhiteboardQuery.DryRun() returned nil")
			}
		})
	}
}

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
}

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

func TestExportWhiteboardPreview(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)

	chdirTemp(t)

	// Mock download preview image API response with RawBody
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/board/v1/whiteboards/test-token-preview/download_as_image",
		Status:  200,
		RawBody: []byte("fake PNG image data"),
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
	// We expect an error here, but don't fail the test because it's testing error path
	if err == nil {
		t.Fatalf("Expected API error, but got none")
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
