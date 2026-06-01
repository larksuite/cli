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

func TestIM_ChatDisbandDryRun(t *testing.T) {
	setIMDisbandDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"im", "+chat-disband",
			"--chat-id", "oc_dryrun",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	if !strings.Contains(result.Stdout, "/open-apis/im/v1/chats/oc_dryrun") ||
		!strings.Contains(result.Stdout, `"method": "DELETE"`) ||
		!strings.Contains(result.Stdout, "high-risk") {
		t.Fatalf("dry-run output missing chat disband request shape:\n%s", result.Stdout)
	}
}

func setIMDisbandDryRunConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
