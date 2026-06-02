// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/shortcuts/common"
)

func TestBuildTaskCreateBody_StructuredErrors(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		summary    string
		due        string
		wantSubstr string
	}{
		{
			name:       "invalid JSON data returns validation error",
			data:       "not-json",
			summary:    "test",
			wantSubstr: "--data must be a valid JSON object",
		},
		{
			name:       "missing summary returns validation error",
			data:       "",
			summary:    "",
			wantSubstr: "task summary is required",
		},
		{
			name:       "invalid due time returns validation error",
			data:       "",
			summary:    "test task",
			due:        "not-a-valid-time",
			wantSubstr: "failed to parse due time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("data", tt.data, "")
			cmd.Flags().String("summary", tt.summary, "")
			cmd.Flags().String("description", "", "")
			cmd.Flags().String("assignee", "", "")
			cmd.Flags().String("follower", "", "")
			cmd.Flags().String("due", tt.due, "")
			cmd.Flags().String("tasklist-id", "", "")
			cmd.Flags().String("idempotency-key", "", "")

			runtime := &common.RuntimeContext{Cmd: cmd}
			_, err := buildTaskCreateBody(runtime)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error type = %T, want *errs.ValidationError; error = %v", err, err)
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("ProblemOf(%T) returned !ok", err)
			}
			if p.Subtype != errs.SubtypeInvalidArgument {
				t.Errorf("subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidArgument)
			}
			if got := output.ExitCodeOf(err); got != output.ExitValidation {
				t.Errorf("exit code = %d, want %d", got, output.ExitValidation)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("message = %q, want substring %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}

func TestBuildTaskUpdateBody_StructuredErrors(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		summary    string
		due        string
		wantSubstr string
	}{
		{
			name:       "invalid JSON data returns validation error",
			data:       "not-json",
			summary:    "",
			due:        "",
			wantSubstr: "--data must be a valid JSON object",
		},
		{
			name:       "no fields to update returns validation error",
			data:       "",
			summary:    "",
			due:        "",
			wantSubstr: "no fields to update",
		},
		{
			name:       "invalid due time returns validation error",
			data:       "",
			summary:    "",
			due:        "not-a-valid-time",
			wantSubstr: "failed to parse due time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("data", tt.data, "")
			cmd.Flags().String("summary", tt.summary, "")
			cmd.Flags().String("description", "", "")
			cmd.Flags().String("due", tt.due, "")

			runtime := &common.RuntimeContext{Cmd: cmd}
			_, err := buildTaskUpdateBody(runtime)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error type = %T, want *errs.ValidationError; error = %v", err, err)
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("ProblemOf(%T) returned !ok", err)
			}
			if p.Subtype != errs.SubtypeInvalidArgument {
				t.Errorf("subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidArgument)
			}
			if got := output.ExitCodeOf(err); got != output.ExitValidation {
				t.Errorf("exit code = %d, want %d", got, output.ExitValidation)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("message = %q, want substring %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}
