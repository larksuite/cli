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

// TestCalendar_FreebusyDryRun asserts the freebusy shortcut's outgoing request
// shape without hitting the API. Covers all four --type values plus
// --min-duration passthrough.
func TestCalendar_FreebusyDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	cases := []struct {
		name          string
		typ           string
		userIDs       []string
		minDuration   string
		wantUsersLen  int
		wantMinInBody bool
	}{
		{"type=busy default view", "busy", []string{"ou_dry_a"}, "", 1, false},
		{"type=raw_busy carries rsvp", "raw_busy", []string{"ou_dry_a"}, "", 1, false},
		{"type=free with min-duration", "free", []string{"ou_dry_a"}, "45m", 1, true},
		{"type=common_free multi-user", "common_free", []string{"ou_dry_a", "ou_dry_b"}, "30m", 2, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args := []string{
				"calendar", "+freebusy",
				"--start", "2026-04-25T09:00:00+08:00",
				"--end", "2026-04-25T18:00:00+08:00",
				"--type", c.typ,
				"--dry-run",
			}
			for _, id := range c.userIDs {
				args = append(args, "--user-id", id)
			}
			if c.minDuration != "" {
				args = append(args, "--min-duration", c.minDuration)
			}

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      args,
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
			require.Equal(t, "/open-apis/calendar/v4/freebusy/batch", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
			require.Equal(t, c.typ, clie2e.DryRunGet(out, "type").String(), "stdout:\n%s", out)
			require.Equal(t, true, clie2e.DryRunGet(out, "api.0.body.need_rsvp_status").Bool(), "stdout:\n%s", out)

			userIDs := clie2e.DryRunGet(out, "api.0.body.user_ids").Array()
			require.Len(t, userIDs, c.wantUsersLen, "stdout:\n%s", out)
			for i, id := range c.userIDs {
				assert.Equal(t, id, userIDs[i].String(), "stdout:\n%s", out)
			}

			if c.wantMinInBody {
				require.Equal(t, c.minDuration, clie2e.DryRunGet(out, "min_duration").String(), "stdout:\n%s", out)
			}
		})
	}
}

// TestCalendar_FreebusyDryRun_BotWithoutUserIDsFails asserts the shortcut
// refuses to run under bot identity when no --user-id is provided; that fallback
// only exists for user identity. Verified in dry-run so the check does not
// depend on network state.
func TestCalendar_FreebusyDryRun_BotWithoutUserIDsFails(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"calendar", "+freebusy",
			"--start", "2026-04-25T09:00:00+08:00",
			"--end", "2026-04-25T18:00:00+08:00",
			"--type", "busy",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	assert.NotEqual(t, 0, result.ExitCode, "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
}

// TestCalendar_Freebusy runs live against the API and asserts the JSON envelope
// shape for each --type view. Live-safe: read-only query over a small window.
func TestCalendar_Freebusy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)
	openID := getCurrentUserOpenIDForCalendar(t, ctx)

	// A one-day window is enough for a live freebusy read; keeps the response
	// small and the assertions cheap.
	start := time.Now().UTC().Truncate(24 * time.Hour)
	end := start.Add(24 * time.Hour)

	t.Run("type=busy for self as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+freebusy",
				"--start", unixSecondsRFC3339(start),
				"--end", unixSecondsRFC3339(end),
				"--user-id", openID,
				"--type", "busy",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		users := gjson.Get(result.Stdout, "data.users")
		require.True(t, users.IsArray(), "stdout:\n%s", result.Stdout)
		require.GreaterOrEqual(t, len(users.Array()), 1, "stdout:\n%s", result.Stdout)
		assert.Equal(t, openID, gjson.Get(result.Stdout, "data.users.0.user_id").String(), "stdout:\n%s", result.Stdout)
		assert.True(t, gjson.Get(result.Stdout, "data.users.0.busy").IsArray(), "stdout:\n%s", result.Stdout)
	})

	t.Run("type=raw_busy for self as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+freebusy",
				"--start", unixSecondsRFC3339(start),
				"--end", unixSecondsRFC3339(end),
				"--user-id", openID,
				"--type", "raw_busy",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, openID, gjson.Get(result.Stdout, "data.users.0.user_id").String(), "stdout:\n%s", result.Stdout)
		assert.True(t, gjson.Get(result.Stdout, "data.users.0.raw_busy").IsArray(), "stdout:\n%s", result.Stdout)
	})

	t.Run("type=free for self as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+freebusy",
				"--start", unixSecondsRFC3339(start),
				"--end", unixSecondsRFC3339(end),
				"--user-id", openID,
				"--type", "free",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, openID, gjson.Get(result.Stdout, "data.users.0.user_id").String(), "stdout:\n%s", result.Stdout)
		assert.True(t, gjson.Get(result.Stdout, "data.users.0.free").IsArray(), "stdout:\n%s", result.Stdout)
	})

	t.Run("type=common_free for self as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+freebusy",
				"--start", unixSecondsRFC3339(start),
				"--end", unixSecondsRFC3339(end),
				"--user-id", openID,
				"--type", "common_free",
				"--min-duration", "30m",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, "data.common_free").IsArray(), "stdout:\n%s", result.Stdout)
	})

	t.Run("pretty output does not fail", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"calendar", "+freebusy",
				"--start", unixSecondsRFC3339(start),
				"--end", unixSecondsRFC3339(end),
				"--user-id", openID,
				"--type", "busy",
			},
			DefaultAs: "user",
			Format:    "pretty",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
	})
}
