// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"context"
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
