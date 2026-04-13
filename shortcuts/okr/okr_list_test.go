// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestListOKR(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		formatFlag     string
		expectedOutput []string
	}{
		{
			name:       "pretty format with period",
			args:       []string{"--period-id", "period-001"},
			formatFlag: "pretty",
			expectedOutput: []string{
				"OKR: Test OKR",
				"O1: Increase revenue",
				"KR1: Achieve $1M ARR",
			},
		},
		{
			name:       "json format with period",
			args:       []string{"--period-id", "period-001"},
			formatFlag: "json",
			expectedOutput: []string{
				`"okr_list"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := okrShortcutTestFactory(t)
			warmTenantToken(t, f, reg)

			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/okr/v1/users/ou_testuser/okrs",
				Body: map[string]interface{}{
					"code": 0, "msg": "success",
					"data": map[string]interface{}{
						"total": float64(1),
						"okr_list": []interface{}{
							map[string]interface{}{
								"id":   "okr-001",
								"name": "Test OKR",
								"objective_list": []interface{}{
									map[string]interface{}{
										"id":      "obj-001",
										"content": "Increase revenue",
										"progress_rate": map[string]interface{}{
											"percent": float64(50),
											"status":  "0",
										},
										"kr_list": []interface{}{
											map[string]interface{}{
												"id":      "kr-001",
												"content": "Achieve $1M ARR",
												"progress_rate": map[string]interface{}{
													"percent": float64(30),
													"status":  "0",
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

			shortcut := ListOKR
			shortcut.AuthTypes = []string{"user", "bot"}

			args := append([]string{"+list", "--format", tt.formatFlag, "--as", "bot"}, tt.args...)
			err := runMountedOkrShortcut(t, shortcut, args, f, stdout)
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

func TestListOKREmpty(t *testing.T) {
	f, stdout, _, reg := okrShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v1/users/ou_testuser/okrs",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"total":    float64(0),
				"okr_list": []interface{}{},
			},
		},
	})

	shortcut := ListOKR
	shortcut.AuthTypes = []string{"user", "bot"}

	err := runMountedOkrShortcut(t, shortcut, []string{"+list", "--period-id", "p1", "--format", "pretty", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "No OKRs found") {
		t.Errorf("expected empty message, got: %s", out)
	}
}
