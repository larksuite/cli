// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"strconv"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func getPrimaryCalendarID(t *testing.T, ctx context.Context) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"calendar", "calendars", "primary"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, 0)

	calendarID := gjson.Get(result.Stdout, "data.calendars.0.calendar.calendar_id").String()
	require.NotEmpty(t, calendarID, "stdout:\n%s", result.Stdout)
	return calendarID
}

func getCurrentUserPrimaryCalendarID(t *testing.T, ctx context.Context) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"calendar", "calendars", "primary"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, 0)

	calendarID := gjson.Get(result.Stdout, "data.calendars.0.calendar.calendar_id").String()
	require.NotEmpty(t, calendarID, "stdout:\n%s", result.Stdout)
	return calendarID
}

func getCurrentUserOpenIDForCalendar(t *testing.T, ctx context.Context) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"contact", "+get-user"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	openID := gjson.Get(result.Stdout, "data.user.open_id").String()
	require.NotEmpty(t, openID, "stdout:\n%s", result.Stdout)
	return openID
}

func findCalendarByID(t *testing.T, ctx context.Context, calendarID string) gjson.Result {
	t.Helper()

	require.NotEmpty(t, calendarID, "calendar ID is required")

	pageToken := ""
	seenPageTokens := map[string]struct{}{}
	for {
		params := map[string]any{
			"page_size": 50,
		}
		if pageToken != "" {
			if _, seen := seenPageTokens[pageToken]; seen {
				t.Fatalf("calendar list pagination loop detected for calendar %q, repeated page_token %q", calendarID, pageToken)
			}
			seenPageTokens[pageToken] = struct{}{}
			params["page_token"] = pageToken
		}

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"calendar", "calendars", "list"},
			DefaultAs: "bot",
			Params:    params,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		calendar := gjson.Get(result.Stdout, `data.calendar_list.#(calendar_id=="`+calendarID+`")`)
		if calendar.Exists() {
			return calendar
		}

		hasMore := gjson.Get(result.Stdout, "data.has_more").Bool()
		pageToken = gjson.Get(result.Stdout, "data.page_token").String()
		if !hasMore || pageToken == "" {
			t.Fatalf("calendar %q not found in listed pages, last stdout:\n%s", calendarID, result.Stdout)
		}
	}
}

func unixSecondsRFC3339(t time.Time) string {
	return strconv.FormatInt(t.Unix(), 10)
}
