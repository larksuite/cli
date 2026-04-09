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

// createEvent creates a calendar event and registers cleanup.
// Returns the event_id.
func createEvent(t *testing.T, parentT *testing.T, ctx context.Context, calendarID string, summary string) string {
	t.Helper()

	startTime := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
	endTime := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"calendar", "+create",
			"--summary", summary,
			"--start", startTime,
			"--end", endTime,
			"--calendar-id", calendarID,
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	eventID := gjson.Get(result.Stdout, "data.event_id").String()
	require.NotEmpty(t, eventID, "stdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		deleteResult, deleteErr := clie2e.RunCmd(context.Background(), clie2e.Request{
			Args: []string{"calendar", "events", "delete"},
			Params: map[string]any{
				"calendar_id": calendarID,
				"event_id":    eventID,
			},
		})
		if deleteErr != nil {
			parentT.Errorf("delete event %s: %v", eventID, deleteErr)
			return
		}
		if deleteResult.ExitCode != 0 {
			parentT.Errorf("delete event %s failed: exit=%d stdout=%s stderr=%s", eventID, deleteResult.ExitCode, deleteResult.Stdout, deleteResult.Stderr)
		}
	})

	return eventID
}

// getPrimaryCalendarID returns the primary calendar ID.
func getPrimaryCalendarID(t *testing.T, ctx context.Context) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"calendar", "calendars", "primary"},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, 0)

	calendarID := gjson.Get(result.Stdout, "data.calendars.0.calendar.calendar_id").String()
	require.NotEmpty(t, calendarID, "stdout:\n%s", result.Stdout)

	return calendarID
}