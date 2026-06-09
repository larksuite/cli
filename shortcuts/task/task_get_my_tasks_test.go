// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

func TestGetMyTasks_LocalTimeFormatting(t *testing.T) {
	tsMs := int64(1775174400000)
	tsStr := strconv.FormatInt(tsMs, 10)
	expectedDueTimeStr := time.UnixMilli(tsMs).Local().Format("2006-01-02 15:04")
	expectedCreatedDateStr := time.UnixMilli(tsMs).Local().Format("2006-01-02")
	expectedRFC3339 := time.UnixMilli(tsMs).Local().Format(time.RFC3339)

	tests := []struct {
		name           string
		formatFlag     string
		pageToken      string
		stubURL        string
		expectedOutput []string
	}{
		{
			name:       "pretty format",
			formatFlag: "pretty",
			stubURL:    "/open-apis/task/v2/tasks",
			expectedOutput: []string{
				"Due: " + expectedDueTimeStr,
				"Created: " + expectedCreatedDateStr,
			},
		},
		{
			name:       "json format",
			formatFlag: "json",
			stubURL:    "/open-apis/task/v2/tasks",
			expectedOutput: []string{
				`"due_at": "` + expectedRFC3339 + `"`,
				`"created_at": "` + expectedRFC3339 + `"`,
			},
		},
		{
			name:       "start from page token",
			formatFlag: "json",
			pageToken:  "pt_001",
			stubURL:    "page_token=pt_001",
			expectedOutput: []string{
				`"guid": "task-123"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := taskShortcutTestFactory(t)
			warmTenantToken(t, f, reg)

			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    tt.stubURL,
				Body: map[string]interface{}{
					"code": 0, "msg": "success",
					"data": map[string]interface{}{
						"items": []interface{}{
							map[string]interface{}{
								"guid":       "task-123",
								"summary":    "Test Task",
								"created_at": tsStr,
								"due": map[string]interface{}{
									"timestamp": tsStr,
								},
								"url": "https://example.com/task-123",
							},
						},
						"has_more":   false,
						"page_token": "",
					},
				},
			})

			s := GetMyTasks
			s.AuthTypes = []string{"bot", "user"}

			args := []string{"+get-my-tasks", "--format", tt.formatFlag, "--as", "bot"}
			if tt.pageToken != "" {
				args = append(args, "--page-token", tt.pageToken)
			}
			err := runMountedTaskShortcut(t, s, args, f, stdout)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			out := stdout.String()
			outNorm := strings.ReplaceAll(out, `":"`, `": "`)

			for _, expected := range tt.expectedOutput {
				if !strings.Contains(outNorm, expected) && !strings.Contains(out, expected) {
					t.Errorf("output missing expected string (%s), got: %s", expected, out)
				}
			}
		})
	}
}

// TestGetMyTasks_InvalidTimeFlags locks the three time-flag validation arms in
// Execute (--created_at / --due-start / --due-end). The parse runs before any
// API call, so a malformed value deterministically surfaces a typed
// *errs.ValidationError (exit 2) regardless of credentials — the command runs
// as user with a throwaway token. Each error carries the corresponding --flag
// param so the caller can point at the offending input.
func TestGetMyTasks_InvalidTimeFlags(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		wantParam string
	}{
		{name: "created_at", flag: "--created_at", wantParam: "--created_at"},
		{name: "due-start", flag: "--due-start", wantParam: "--due-start"},
		{name: "due-end", flag: "--due-end", wantParam: "--due-end"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := taskShortcutTestFactory(t)

			s := GetMyTasks
			s.AuthTypes = []string{"bot", "user"}

			args := []string{"+get-my-tasks", tt.flag, "not-a-time", "--as", "user"}
			err := runMountedTaskShortcut(t, s, args, f, stdout)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.flag)
			}

			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error type = %T, want *errs.ValidationError; error = %v", err, err)
			}
			if ve.Subtype != errs.SubtypeInvalidArgument {
				t.Errorf("subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
			}
			if got := output.ExitCodeOf(err); got != output.ExitValidation {
				t.Errorf("exit code = %d, want %d", got, output.ExitValidation)
			}
			if ve.Param != tt.wantParam {
				t.Errorf("param = %q, want %q", ve.Param, tt.wantParam)
			}
		})
	}
}
