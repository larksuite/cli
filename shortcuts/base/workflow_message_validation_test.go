// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestBaseWorkflowMessageActionValidation(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantPath     string
		wantHint     string
		unwantedHint string
	}{
		{
			name:     "data must be object",
			body:     `{"steps":[{"type":"LarkMessageAction","data":[]}]}`,
			wantPath: "--json.steps[0].data",
			wantHint: "JSON object",
		},
		{
			name:         "receiver is required",
			body:         `{"steps":[{"type":"LarkMessageAction","data":{"content":[{}]}}]}`,
			wantPath:     "--json.steps[0].data.receiver",
			wantHint:     "non-empty JSON array",
			unwantedHint: "btn_list",
		},
		{
			name:     "content must not be empty",
			body:     `{"steps":[{"type":"LarkMessageAction","data":{"receiver":[{}],"content":[]}}]}`,
			wantPath: "--json.steps[0].data.content",
			wantHint: "non-empty JSON array",
		},
		{
			name:     "send to everyone must be boolean when present",
			body:     `{"steps":[{"type":"LarkMessageAction","data":{"receiver":[{}],"content":[{}],"send_to_everyone":"yes"}}]}`,
			wantPath: "--json.steps[0].data.send_to_everyone",
			wantHint: "true or false",
		},
		{
			name:     "button list validates later message action",
			body:     `{"steps":[{"type":"HTTPClientAction","data":null},{"type":"LarkMessageAction","data":{"receiver":[{}],"content":[{}],"btn_list":{}}}]}`,
			wantPath: "--json.steps[1].data.btn_list",
			wantHint: "empty array is valid",
		},
	}

	for _, tt := range tests {
		for _, command := range workflowValidationCommands(tt.body) {
			t.Run(tt.name+"/"+command.name, func(t *testing.T) {
				err := command.shortcut.Validate(context.Background(), newBaseTestRuntime(command.flags, nil, nil))
				problem, ok := errs.ProblemOf(err)
				if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
					t.Fatalf("expected validation/invalid_argument problem, got %T %v", err, err)
				}

				var validationErr *errs.ValidationError
				if !errors.As(err, &validationErr) {
					t.Fatalf("expected ValidationError, got %T %v", err, err)
				}
				if validationErr.Param != "--json" {
					t.Fatalf("param=%q, want --json", validationErr.Param)
				}
				if cause := errors.Unwrap(err); cause != nil {
					t.Fatalf("semantic validation error cause=%v, want nil", cause)
				}
				if !strings.Contains(validationErr.Message, tt.wantPath) {
					t.Fatalf("message=%q, want path %q", validationErr.Message, tt.wantPath)
				}
				if !strings.Contains(validationErr.Hint, tt.wantHint) {
					t.Fatalf("hint=%q, want %q", validationErr.Hint, tt.wantHint)
				}
				if tt.unwantedHint != "" && strings.Contains(validationErr.Hint, tt.unwantedHint) {
					t.Fatalf("hint=%q must not include unrelated %q guidance", validationErr.Hint, tt.unwantedHint)
				}
			})
		}
	}
}

func TestBaseWorkflowMessageActionValidationAcceptsGeneralShapes(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "optional fields omitted",
			body: `{"steps":[{"type":"LarkMessageAction","data":{"receiver":[{}],"content":[{}]}}]}`,
		},
		{
			name: "optional fields present with valid types",
			body: `{"steps":[{"type":"LarkMessageAction","data":{"receiver":[{}],"content":[{}],"send_to_everyone":false,"btn_list":[]}}]}`,
		},
		{
			name: "unrelated action remains server validated",
			body: `{"steps":[{"type":"HTTPClientAction","data":null},{"type":"LarkMessageAction","data":{"receiver":[{}],"content":[{}]}}]}`,
		},
	}

	for _, tt := range tests {
		for _, command := range workflowValidationCommands(tt.body) {
			t.Run(tt.name+"/"+command.name, func(t *testing.T) {
				if err := command.shortcut.Validate(context.Background(), newBaseTestRuntime(command.flags, nil, nil)); err != nil {
					t.Fatalf("validation rejected a supported shape: %v", err)
				}
			})
		}
	}
}

type workflowValidationCommand struct {
	name     string
	shortcut common.Shortcut
	flags    map[string]string
}

func workflowValidationCommands(body string) []workflowValidationCommand {
	return []workflowValidationCommand{
		{
			name:     "create",
			shortcut: BaseWorkflowCreate,
			flags:    map[string]string{"base-token": "app_x", "json": body},
		},
		{
			name:     "update",
			shortcut: BaseWorkflowUpdate,
			flags:    map[string]string{"base-token": "app_x", "workflow-id": "wkf_x", "json": body},
		},
	}
}
