// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Workflow Coverage:
//
//	| t.Run | Command |
//	| --- | --- |
//	| `Setup` | `base +base-create`, `base +table-create`, `base +dashboard-create`, `base +dashboard-block-create` |
//	| `dashboard list` | `base +dashboard-list` |
//	| `dashboard get` | `base +dashboard-get` |
//	| `dashboard update` | `base +dashboard-update` |
//	| `dashboard block list` | `base +dashboard-block-list` |
//	| `dashboard block get` | `base +dashboard-block-get` |
//	| `dashboard block update` | `base +dashboard-block-update` |
//	| `dashboard block delete` | `base +dashboard-block-delete` |
//	| `dashboard delete` | `base +dashboard-delete` |
func TestBase_DashboardWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBase(t, ctx, "lark-cli-e2e-base-dashboard-"+testSuffix())
	tableName := "lark-cli-e2e-dashboard-table-" + testSuffix()
	tableID, _, _ := createTable(t, parentT, ctx, baseToken, tableName, `[{"name":"Amount","type":"number"}]`, "")
	dashboardID := createDashboard(t, parentT, ctx, baseToken, "lark-cli-e2e-sales-dashboard-"+testSuffix())
	blockID := createBlock(t, parentT, ctx, baseToken, dashboardID, "Amount Stats", "statistics", `{"table_name":"`+tableName+`","series":[{"field_name":"Amount","rollup":"sum"}]}`)

	t.Run("dashboard list", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+dashboard-list", "--base-token", baseToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot dashboard list capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, "data.items.#(dashboard_id==\""+dashboardID+"\")").Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("dashboard get", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+dashboard-get", "--base-token", baseToken, "--dashboard-id", dashboardID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot dashboard get capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, dashboardID, gjson.Get(result.Stdout, "data.dashboard.dashboard_id").String())
	})

	t.Run("dashboard update", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+dashboard-update", "--base-token", baseToken, "--dashboard-id", dashboardID, "--name", "Sales Dashboard Updated", "--theme-style", "SimpleBlue"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot dashboard update capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, "Sales Dashboard Updated", gjson.Get(result.Stdout, "data.dashboard.name").String())
	})

	t.Run("dashboard block list", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+dashboard-block-list", "--base-token", baseToken, "--dashboard-id", dashboardID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot dashboard block list capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, "data.items.#(block_id==\""+blockID+"\")").Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("dashboard block get", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+dashboard-block-get", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot dashboard block get capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, blockID, gjson.Get(result.Stdout, "data.block.block_id").String())
	})

	t.Run("dashboard block update", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+dashboard-block-update", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID, "--name", "Amount Stats Updated", "--data-config", `{"table_name":"` + tableName + `","series":[{"field_name":"Amount","rollup":"SUM"}]}`},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot dashboard block update capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, "Amount Stats Updated", gjson.Get(result.Stdout, "data.block.name").String())
	})

	t.Run("dashboard block delete", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+dashboard-block-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID, "--yes"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot dashboard block delete capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, blockID, gjson.Get(result.Stdout, "data.block_id").String())
	})

	t.Run("dashboard delete", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+dashboard-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--yes"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot dashboard delete capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, dashboardID, gjson.Get(result.Stdout, "data.dashboard_id").String())
	})

	_ = tableID
}

// Workflow Coverage:
//
//	| t.Run | Command |
//	| --- | --- |
//	| `Setup` | `base +base-create`, `base +table-create`, `base +form-create` |
//	| `form get` | `base +form-get` |
//	| `form list` | `base +form-list` |
//	| `form update` | `base +form-update` |
//	| `form questions create` | `base +form-questions-create` |
//	| `form questions list` | `base +form-questions-list` |
//	| `form questions update` | `base +form-questions-update` |
//	| `form questions delete` | `base +form-questions-delete` |
//	| `form delete` | `base +form-delete` |
func TestBase_FormWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBase(t, ctx, "lark-cli-e2e-base-form-"+testSuffix())
	tableID, _, _ := createTable(t, parentT, ctx, baseToken, "lark-cli-e2e-form-table-"+testSuffix(), `[{"name":"Name","type":"text"}]`, "")
	formID := createForm(t, parentT, ctx, baseToken, tableID, "lark-cli-e2e-survey-"+testSuffix())

	var questionID string

	t.Run("form get", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+form-get", "--base-token", baseToken, "--table-id", tableID, "--form-id", formID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot form get capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, formID, gjson.Get(result.Stdout, "data.id").String())
	})

	t.Run("form list", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+form-list", "--base-token", baseToken, "--table-id", tableID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot form list capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, "data.forms.#(id==\""+formID+"\")").Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("form update", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+form-update", "--base-token", baseToken, "--table-id", tableID, "--form-id", formID, "--name", "Survey Updated", "--description", "updated description"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot form update capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, "Survey Updated", gjson.Get(result.Stdout, "data.name").String())
	})

	t.Run("form questions create", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+form-questions-create", "--base-token", baseToken, "--table-id", tableID, "--form-id", formID, "--questions", `[{"type":"text","title":"Your Name","required":true}]`},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot form question create capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		questionID = gjson.Get(result.Stdout, "data.questions.0.id").String()
		require.NotEmpty(t, questionID, "stdout:\n%s", result.Stdout)
	})

	t.Run("form questions list", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+form-questions-list", "--base-token", baseToken, "--table-id", tableID, "--form-id", formID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot form question list capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, "data.questions.#(id==\""+questionID+"\")").Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("form questions update", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+form-questions-update", "--base-token", baseToken, "--table-id", tableID, "--form-id", formID, "--questions", `[{"id":"` + questionID + `","title":"Your Name Updated","required":true}]`},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot form question update capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, "Your Name Updated", gjson.Get(result.Stdout, "data.questions.0.title").String())
	})

	t.Run("form questions delete", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+form-questions-delete", "--base-token", baseToken, "--table-id", tableID, "--form-id", formID, "--question-ids", `["` + questionID + `"]`, "--yes"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot form question delete capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, true, gjson.Get(result.Stdout, "data.deleted").Bool(), "stdout:\n%s", result.Stdout)
	})

	t.Run("form delete", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+form-delete", "--base-token", baseToken, "--table-id", tableID, "--form-id", formID, "--yes"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot form delete capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, true, gjson.Get(result.Stdout, "data.deleted").Bool(), "stdout:\n%s", result.Stdout)
	})
}
