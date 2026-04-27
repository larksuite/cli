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

func TestCalendar_UpdateDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"calendar", "+update",
			"--calendar-id", "cal_dry",
			"--event-id", "evt_dry",
			"--summary", "updated dry-run",
			"--start", "2026-04-25T10:00:00+08:00",
			"--end", "2026-04-25T11:00:00+08:00",
			"--remove-attendee-ids", "ou_old,omm_oldroom",
			"--add-attendee-ids", "ou_new,oc_group,omm_newroom",
			"--notify=false",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "PATCH", gjson.Get(out, "api.0.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/calendar/v4/calendars/cal_dry/events/evt_dry", gjson.Get(out, "api.0.url").String(), "stdout:\n%s", out)
	require.Equal(t, "updated dry-run", gjson.Get(out, "api.0.body.summary").String(), "stdout:\n%s", out)
	require.False(t, gjson.Get(out, "api.0.body.need_notification").Bool(), "stdout:\n%s", out)

	require.Equal(t, "POST", gjson.Get(out, "api.1.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/calendar/v4/calendars/cal_dry/events/evt_dry/attendees/batch_delete", gjson.Get(out, "api.1.url").String(), "stdout:\n%s", out)
	require.Equal(t, "ou_old", gjson.Get(out, `api.1.body.delete_ids.#(type=="user").user_id`).String(), "stdout:\n%s", out)
	require.Equal(t, "omm_oldroom", gjson.Get(out, `api.1.body.delete_ids.#(type=="resource").room_id`).String(), "stdout:\n%s", out)

	require.Equal(t, "POST", gjson.Get(out, "api.2.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/calendar/v4/calendars/cal_dry/events/evt_dry/attendees", gjson.Get(out, "api.2.url").String(), "stdout:\n%s", out)
	require.Equal(t, "ou_new", gjson.Get(out, `api.2.body.attendees.#(type=="user").user_id`).String(), "stdout:\n%s", out)
	require.Equal(t, "oc_group", gjson.Get(out, `api.2.body.attendees.#(type=="chat").chat_id`).String(), "stdout:\n%s", out)
	require.Equal(t, "omm_newroom", gjson.Get(out, `api.2.body.attendees.#(type=="resource").room_id`).String(), "stdout:\n%s", out)
}
