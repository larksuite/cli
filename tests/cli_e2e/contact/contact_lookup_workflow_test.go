// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
)

func TestContact_LookupWorkflowAsUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	var selfOpenID string

	t.Run("get self as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"contact", "+get-user"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		selfOpenID = gjson.Get(result.Stdout, "data.user.open_id").String()
		require.NotEmpty(t, selfOpenID, "stdout:\n%s", result.Stdout)
	})

	t.Run("get self by open id as user", func(t *testing.T) {
		require.NotEmpty(t, selfOpenID, "self open_id should be populated before get-by-id")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"contact", "+get-user", "--user-id", selfOpenID},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		require.Equal(t, selfOpenID, gjson.Get(result.Stdout, "data.user.user_id").String(), "stdout:\n%s", result.Stdout)
	})
}

func TestContact_LookupWorkflowAsBot(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	var targetOpenID string

	t.Run("discover user via api as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"api", "get", "/open-apis/contact/v3/users", "--params", `{"department_id":"0","page_size":10}`},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			stderrLower := strings.ToLower(result.Stderr)
			if strings.Contains(stderrLower, "permission denied") || strings.Contains(stderrLower, "99991679") {
				t.Skipf("skip bot contact workflow due to missing bot contact permissions: %s", result.Stderr)
			}
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		items := gjson.Get(result.Stdout, "data.items")
		require.True(t, items.Exists(), "expected contact API response to include data.items; stdout:\n%s", result.Stdout)
		if len(items.Array()) == 0 {
			t.Skip("skip bot contact workflow because the live test tenant has no users visible to the bot")
		}

		targetOpenID = items.Get("0.open_id").String()
		require.NotEmpty(t, targetOpenID, "expected to find at least one user via raw API")
	})

	t.Run("get user by open id as bot", func(t *testing.T) {
		if targetOpenID == "" {
			t.Skip("skip bot get-user-by-id because discover-user-via-api did not provide targetOpenID")
		}

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"contact", "+get-user", "--user-id", targetOpenID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		require.Equal(t, targetOpenID, gjson.Get(result.Stdout, "data.user.open_id").String(), "stdout:\n%s", result.Stdout)
	})
}
