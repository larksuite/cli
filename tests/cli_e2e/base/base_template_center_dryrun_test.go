// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseTemplateCenterDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	tests := []struct {
		name string
		args []string
		want map[string]string
	}{
		{
			name: "categories",
			args: []string{"base", "+template-categories", "--dry-run"},
			want: map[string]string{
				"api.0.method": "GET",
				"api.0.url":    "/open-apis/base/v3/bases/templates/category",
			},
		},
		{
			name: "list",
			args: []string{"base", "+template-list", "--category-key", "office", "--page-size", "20", "--offset", "cursor_1", "--dry-run"},
			want: map[string]string{
				"api.0.method":              "GET",
				"api.0.url":                 "/open-apis/base/v3/bases/templates",
				"api.0.params.category_key": "office",
				"api.0.params.offset":       "cursor_1",
			},
		},
		{
			name: "search",
			args: []string{"base", "+template-search", "--keyword", "AI", "--limit", "10", "--dry-run"},
			want: map[string]string{
				"api.0.method":         "GET",
				"api.0.url":            "/open-apis/base/v3/bases/templates/search",
				"api.0.params.keyword": "AI",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: tt.args, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			for path, want := range tt.want {
				require.Equal(t, want, clie2e.DryRunGet(result.Stdout, path).String(), result.Stdout)
			}
			if tt.name == "list" {
				require.Equal(t, int64(20), clie2e.DryRunGet(result.Stdout, "api.0.params.limit").Int(), result.Stdout)
			}
			if tt.name == "search" {
				require.Equal(t, int64(10), clie2e.DryRunGet(result.Stdout, "api.0.params.limit").Int(), result.Stdout)
			}
		})
	}
}

func TestBaseTemplateSearchDryRunRejectsBlankKeyword(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+template-search", "--keyword", "   ", "--dry-run"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)

	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--keyword", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Empty(t, result.Stdout)
}
