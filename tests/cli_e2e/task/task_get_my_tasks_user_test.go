// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

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
//	| `precheck user identity` | `auth status` |
//	| `get my tasks with --as user` | `task +get-my-tasks` |
func TestTask_GetMyTasks_User(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	var userOpenID string

	t.Run("precheck user identity", func(t *testing.T) {
		statusResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"auth", "status"},
		})
		require.NoError(t, err)
		if statusResult.ExitCode != 0 {
			t.Skipf("requires user-capable environment; auth status failed: stderr=%s stdout=%s", statusResult.Stderr, statusResult.Stdout)
		}

		userOpenID = gjson.Get(statusResult.Stdout, "userOpenId").String()
		if userOpenID == "" {
			t.Skipf("requires user-capable environment with logged-in user; auth status: %s", statusResult.Stdout)
		}
	})

	t.Run("get my tasks with --as user", func(t *testing.T) {
		if userOpenID == "" {
			t.Skip("requires user-capable environment with logged-in user")
		}

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"task", "+get-my-tasks", "--as", "user"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, "user", gjson.Get(result.Stdout, "identity").String(), "stdout:\n%s", result.Stdout)
		assert.True(t, gjson.Get(result.Stdout, "data.items").IsArray(), "stdout:\n%s", result.Stdout)
		assert.True(t, gjson.Get(result.Stdout, "data.has_more").Exists(), "stdout:\n%s", result.Stdout)
	})
}
