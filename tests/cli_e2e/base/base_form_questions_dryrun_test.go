// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaseFormQuestionsCreateVisibleRuleDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+form-questions-create",
			"--base-token", "bascnXXXX",
			"--table-id", "tblXXXX",
			"--form-id", "vewXXXX",
			"--questions", `[{"type":"text","title":"发票抬头","visible_rule":{"logic":"and","conditions":[["是否需要发票","==","是"]]}}]`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/base/v3/bases/bascnXXXX/tables/tblXXXX/forms/vewXXXX/questions")
	assert.Contains(t, output, `"method": "POST"`)
	// visible_rule must be transcribed verbatim into the request body.
	assert.Contains(t, output, "visible_rule")
	assert.Contains(t, output, "是否需要发票")
}

func TestBaseFormQuestionsUpdateVisibleRuleDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+form-questions-update",
			"--base-token", "bascnXXXX",
			"--table-id", "tblXXXX",
			"--form-id", "vewXXXX",
			"--questions", `[{"id":"q_002","visible_rule":{"logic":"and","conditions":[["q_001","==","是"]]}}]`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/base/v3/bases/bascnXXXX/tables/tblXXXX/forms/vewXXXX/questions")
	assert.Contains(t, output, `"method": "PATCH"`)
	assert.Contains(t, output, "visible_rule")
}

func TestBaseFormQuestionsDeleteKeepFieldDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+form-questions-delete",
			"--base-token", "bascnXXXX",
			"--table-id", "tblXXXX",
			"--form-id", "vewXXXX",
			"--question-ids", `["fldEmail"]`,
			"--keep-field",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/base/v3/bases/bascnXXXX/tables/tblXXXX/forms/vewXXXX/questions")
	assert.Contains(t, output, `"method": "DELETE"`)
	assert.Contains(t, output, `"keep_field": true`)
	assert.Contains(t, output, `"fldEmail"`)
}
