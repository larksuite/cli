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

func TestCalendar_ViewAgenda(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)
	calendarID := getCurrentUserPrimaryCalendarID(t, ctx)

	t.Run("view today agenda as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"calendar", "+agenda", "--calendar-id", calendarID},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, "data").IsArray(), "stdout:\n%s", result.Stdout)
	})

	t.Run("view agenda with date range as user", func(t *testing.T) {
		startDate := time.Now().UTC().Format("2006-01-02")
		endDate := time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"calendar", "+agenda", "--calendar-id", calendarID, "--start", startDate, "--end", endDate},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, "data").IsArray(), "stdout:\n%s", result.Stdout)
	})

	t.Run("view agenda with pretty format as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"calendar", "+agenda"},
			DefaultAs: "user",
			Format:    "pretty",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
	})
}
