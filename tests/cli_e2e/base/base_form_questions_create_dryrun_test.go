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

func TestBaseFormQuestionsCreateDryRunRejectsInvalidJSON(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+form-questions-create",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--form-id", "vew_x",
			"--questions", "{",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)

	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--questions", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "must be a valid JSON array")
	require.Empty(t, result.Stdout)
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
