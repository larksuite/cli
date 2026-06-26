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

func TestCalendar_CreateAllDayDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"calendar", "+create",
			"--summary", "Conference",
			"--start", "2026-05-18",
			"--end", "2026-05-21",
			"--all-day",
			"--calendar-id", "cal_dry",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/calendar/v4/calendars/cal_dry/events", gjson.Get(out, "api.0.url").String(), "stdout:\n%s", out)
	require.Equal(t, "Conference", gjson.Get(out, "api.0.body.summary").String(), "stdout:\n%s", out)
	require.Equal(t, "2026-05-18", gjson.Get(out, "api.0.body.start_time.date").String(), "stdout:\n%s", out)
	require.False(t, gjson.Get(out, "api.0.body.start_time.timestamp").Exists(), "stdout:\n%s", out)
	require.Equal(t, "2026-05-21", gjson.Get(out, "api.0.body.end_time.date").String(), "stdout:\n%s", out)
	require.False(t, gjson.Get(out, "api.0.body.end_time.timestamp").Exists(), "stdout:\n%s", out)
	require.Equal(t, "free", gjson.Get(out, "api.0.body.free_busy_status").String(), "stdout:\n%s", out)
	require.Equal(t, "no_meeting", gjson.Get(out, "api.0.body.vchat.vc_type").String(), "stdout:\n%s", out)
}
