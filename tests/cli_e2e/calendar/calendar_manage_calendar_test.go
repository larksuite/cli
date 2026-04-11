// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestCalendar_ManageCalendar tests the workflow of managing calendars.
func TestCalendar_ManageCalendar(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	calendarSummary := "lark-cli-e2e-cal-" + suffix

	var createdCalendarID string

	t.Run("list calendars", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "calendars", "list"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})

	t.Run("get primary calendar", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "calendars", "primary"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})

	t.Run("create calendar", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "calendars", "create"},
			Data: map[string]any{
				"summary":     calendarSummary,
				"description": "test calendar created by e2e",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		createdCalendarID = gjson.Get(result.Stdout, "data.calendar.calendar_id").String()
		require.NotEmpty(t, createdCalendarID)
	})

	t.Run("update calendar", func(t *testing.T) {
		require.NotEmpty(t, createdCalendarID)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "calendars", "patch"},
			Params: map[string]any{
				"calendar_id": createdCalendarID,
			},
			Data: map[string]any{
				"summary": calendarSummary + "-updated",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})

	t.Run("search calendar", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "calendars", "search"},
			Data: map[string]any{
				"query": calendarSummary,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})

	t.Run("delete calendar", func(t *testing.T) {
		require.NotEmpty(t, createdCalendarID)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "calendars", "delete"},
			Params: map[string]any{
				"calendar_id": createdCalendarID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})
}