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

// raw_busy retains RSVP state; the default busy view merges intervals and drops it.
func hasFreebusyEntry(stdout, userID string, startAt, endAt time.Time, expectedRSVP string) bool {
	for _, user := range gjson.Get(stdout, "data.users").Array() {
		if user.Get("user_id").String() != userID {
			continue
		}
		for _, item := range user.Get("raw_busy").Array() {
			start, startErr := time.Parse(time.RFC3339, item.Get("start_time").String())
			end, endErr := time.Parse(time.RFC3339, item.Get("end_time").String())
			if startErr == nil && endErr == nil && start.Equal(startAt) && end.Equal(endAt) && item.Get("rsvp_status").String() == expectedRSVP {
				return true
			}
		}
	}
	return false
}

func TestCalendar_RSVPWorkflowAsUser(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	userOpenID := getCurrentUserOpenIDForCalendar(t, ctx)
	calendarID := getPrimaryCalendarID(t, ctx)
	userCalendarID := getCurrentUserPrimaryCalendarID(t, ctx)
	startAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Minute)
	endAt := startAt.Add(30 * time.Minute)
	startTime := startAt.Format(time.RFC3339)
	endTime := endAt.Format(time.RFC3339)
	var eventID string

	t.Run("query freebusy as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+freebusy",
				"--type", "raw_busy",
				"--start", startTime,
				"--end", endTime,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		users := gjson.Get(result.Stdout, "data.users")
		require.True(t, users.IsArray(), "stdout:\n%s", result.Stdout)
	})

	t.Run("create invite-only event as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+create",
				"--summary", "lark-cli-e2e-calendar-rsvp-" + clie2e.GenerateSuffix(),
				"--start", startTime,
				"--end", endTime,
				"--calendar-id", calendarID,
				"--attendee-ids", userOpenID,
			},
			DefaultAs: "bot",
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
				DefaultAs: "bot",
				Params: map[string]any{
					"calendar_id": calendarID,
					"event_id":    eventID,
				},
			})
			clie2e.ReportCleanupFailure(parentT, "delete event "+eventID, deleteResult, deleteErr)
		})
	})

	// Invitations are copied to the attendee's primary calendar asynchronously.
	// RSVP must target that calendar, not the organizer bot's calendar.
	t.Run("wait for invitation as user", func(t *testing.T) {
		require.NotEmpty(t, eventID)
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args:      []string{"calendar", "events", "get"},
			DefaultAs: "user",
			Params:    map[string]any{"calendar_id": userCalendarID, "event_id": eventID},
		}, clie2e.RetryOptions{})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, eventID, gjson.Get(result.Stdout, "data.event.event_id").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("reply tentative as user", func(t *testing.T) {
		require.NotEmpty(t, eventID, "event should be created before RSVP")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+rsvp",
				"--calendar-id", userCalendarID,
				"--event-id", eventID,
				"--rsvp-status", "tentative",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, userCalendarID, gjson.Get(result.Stdout, "data.calendar_id").String(), "stdout:\n%s", result.Stdout)
		require.Equal(t, eventID, gjson.Get(result.Stdout, "data.event_id").String(), "stdout:\n%s", result.Stdout)
		require.Equal(t, "tentative", gjson.Get(result.Stdout, "data.rsvp_status").String())
	})

	t.Run("verify tentative freebusy as user", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+freebusy",
				"--type", "raw_busy",
				"--start", startTime,
				"--end", endTime,
			},
			DefaultAs: "user",
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				if result == nil || result.ExitCode != 0 || !gjson.Get(result.Stdout, "ok").Bool() {
					return false
				}
				return !hasFreebusyEntry(result.Stdout, userOpenID, startAt, endAt, "tentative")
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.True(t, hasFreebusyEntry(result.Stdout, userOpenID, startAt, endAt, "tentative"), "expected tentative entry; stdout:\n%s", result.Stdout)
	})

	t.Run("reply accept as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+rsvp",
				"--calendar-id", userCalendarID,
				"--event-id", eventID,
				"--rsvp-status", "accept",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, userCalendarID, gjson.Get(result.Stdout, "data.calendar_id").String(), "stdout:\n%s", result.Stdout)
		require.Equal(t, eventID, gjson.Get(result.Stdout, "data.event_id").String(), "stdout:\n%s", result.Stdout)
		require.Equal(t, "accept", gjson.Get(result.Stdout, "data.rsvp_status").String())
	})

	t.Run("verify accepted freebusy as user", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+freebusy",
				"--type", "raw_busy",
				"--start", startTime,
				"--end", endTime,
			},
			DefaultAs: "user",
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				if result == nil || result.ExitCode != 0 || !gjson.Get(result.Stdout, "ok").Bool() {
					return false
				}
				return !hasFreebusyEntry(result.Stdout, userOpenID, startAt, endAt, "accept")
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.True(t, hasFreebusyEntry(result.Stdout, userOpenID, startAt, endAt, "accept"), "expected accept entry; stdout:\n%s", result.Stdout)
	})
}
