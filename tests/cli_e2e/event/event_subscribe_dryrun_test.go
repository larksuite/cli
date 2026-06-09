// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestEventSubscribeDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"event", "+subscribe",
			"--event-types", "im.message.receive_v1,contact.user.created_v3",
			"--filter", "^im\\.",
			"--output-dir", "events_out",
			"--route", "^im\\.message=dir:./messages",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "event +subscribe", gjson.Get(out, "command").String(), "stdout:\n%s", out)
	require.Equal(t, "app", gjson.Get(out, "app_id").String(), "stdout:\n%s", out)
	require.Equal(t, "im.message.receive_v1,contact.user.created_v3", gjson.Get(out, "event_types").String(), "stdout:\n%s", out)
	require.Equal(t, "^im\\.", gjson.Get(out, "filter").String(), "stdout:\n%s", out)
	require.Equal(t, "events_out", gjson.Get(out, "output_dir").String(), "stdout:\n%s", out)
	require.Equal(t, "^im\\.message=dir:./messages", gjson.Get(out, "route").String(), "stdout:\n%s", out)
}
