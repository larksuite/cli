// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestGetOKR(t *testing.T) {
	tests := []struct {
		name           string
		okrIDs         string
		formatFlag     string
		expectedOutput []string
	}{
		{
			name:       "single ID pretty",
			okrIDs:     "okr-001",
			formatFlag: "pretty",
			expectedOutput: []string{
				"OKR: Test OKR",
				"OKR ID: okr-001",
				"O1: Grow the team",
			},
		},
		{
			name:       "single ID json",
			okrIDs:     "okr-001",
			formatFlag: "json",
			expectedOutput: []string{
				`"okr_list"`,
				`"okr-001"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := okrShortcutTestFactory(t)
			warmTenantToken(t, f, reg)

			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/okr/v1/okrs/batch_get",
				Body: map[string]interface{}{
					"code": 0, "msg": "success",
					"data": map[string]interface{}{
						"okr_list": []interface{}{
							map[string]interface{}{
								"id":        "okr-001",
								"name":      "Test OKR",
								"period_id": "period-001",
								"objective_list": []interface{}{
									map[string]interface{}{
										"id":      "obj-001",
										"content": "Grow the team",
										"kr_list": []interface{}{},
									},
								},
							},
						},
					},
				},
			})

			shortcut := GetOKR
			shortcut.AuthTypes = []string{"user", "bot"}

			err := runMountedOkrShortcut(t, shortcut, []string{"+get", "--okr-ids", tt.okrIDs, "--format", tt.formatFlag, "--as", "bot"}, f, stdout)
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

func TestGetOKRValidation(t *testing.T) {
	t.Run("too many IDs", func(t *testing.T) {
		f, stdout, _, reg := okrShortcutTestFactory(t)
		warmTenantToken(t, f, reg)

		shortcut := GetOKR
		shortcut.AuthTypes = []string{"user", "bot"}

		err := runMountedOkrShortcut(t, shortcut, []string{"+get", "--okr-ids", "1,2,3,4,5,6,7,8,9,10,11", "--as", "bot"}, f, stdout)
		if err == nil {
			t.Fatal("expected validation error for too many IDs")
		}
	})
}
