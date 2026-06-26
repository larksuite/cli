// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// TestWhiteboardUpdate_Validate verifies update flag validation for supported input formats.
func TestWhiteboardUpdate_Validate(t *testing.T) {
	ctx := context.Background()

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
				"source":           "test content",
			},
			wantErr: false,
		},
		{
			name: "valid: plantuml format",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"input_format":     "plantuml",
				"source":           "test content",
			},
			wantErr: false,
		},
		{
			name: "valid: mermaid format",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"input_format":     "mermaid",
				"source":           "test content",
			},
			wantErr: false,
		},
		{
			name: "valid: svg format",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"input_format":     "svg",
				"source":           "<svg/>",
			},
			wantErr: false,
		},
		{
			name: "valid: with idempotent-token",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"idempotent-token": "xxx************xxxx",
				"source":           "test content",
			},
			wantErr: false,
		},
		{
			name: "invalid: bad input_format value",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"input_format":     "invalid",
				"source":           "test content",
			},
			wantErr: true,
		},
		{
			name: "invalid: idempotent-token too short",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"idempotent-token": "short",
				"source":           "test content",
			},
			wantErr: true,
		},
		{
			name: "valid: with overwrite flag",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"source":           "test content",
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

// TestWhiteboardUpdate_Validate_TypedErrors locks the typed-envelope contract
// for +update input validation: failures are *errs.ValidationError with
// SubtypeInvalidArgument and the offending --flag. parseWBcliNodes likewise
// reports malformed --source input as a typed validation error.
func TestWhiteboardUpdate_Validate_TypedErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("idempotent-token too short", func(t *testing.T) {
		rt := newTestRuntime(map[string]string{
			"whiteboard-token": "t",
			"idempotent-token": "short",
			"source":           "{}",
		}, nil)
		assertValidationParam(t, wbUpdateValidate(ctx, rt), "--idempotent-token", false)
	})

	t.Run("bad input_format", func(t *testing.T) {
		rt := newTestRuntime(map[string]string{
			"whiteboard-token": "t",
			"input_format":     "png",
			"source":           "{}",
		}, nil)
		assertValidationParam(t, wbUpdateValidate(ctx, rt), "--input_format", false)
	})

	t.Run("malformed source json", func(t *testing.T) {
		_, err, _ := parseWBcliNodes([]byte("not-json"))
		assertValidationParam(t, err, "--source", true)
	})
}

