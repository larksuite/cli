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

func configureCalendarSubscriptionDryRun(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
	t.Setenv("LARKSUITE_CLI_STRICT_MODE", "off")
	t.Setenv("LARKSUITE_CLI_USER_ACCESS_TOKEN", "")
	t.Setenv("LARKSUITE_CLI_AGENT_ACCESS_TOKEN", "")
	t.Setenv("LARKSUITE_CLI_TENANT_ACCESS_TOKEN", "")
	t.Setenv("LARKSUITE_CLI_TENANT_ACCESS_TOKEN_SOURCE", "")
}

func TestCalendar_SubscriptionDryRun(t *testing.T) {
	configureCalendarSubscriptionDryRun(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	for _, command := range []string{"+subscribe", "+unsubscribe"} {
		t.Run(command, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      []string{"calendar", command, "--calendar-id", "cal_dry@group.calendar.example", "--dry-run"},
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			result.AssertStdoutStatus(t, true)
			require.Equal(t, "bot", gjson.Get(result.Stdout, "identity").String())
			require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			require.Equal(t, "/open-apis/calendar/v4/calendars/cal_dry@group.calendar.example/"+command[1:],
				clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			require.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params").Exists())
			require.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body").Exists())
			require.False(t, clie2e.DryRunGet(result.Stdout, "api.1").Exists())
		})
	}
}

func TestCalendar_SubscriptionRequiresCalendarID(t *testing.T) {
	configureCalendarSubscriptionDryRun(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	for _, command := range []string{"+subscribe", "+unsubscribe"} {
		t.Run(command, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      []string{"calendar", command, "--dry-run"},
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			require.Empty(t, result.Stdout)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
			require.Contains(t, result.Stderr, "calendar-id")
		})
	}
}

func TestCalendar_SubscriptionGuidanceEmbedded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	for _, command := range []string{"+subscribe", "+unsubscribe"} {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"calendar", command, "--help"}})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "Risk: write")
		require.Contains(t, result.Stdout, "lark-calendar")
	}
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"skills", "read", "lark-calendar"},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Contains(t, result.Stdout, "+subscribe")
	require.Contains(t, result.Stdout, "+unsubscribe")
}
