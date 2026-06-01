// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestIM_MessageUpdateDryRun(t *testing.T) {
	setIMDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"im", "+messages-update",
			"--message-id", "om_dryrun",
			"--text", "updated",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	if !strings.Contains(result.Stdout, "/open-apis/im/v1/messages/om_dryrun") ||
		!strings.Contains(result.Stdout, `"method": "PUT"`) ||
		!strings.Contains(result.Stdout, `"msg_type": "text"`) {
		t.Fatalf("dry-run output missing update request shape:\n%s", result.Stdout)
	}
}

func TestIM_MessageCardUpdateDryRun(t *testing.T) {
	setIMDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"im", "+messages-card-update",
			"--message-id", "om_dryrun",
			"--content", `{"config":{"update_multi":true},"elements":[{"tag":"div","text":{"tag":"plain_text","content":"updated"}}]}`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	if !strings.Contains(result.Stdout, "/open-apis/im/v1/messages/om_dryrun") ||
		!strings.Contains(result.Stdout, `"method": "PATCH"`) ||
		!strings.Contains(result.Stdout, `update_multi`) {
		t.Fatalf("dry-run output missing card update request shape:\n%s", result.Stdout)
	}
}

func setIMDryRunConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
