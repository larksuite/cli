// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	imshortcuts "github.com/larksuite/cli/shortcuts/im"
	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

// Placeholder substitutions turning copyable help examples into syntactically
// valid dry-run invocations. IDs are obvious fakes; --dry-run never hits the API.
var tipsPlaceholderValues = map[string]string{
	"<chat_id>":       "oc_e2etest000000000000000000",
	"<open_id>":       "ou_e2etest000000000000000000",
	"<message_id>":    "om_e2etest000000000000000000",
	"<thread_id>":     "omt_e2etest00000000000000000",
	"<file_key>":      "file_v3_e2etest0000000000000",
	"<image_key>":     "img_v3_e2etest00000000000000",
	"<open_id1>":      "ou_e2etest000000000000000001",
	"<open_id2>":      "ou_e2etest000000000000000002",
	"<message_id1>":   "om_e2etest000000000000000001",
	"<message_id2>":   "om_e2etest000000000000000002",
	"<feed_group_id>": "ofg_e2etest00000000000000000",
	"<chat_id1>":      "oc_e2etest000000000000000001",
	"<chat_id2>":      "oc_e2etest000000000000000002",
}

// allExampleArgs extracts every "Example:" tip of the shortcut, replaces
// placeholders, and returns one argv (after "lark-cli") per example.
func allExampleArgs(t *testing.T, command string) [][]string {
	t.Helper()
	for _, sc := range imshortcuts.Shortcuts() {
		if sc.Command != command {
			continue
		}
		prefix := "Example: lark-cli "
		var all [][]string
		for _, tip := range sc.Tips {
			if !strings.HasPrefix(tip, prefix) {
				continue
			}
			line := strings.TrimPrefix(tip, prefix)
			for ph, v := range tipsPlaceholderValues {
				line = strings.ReplaceAll(line, ph, v)
			}
			all = append(all, splitExampleArgs(t, line))
		}
		if len(all) == 0 {
			t.Fatalf("%s has no Example tip", command)
		}
		return all
	}
	t.Fatalf("shortcut %s not found", command)
	return nil
}

// firstExampleArgs extracts the first "Example:" tip of the shortcut.
func firstExampleArgs(t *testing.T, command string) []string {
	t.Helper()
	return allExampleArgs(t, command)[0]
}

// hasAsFlag reports whether the example already carries an explicit --as,
// in which case the test must run it verbatim instead of injecting one.
func hasAsFlag(args []string) bool {
	for _, a := range args {
		if a == "--as" {
			return true
		}
	}
	return false
}

// splitExampleArgs splits a shell-like example line on spaces, honoring
// double-quoted segments (the only quoting style used in Tips examples).
func splitExampleArgs(t *testing.T, line string) []string {
	t.Helper()
	var args []string
	var cur strings.Builder
	inQuote := false
	for _, r := range line {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if inQuote {
		t.Fatalf("unbalanced quotes in example: %s", line)
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}

func runFirstExampleDryRun(t *testing.T, command string, wantAPIPath string) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "im_tips_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "im_tips_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	exampleArgs := firstExampleArgs(t, command)
	args := append(exampleArgs, "--dry-run")
	result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, WorkDir: t.TempDir()})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Contains(t, result.Stdout, wantAPIPath,
		"dry-run output should reference the upstream API path")
}

func TestIMTipsFirstExampleDryRunMessagesSend(t *testing.T) {
	runFirstExampleDryRun(t, "+messages-send", "/open-apis/im/v1/messages")
}

func TestIMTipsFirstExampleDryRunChatMessagesList(t *testing.T) {
	runFirstExampleDryRun(t, "+chat-messages-list", "/open-apis/im/v1/messages")
}

func TestIMTipsFirstExampleDryRunResourcesDownload(t *testing.T) {
	runFirstExampleDryRun(t, "+messages-resources-download", "/open-apis/im/v1/messages/")
}

// tipsExampleAllTargets mirrors shortcuts/im/tips_examples_test.go's
// tipsExampleTargets: the 12 high-frequency + 6 feed/flag shortcuts whose
// help carries a locked copyable "Example:" tip. Kept as a literal copy here
// because that list lives in an internal _test.go file not visible outside
// the shortcuts/im package.
var tipsExampleAllTargets = []string{
	"+messages-send", "+messages-search", "+chat-messages-list", "+messages-reply",
	"+chat-search", "+chat-list", "+messages-mget", "+threads-messages-list",
	"+messages-resources-download", "+chat-create", "+chat-update", "+chat-members-list",
	"+feed-shortcut-create", "+feed-shortcut-remove",
	"+feed-group-list-item", "+feed-group-query-item",
	"+flag-create", "+flag-cancel",
}

