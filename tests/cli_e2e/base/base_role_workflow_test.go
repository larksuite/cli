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
	clie2e.SkipWithoutTenantAccessToken(t)
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
	roleID := createRole(t, ctx, baseToken, `{"role_name":"`+roleName+`","role_type":"custom_role"}`)
	require.NotEmpty(t, roleID, "created role ID should be returned")

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

	t.Run("list as bot", func(t *testing.T) {
		var lastStdout string
		pollTimeout := 30 * time.Second
		pollCtx, pollCancel := context.WithTimeout(ctx, pollTimeout)
		defer pollCancel()

		err := clie2e.WaitForCondition(pollCtx, clie2e.WaitOptions{
			Timeout:  pollTimeout,
			Interval: 3 * time.Second,
		}, func() (bool, error) {
			result, err := clie2e.RunCmd(pollCtx, clie2e.Request{
				Args:      []string{"base", "+role-list", "--base-token", baseToken},
				DefaultAs: "bot",
			})
			if err != nil {
				return false, err
			}
			lastStdout = result.Stdout
			if result.ExitCode != 0 {
				return false, result.RunErr
			}
			if !gjson.Get(result.Stdout, "ok").Bool() {
				return false, nil
			}

			roleListPayload := gjson.Get(result.Stdout, "data.data").String()
			if roleListPayload == "" || !gjson.Valid(roleListPayload) {
				return false, nil
			}

			roleItems := gjson.Get(roleListPayload, "base_roles").Array()
			for _, item := range roleItems {
				rolePayload := item.String()
				if !gjson.Valid(rolePayload) {
					continue
				}
				if gjson.Get(rolePayload, "role_id").String() == roleID && gjson.Get(rolePayload, "role_name").String() == roleName {
					return true, nil
				}
			}
			return false, nil
		})
		require.NoError(t, err, "role %q should appear in list; last stdout:\n%s", roleName, lastStdout)
	})

	t.Run("get role as bot", func(t *testing.T) {
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

	t.Run("update role as bot", func(t *testing.T) {
		require.NotEmpty(t, roleID, "role ID should be resolved before update")

		updatedRoleName := roleName + " Updated"
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+role-update", "--base-token", baseToken, "--role-id", roleID, "--json", `{"role_name":"` + updatedRoleName + `","role_type":"custom_role"}`, "--yes"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		pollTimeout := 30 * time.Second
		pollCtx, pollCancel := context.WithTimeout(ctx, pollTimeout)
		defer pollCancel()

		err = clie2e.WaitForCondition(pollCtx, clie2e.WaitOptions{
			Timeout:  pollTimeout,
			Interval: 3 * time.Second,
		}, func() (bool, error) {
			getResult, getErr := clie2e.RunCmd(pollCtx, clie2e.Request{
				Args:      []string{"base", "+role-get", "--base-token", baseToken, "--role-id", roleID},
				DefaultAs: "bot",
			})
			if getErr != nil {
				return false, getErr
			}
			if getResult.ExitCode != 0 {
				return false, getResult.RunErr
			}

			rolePayload := gjson.Get(getResult.Stdout, "data.data").String()
			return gjson.Valid(rolePayload) && gjson.Get(rolePayload, "role_name").String() == updatedRoleName, nil
		})
		require.NoError(t, err, "role name should converge to %q", updatedRoleName)
	})

}