// assertValidationParam verifies a validation error carries the expected flag param.
func assertValidationParam(t *testing.T, err error, wantParam string, wantJSONCause bool) {
	t.Helper()
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
	if ve.Param != wantParam {
		t.Errorf("Param = %q, want %q", ve.Param, wantParam)
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
	if wantJSONCause {
		var syntaxErr *json.SyntaxError
		if !errors.As(err, &syntaxErr) {
			t.Fatalf("expected json syntax cause to be preserved, err=%v", err)
		}
	}
}

// TestGetFormat verifies input format defaults and explicit format selection.
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
		{
			name:     "svg returns svg",
			flagVal:  FormatSVG,
			expected: FormatSVG,
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

// TestWhiteboardUpdate_ShortcutRegistration verifies the shortcut metadata for update commands.
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

// TestShortcutsIncludesExpectedCommands verifies the whiteboard shortcut registry includes query and update.
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

// TestParseWBcliNodes verifies whiteboard CLI output parsing for raw and wrapped node payloads.
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

// TestWBUpdateDryRun verifies dry-run requests for the supported whiteboard update formats.
func TestWBUpdateDryRun(t *testing.T) {
	ctx := context.Background()

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
				"source":           `{"code":0,"data":{"to":"openapi","result":{"nodes":[]}}}`,
			},
		},
		{
			name: "dry run plantuml format",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"input_format":     "plantuml",
				"source":           "@@startuml\nBob -> Alice : hello\n@@enduml",
			},
		},
		{
			name: "dry run mermaid format",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"input_format":     "mermaid",
				"source":           "graph TD\nA-->B",
			},
		},
		{
			name: "dry run svg format",
			flags: map[string]string{
				"whiteboard-token": "test-token-123",
				"input_format":     "svg",
				"source":           "<svg/>",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRuntime(tt.flags, tt.boolFlags)
			dryRun := wbUpdateDryRun(ctx, rt)
			if dryRun == nil {
				t.Fatalf("wbUpdateDryRun() returned nil")
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

func runUpdateShortcut(t *testing.T, shortcut common.Shortcut, args []string, factory *cmdutil.Factory, stdout *bytes.Buffer) error {
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

// TestWhiteboardUpdateExecute_RawFormat verifies raw node updates call the raw nodes endpoint.
func TestWhiteboardUpdateExecute_RawFormat(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

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

	source := `{"code":0,"data":{"to":"openapi","result":{"nodes":[]}}}`
	args := []string{"+update", "--whiteboard-token", "test-token-123", "--input_format", "raw", "--source", source}
	if err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
}

// TestWhiteboardUpdateExecute_PlantUMLFormat verifies PlantUML updates use the diagram import endpoint.
func TestWhiteboardUpdateExecute_PlantUMLFormat(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

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

	source := `@@startuml
Bob -> Alice : hello
@@enduml`
	args := []string{"+update", "--whiteboard-token", "test-token-plantuml", "--input_format", "plantuml", "--source", source}
	if err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
}

// TestWhiteboardUpdateExecute_PlantUMLInvalidResponse verifies missing node IDs are treated as invalid responses.
func TestWhiteboardUpdateExecute_PlantUMLInvalidResponse(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-plantuml-invalid-response/nodes/plantuml",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	})

	source := `@@startuml
Bob -> Alice : hello
@@enduml`
	args := []string{"+update", "--whiteboard-token", "test-token-plantuml-invalid-response", "--input_format", "plantuml", "--source", source}
	err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout)
	assertInvalidResponse(t, err)
}

// TestWhiteboardUpdateExecute_MermaidFormat verifies Mermaid updates use the diagram import endpoint.
func TestWhiteboardUpdateExecute_MermaidFormat(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

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

	source := `graph TD
A-->B`
	args := []string{"+update", "--whiteboard-token", "test-token-mermaid", "--input_format", "mermaid", "--source", source}
	if err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
}

// TestWhiteboardUpdateExecute_SVGFormat verifies svg update requests use syntax_type=3 and send the source payload.
func TestWhiteboardUpdateExecute_SVGFormat(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	// SVG shares the /nodes/plantuml endpoint with plantuml/mermaid via syntax_type=3.
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-svg/nodes/plantuml",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node_id": "node1",
			},
		},
	}
	reg.Register(stub)

	source := `<svg xmlns="http://www.w3.org/2000/svg"/>`
	args := []string{"+update", "--whiteboard-token", "test-token-svg", "--input_format", "svg", "--source", source}
	if err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v\nraw=%s", err, string(stub.CapturedBody))
	}

	if got := body["syntax_type"]; got != float64(3) {
		t.Fatalf("syntax_type = %#v, want 3; body=%s", got, string(stub.CapturedBody))
	}
	if got := body["plant_uml_code"]; got != source {
		t.Fatalf("plant_uml_code = %#v, want %q; body=%s", got, source, string(stub.CapturedBody))
	}
}

// TestWhiteboardUpdateExecute_RawInvalidResponse verifies malformed raw update responses are rejected.
func TestWhiteboardUpdateExecute_RawInvalidResponse(t *testing.T) {
	tests := []struct {
		name  string
		token string
		data  map[string]interface{}
	}{
		{
			name:  "missing ids",
			token: "test-token-raw-missing-ids",
			data:  map[string]interface{}{},
		},
		{
			name:  "non-string id",
			token: "test-token-raw-bad-id",
			data:  map[string]interface{}{"ids": []interface{}{"node1", 2}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, reg := newUpdateExecuteFactory(t)
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/board/v1/whiteboards/" + tt.token + "/nodes",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "success",
					"data": tt.data,
				},
			})

			source := `{"code":0,"data":{"to":"openapi","result":{"nodes":[]}}}`
			args := []string{"+update", "--whiteboard-token", tt.token, "--input_format", "raw", "--source", source}
			err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout)
			assertInvalidResponse(t, err)
		})
	}
}

