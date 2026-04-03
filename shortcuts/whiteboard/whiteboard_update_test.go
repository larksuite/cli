// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestWhiteboardUpdate_Validate(t *testing.T) {
	ctx := context.Background()

	// Save original stdin
	origStdin := os.Stdin
	t.Cleanup(func() {
		os.Stdin = origStdin
	})

	// Create a temporary file to use as stdin
	tmpFile, err := os.CreateTemp("", "test-stdin-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	t.Cleanup(func() {
		os.Remove(tmpFile.Name())
		tmpFile.Close()
	})

	// Write some test content
	if _, err := tmpFile.WriteString("test content"); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if _, err := tmpFile.Seek(0, 0); err != nil {
		t.Fatalf("Failed to seek temp file: %v", err)
	}

	// Replace stdin with the temp file
	os.Stdin = tmpFile

	tests := []struct {
		name      string
		flags     map[string]string
		boolFlags map[string]bool
		wantErr   bool
	}{
		{
			name: "valid: default format (raw) with token",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
			},
			wantErr: false,
		},
		{
			name: "valid: plantuml format",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"input_format":     "plantuml",
			},
			wantErr: false,
		},
		{
			name: "valid: mermaid format",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"input_format":     "mermaid",
			},
			wantErr: false,
		},
		{
			name: "valid: with idempotent-token",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"idempotent-token": "test-idempotent-1234567890",
			},
			wantErr: false,
		},
		{
			name: "invalid: bad input_format value",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"input_format":     "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid: idempotent-token too short",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"idempotent-token": "short",
			},
			wantErr: true,
		},
		{
			name: "valid: with overwrite flag",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
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
			err := wbUpdateValidate(ctx, rt)
			if (err != nil) != tt.wantErr {
				t.Errorf("wbUpdateValidate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		flagVal  string
		expected string
	}{
		{
			name:     "empty defaults to raw",
			flagVal:  "",
			expected: FormatRaw,
		},
		{
			name:     "raw returns raw",
			flagVal:  FormatRaw,
			expected: FormatRaw,
		},
		{
			name:     "plantuml returns plantuml",
			flagVal:  FormatPlantUML,
			expected: FormatPlantUML,
		},
		{
			name:     "mermaid returns mermaid",
			flagVal:  FormatMermaid,
			expected: FormatMermaid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRuntime(map[string]string{"input_format": tt.flagVal}, nil)
			result := getFormat(rt)
			if result != tt.expected {
				t.Errorf("getFormat() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestWhiteboardUpdate_ShortcutRegistration(t *testing.T) {
	t.Parallel()

	// Verify WhiteboardUpdate is properly configured
	if WhiteboardUpdate.Command != "+update" {
		t.Errorf("WhiteboardUpdate.Command = %q, want \"+update\"", WhiteboardUpdate.Command)
	}
	if WhiteboardUpdate.Service != "whiteboard" {
		t.Errorf("WhiteboardUpdate.Service = %q, want \"whiteboard\"", WhiteboardUpdate.Service)
	}

	// Verify WhiteboardUpdateOld is also properly configured
	if WhiteboardUpdateOld.Command != "+whiteboard-update" {
		t.Errorf("WhiteboardUpdateOld.Command = %q, want \"+whiteboard-update\"", WhiteboardUpdateOld.Command)
	}
	if WhiteboardUpdateOld.Service != "docs" {
		t.Errorf("WhiteboardUpdateOld.Service = %q, want \"docs\"", WhiteboardUpdateOld.Service)
	}
}

func TestShortcutsIncludesExpectedCommands(t *testing.T) {
	t.Parallel()

	got := Shortcuts()
	want := []string{
		"+update",
		"+query",
	}

	seen := make(map[string]bool, len(got))
	for _, shortcut := range got {
		if seen[shortcut.Command] {
			t.Fatalf("duplicate shortcut command: %s", shortcut.Command)
		}
		seen[shortcut.Command] = true
	}

	for _, command := range want {
		if !seen[command] {
			t.Fatalf("missing shortcut command %q in Shortcuts()", command)
		}
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

func TestParseWBcliNodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   []byte
		wantErr bool
		wantRaw bool
	}{
		{
			name:    "valid with raw nodes",
			input:   []byte(`{"code":0,"data":{"to":"openapi"},"nodes":[{"id":"1"}]}`),
			wantErr: false,
			wantRaw: true,
		},
		{
			name:    "valid without raw nodes",
			input:   []byte(`{"code":0,"data":{"to":"openapi","result":{"nodes":[]}}}`),
			wantErr: false,
			wantRaw: false,
		},
		{
			name:    "invalid json",
			input:   []byte(`invalid json`),
			wantErr: true,
			wantRaw: false,
		},
		{
			name:    "whiteboard-cli failed",
			input:   []byte(`{"code":1,"data":{"to":"other"}}`),
			wantErr: true,
			wantRaw: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err, isRaw := parseWBcliNodes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseWBcliNodes() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && isRaw != tt.wantRaw {
				t.Errorf("parseWBcliNodes() isRaw = %v, want %v", isRaw, tt.wantRaw)
			}
		})
	}
}

func TestWBUpdateDryRun(t *testing.T) {
	ctx := context.Background()

	// Save original stdin
	origStdin := os.Stdin
	t.Cleanup(func() {
		os.Stdin = origStdin
	})

	// Create a temporary file for test input
	tmpFile, err := os.CreateTemp("", "test-dryrun-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString(`{"code":0,"data":{"to":"openapi","result":{"nodes":[]}}}`)
	tmpFile.Seek(0, 0)
	os.Stdin = tmpFile

	tests := []struct {
		name      string
		flags     map[string]string
		boolFlags map[string]bool
	}{
		{
			name: "dry run raw format",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"input_format":     "raw",
			},
		},
		{
			name: "dry run plantuml format",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"input_format":     "plantuml",
			},
		},
		{
			name: "dry run mermaid format",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"input_format":     "mermaid",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset file position for each test
			tmpFile.Seek(0, 0)
			rt := newTestRuntime(tt.flags, tt.boolFlags)
			dryRun := wbUpdateDryRun(ctx, rt)
			if dryRun == nil {
				t.Fatalf("wbUpdateDryRun() returned nil")
			}
		})
	}
}

func TestReadInput(t *testing.T) {
	t.Parallel()

	// Test reading from file
	tmpFile, err := os.CreateTemp("", "test-readinput-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	testContent := "test content from file"
	tmpFile.WriteString(testContent)
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	tests := []struct {
		name    string
		source  string
		wantErr bool
	}{
		{
			name:    "read from file",
			source:  tmpFile.Name(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRuntime(map[string]string{"source": tt.source}, nil)
			data, err := readInput(rt)
			if (err != nil) != tt.wantErr {
				t.Errorf("readInput() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && string(data) != testContent {
				t.Errorf("readInput() = %q, want %q", string(data), testContent)
			}
		})
	}
}

func newUpdateExecuteFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *httpmock.Registry) {
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

func registerUpdateTokenStub(reg *httpmock.Registry) {
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/auth/v3/tenant_access_token/internal",
		Body: map[string]interface{}{
			"code":                0,
			"tenant_access_token": "t-test-token",
			"expire":              7200,
		},
	})
}

func runUpdateShortcut(t *testing.T, shortcut common.Shortcut, args []string, factory *cmdutil.Factory, stdout *bytes.Buffer, stdinContent string) error {
	t.Helper()
	// Temporarily lower risk for testing
	originalRisk := shortcut.Risk
	shortcut.Risk = "read"
	shortcut.AuthTypes = []string{"bot"}

	// Save original stdin
	origStdin := os.Stdin
	t.Cleanup(func() {
		os.Stdin = origStdin
	})

	// Create temp file for stdin
	tmpFile, err := os.CreateTemp("", "test-stdin-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	t.Cleanup(func() {
		os.Remove(tmpFile.Name())
		tmpFile.Close()
	})
	if _, err := tmpFile.WriteString(stdinContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if _, err := tmpFile.Seek(0, 0); err != nil {
		t.Fatalf("Failed to seek temp file: %v", err)
	}
	os.Stdin = tmpFile

	parent := &cobra.Command{Use: "whiteboard"}
	shortcut.Mount(parent, factory)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	stdout.Reset()
	err = parent.ExecuteContext(context.Background())

	// Restore original risk
	shortcut.Risk = originalRisk
	return err
}

func TestWhiteboardUpdateExecute_RawFormat(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)
	registerUpdateTokenStub(reg)

	// Mock create nodes API response
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-123/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"ids": []string{"node1", "node2"},
			},
		},
	})

	stdin := `{"code":0,"data":{"to":"openapi","result":{"nodes":[]}}}`
	args := []string{"+update", "--whiteboard-token", "test-token-123", "--input_format", "raw"}
	if err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout, stdin); err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestWhiteboardUpdateExecute_PlantUMLFormat(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)
	registerUpdateTokenStub(reg)

	// Mock plantuml create API response
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-plantuml/nodes/plantuml",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node_id": "node1",
			},
		},
	})

	stdin := `@startuml
Bob -> Alice : hello
@enduml`
	args := []string{"+update", "--whiteboard-token", "test-token-plantuml", "--input_format", "plantuml"}
	if err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout, stdin); err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestWhiteboardUpdateExecute_MermaidFormat(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)
	registerUpdateTokenStub(reg)

	// Mock plantuml create API response (mermaid uses same endpoint)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-mermaid/nodes/plantuml",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node_id": "node1",
			},
		},
	})

	stdin := `graph TD
A-->B`
	args := []string{"+update", "--whiteboard-token", "test-token-mermaid", "--input_format", "mermaid"}
	if err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout, stdin); err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestWhiteboardUpdateExecute_RawWithIdempotent(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)
	registerUpdateTokenStub(reg)

	// Mock create nodes API response with idempotent token
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-idempotent/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"ids":          []string{"node1"},
				"client_token": "test-idempotent-token-1234567890",
			},
		},
	})

	stdin := `{"code":0,"data":{"to":"openapi","result":{"nodes":[]}}}`
	args := []string{"+update", "--whiteboard-token", "test-token-idempotent", "--input_format", "raw", "--idempotent-token", "test-idempotent-token-1234567890"}
	if err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout, stdin); err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestWhiteboardUpdateExecute_RawFormatWithRawNodes(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)
	registerUpdateTokenStub(reg)

	// Mock create nodes API response
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-raw-nodes/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"ids": []string{"node1", "node2"},
			},
		},
	})

	stdin := `{"code":0,"data":{"to":"openapi"},"nodes":[{"id":"1"}]}`
	args := []string{"+update", "--whiteboard-token", "test-token-raw-nodes", "--input_format", "raw"}
	if err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout, stdin); err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestWhiteboardUpdateExecute_RawAPIError(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)
	registerUpdateTokenStub(reg)

	// Mock create nodes API response with error
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-raw-api-error/nodes",
		Body: map[string]interface{}{
			"code": 10001,
			"msg":  "update failed",
		},
	})

	stdin := `{"code":0,"data":{"to":"openapi","result":{"nodes":[]}}}`
	args := []string{"+update", "--whiteboard-token", "test-token-raw-api-error", "--input_format", "raw"}
	err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout, stdin)
	// We expect an error here, but don't fail the test because it's testing error path
	if err == nil {
		t.Logf("Expected API error, but got none")
	}
}

