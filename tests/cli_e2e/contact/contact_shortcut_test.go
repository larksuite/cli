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

func TestContact_GetUser_BotWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	var targetOpenID string

	t.Run("discover-user-via-api", func(t *testing.T) {
		// Bot identity cannot use +search-user or +get-user (self).
		// However, it CAN call the raw API to list users if it has contact permissions.
		// We use this to discover a real open_id for the next step.
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"api", "get", "/open-apis/contact/v3/users"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		targetOpenID = gjson.Get(result.Stdout, "data.items.0.open_id").String()

		require.NotEmpty(t, targetOpenID, "expected to find at least one user via raw API")
	})

	t.Run("get-user-by-id-as-bot", func(t *testing.T) {
		require.NotEmpty(t, targetOpenID, "targetOpenID should be populated")
		// DefaultAs is automatically "bot" in the clie2e framework
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"contact", "+get-user", "--user-id", targetOpenID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		returnedID := gjson.Get(result.Stdout, "data.user.open_id").String()
		require.Equal(t, targetOpenID, returnedID)
	})
}
