// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func TestSchemaCmd_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	var gotOpts *SchemaOptions
	cmd := NewCmdSchema(f, func(opts *SchemaOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"calendar.events.list", "--format", "pretty"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.Path != "calendar.events.list" {
		t.Errorf("expected path calendar.events.list, got %s", gotOpts.Path)
	}
	if gotOpts.Format != "pretty" {
		t.Errorf("expected Format=pretty, got %s", gotOpts.Format)
	}
}

func TestSchemaCmd_NoArgs_Pretty(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"--format", "pretty"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Available services") {
		t.Error("expected service list in pretty mode")
	}
}

func TestSchemaCmd_NoArgs_JSON_IsArray(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{}) // default --format json
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(out, "[") {
		head := out
		if len(head) > 80 {
			head = head[:80]
		}
		t.Errorf("expected JSON array root, first 80 chars:\n%s", head)
	}
	var envs []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &envs); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(envs) < 193 {
		t.Errorf("envelopes count = %d, want >= 193", len(envs))
	}
}

func TestSchemaCmd_JSONIsEnvelope(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"im.images.create", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, stdout.String())
	}
	if env["name"] != "im images create" {
		t.Errorf("name = %v, want \"im images create\"", env["name"])
	}
	for _, key := range []string{"description", "inputSchema", "outputSchema", "_meta"} {
		if _, ok := env[key]; !ok {
			t.Errorf("missing top-level key: %s", key)
		}
	}
	meta, _ := env["_meta"].(map[string]interface{})
	if meta["envelope_version"] != "1.0" {
		t.Errorf("envelope_version = %v, want \"1.0\"", meta["envelope_version"])
	}
}

func TestSchemaCmd_SpaceSeparatedPath_EqualsDotted(t *testing.T) {
	f1, out1, _, _ := cmdutil.TestFactory(t, nil)
	cmd1 := NewCmdSchema(f1, nil)
	cmd1.SetArgs([]string{"im", "images", "create"})
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("space form failed: %v", err)
	}

	f2, out2, _, _ := cmdutil.TestFactory(t, nil)
	cmd2 := NewCmdSchema(f2, nil)
	cmd2.SetArgs([]string{"im.images.create"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("dotted form failed: %v", err)
	}

	if out1.String() != out2.String() {
		t.Errorf("space and dotted forms produced different output")
	}
}

func TestSchemaCmd_ServiceListIsArray(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"im"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var envs []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envs); err != nil {
		t.Fatalf("unmarshal failed: %v\n%s", err, stdout.String())
	}
	if len(envs) == 0 {
		t.Fatal("expected non-empty array for service im")
	}
	for _, e := range envs {
		name, _ := e["name"].(string)
		if !strings.HasPrefix(name, "im ") {
			t.Errorf("envelope name %q does not start with \"im \"", name)
		}
	}
}

func TestSchemaCmd_HighRiskYesInjection(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"im.messages.delete"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	is, _ := env["inputSchema"].(map[string]interface{})
	props, _ := is["properties"].(map[string]interface{})
	if _, ok := props["yes"]; !ok {
		t.Errorf("inputSchema.properties.yes missing for high-risk-write command")
	}
}

func TestSchemaCmd_NoYesForReadRisk(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"im.reactions.list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	is, _ := env["inputSchema"].(map[string]interface{})
	props, _ := is["properties"].(map[string]interface{})
	if _, ok := props["yes"]; ok {
		t.Errorf("yes property should not appear for risk=read command")
	}
}

func TestSchemaCmd_PrettyUnchanged_KeyTextPresent(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"im.images.create", "--format", "pretty"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	// Existing pretty rendering surfaces these markers — they must still appear
	for _, want := range []string{"Parameters:", "Response:", "Identity:", "Scopes:", "CLI:"} {
		if !strings.Contains(out, want) {
			t.Errorf("pretty output missing marker %q", want)
		}
	}
}

func TestSchemaCmd_UnknownService(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"nonexistent_service"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for unknown service")
	}
	if !strings.Contains(err.Error(), "Unknown service") {
		t.Errorf("expected 'Unknown service' error, got: %v", err)
	}
}

