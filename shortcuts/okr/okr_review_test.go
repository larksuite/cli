// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestQueryReview(t *testing.T) {
	tests := []struct {
		name           string
		userIDs        string
		periodID       string
		formatFlag     string
		expectedOutput []string
	}{
		{
			name:       "pretty format",
			userIDs:    "ou_user1",
			periodID:   "period-001",
			formatFlag: "pretty",
			expectedOutput: []string{
				"User: ou_user1",
				"Period: period-001",
			},
		},
		{
			name:       "json format",
			userIDs:    "ou_user1",
			periodID:   "period-001",
			formatFlag: "json",
			expectedOutput: []string{
				`"review_list"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := okrShortcutTestFactory(t)
			warmTenantToken(t, f, reg)

			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/okr/v1/reviews/query",
				Body: map[string]interface{}{
					"code": 0, "msg": "success",
					"data": map[string]interface{}{
						"review_list": []interface{}{
							map[string]interface{}{
								"user_id": map[string]interface{}{
									"open_id": "ou_user1",
								},
								"review_period_list": []interface{}{
									map[string]interface{}{
										"period_id":            "period-001",
										"cycle_review_list":    []interface{}{},
										"progress_report_list": []interface{}{},
									},
								},
							},
						},
					},
				},
			})

			shortcut := QueryReview
			shortcut.AuthTypes = []string{"user", "bot"}

			err := runMountedOkrShortcut(t, shortcut, []string{"+review", "--user-ids", tt.userIDs, "--period-id", tt.periodID, "--format", tt.formatFlag, "--as", "bot"}, f, stdout)
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

func TestQueryReviewValidation(t *testing.T) {
	t.Run("too many user IDs", func(t *testing.T) {
		f, stdout, _, reg := okrShortcutTestFactory(t)
		warmTenantToken(t, f, reg)

		shortcut := QueryReview
		shortcut.AuthTypes = []string{"user", "bot"}

		err := runMountedOkrShortcut(t, shortcut, []string{"+review", "--user-ids", "1,2,3,4,5,6", "--period-id", "p1", "--as", "bot"}, f, stdout)
		if err == nil {
			t.Fatal("expected validation error for too many user IDs")
		}
	})
}

func TestQueryReviewEmpty(t *testing.T) {
	f, stdout, _, reg := okrShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v1/reviews/query",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"review_list": []interface{}{},
			},
		},
	})

	shortcut := QueryReview
	shortcut.AuthTypes = []string{"user", "bot"}

	err := runMountedOkrShortcut(t, shortcut, []string{"+review", "--user-ids", "ou_user1", "--period-id", "p1", "--format", "pretty", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "No OKR reviews found") {
		t.Errorf("expected empty message, got: %s", out)
	}
}
