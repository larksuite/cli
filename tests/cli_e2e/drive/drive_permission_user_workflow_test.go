// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDrive_PermissionMembersAuthWorkflow tests the permission.members.auth resource command.
// Workflow: import a doc -> check auth permissions on the doc.
func TestDrive_PermissionMembersAuthWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	testContent := "# Lark CLI E2E Permission Auth Test\n\nDocument for testing permission.members.auth.\nTimestamp: " + suffix

	docToken := importTestDoc(t, parentT, ctx, "permission-auth", testContent)
	require.NotEmpty(t, docToken)

	t.Run("check view permission", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "permission.members", "auth"},
			Params: map[string]any{
				"token": docToken,
				"type":  "docx",
				"action": "view",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		authResult := gjson.Get(result.Stdout, "data.auth_result")
		require.True(t, authResult.Bool(), "should have view permission on own doc, stdout:\n%s", result.Stdout)
	})

	t.Run("check edit permission", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "permission.members", "auth"},
			Params: map[string]any{
				"token": docToken,
				"type":  "docx",
				"action": "edit",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		authResult := gjson.Get(result.Stdout, "data.auth_result")
		require.True(t, authResult.Bool(), "should have edit permission on own doc, stdout:\n%s", result.Stdout)
	})
}

// TestDrive_UserSubscriptionWorkflow tests the user subscription commands.
// Workflow: subscribe to comment events -> check status -> remove subscription.
func TestDrive_UserSubscriptionWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	eventType := "drive.notice.comment_add_v1"

	// Step 1: Subscribe to comment events
	t.Run("subscribe to comment events", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "user", "subscription"},
			Data: map[string]any{
				"event_type": eventType,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0) // Returns code: 0, not ok: true
	})

	// Step 2: Check subscription status
	t.Run("check subscription status", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "user", "subscription_status"},
			Params: map[string]any{
				"event_type": eventType,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		// The response should indicate subscription status
		status := gjson.Get(result.Stdout, "data")
		require.NotEmpty(t, status.Raw, "subscription status should be returned, stdout:\n%s", result.Stdout)
	})

	// Step 3: Remove subscription
	t.Run("remove subscription", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "user", "remove_subscription"},
			Params: map[string]any{
				"event_type": eventType,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0) // Returns code: 0, not ok: true
	})
}

// TestDrive_PermissionMembersTransferOwnerWorkflow tests permission.members.transfer_owner.
// Note: This requires a real user open_id to transfer ownership to.
// This test is skipped as it requires user identity and a valid target user.
func TestDrive_PermissionMembersTransferOwnerWorkflow(t *testing.T) {
	t.Skip("requires a real user open_id and user-capable test environment; permission.members.transfer_owner needs a valid target user ID")
}