func TestPrintMethodDetail_FileUpload(t *testing.T) {
	spec := map[string]interface{}{
		"name":        "im",
		"servicePath": "/open-apis/im/v1",
	}
	method := map[string]interface{}{
		"path":        "images",
		"httpMethod":  "POST",
		"description": "Upload an image",
		"requestBody": map[string]interface{}{
			"image_type": map[string]interface{}{
				"type":     "string",
				"required": true,
			},
			"image": map[string]interface{}{
				"type":     "file",
				"required": true,
			},
		},
		"accessTokens": []interface{}{"user", "tenant"},
	}

	var buf bytes.Buffer
	printMethodDetail(&buf, spec, "images", "create", method)
	out := buf.String()

	if !strings.Contains(out, "file upload") {
		t.Errorf("expected 'file upload' marker in output, got:\n%s", out)
	}
	if !strings.Contains(out, "--file") {
		t.Errorf("expected '--file' in output, got:\n%s", out)
	}
	if !strings.Contains(out, `"image"`) {
		t.Errorf("expected default field name 'image' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "--file <path>") {
		t.Errorf("expected CLI example with --file <path>, got:\n%s", out)
	}
}

func TestPrintMethodDetail_NoFileUpload(t *testing.T) {
	spec := map[string]interface{}{
		"name":        "calendar",
		"servicePath": "/open-apis/calendar/v4",
	}
	method := map[string]interface{}{
		"path":        "events",
		"httpMethod":  "POST",
		"description": "Create an event",
		"requestBody": map[string]interface{}{
			"summary": map[string]interface{}{
				"type":     "string",
				"required": true,
			},
		},
	}

	var buf bytes.Buffer
	printMethodDetail(&buf, spec, "events", "create", method)
	out := buf.String()

	if strings.Contains(out, "file upload") {
		t.Errorf("did not expect 'file upload' marker for non-file method, got:\n%s", out)
	}
	if strings.Contains(out, "--file") {
		t.Errorf("did not expect '--file' for non-file method, got:\n%s", out)
	}
}

func TestHasFileFields(t *testing.T) {
	tests := []struct {
		name       string
		method     map[string]interface{}
		wantBool   bool
		wantFields []string
	}{
		{
			name: "has file field",
			method: map[string]interface{}{
				"requestBody": map[string]interface{}{
					"image": map[string]interface{}{"type": "file"},
					"name":  map[string]interface{}{"type": "string"},
				},
			},
			wantBool:   true,
			wantFields: []string{"image"},
		},
		{
			name: "no file field",
			method: map[string]interface{}{
				"requestBody": map[string]interface{}{
					"name": map[string]interface{}{"type": "string"},
				},
			},
			wantBool:   false,
			wantFields: nil,
		},
		{
			name:       "no requestBody",
			method:     map[string]interface{}{},
			wantBool:   false,
			wantFields: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, names := hasFileFields(tt.method)
			if got != tt.wantBool {
				t.Errorf("hasFileFields() = %v, want %v", got, tt.wantBool)
			}
			if tt.wantFields == nil && names != nil {
				t.Errorf("expected nil names, got %v", names)
			}
			if tt.wantFields != nil && len(names) != len(tt.wantFields) {
				t.Errorf("expected %d field names, got %d", len(tt.wantFields), len(names))
			}
		})
	}
}

func TestCompleteSchemaPathForSpec(t *testing.T) {
	resources := map[string]interface{}{
		"records": map[string]interface{}{
			"methods": map[string]interface{}{
				"create": map[string]interface{}{},
				"list":   map[string]interface{}{},
			},
		},
		"record_permissions": map[string]interface{}{
			"methods": map[string]interface{}{
				"get": map[string]interface{}{},
			},
		},
	}

	got := completeSchemaPathForSpec("base", resources, "records.cr")
	if len(got) != 1 || got[0] != "base.records.create" {
		t.Fatalf("completions = %v, want [base.records.create]", got)
	}

	got = completeSchemaPathForSpec("base", resources, "record")
	if len(got) != 2 || got[0] != "base.record_permissions." || got[1] != "base.records." {
		t.Fatalf("resource completions = %v", got)
	}
}

func TestFilterSpecByStrictMode_RemovesIncompatibleMethodsFromCompletionSource(t *testing.T) {
	spec := map[string]interface{}{
		"resources": map[string]interface{}{
			"records": map[string]interface{}{
				"methods": map[string]interface{}{
					"list":   map[string]interface{}{"accessTokens": []interface{}{"tenant"}},
					"create": map[string]interface{}{"accessTokens": []interface{}{"user"}},
				},
			},
		},
	}

	filtered := filterSpecByStrictMode(spec, core.StrictModeBot)
	resources, _ := filtered["resources"].(map[string]interface{})
	got := completeSchemaPathForSpec("base", resources, "records.")
	if len(got) != 1 || got[0] != "base.records.list" {
		t.Fatalf("filtered completions = %v, want [base.records.list]", got)
	}
}