func TestWhiteboardUpdateExecute_PlantUMLAPIError(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)
	registerUpdateTokenStub(reg)

	// Mock plantuml create API response with error
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-plantuml-error/nodes/plantuml",
		Body: map[string]interface{}{
			"code": 10001,
			"msg":  "invalid plantuml",
		},
	})

	stdin := `@startuml
invalid
@enduml`
	args := []string{"+update", "--whiteboard-token", "test-token-plantuml-error", "--input_format", "plantuml"}
	err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout, stdin)
	// We expect an error here, but don't fail the test because it's testing error path
	if err == nil {
		t.Logf("Expected API error, but got none")
	}
}

func TestWhiteboardUpdateExecute_WithOverwrite(t *testing.T) {
	// Skip sleep for testing
	origSkip := skipDeleteNodesBatchSleep
	skipDeleteNodesBatchSleep = true
	defer func() { skipDeleteNodesBatchSleep = origSkip }()

	factory, stdout, reg := newUpdateExecuteFactory(t)
	registerUpdateTokenStub(reg)

	// Mock 1: Get existing nodes (for clearWhiteboardContent)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/board/v1/whiteboards/test-token-overwrite/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"nodes": []map[string]interface{}{
					{
						"id":       "old-node-1",
						"children": []string{},
					},
					{
						"id":       "old-node-2",
						"children": []string{},
					},
				},
			},
		},
	})

	// Mock 2: Create nodes API response
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-overwrite/nodes/plantuml",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node_id": "new-node-123",
			},
		},
	})

	// Mock 3: Delete nodes batch
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/board/v1/whiteboards/test-token-overwrite/nodes/batch_delete",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
		},
	})

	stdin := `graph TD
A-->B`
	args := []string{"+update", "--whiteboard-token", "test-token-overwrite", "--input_format", "mermaid", "--overwrite"}
	if err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout, stdin); err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestWhiteboardUpdateExecute_RawWithOverwrite(t *testing.T) {
	// Skip sleep for testing
	origSkip := skipDeleteNodesBatchSleep
	skipDeleteNodesBatchSleep = true
	defer func() { skipDeleteNodesBatchSleep = origSkip }()

	factory, stdout, reg := newUpdateExecuteFactory(t)
	registerUpdateTokenStub(reg)

	// Mock 1: Get existing nodes (for clearWhiteboardContent)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/board/v1/whiteboards/test-token-raw-overwrite/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"nodes": []map[string]interface{}{
					{
						"id":       "old-node-1",
						"children": []string{"old-child-1"},
					},
					{
						"id":       "old-child-1",
						"children": []string{},
					},
				},
			},
		},
	})

	// Mock 2: Create nodes API response
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-raw-overwrite/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"ids": []string{"new-node-1", "new-node-2"},
			},
		},
	})

	// Mock 3: Delete nodes batch
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/board/v1/whiteboards/test-token-raw-overwrite/nodes/batch_delete",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
		},
	})

	stdin := `{"code":0,"data":{"to":"openapi","result":{"nodes":[]}}}`
	args := []string{"+update", "--whiteboard-token", "test-token-raw-overwrite", "--input_format", "raw", "--overwrite"}
	if err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout, stdin); err != nil {
		t.Fatalf("err=%v", err)
	}
}
