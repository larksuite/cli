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

// TestIMMessageUnifiedInputDryRun verifies that message send, reply, and edit
// use the same unified file, stdin, escape, and single-pass input semantics as
// other shortcuts such as docs +update.
func TestIMMessageUnifiedInputDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "im_message_input_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "im_message_input_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "content.json"), []byte(`{"text":"content from file"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "message.txt"), []byte("text from file\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "message.md"), []byte("# Markdown from file\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "at-message.txt"), []byte("@from-file\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "large-number.json"), []byte(`{"value":1e400}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "invalid.json"), []byte(`{"text":"QA_SYNTHETIC_PRIVATE_MARKER_93827",bad}`), 0o600))

	commands := []struct {
		name string
		args []string
	}{
		{name: "send", args: []string{"im", "+messages-send", "--chat-id", "oc_test"}},
		{name: "reply", args: []string{"im", "+messages-reply", "--message-id", "om_test"}},
		{name: "edit", args: []string{"im", "+messages-edit", "--message-id", "om_test"}},
	}
	inputs := []struct {
		name        string
		args        []string
		stdin       []byte
		wantContent string
	}{
		{name: "content-file", args: []string{"--content", "@./content.json"}, wantContent: "content from file"},
		{name: "content-stdin", args: []string{"--content", "-"}, stdin: []byte(`{"text":"content from stdin"}`), wantContent: "content from stdin"},
		{name: "text-file", args: []string{"--text", "@./message.txt"}, wantContent: "text from file"},
		{name: "text-stdin", args: []string{"--text", "-"}, stdin: []byte("text from stdin"), wantContent: "text from stdin"},
		{name: "markdown-file", args: []string{"--markdown", "@./message.md"}, wantContent: "Markdown from file"},
		{name: "markdown-stdin", args: []string{"--markdown", "-"}, stdin: []byte("# Markdown from stdin"), wantContent: "Markdown from stdin"},
		{name: "literal-at-escape", args: []string{"--text", "@@owner"}, wantContent: "@owner"},
		{name: "file-is-single-pass", args: []string{"--text", "@./at-message.txt"}, wantContent: "@from-file"},
		{name: "stdin-is-single-pass", args: []string{"--text", "-"}, stdin: []byte("@from-stdin"), wantContent: "@from-stdin"},
		{name: "large-number-content-file", args: []string{"--content", "@./large-number.json"}, wantContent: "1e400"},
	}

	for _, command := range commands {
		for _, input := range inputs {
			t.Run(command.name+"/"+input.name, func(t *testing.T) {
				args := append(append([]string{}, command.args...), input.args...)
				args = append(args, "--dry-run")
				result, err := clie2e.RunCmd(ctx, clie2e.Request{
					Args:      args,
					Stdin:     input.stdin,
					WorkDir:   workDir,
					DefaultAs: "bot",
				})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				require.Contains(t, gjson.Get(result.Stdout, "data.api.0.body.content").String(), input.wantContent, result.Stdout)
			})
		}
	}

	for _, command := range commands {
		t.Run(command.name+"/missing-file", func(t *testing.T) {
			args := append(append([]string{}, command.args...), "--text", "@./missing.txt", "--dry-run")
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      args,
				WorkDir:   workDir,
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			require.Empty(t, result.Stdout)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
			require.Equal(t, "--text", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
			require.NotEmpty(t, gjson.Get(result.Stderr, "error.message").String(), result.Stderr)
		})
	}

	for _, command := range commands {
		t.Run(command.name+"/invalid-content-file", func(t *testing.T) {
			args := append(append([]string{}, command.args...), "--content", "@./invalid.json", "--dry-run")
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      args,
				WorkDir:   workDir,
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			require.Empty(t, result.Stdout)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
			require.Equal(t, "--content", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
			require.NotEmpty(t, gjson.Get(result.Stderr, "error.message").String(), result.Stderr)
			require.NotContains(t, result.Stderr, "QA_SYNTHETIC_PRIVATE_MARKER_93827")
		})
	}
}
