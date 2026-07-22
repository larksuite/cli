// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"fmt"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCalendar_SearchEventDryRunAsBot(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"calendar", "+search-event",
			"--calendar-id", "cal_dry",
			"--query", "calendar-e2e-search",
			"--start", "2026-07-22T10:00:00+08:00",
			"--end", "2026-07-22T11:00:00+08:00",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "/open-apis/calendar/v4/calendars/cal_dry/events/search_event", clie2e.DryRunGet(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, "cal_dry", clie2e.DryRunGet(result.Stdout, "calendar_id").String(), "stdout:\n%s", result.Stdout)
}

func TestCalendar_SearchEventWorkflowAsBot(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	calendarID := getPrimaryCalendarID(t, ctx)
	eventSummary := "calendar-e2e-search-" + clie2e.GenerateSuffix()
	startAt := time.Now().UTC().Add(time.Hour).Truncate(time.Minute)
	endAt := startAt.Add(30 * time.Minute)

	var eventID string
	t.Run("create searchable event as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+create",
				"--calendar-id", calendarID,
				"--summary", eventSummary,
				"--start", startAt.Format(time.RFC3339),
				"--end", endAt.Format(time.RFC3339),
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		eventID = gjson.Get(result.Stdout, "data.event_id").String()
		require.NotEmpty(t, eventID, "stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			if eventID == "" {
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
			})
			clie2e.ReportCleanupFailure(parentT, "delete event "+eventID, deleteResult, deleteErr)
		})
	})

	t.Run("find created event with shortcut as bot", func(t *testing.T) {
		require.NotEmpty(t, eventID)

		var lastResult *clie2e.Result
		err := clie2e.WaitForCondition(ctx, clie2e.WaitOptions{
			Timeout:  30 * time.Second,
			Interval: 2 * time.Second,
			TimeoutError: func() error {
				if lastResult == nil {
					return fmt.Errorf("created event was not searchable before timeout")
				}
				return fmt.Errorf("created event was not searchable before timeout; stdout=%s stderr=%s", lastResult.Stdout, lastResult.Stderr)
			},
		}, func() (bool, error) {
			result, runErr := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"calendar", "+search-event",
					"--calendar-id", calendarID,
					"--query", eventSummary,
					"--start", startAt.Add(-time.Minute).Format(time.RFC3339),
					"--end", endAt.Add(time.Minute).Format(time.RFC3339),
				},
				DefaultAs: "bot",
			})
			if runErr != nil {
				return false, runErr
			}
			lastResult = result
			if result.ExitCode != 0 {
				return false, fmt.Errorf("search command failed: exit=%d stderr=%s", result.ExitCode, result.Stderr)
			}
			return gjson.Get(result.Stdout, `data.items.#(event_id=="`+eventID+`").event_id`).String() == eventID, nil
		})
		require.NoError(t, err)
	})
}
