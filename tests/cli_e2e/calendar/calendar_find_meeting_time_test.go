// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

// TestCalendar_FindMeetingTime tests the workflow of finding available meeting times.
func TestCalendar_FindMeetingTime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	startTime := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
	endTime := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02T15:04:05Z")

	t.Run("find available meeting times", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "+suggestion",
				"--start", startTime,
				"--end", endTime,
				"--duration-minutes", "30",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("find meeting times with timezone", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "+suggestion",
				"--start", startTime,
				"--end", endTime,
				"--duration-minutes", "60",
				"--timezone", "Asia/Shanghai",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("find meeting times with attendees", func(t *testing.T) {
		t.Skip("requires valid attendee open_id (ou_xxx) - using placeholder will cause API error")
	})
}