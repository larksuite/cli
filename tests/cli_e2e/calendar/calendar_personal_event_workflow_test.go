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

func TestCalendar_PersonalEventWorkflowAsUser(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	suffix := clie2e.GenerateSuffix()
	eventSummary := "lark-cli-e2e-personal-event-" + suffix
	eventDescription := "created by calendar personal event workflow"
	startAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Minute)
	endAt := startAt.Add(30 * time.Minute)
	startTime := startAt.Format(time.RFC3339)
	endTime := endAt.Format(time.RFC3339)

	var calendarID string
	var eventID string

	t.Run("get primary calendar as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"calendar", "calendars", "primary"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		calendarID = gjson.Get(result.Stdout, "data.calendars.0.calendar.calendar_id").String()
		require.NotEmpty(t, calendarID, "stdout:\n%s", result.Stdout)
	})

	t.Run("create personal event with shortcut as user", func(t *testing.T) {
		require.NotEmpty(t, calendarID, "calendar should be loaded before creating an event")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+create",
				"--summary", eventSummary,
				"--start", startTime,
				"--end", endTime,
				"--calendar-id", calendarID,
				"--description", eventDescription,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		eventID = gjson.Get(result.Stdout, "data.event_id").String()
		require.NotEmpty(t, eventID, "stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			cleanupCtx, cancel := clie2e.CleanupContext()
			defer cancel()

			deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
				Args:      []string{"calendar", "events", "delete"},
				DefaultAs: "user",
				Params: map[string]any{
					"calendar_id": calendarID,
					"event_id":    eventID,
				},
				Yes: true,
			})
			clie2e.ReportCleanupFailure(parentT, "delete event "+eventID, deleteResult, deleteErr)
		})
	})

	t.Run("get created event as user", func(t *testing.T) {
		require.NotEmpty(t, calendarID, "calendar should be loaded before getting an event")
		require.NotEmpty(t, eventID, "event should be created before reading it back")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"calendar", "events", "get"},
			DefaultAs: "user",
			Params: map[string]any{
				"calendar_id": calendarID,
				"event_id":    eventID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, eventID, gjson.Get(result.Stdout, "data.event.event_id").String())
		assert.Equal(t, eventSummary, gjson.Get(result.Stdout, "data.event.summary").String())
		assert.Equal(t, eventDescription, gjson.Get(result.Stdout, "data.event.description").String())
		assert.Equal(t, unixSecondsRFC3339(startAt), gjson.Get(result.Stdout, "data.event.start_time.timestamp").String())
		assert.Equal(t, unixSecondsRFC3339(endAt), gjson.Get(result.Stdout, "data.event.end_time.timestamp").String())
	})

	t.Run("find created event in agenda as user", func(t *testing.T) {
		require.NotEmpty(t, eventID, "event should be created before checking agenda")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+agenda",
				"--start", startTime,
				"--end", endTime,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		matchedEvent := gjson.Get(result.Stdout, `data.#(event_id=="`+eventID+`")`)
		require.True(t, matchedEvent.Exists(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, eventSummary, matchedEvent.Get("summary").String())

		agendaStart, parseErr := time.Parse(time.RFC3339, matchedEvent.Get("start_time.datetime").String())
		require.NoError(t, parseErr, "stdout:\n%s", result.Stdout)
		agendaEnd, parseErr := time.Parse(time.RFC3339, matchedEvent.Get("end_time.datetime").String())
		require.NoError(t, parseErr, "stdout:\n%s", result.Stdout)
		assert.True(t, agendaStart.Equal(startAt), "stdout:\n%s", result.Stdout)
		assert.True(t, agendaEnd.Equal(endAt), "stdout:\n%s", result.Stdout)
	})
}
