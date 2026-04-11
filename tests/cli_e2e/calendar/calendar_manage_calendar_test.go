// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestCalendar_ManageCalendar tests the workflow of managing calendars.
func TestCalendar_ManageCalendar(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	calendarSummary := "lark-cli-e2e-cal-" + suffix
	updatedCalendarSummary := calendarSummary + "-updated"
	calendarDescription := "test calendar created by e2e"

	var createdCalendarID string

	t.Run("list calendars", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"calendar", "calendars", "list"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		require.NotEmpty(t, gjson.Get(result.Stdout, "data.calendar_list").Array(), "stdout:\n%s", result.Stdout)
	})

	t.Run("get primary calendar", func(t *testing.T) {
		primaryCalendarID := getPrimaryCalendarID(t, ctx)
		require.NotEmpty(t, primaryCalendarID)
	})

	t.Run("create calendar", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"calendar", "calendars", "create"},
			DefaultAs: "bot",
			Data: map[string]any{
				"summary":     calendarSummary,
				"description": calendarDescription,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		createdCalendarID = gjson.Get(result.Stdout, "data.calendar.calendar_id").String()
		require.NotEmpty(t, createdCalendarID)
	})

	t.Run("get created calendar", func(t *testing.T) {
		require.NotEmpty(t, createdCalendarID)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"calendar", "calendars", "get"},
			DefaultAs: "bot",
			Params: map[string]any{
				"calendar_id": createdCalendarID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		assert.Equal(t, createdCalendarID, gjson.Get(result.Stdout, "data.calendar_id").String())
		assert.Equal(t, calendarSummary, gjson.Get(result.Stdout, "data.summary").String())
		assert.Equal(t, calendarDescription, gjson.Get(result.Stdout, "data.description").String())
	})

	t.Run("find created calendar in list", func(t *testing.T) {
		require.NotEmpty(t, createdCalendarID)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"calendar", "calendars", "list"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		require.True(t, gjson.Get(result.Stdout, `data.calendar_list.#(calendar_id=="`+createdCalendarID+`")`).Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("update calendar", func(t *testing.T) {
		require.NotEmpty(t, createdCalendarID)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"calendar", "calendars", "patch"},
			DefaultAs: "bot",
			Params: map[string]any{
				"calendar_id": createdCalendarID,
			},
			Data: map[string]any{
				"summary": updatedCalendarSummary,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})

	t.Run("verify updated calendar", func(t *testing.T) {
		require.NotEmpty(t, createdCalendarID)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"calendar", "calendars", "get"},
			DefaultAs: "bot",
			Params: map[string]any{
				"calendar_id": createdCalendarID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		assert.Equal(t, updatedCalendarSummary, gjson.Get(result.Stdout, "data.summary").String())
	})

	t.Run("delete calendar", func(t *testing.T) {
		require.NotEmpty(t, createdCalendarID)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"calendar", "calendars", "delete"},
			DefaultAs: "bot",
			Params: map[string]any{
				"calendar_id": createdCalendarID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})
}
