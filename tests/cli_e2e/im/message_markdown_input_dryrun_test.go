// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIMMessagesSendMarkdownFileDryRunPreservesShellSyntax(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "im_markdown_input_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "im_markdown_input_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	workDir := t.TempDir()
	markdown := "## 发布清单\n\n```sh\necho $HOME && $(do-not-run)\n```\n\n中文内容保持完整。\n"
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "message.md"), []byte(markdown), 0o600))

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"im", "+messages-send",
			"--chat-id", "oc_123",
			"--markdown", "@./message.md",
			"--dry-run",
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	content := gjson.Get(result.Stdout, "data.request.body.content").String()
	require.Contains(t, content, "echo $HOME && $(do-not-run)")
	require.Contains(t, content, "中文内容保持完整。")
}
