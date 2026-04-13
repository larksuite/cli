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

func TestBase_RoleWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-base-role-"+clie2e.GenerateSuffix())
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+advperm-enable", "--base-token", baseToken},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	roleName := "Reviewer-" + clie2e.GenerateSuffix()
	createRole(t, ctx, baseToken, `{"role_name":"`+roleName+`","role_type":"custom_role"}`)
	roleID := ""

	parentT.Cleanup(func() {
		if roleID == "" {
			return
		}

		cleanupCtx, cancel := cleanupContext()
		defer cancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"base", "+role-delete", "--base-token", baseToken, "--role-id", roleID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			reportCleanupFailure(parentT, "delete role "+roleID, deleteResult, deleteErr)
		}
	})

	t.Run("list", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+role-list", "--base-token", baseToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		roleListPayload := gjson.Get(result.Stdout, "data.data").String()
		require.NotEmpty(t, roleListPayload, "stdout:\n%s", result.Stdout)
		assert.True(t, gjson.Valid(roleListPayload), "role list payload should be valid JSON: %s", roleListPayload)

		roleItems := gjson.Get(roleListPayload, "base_roles").Array()
		assert.NotEmpty(t, roleItems, "role list should contain at least one role: %s", roleListPayload)

		found := false
		for _, item := range roleItems {
			rolePayload := item.String()
			if !gjson.Valid(rolePayload) {
				continue
			}
			if gjson.Get(rolePayload, "role_name").String() == roleName {
				roleID = gjson.Get(rolePayload, "role_id").String()
				found = true
				break
			}
		}
		require.True(t, found, "stdout:\n%s", result.Stdout)
		require.NotEmpty(t, roleID, "stdout:\n%s", result.Stdout)
	})

	t.Run("get", func(t *testing.T) {
		require.NotEmpty(t, roleID, "role ID should be resolved before get")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+role-get", "--base-token", baseToken, "--role-id", roleID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		rolePayload := gjson.Get(result.Stdout, "data.data").String()
		require.NotEmpty(t, rolePayload, "stdout:\n%s", result.Stdout)
		require.True(t, gjson.Valid(rolePayload), "stdout:\n%s", result.Stdout)
		assert.Equal(t, roleID, gjson.Get(rolePayload, "role_id").String())
	})

	t.Run("update", func(t *testing.T) {
		require.NotEmpty(t, roleID, "role ID should be resolved before update")

		updatedRoleName := roleName + " Updated"
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+role-update", "--base-token", baseToken, "--role-id", roleID, "--json", `{"role_name":"` + updatedRoleName + `","role_type":"custom_role"}`, "--yes"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		getResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+role-get", "--base-token", baseToken, "--role-id", roleID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		getResult.AssertExitCode(t, 0)
		getResult.AssertStdoutStatus(t, true)

		rolePayload := gjson.Get(getResult.Stdout, "data.data").String()
		require.NotEmpty(t, rolePayload, "stdout:\n%s", getResult.Stdout)
		require.True(t, gjson.Valid(rolePayload), "stdout:\n%s", getResult.Stdout)
		assert.Equal(t, updatedRoleName, gjson.Get(rolePayload, "role_name").String())
	})

}
