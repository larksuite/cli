// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestIMMessagesSendContentFileAndStdinDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	cardJSON := `{"type":"template","data":{"template_id":"tpl_123"}}`

	t.Run("content from file", func(t *testing.T) {
		dir := t.TempDir()
		cardPath := filepath.Join(dir, "card.json")
		require.NoError(t, os.WriteFile(cardPath, []byte(cardJSON), 0644))

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-send",
				"--as", "bot",
				"--chat-id", "oc_dryrun_chat",
				"--msg-type", "interactive",
				"--content", "@" + cardPath,
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "/open-apis/im/v1/messages")
		require.Contains(t, result.Stdout, "interactive")
		require.Contains(t, result.Stdout, "tpl_123")
		require.NotContains(t, result.Stdout, "@"+cardPath)
	})

	t.Run("content from stdin", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-send",
				"--as", "bot",
				"--chat-id", "oc_dryrun_chat",
				"--msg-type", "interactive",
				"--content", "-",
				"--dry-run",
			},
			DefaultAs: "bot",
			Stdin:     []byte(cardJSON),
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "/open-apis/im/v1/messages")
		require.Contains(t, result.Stdout, "interactive")
		require.Contains(t, result.Stdout, "tpl_123")
		require.NotContains(t, strings.TrimSpace(result.Stdout), `"content":"-"`)
	})
}

func setDryRunConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_APP_ID", "cli_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret_dryrun_test")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
