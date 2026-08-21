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
	"unicode"

	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/registry"
	imshortcuts "github.com/larksuite/cli/shortcuts/im"
	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Placeholder substitutions turning copyable help examples into syntactically
// valid dry-run invocations. IDs are obvious fakes; --dry-run never hits the API.
var tipsPlaceholderValues = map[string]string{
	"<chat_id>":            "oc_e2etest000000000000000000",
	"<open_id>":            "ou_e2etest000000000000000000",
	"<message_id>":         "om_e2etest000000000000000000",
	"<thread_id>":          "omt_e2etest00000000000000000",
	"<file_key>":           "file_v3_e2etest0000000000000",
	"<image_key>":          "img_v3_e2etest00000000000000",
	"<open_id1>":           "ou_e2etest000000000000000001",
	"<open_id2>":           "ou_e2etest000000000000000002",
	"<message_id1>":        "om_e2etest000000000000000001",
	"<message_id2>":        "om_e2etest000000000000000002",
	"<feed_group_id>":      "ofg_e2etest00000000000000000",
	"<chat_id1>":           "oc_e2etest000000000000000001",
	"<chat_id2>":           "oc_e2etest000000000000000002",
	"<user_id_or_open_id>": "ou_e2etest000000000000000003",
	"<generated_uuid>":     "11111111-1111-4111-8111-111111111111",
	"<token>":              "callback-token",
}

// allExampleArgs extracts every affordance example for the shortcut, replaces
// placeholders, and returns one argv (after "lark-cli") per example.
func allExampleArgs(t *testing.T, command string) [][]string {
	t.Helper()
	affordance.SetSource(os.DirFS("../../../affordance"))
	raw, ok := affordance.For("im", command)
	if !ok {
		t.Fatalf("%s has no affordance", command)
	}
	parsed, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
	if !ok || len(parsed.Examples) == 0 {
		t.Fatalf("%s has no affordance example", command)
	}
	all := make([][]string, 0, len(parsed.Examples))
	for _, example := range parsed.Examples {
		line := strings.TrimPrefix(example.Command, "lark-cli ")
		for ph, v := range tipsPlaceholderValues {
			line = strings.ReplaceAll(line, ph, v)
		}
		all = append(all, splitExampleArgs(t, line))
	}
	return all
}

// firstExampleArgs extracts the first affordance example of the shortcut.
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

