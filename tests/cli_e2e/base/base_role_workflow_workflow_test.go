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

func TestBase_RoleAdvpermAndWorkflowCoverage(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBase(t, ctx, uniqueName("lark-cli-e2e-base-admin"))

	t.Run("advperm enable", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+advperm-enable", "--base-token", baseToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot advanced permission enable capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	roleID := createRole(t, parentT, ctx, baseToken, `{"role_name":"Reviewer","role_type":"custom_role"}`)

	t.Run("role list", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+role-list", "--base-token", baseToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot role list capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, "data.#(role_id==\""+roleID+"\")").Exists() || gjson.Get(result.Stdout, "data.data.#(role_id==\""+roleID+"\")").Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("role get", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+role-get", "--base-token", baseToken, "--role-id", roleID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot role get capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, roleID, gjson.Get(result.Stdout, "data.role_id").String())
	})

	t.Run("role update", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+role-update", "--base-token", baseToken, "--role-id", roleID, "--json", `{"role_name":"Reviewer Updated"}`, "--yes"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot role update capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, "Reviewer Updated", gjson.Get(result.Stdout, "data.role_name").String())
	})

	workflowID := createWorkflow(t, ctx, baseToken, `{"title":"My Workflow","steps":[]}`)

	t.Run("workflow list", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+workflow-list", "--base-token", baseToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot workflow list capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, "data.items.#(workflow_id==\""+workflowID+"\")").Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("workflow get", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+workflow-get", "--base-token", baseToken, "--workflow-id", workflowID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot workflow get capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, workflowID, gjson.Get(result.Stdout, "data.workflow_id").String())
	})

	t.Run("workflow update", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+workflow-update", "--base-token", baseToken, "--workflow-id", workflowID, "--json", `{"title":"My Workflow Updated","steps":[]}`},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot workflow update capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, "My Workflow Updated", gjson.Get(result.Stdout, "data.title").String())
	})

	t.Run("workflow enable", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+workflow-enable", "--base-token", baseToken, "--workflow-id", workflowID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot workflow enable capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("workflow disable", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+workflow-disable", "--base-token", baseToken, "--workflow-id", workflowID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot workflow disable capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("role delete", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+role-delete", "--base-token", baseToken, "--role-id", roleID, "--yes"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot role delete capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("advperm disable", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+advperm-disable", "--base-token", baseToken, "--yes"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot advanced permission disable capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})
}
