// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestCalendar_SubscriptionWorkflow exercises subscriptions on an isolated
// public shared calendar. It needs a bot with calendar create/read/delete and
// subscribe scopes; it never changes a preexisting calendar or subscription.
func TestCalendar_SubscriptionWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	parentT := t
	var calendarID string
	summary := "lark-cli-e2e-subscription-" + clie2e.GenerateSuffix()

	t.Run("create public shared calendar as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "calendars", "create"}, DefaultAs: "bot",
			Data: map[string]any{"summary": summary, "permissions": "public"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		calendarID = gjson.Get(result.Stdout, "data.calendar.calendar_id").String()
		require.NotEmpty(t, calendarID)
		parentT.Cleanup(func() {
			cleanupCtx, cancel := clie2e.CleanupContext()
			defer cancel()
			result, err := clie2e.RunCmd(cleanupCtx, clie2e.Request{
				Args: []string{"calendar", "calendars", "delete"}, DefaultAs: "bot",
				Params: map[string]any{"calendar_id": calendarID}, Yes: true,
			})
			clie2e.ReportCleanupFailure(parentT, "delete subscription test calendar", result, err)
		})
		require.Equal(t, "shared", gjson.Get(result.Stdout, "data.calendar.type").String())
	})
	require.NotEmpty(t, calendarID)

	// Creating a shared calendar subscribes its owner. Cancel that initial
	// subscription before testing the explicit subscribe transition.
	t.Run("unsubscribe initial owner subscription as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "+unsubscribe", "--calendar-id", calendarID}, DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Empty(t, gjson.Get(result.Stdout, "data").Value())
	})

	t.Run("subscribe to shared calendar as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "+subscribe", "--calendar-id", calendarID}, DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, "bot", gjson.Get(result.Stdout, "identity").String())
		// Success carries no payload; the readback below proves the transition.
		require.Empty(t, gjson.Get(result.Stdout, "data").Value())
	})

	t.Run("read subscribed calendar as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "calendars", "get"}, DefaultAs: "bot",
			Params: map[string]any{"calendar_id": calendarID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, calendarID, gjson.Get(result.Stdout, "data.calendar_id").String())
		require.Equal(t, summary, gjson.Get(result.Stdout, "data.summary").String())
		// Get Calendar's deletion marker is optional; a returned value must
		// still be false for this newly created calendar.
		var calendar struct {
			IsDeleted *bool `json:"is_deleted"`
		}
		require.NoError(t, json.Unmarshal([]byte(gjson.Get(result.Stdout, "data").Raw), &calendar))
		if calendar.IsDeleted != nil {
			require.False(t, *calendar.IsDeleted)
		}
	})

	t.Run("unsubscribe shared calendar as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "+unsubscribe", "--calendar-id", calendarID}, DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Empty(t, gjson.Get(result.Stdout, "data").Value())
	})

	t.Run("repeat unsubscribe reports persisted state", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "+unsubscribe", "--calendar-id", calendarID}, DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 1)
		require.Empty(t, result.Stdout)
		require.Equal(t, "api", gjson.Get(result.Stderr, "error.type").String())
		require.Equal(t, int64(191007), gjson.Get(result.Stderr, "error.code").Int())
		require.Contains(t, gjson.Get(result.Stderr, "error.hint").String(), "already unsubscribed")
	})
}
