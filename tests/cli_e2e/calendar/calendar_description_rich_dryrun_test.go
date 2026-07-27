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

// richTextMarkdown is a Markdown rich-text payload carrying a doc link and
// styling. The CLI forwards it verbatim in description_rich; the OpenAPI service
// converts Markdown <-> ClientVars.
const richTextMarkdown = "见 [设计文档](https://bytedance.feishu.cn/docx/abc) 和 **重点**"

// TestCalendar_CreateDescriptionRichDryRun verifies that +create treats
// --description as Markdown rich text and forwards it as-is under the
// description_rich body field, omitting the plain description field — the
// service treats the two as mutually exclusive.
func TestCalendar_CreateDescriptionRichDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"calendar", "+create",
			"--calendar-id", "cal_dry",
			"--summary", "rich dry-run",
			"--start", "2026-04-25T10:00:00+08:00",
			"--end", "2026-04-25T11:00:00+08:00",
			"--description", richTextMarkdown,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/calendar/v4/calendars/cal_dry/events", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
	require.Equal(t, richTextMarkdown, clie2e.DryRunGet(out, "api.0.body.description_rich").String(), "stdout:\n%s", out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.description").Exists(), "plain description must not be sent; stdout:\n%s", out)
}

// TestCalendar_CreateDescriptionRichOnlyDryRun verifies that +create forwards
// --description under description_rich and omits the plain description body
// field entirely. Sending an empty description would suppress the server's
// plain-preview backfill and break first-load rendering.
func TestCalendar_CreateDescriptionRichOnlyDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"calendar", "+create",
			"--calendar-id", "cal_dry",
			"--summary", "rich only dry-run",
			"--start", "2026-04-25T10:00:00+08:00",
			"--end", "2026-04-25T11:00:00+08:00",
			"--description", richTextMarkdown,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, richTextMarkdown, clie2e.DryRunGet(out, "api.0.body.description_rich").String(), "stdout:\n%s", out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.description").Exists(), "description must be omitted when not set; stdout:\n%s", out)
}

// TestCalendar_UpdateDescriptionRichDryRun verifies that +update forwards the
// rich-text payload as-is under the description_rich body field.
func TestCalendar_UpdateDescriptionRichDryRun(t *testing.T) {
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
			"--description", richTextMarkdown,
			"--notify=false",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "PATCH", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/calendar/v4/calendars/cal_dry/events/evt_dry", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
	require.Equal(t, richTextMarkdown, clie2e.DryRunGet(out, "api.0.body.description_rich").String(), "stdout:\n%s", out)
}
