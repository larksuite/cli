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

func unixSecondsRFC3339(t time.Time) string {
	return strconv.FormatInt(t.Unix(), 10)
}
