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

func TestCalendar_UpdateEventWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	suffix := clie2e.GenerateSuffix()
	calendarID := getPrimaryCalendarID(t, ctx)
	userOpenID := getCurrentUserOpenIDForCalendar(t, ctx)

	startAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Minute)
	endAt := startAt.Add(30 * time.Minute)
	updatedStartAt := startAt.Add(30 * time.Minute)
	updatedEndAt := updatedStartAt.Add(45 * time.Minute)

	createdSummary := "lark-cli-e2e-update-before-" + suffix
	updatedSummary := "lark-cli-e2e-update-after-" + suffix
	updatedDescription := "updated by calendar update workflow"

	var eventID string
	var deletedEvent bool

	t.Run("create event as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "+create",
				"--summary", createdSummary,
				"--start", startAt.Format(time.RFC3339),
				"--end", endAt.Format(time.RFC3339),
				"--calendar-id", calendarID,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		eventID = gjson.Get(result.Stdout, "data.event_id").String()
		require.NotEmpty(t, eventID, "stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			if eventID == "" || deletedEvent {
				return
			}
			cleanupCtx, cleanupCancel := clie2e.CleanupContext()
			defer cleanupCancel()

			deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
				Args:      []string{"calendar", "events", "delete"},
				DefaultAs: "bot",
				Params: map[string]any{
					"calendar_id": calendarID,
					"event_id":    eventID,
				},
				Yes: true,
			})
			clie2e.ReportCleanupFailure(parentT, "delete event "+eventID, deleteResult, deleteErr)
		})
	})

	t.Run("update event and add attendee as bot", func(t *testing.T) {
		require.NotEmpty(t, eventID)
		require.NotEmpty(t, userOpenID)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "+update",
				"--event-id", eventID,
				"--calendar-id", calendarID,
				"--summary", updatedSummary,
				"--description", updatedDescription,
				"--start", updatedStartAt.Format(time.RFC3339),
				"--end", updatedEndAt.Format(time.RFC3339),
				"--add-attendee-ids", userOpenID,
				"--notify=false",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, eventID, gjson.Get(result.Stdout, "data.event_id").String())
	})

	t.Run("verify updated event as bot", func(t *testing.T) {
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
		assert.Equal(t, updatedSummary, gjson.Get(result.Stdout, "data.event.summary").String())
		assert.Equal(t, updatedDescription, gjson.Get(result.Stdout, "data.event.description").String())
		assert.Equal(t, unixSecondsRFC3339(updatedStartAt), gjson.Get(result.Stdout, "data.event.start_time.timestamp").String())
		assert.Equal(t, unixSecondsRFC3339(updatedEndAt), gjson.Get(result.Stdout, "data.event.end_time.timestamp").String())
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
			Yes: true,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
		deletedEvent = true
	})
}
