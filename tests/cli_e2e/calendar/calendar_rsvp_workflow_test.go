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

func requireFreebusyEntry(t *testing.T, stdout string, startAt time.Time, endAt time.Time, expectedRSVP string) {
	t.Helper()

	var matched gjson.Result
	for _, item := range gjson.Parse(stdout).Get("data").Array() {
		itemStart, err := time.Parse(time.RFC3339, item.Get("start_time").String())
		require.NoError(t, err, "stdout:\n%s", stdout)
		itemEnd, err := time.Parse(time.RFC3339, item.Get("end_time").String())
		require.NoError(t, err, "stdout:\n%s", stdout)

		if !itemStart.Equal(startAt) || !itemEnd.Equal(endAt) {
			continue
		}
		if item.Get("rsvp_status").String() != expectedRSVP {
			continue
		}
		matched = item
		break
	}

	require.True(t, matched.Exists(), "expected freebusy entry start=%s end=%s rsvp=%s in stdout:\n%s", startAt.Format(time.RFC3339), endAt.Format(time.RFC3339), expectedRSVP, stdout)
	assert.Equal(t, expectedRSVP, matched.Get("rsvp_status").String(), "stdout:\n%s", stdout)
}

func TestCalendar_RSVPWorkflowAsUser(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	userOpenID := getCurrentUserOpenIDForCalendar(t, ctx)
	calendarID := getPrimaryCalendarID(t, ctx)
	startAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Minute)
	endAt := startAt.Add(30 * time.Minute)
	startTime := startAt.Format(time.RFC3339)
	endTime := endAt.Format(time.RFC3339)
	var eventID string

	t.Run("query freebusy as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+freebusy",
				"--start", startTime,
				"--end", endTime,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		data := gjson.Get(result.Stdout, "data")
		require.True(t, data.IsArray() || data.Type == gjson.Null, "stdout:\n%s", result.Stdout)
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

	t.Run("reply tentative as user", func(t *testing.T) {
		require.NotEmpty(t, eventID, "event should be created before RSVP")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+rsvp",
				"--calendar-id", calendarID,
				"--event-id", eventID,
				"--rsvp-status", "tentative",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, calendarID, gjson.Get(result.Stdout, "data.calendar_id").String(), "stdout:\n%s", result.Stdout)
		require.Equal(t, eventID, gjson.Get(result.Stdout, "data.event_id").String(), "stdout:\n%s", result.Stdout)
		require.Equal(t, "tentative", gjson.Get(result.Stdout, "data.rsvp_status").String())
	})

	t.Run("verify tentative freebusy as user", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+freebusy",
				"--start", startTime,
				"--end", endTime,
			},
			DefaultAs: "user",
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				if result == nil || result.ExitCode != 0 || !gjson.Get(result.Stdout, "status").Bool() {
					return true
				}
				for _, item := range gjson.Parse(result.Stdout).Get("data").Array() {
					itemStart, err := time.Parse(time.RFC3339, item.Get("start_time").String())
					if err != nil {
						return true
					}
					itemEnd, err := time.Parse(time.RFC3339, item.Get("end_time").String())
					if err != nil {
						return true
					}
					if itemStart.Equal(startAt) && itemEnd.Equal(endAt) && item.Get("rsvp_status").String() == "tentative" {
						return false
					}
				}
				return true
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		requireFreebusyEntry(t, result.Stdout, startAt, endAt, "tentative")
	})

	t.Run("reply accept as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+rsvp",
				"--calendar-id", calendarID,
				"--event-id", eventID,
				"--rsvp-status", "accept",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, calendarID, gjson.Get(result.Stdout, "data.calendar_id").String(), "stdout:\n%s", result.Stdout)
		require.Equal(t, eventID, gjson.Get(result.Stdout, "data.event_id").String(), "stdout:\n%s", result.Stdout)
		require.Equal(t, "accept", gjson.Get(result.Stdout, "data.rsvp_status").String())
	})

	t.Run("verify accepted freebusy as user", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+freebusy",
				"--start", startTime,
				"--end", endTime,
			},
			DefaultAs: "user",
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				if result == nil || result.ExitCode != 0 || !gjson.Get(result.Stdout, "status").Bool() {
					return true
				}
				for _, item := range gjson.Parse(result.Stdout).Get("data").Array() {
					itemStart, err := time.Parse(time.RFC3339, item.Get("start_time").String())
					if err != nil {
						return true
					}
					itemEnd, err := time.Parse(time.RFC3339, item.Get("end_time").String())
					if err != nil {
						return true
					}
					if itemStart.Equal(startAt) && itemEnd.Equal(endAt) && item.Get("rsvp_status").String() == "accept" {
						return false
					}
				}
				return true
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		requireFreebusyEntry(t, result.Stdout, startAt, endAt, "accept")
	})
}
