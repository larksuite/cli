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

func TestCalendar_TransferDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"calendar", "+transfer",
			"--calendar-id", "cal_dry",
			"--event-id", "uid_dry_1742515200",
			"--to-user-id", "ou_receiver",
			"--remove-original-organizer",
			"--yes",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	// api.0 = the event pre-read; api.1 = the transfer itself. Asking for the
	// removal settles the outcome, so no calendar type read is planned.
	require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "/open-apis/calendar/v4/calendars/cal_dry/events/uid_dry_1742515200",
		clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.1.method").String(), out)
	require.Equal(t, "/open-apis/calendar/v4/calendars/cal_dry/events/uid_dry_1742515200/transfer",
		clie2e.DryRunGet(out, "api.1.url").String(), out)
	require.Equal(t, "open_id", clie2e.DryRunGet(out, "api.1.params.user_id_type").String(), out)
	require.Equal(t, "ou_receiver", clie2e.DryRunGet(out, "api.1.body.to_user_id").String(), out)
	require.True(t, clie2e.DryRunGet(out, "api.1.body.need_remove_original_organizer").Bool(), out)
	require.False(t, clie2e.DryRunGet(out, "api.2").Exists(), out)
}

// Keeping the original organizer is decided by the server, not by an extra
// read: the plan is still the recurrence pre-read plus the transfer.
func TestCalendar_TransferKeepOrganizerDryRun_ReadsNoCalendar(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"calendar", "+transfer",
			"--calendar-id", "cal_dry",
			"--event-id", "uid_dry_1742515200",
			"--to-user-id", "ou_receiver",
			"--yes",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "/open-apis/calendar/v4/calendars/cal_dry/events/uid_dry_1742515200",
		clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.1.method").String(), out)
	require.Equal(t, "/open-apis/calendar/v4/calendars/cal_dry/events/uid_dry_1742515200/transfer",
		clie2e.DryRunGet(out, "api.1.url").String(), out)
	require.False(t, clie2e.DryRunGet(out, "api.1.body.need_remove_original_organizer").Bool(), out)
	require.False(t, clie2e.DryRunGet(out, "api.2").Exists(), out)
}

// --transfer-series skips the recurrence pre-read. An omitted --calendar-id
// targets the primary calendar, which needs no calendar type read either.
func TestCalendar_TransferSeriesDryRun_SkipsPreRead(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"calendar", "+transfer",
			"--event-id", "uid_dry_1742515200",
			"--to-user-id", "ou_receiver",
			"--transfer-series",
			"--yes",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "/open-apis/calendar/v4/calendars/primary/events/uid_dry_1742515200/transfer",
		clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.False(t, clie2e.DryRunGet(out, "api.1").Exists(), out)
}
