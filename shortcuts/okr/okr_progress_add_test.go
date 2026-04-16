// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAddProgress(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		formatFlag     string
		expectedOutput []string
	}{
		{
			name:       "add with text pretty",
			args:       []string{"--target-id", "kr-001", "--target-type", "key_result", "--text", "Completed 80% of development"},
			formatFlag: "pretty",
			expectedOutput: []string{
				"Progress record added successfully!",
				"Progress ID: prog-001",
			},
		},
		{
			name:       "add with text json",
			args:       []string{"--target-id", "obj-001", "--target-type", "objective", "--text", "On track"},
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
				Method: "POST",
				URL:    "/open-apis/okr/v1/progress_records",
				Body: map[string]interface{}{
					"code": 0, "msg": "success",
					"data": map[string]interface{}{
						"progress_id": "prog-001",
					},
				},
			})

			shortcut := AddProgress
			shortcut.AuthTypes = []string{"user", "bot"}

			args := append([]string{"+progress-add", "--format", tt.formatFlag, "--as", "bot"}, tt.args...)
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

func TestAddProgressValidation(t *testing.T) {
	t.Run("missing content", func(t *testing.T) {
		f, stdout, _, reg := okrShortcutTestFactory(t)
		warmTenantToken(t, f, reg)

		shortcut := AddProgress
		shortcut.AuthTypes = []string{"user", "bot"}

		err := runMountedOkrShortcut(t, shortcut, []string{"+progress-add", "--target-id", "kr-001", "--target-type", "key_result", "--as", "bot"}, f, stdout)
		if err == nil {
			t.Fatal("expected validation error for missing content")
		}
	})
}
