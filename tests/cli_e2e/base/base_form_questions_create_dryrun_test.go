// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseFormQuestionsCreateDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+form-questions-create",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--form-id", "vew_x",
			"--questions", `[{"type":"text","title":"Risk","required":true}]`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms/vew_x/questions", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "text", clie2e.DryRunGet(out, "api.0.body.questions.0.type").String(), out)
	require.Equal(t, "Risk", clie2e.DryRunGet(out, "api.0.body.questions.0.title").String(), out)
	require.True(t, clie2e.DryRunGet(out, "api.0.body.questions.0.required").Bool(), out)
}

func TestBaseFormQuestionsCreateExistingFieldDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+form-questions-create",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--form-id", "vew_x",
			"--questions", `[{"use_existing_field":true,"field_id":"fldEmail","title":"Email"}]`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms/vew_x/questions", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.True(t, clie2e.DryRunGet(out, "api.0.body.questions.0.use_existing_field").Bool(), out)
	require.Equal(t, "fldEmail", clie2e.DryRunGet(out, "api.0.body.questions.0.field_id").String(), out)
}

func TestBaseFormQuestionsCreateDryRunRejectsInvalidInput(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	tests := []struct {
		name    string
		input   string
		message string
	}{
		{name: "malformed JSON", input: "{", message: "must be a valid JSON array"},
		{name: "non-array JSON", input: "{}", message: "must be a valid JSON array"},
		{name: "null", input: "null", message: "must be a non-null JSON array"},
		{name: "non-object item", input: "[1]", message: "item 1 must be an object"},
		{name: "missing title", input: `[{"type":"text"}]`, message: `item 1 must include a non-empty string "title"`},
		{name: "blank title", input: `[{"title":" ","type":"text"}]`, message: `item 1 must include a non-empty string "title"`},
		{name: "missing type", input: `[{"title":"Risk"}]`, message: `item 1 must include a non-empty string "type"`},
		{name: "non-string type", input: `[{"title":"Risk","type":1}]`, message: `item 1 must include a non-empty string "type"`},
		{name: "existing field missing field_id", input: `[{"use_existing_field":true}]`, message: `item 1 with use_existing_field must include a non-empty string "field_id"`},
		{name: "more than ten items", input: `[{},{},{},{},{},{},{},{},{},{},{}]`, message: "must contain at most 10 items"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"base", "+form-questions-create",
					"--base-token", "app_x",
					"--table-id", "tbl_x",
					"--form-id", "vew_x",
					"--questions", tt.input,
					"--dry-run",
				},
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)

			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
			require.Equal(t, "--questions", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
			require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), tt.message)
			require.Empty(t, result.Stdout)
		})
	}
}

func TestBaseFormQuestionsCreateHelpShowsExistingQuestionGuard(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+form-questions-create", "--help"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Contains(t, strings.ToLower(result.Stdout), "form may already contain questions")
	require.Contains(t, result.Stdout, "+form-questions-list")
	require.Contains(t, result.Stdout, "+form-questions-update")
}