// asFlagValue returns the value following --as in the example, or "".
func asFlagValue(args []string) string {
	for i, a := range args {
		if a == "--as" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// TestIMTipsAllExamplesDryRun extends the executability lock from the 3
// path-assertion tests above (messages-send, chat-messages-list,
// resources-download) to every "Example:" tip of all 18 shortcuts: each
// example, with placeholders substituted and --dry-run appended, runs
// VERBATIM — no identity is injected, so the test proves the copied example
// itself is runnable, not a framework-completed variant of it. Examples that
// carry an explicit --as additionally assert the resolved identity equals
// that value.
func TestIMTipsAllExamplesDryRun(t *testing.T) {
	for _, cmd := range tipsExampleAllTargets {
		for i, exampleArgs := range allExampleArgs(t, cmd) {
			t.Run(fmt.Sprintf("%s/example_%d", cmd, i+1), func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				t.Setenv("LARKSUITE_CLI_APP_ID", "im_tips_dryrun_test")
				t.Setenv("LARKSUITE_CLI_APP_SECRET", "im_tips_dryrun_secret")
				t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				result, err := clie2e.RunCmd(ctx, clie2e.Request{
					Args:    append(append([]string{}, exampleArgs...), "--dry-run", "--json"),
					WorkDir: t.TempDir(),
				})
				require.NoError(t, err)
				require.NoError(t, result.RunErr, "binary: %s args: %v", result.BinaryPath, result.Args)
				result.AssertExitCode(t, 0)

				if wantAs := asFlagValue(exampleArgs); wantAs != "" {
					var envelope struct {
						Identity string `json:"identity"`
					}
					require.NoError(t, json.Unmarshal([]byte(result.Stdout), &envelope),
						"dry-run --json stdout should be a JSON envelope")
					require.Equal(t, wantAs, envelope.Identity,
						"example pins --as %s, resolved identity must match", wantAs)
				}
			})
		}
	}
}

// TestIMTipsSendReplyIdentityLock guards the governance rule that send/reply
// examples must pin `--as bot` explicitly: under a config whose defaultAs is
// "user" (the adversarial case Codex review #4 exposed — a developer machine
// with a user login), running each send/reply example VERBATIM must still
// resolve to bot identity. If someone drops --as bot from an example, the
// bare example resolves to user under this config and the assertion fails.
func TestIMTipsSendReplyIdentityLock(t *testing.T) {
	for _, cmd := range []string{"+messages-send", "+messages-reply"} {
		for i, exampleArgs := range allExampleArgs(t, cmd) {
			t.Run(fmt.Sprintf("%s/example_%d", cmd, i+1), func(t *testing.T) {
				require.True(t, hasAsFlag(exampleArgs),
					"send/reply examples must carry an explicit --as bot")

				cfgDir := t.TempDir()
				// "test-secret" is the content scanner's own named placeholder
				// (publiccontent rules), kept inline so the scanner can see and
				// clear the value rather than having it hidden behind printf.
				cfg := `{"currentApp":"im_tips_identity_lock","apps":[{"appId":"im_tips_identity_lock","appSecret":"test-secret","brand":"feishu","defaultAs":"user","users":[]}]}`
				require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(cfg), 0o600))
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", cfgDir)

				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				result, err := clie2e.RunCmd(ctx, clie2e.Request{
					Args:    append(append([]string{}, exampleArgs...), "--dry-run", "--json"),
					WorkDir: t.TempDir(),
				})
				require.NoError(t, err)
				require.NoError(t, result.RunErr, "binary: %s args: %v", result.BinaryPath, result.Args)
				result.AssertExitCode(t, 0)

				var envelope struct {
					Identity string `json:"identity"`
				}
				require.NoError(t, json.Unmarshal([]byte(result.Stdout), &envelope),
					"dry-run --json stdout should be a JSON envelope")
				require.Equal(t, "bot", envelope.Identity,
					"example run verbatim under a user-default config must still send as bot")
			})
		}
	}
}