// splitExampleArgs handles the shell syntax used by affordance examples:
// single/double quotes, escapes, and backslash-newline continuation.
func splitExampleArgs(t *testing.T, line string) []string {
	t.Helper()
	var args []string
	var cur strings.Builder
	var quote rune
	escaped := false
	for _, r := range line {
		if escaped {
			escaped = false
			if r != '\n' {
				cur.WriteRune(r)
			}
			continue
		}
		switch {
		case r == '\\' && quote != '\'':
			escaped = true
		case quote == 0 && (r == '\'' || r == '"'):
			quote = r
		case quote != 0 && r == quote:
			quote = 0
		case quote == 0 && unicode.IsSpace(r):
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if escaped || quote != 0 {
		t.Fatalf("unbalanced quotes in example: %s", line)
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}

// allIMAffordanceMethods discovers every current entry from affordance/im.md.
// Command-form headings are resolved through the same registry mapping used by
// the production affordance loader; escape-hatch headings use the dotted
// fallback key.
func allIMAffordanceMethods(t *testing.T) []string {
	t.Helper()
	source, err := os.ReadFile("../../../affordance/im.md")
	require.NoError(t, err)

	byCommandForm := map[string]string{}
	service, ok := registry.SchemaCatalog().Service("im")
	require.True(t, ok, "IM service missing from schema catalog")
	for _, ref := range apicatalog.ServiceMethods(service, nil) {
		byCommandForm[strings.Join(ref.CommandPath()[1:], " ")] = ref.Method.ID
	}

	methods := make([]string, 0, 53)
	for _, line := range strings.Split(string(source), "\n") {
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		heading := strings.TrimSpace(strings.TrimPrefix(line, "## "))
		if strings.HasPrefix(heading, "+") {
			methods = append(methods, heading)
			continue
		}
		if method, found := byCommandForm[heading]; found {
			methods = append(methods, method)
			continue
		}
		methods = append(methods, strings.ReplaceAll(heading, " ", "."))
	}
	require.NotEmpty(t, methods)
	return methods
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
// path-assertion tests above to every current IM affordance entry: each
// example, with placeholders substituted and --dry-run appended, runs
// VERBATIM — no identity is injected, so the test proves the copied example
// itself is runnable, not a framework-completed variant of it. Examples that
// carry an explicit --as additionally assert the resolved identity equals
// that value.
func TestIMTipsAllExamplesDryRun(t *testing.T) {
	for _, cmd := range allIMAffordanceMethods(t) {
		for i, exampleArgs := range allExampleArgs(t, cmd) {
			t.Run(fmt.Sprintf("%s/example_%d", cmd, i+1), func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				t.Setenv("LARKSUITE_CLI_APP_ID", "im_tips_dryrun_test")
				t.Setenv("LARKSUITE_CLI_APP_SECRET", "im_tips_dryrun_secret")
				t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

				workDir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(workDir, "diagram.png"), []byte("dry-run fixture"), 0o600))
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				result, err := clie2e.RunCmd(ctx, clie2e.Request{
					Args:    append(append([]string{}, exampleArgs...), "--dry-run", "--json"),
					WorkDir: workDir,
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

// TestIMTipsSendReplyRespectConfiguredDefault verifies that identity-neutral
// examples do not override the configured actor. Explicit actor requests are
// handled by the identity tip and agent guidance, not by hard-coded examples.
func TestIMTipsSendReplyRespectConfiguredDefault(t *testing.T) {
	for _, cmd := range []string{"+messages-send", "+messages-reply"} {
		for i, exampleArgs := range allExampleArgs(t, cmd) {
			for _, defaultAs := range []string{"user", "bot"} {
				t.Run(fmt.Sprintf("%s/example_%d/default_%s", cmd, i+1, defaultAs), func(t *testing.T) {
					require.False(t, hasAsFlag(exampleArgs),
						"identity-neutral send/reply examples must omit --as")

					cfgDir := t.TempDir()
					cfg := fmt.Sprintf(`{"currentApp":"im_tips_default","apps":[{"appId":"im_tips_default","appSecret":"test-secret","brand":"feishu","defaultAs":%q,"users":[]}]}`, defaultAs)
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
					require.Equal(t, defaultAs, envelope.Identity)
				})
			}
		}
	}
}

func TestIMDualIdentityHelpGuidance(t *testing.T) {
	const identityTip = `Identity: "use my identity" -> --as user; "use the app/bot" -> --as bot; omit --as only when no actor is specified.`
	const contentTip = "Content: use one of --text, --markdown, --content, or a media flag; --msg-type applies only to --content JSON."

	for _, sc := range imshortcuts.Shortcuts() {
		hasUser, hasBot := false, false
		for _, identity := range sc.AuthTypes {
			hasUser = hasUser || identity == "user"
			hasBot = hasBot || identity == "bot"
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"im", sc.Command, "--help"}})
		cancel()
		require.NoError(t, err, sc.Command)
		require.NoError(t, result.RunErr, sc.Command)
		result.AssertExitCode(t, 0)

		wantIdentityTips := 0
		if hasUser && hasBot {
			wantIdentityTips = 1
			require.NotContains(t, result.Stdout, "Use this user-only shortcut", sc.Command)
		}
		require.Equal(t, wantIdentityTips, strings.Count(result.Stdout, identityTip), sc.Command)

		wantContentTips := 0
		if sc.Command == "+messages-send" || sc.Command == "+messages-reply" {
			wantContentTips = 1
		}
		require.Equal(t, wantContentTips, strings.Count(result.Stdout, contentTip), sc.Command)
	}
}

func TestIMMarkdownMsgTypeProvidesExecutableRecovery(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{
			name: "send",
			args: []string{"im", "+messages-send", "--chat-id", "oc_e2etest000000000000000000", "--msg-type", "markdown", "--content", "hello"},
		},
		{
			name: "reply",
			args: []string{"im", "+messages-reply", "--message-id", "om_e2etest000000000000000000", "--msg-type", "markdown", "--content", "hello"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: append(tc.args, "--json")})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
			require.Equal(t, "--msg-type", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
			require.Equal(t, markdownMsgTypeMessageForE2E, gjson.Get(result.Stderr, "error.message").String(), result.Stderr)
			require.Equal(t, markdownMsgTypeHintForE2E, gjson.Get(result.Stderr, "error.hint").String(), result.Stderr)
		})
	}
}

const (
	markdownMsgTypeMessageForE2E = "markdown is an input format, not a msg_type"
	markdownMsgTypeHintForE2E    = "Replace `--msg-type markdown --content <text>` with `--markdown <text>`."
)
