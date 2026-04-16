// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestGetProgressRecord(t *testing.T) {
	tests := []struct {
		name           string
		progressID     string
		formatFlag     string
		expectedOutput []string
	}{
		{
			name:       "pretty format",
			progressID: "prog-001",
			formatFlag: "pretty",
			expectedOutput: []string{
				"Progress ID: prog-001",
				"Target: kr-001 (key_result)",
			},
		},
		{
			name:       "json format",
			progressID: "prog-001",
			formatFlag: "json",
			expectedOutput: []string{
				`"progress_id"`,
				`"prog-001"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := okrShortcutTestFactory(t)
			warmTenantToken(t, f, reg)

			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/okr/v1/progress_records/" + tt.progressID,
				Body: map[string]interface{}{
					"code": 0, "msg": "success",
					"data": map[string]interface{}{
						"progress_id": tt.progressID,
						"target_id":   "kr-001",
						"target_type": "3",
						"modify_time": "1700000000000",
						"content": map[string]interface{}{
							"blocks": []interface{}{
								map[string]interface{}{
									"type": "paragraph",
									"paragraph": map[string]interface{}{
										"elements": []interface{}{
											map[string]interface{}{
												"type": "textRun",
												"textRun": map[string]interface{}{
													"text": "Progress update content",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			})

			shortcut := GetProgress
			shortcut.AuthTypes = []string{"user", "bot"}

			err := runMountedOkrShortcut(t, shortcut, []string{"+progress-get", "--progress-id", tt.progressID, "--format", tt.formatFlag, "--as", "bot"}, f, stdout)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			out := stdout.String()
			for _, expected := range tt.expectedOutput {
				if !strings.Contains(out, expected) {
					t.Errorf("output missing expected string (%s), got: %s", expected, out)
				}
			}
		})
	}
}