// TestWhiteboardUpdateExecute_RawWithIdempotent verifies raw updates pass through the idempotency token.
func TestWhiteboardUpdateExecute_RawWithIdempotent(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	// Mock create nodes API response with idempotent token
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-idempotent/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"ids":          []string{"node1"},
				"client_token": "test-token-1234567890",
			},
		},
	})

	source := `{"code":0,"data":{"to":"openapi","result":{"nodes":[]}}}`
	args := []string{"+update", "--whiteboard-token", "test-token-idempotent", "--input_format", "raw", "--idempotent-token", "test-token-1234567890", "--source", source}
	if err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
}

// TestWhiteboardUpdateExecute_RawFormatWithRawNodes verifies raw-node payloads are forwarded without DSL wrapping.
func TestWhiteboardUpdateExecute_RawFormatWithRawNodes(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

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

	source := `{"code":0,"data":{"to":"openapi"},"nodes":[{"id":"1"}]}`
	args := []string{"+update", "--whiteboard-token", "test-token-raw-nodes", "--input_format", "raw", "--source", source}
	if err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
}

// TestWhiteboardUpdateExecute_RawAPIError verifies raw update API failures preserve typed error metadata and hints.
func TestWhiteboardUpdateExecute_RawAPIError(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	// Mock create nodes API response with error
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-raw-api-error/nodes",
		Body: map[string]interface{}{
			"code": 10001,
			"msg":  "update failed",
		},
	})

	// Top-level "nodes" is the raw open-api format (isRaw=true), which triggers
	// the raw-edit recovery hint on API failure.
	source := `{"nodes":[{"type":"composite_shape"}]}`
	args := []string{"+update", "--whiteboard-token", "test-token-raw-api-error", "--input_format", "raw", "--source", source}
	err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout)
	if err == nil {
		t.Fatalf("expected API error, but got none")
	}
	// The update boundary now yields a typed envelope carrying the Lark code.
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error is not a typed errs.* envelope: %T (%v)", err, err)
	}
	if p.Code != 10001 {
		t.Errorf("Problem.Code = %d, want 10001", p.Code)
	}
	// Raw (open-api JSON) input failures steer the user back to the recommended
	// DSL workflow via a recovery hint on the typed envelope.
	if !strings.Contains(p.Hint, "not advised to edit openapi format json directly") {
		t.Errorf("Problem.Hint missing raw-edit guidance, got %q", p.Hint)
	}
}

// TestWhiteboardUpdateExecute_PlantUMLAPIError verifies diagram update API failures preserve typed error metadata.
func TestWhiteboardUpdateExecute_PlantUMLAPIError(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	// Mock plantuml create API response with error
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-token-plantuml-error/nodes/plantuml",
		Body: map[string]interface{}{
			"code": 10001,
			"msg":  "invalid plantuml",
		},
	})

	source := `@@startuml
invalid
@@enduml`
	args := []string{"+update", "--whiteboard-token", "test-token-plantuml-error", "--input_format", "plantuml", "--source", source}
	err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout)
	if err == nil {
		t.Fatalf("expected API error, but got none")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error is not a typed errs.* envelope: %T (%v)", err, err)
	}
	if p.Code != 10001 {
		t.Errorf("Problem.Code = %d, want 10001", p.Code)
	}
}

// TestWhiteboardUpdateExecute_WithOverwrite verifies diagram updates send overwrite=true when requested.
func TestWhiteboardUpdateExecute_WithOverwrite(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	// Mock: Create nodes API response with overwrite in request body
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

	source := `graph TD
A-->B`
	args := []string{"+update", "--whiteboard-token", "test-token-overwrite", "--input_format", "mermaid", "--overwrite", "--source", source}
	if err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
}

// TestWhiteboardUpdateExecute_RawWithOverwrite verifies raw updates send overwrite=true when requested.
func TestWhiteboardUpdateExecute_RawWithOverwrite(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	// Mock: Create nodes API response with overwrite in request body
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

	source := `{"code":0,"data":{"to":"openapi","result":{"nodes":[]}}}`
	args := []string{"+update", "--whiteboard-token", "test-token-raw-overwrite", "--input_format", "raw", "--overwrite", "--source", source}
	if err := runUpdateShortcut(t, WhiteboardUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
}
