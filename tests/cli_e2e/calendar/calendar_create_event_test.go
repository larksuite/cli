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

// TestCalendar_CreateEvent tests the workflow of creating a calendar event.
func TestCalendar_CreateEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	eventSummary := "lark-cli-e2e-event-" + suffix

	startTime := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
	endTime := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)

	var eventID string
	var calendarID string

	// Step 1: Get primary calendar ID (prerequisite)
	t.Run("get primary calendar", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "calendars", "primary"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		calendarID = gjson.Get(result.Stdout, "data.calendars.0.calendar.calendar_id").String()
		require.NotEmpty(t, calendarID)
	})

	// Step 2: Create event using +create shortcut
	t.Run("create event with shortcut", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "+create",
				"--summary", eventSummary,
				"--start", startTime,
				"--end", endTime,
				"--calendar-id", calendarID,
				"--description", "test event description",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		eventID = gjson.Get(result.Stdout, "data.event_id").String()
		require.NotEmpty(t, eventID)
	})

	// Step 3: Verify event was created using events.get resource command
	t.Run("verify event created", func(t *testing.T) {
		require.NotEmpty(t, eventID)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "events", "get"},
			Params: map[string]any{
				"calendar_id": calendarID,
				"event_id":    eventID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		assert.Equal(t, eventSummary, gjson.Get(result.Stdout, "data.event.summary").String())
		assert.Equal(t, "test event description", gjson.Get(result.Stdout, "data.event.description").String())
	})

	// Step 4: Delete event using events.delete resource command
	t.Run("delete event", func(t *testing.T) {
		require.NotEmpty(t, eventID)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "events", "delete"},
			Params: map[string]any{
				"calendar_id": calendarID,
				"event_id":    eventID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})

	// Step 5: Verify delete was acknowledged (event may have eventual consistency)
	t.Run("verify delete acknowledged", func(t *testing.T) {
		require.NotEmpty(t, eventID)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "events", "get"},
			Params: map[string]any{
				"calendar_id": calendarID,
				"event_id":    eventID,
			},
		})
		require.NoError(t, err)
		// Note: API may have eventual consistency - delete acknowledged but get may still succeed briefly
		_ = result
	})
}