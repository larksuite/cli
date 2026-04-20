// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"bytes"
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

func TestSchemaCmd_NoArgs(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Available services") {
		t.Error("expected service list output")
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
