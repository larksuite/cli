// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/httpmock"
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
		expectedOutput []string
	}{
		{
			name:       "pretty format",
			formatFlag: "pretty",
			expectedOutput: []string{
				"Due: " + expectedDueTimeStr,
				"Created: " + expectedCreatedDateStr,
			},
		},
		{
			name:       "json format",
			formatFlag: "json",
			expectedOutput: []string{
				`"due_at": "` + expectedRFC3339 + `"`,
				`"created_at": "` + expectedRFC3339 + `"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := taskShortcutTestFactory(t)
			warmTenantToken(t, f, reg)

			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/task/v2/tasks",
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

			err := runMountedTaskShortcut(t, s, []string{"+get-my-tasks", "--format", tt.formatFlag, "--as", "bot"}, f, stdout)
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
