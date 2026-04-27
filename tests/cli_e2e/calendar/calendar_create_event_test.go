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
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	eventSummary := "lark-cli-e2e-event-" + suffix
	eventDescription := "test event description"

	startAt := time.Now().UTC().Add(1 * time.Hour).Truncate(time.Minute)
	endAt := startAt.Add(1 * time.Hour)
	startTime := startAt.Format(time.RFC3339)
	endTime := endAt.Format(time.RFC3339)

	var eventID string
	var deletedEvent bool
	calendarID := getPrimaryCalendarID(t, ctx)

	t.Run("create event with shortcut as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "+create",
				"--summary", eventSummary,
				"--start", startTime,
				"--end", endTime,
				"--calendar-id", calendarID,
				"--description", eventDescription,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		eventID = gjson.Get(result.Stdout, "data.event_id").String()
		require.NotEmpty(t, eventID)

		parentT.Cleanup(func() {
			if eventID == "" || deletedEvent {
				return
			}

			cleanupCtx, cancel := clie2e.CleanupContext()
			defer cancel()

			deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
				Args:      []string{"calendar", "events", "delete"},
				DefaultAs: "bot",
				Params: map[string]any{
					"calendar_id": calendarID,
					"event_id":    eventID,
				},
			})
			clie2e.ReportCleanupFailure(parentT, "delete event "+eventID, deleteResult, deleteErr)
		})
	})

	t.Run("verify event created as bot", func(t *testing.T) {
		require.NotEmpty(t, eventID)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"calendar", "events", "get"},
			DefaultAs: "bot",
			Params: map[string]any{
				"calendar_id": calendarID,
				"event_id":    eventID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		assert.Equal(t, eventSummary, gjson.Get(result.Stdout, "data.event.summary").String())
		assert.Equal(t, eventDescription, gjson.Get(result.Stdout, "data.event.description").String())
		assert.Equal(t, unixSecondsRFC3339(startAt), gjson.Get(result.Stdout, "data.event.start_time.timestamp").String())
		assert.Equal(t, unixSecondsRFC3339(endAt), gjson.Get(result.Stdout, "data.event.end_time.timestamp").String())
	})

	t.Run("delete event as bot", func(t *testing.T) {
		require.NotEmpty(t, eventID)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"calendar", "events", "delete"},
			DefaultAs: "bot",
			Params: map[string]any{
				"calendar_id": calendarID,
				"event_id":    eventID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		deletedEvent = true
	})
}
