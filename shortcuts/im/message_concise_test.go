// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/shortcuts/common"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/spf13/cobra"
)

func TestRenderMessagesConciseChatConversation(t *testing.T) {
	longContent := strings.Repeat("full message content ", 8)
	messages := []map[string]interface{}{
		{
			"message_id":       "om_root",
			"thread_id":        "omt_thread",
			"msg_type":         "text",
			"create_time":      "2026-08-26 06:17",
			"content":          longContent,
			"message_app_link": "https://applink.feishu.cn/client/thread/open?open_thread_id=omt_thread",
			"sender": map[string]interface{}{
				"id":          "ou_sender",
				"name":        "Alice",
				"sender_type": "user",
			},
			"reactions": map[string]interface{}{
				"counts": []interface{}{
					map[string]interface{}{"reaction_type": "THUMBSUP", "count": float64(2)},
				},
				"details": []interface{}{map[string]interface{}{"reaction_id": "reaction-detail-is-omitted"}},
			},
			"thread_replies": []map[string]interface{}{
				{
					"message_id": "om_root",
					"thread_id":  "omt_thread",
					"msg_type":   "text",
					"content":    "duplicate root",
				},
				{
					"message_id":  "om_reply",
					"thread_id":   "omt_thread",
					"msg_type":    "post",
					"create_time": "2026-08-26 06:18",
					"content":     "reply line one\nreply line two",
					"sender": map[string]interface{}{
						"id":          "cli_bot",
						"name":        "Helper Bot",
						"sender_type": "app",
						"open_bot_id": "ou_bot",
					},
				},
			},
			"thread_has_more": true,
		},
	}

	var out bytes.Buffer
	err := renderMessagesConcise(&out, conciseMessageView{
		Title:     "Chat messages",
		ChatID:    "oc_chat",
		Messages:  messages,
		HasMore:   true,
		NextToken: "next-page",
	})
	if err != nil {
		t.Fatalf("renderMessagesConcise() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"# Chat messages",
		"- chat_id: `oc_chat`",
		"- Alice (`ou_sender`, user)",
		"- Helper Bot (`cli_bot`, app, bot_open_id: `ou_bot`)",
		longContent,
		"message_id: `om_root`",
		"thread_id: `omt_thread`",
		"app_link: <https://applink.feishu.cn/client/thread/open?open_thread_id=omt_thread>",
		"**Reply**",
		"message_id: `om_reply`",
		"> reply line one",
		"> reply line two",
		"reactions: `THUMBSUP x2`",
		"thread_replies: incomplete",
		"- messages: 1",
		"- thread_replies: 1",
		"- threads: 1",
		"- has_more: true",
		"- next_token: `next-page`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("concise output missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"duplicate root", "reaction-detail-is-omitted"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("concise output contains %q:\n%s", forbidden, got)
		}
	}
}

func TestRenderMessagesConciseMarksUnavailableAndDeletedContent(t *testing.T) {
	messages := []map[string]interface{}{
		{
			"message_id":           "om_deleted",
			"msg_type":             "text",
			"content":              "must not be rendered",
			"deleted":              true,
			"reactions_error":      true,
			"thread_replies_error": true,
		},
	}

	var out bytes.Buffer
	if err := renderMessagesConcise(&out, conciseMessageView{
		Title:    "Messages",
		Messages: messages,
	}); err != nil {
		t.Fatalf("renderMessagesConcise() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"[deleted]", "reactions: unavailable", "thread_replies: unavailable"} {
		if !strings.Contains(got, want) {
			t.Fatalf("concise output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "must not be rendered") {
		t.Fatalf("deleted message leaked original content:\n%s", got)
	}
}

func TestRenderMessagesConciseEscapesUntrustedMetadata(t *testing.T) {
	const forged = "\n## forged"
	messages := []map[string]interface{}{
		{
			"message_id":       "om_bad`" + forged,
			"thread_id":        "omt_bad`" + forged,
			"msg_type":         "text`" + forged,
			"create_time":      "2026-08-26`" + forged,
			"content":          "normal body",
			"message_app_link": "https://safe.example>" + forged,
			"sender": map[string]interface{}{
				"id":          "ou_bad`" + forged,
				"name":        "\x1b[31m**Mallory**" + forged + "\u202e",
				"sender_type": "user" + forged,
				"open_bot_id": "ou_bot`" + forged,
			},
			"reactions": map[string]interface{}{
				"counts": []interface{}{
					map[string]interface{}{"reaction_type": "SMILE`" + forged, "count": float64(1)},
				},
			},
		},
	}

	var out bytes.Buffer
	if err := renderMessagesConcise(&out, conciseMessageView{
		Title:     "Messages" + forged,
		ChatID:    "oc_bad`" + forged,
		Messages:  messages,
		NextToken: "next`" + forged,
	}); err != nil {
		t.Fatalf("renderMessagesConcise() error = %v", err)
	}

	got := out.String()
	if strings.Contains(got, forged) {
		t.Fatalf("untrusted metadata injected a Markdown heading:\n%s", got)
	}
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\u202e") {
		t.Fatalf("untrusted metadata retained terminal control characters: %q", got)
	}
	if strings.Contains(got, "<https://safe.example>") {
		t.Fatalf("malformed app link remained an active autolink:\n%s", got)
	}
	for _, want := range []string{
		"# Messages \\#\\# forged",
		"chat_id: ``oc_bad` ## forged``",
		"app_link: `https://safe.example> ## forged`",
		"next_token: ``next` ## forged``",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("escaped output missing %q:\n%s", want, got)
		}
	}
}

func TestMessageListConciseFormatIsCommandScoped(t *testing.T) {
	t.Cleanup(func() { cmdutil.SetFlagCompletionsEnabled(false) })
	cmdutil.SetFlagCompletionsEnabled(true)

	for _, shortcut := range []*common.Shortcut{&ImChatMessageList, &ImThreadsMessagesList} {
		runtime, _ := newMountedIMRuntime(t, shortcut)
		flag := runtime.Cmd.Flags().Lookup("format")
		if flag == nil || !strings.Contains(flag.Usage, "concise") {
			t.Fatalf("%s --format usage = %#v, want concise", shortcut.Command, flag)
		}
		if findDeclaredFormatFlag(*shortcut) != nil {
			t.Fatalf("%s must use the framework-owned --format flag", shortcut.Command)
		}
		complete, ok := runtime.Cmd.GetFlagCompletionFunc("format")
		if !ok {
			t.Fatalf("%s has no --format completion", shortcut.Command)
		}
		values, _ := complete(runtime.Cmd, nil, "")
		want := []string{"json", "pretty", "concise", "table", "ndjson", "csv"}
		if !slices.Equal(values, want) {
			t.Fatalf("%s format completion = %v, want %v", shortcut.Command, values, want)
		}
		if runtime.Cmd.Flags().Lookup("json") == nil {
			t.Fatalf("%s lost --json shorthand", shortcut.Command)
		}

		help := &bytes.Buffer{}
		runtime.Cmd.SetOut(help)
		runtime.Cmd.SetErr(help)
		if err := runtime.Cmd.Help(); err != nil {
			t.Fatalf("%s Help() error = %v", shortcut.Command, err)
		}
		if !strings.Contains(help.String(), "concise") {
			t.Fatalf("%s help does not advertise concise:\n%s", shortcut.Command, help.String())
		}
	}

	runtime, _ := newMountedIMRuntime(t, &ImChatList)
	flag := runtime.Cmd.Flags().Lookup("format")
	if flag == nil || strings.Contains(flag.Usage, "concise") {
		t.Fatalf("%s unexpectedly advertises concise: %#v", ImChatList.Command, flag)
	}
	complete, ok := runtime.Cmd.GetFlagCompletionFunc("format")
	if !ok {
		t.Fatalf("%s has no --format completion", ImChatList.Command)
	}
	values, _ := complete(runtime.Cmd, nil, "")
	if slices.Contains(values, "concise") {
		t.Fatalf("%s unexpectedly completes concise: %v", ImChatList.Command, values)
	}
	help := &bytes.Buffer{}
	runtime.Cmd.SetOut(help)
	runtime.Cmd.SetErr(help)
	if err := runtime.Cmd.Help(); err != nil {
		t.Fatalf("%s Help() error = %v", ImChatList.Command, err)
	}
	if strings.Contains(help.String(), "concise") {
		t.Fatalf("%s help unexpectedly advertises concise:\n%s", ImChatList.Command, help.String())
	}
}

func findDeclaredFormatFlag(shortcut common.Shortcut) *common.Flag {
	for i := range shortcut.Flags {
		flag := &shortcut.Flags[i]
		if flag.Name == "format" {
			return flag
		}
	}
	return nil
}

func TestMessageListMountedFormatCompatibility(t *testing.T) {
	for _, shortcut := range []common.Shortcut{ImChatMessageList, ImThreadsMessagesList} {
		t.Run(shortcut.Command, func(t *testing.T) {
			t.Run("unknown warning and JSON fallback", func(t *testing.T) {
				stdout, stderr, err := runMountedMessageListFormat(t, shortcut, "unknown-format")
				if err != nil {
					t.Fatalf("Execute() error = %v", err)
				}
				var envelope map[string]interface{}
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
					t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
				}
				if !strings.Contains(stderr.String(), `warning: unknown format "unknown-format", falling back to json`) {
					t.Fatalf("stderr = %q, want unknown-format warning", stderr.String())
				}
			})

			t.Run("uppercase JSON", func(t *testing.T) {
				stdout, stderr, err := runMountedMessageListFormat(t, shortcut, "JSON")
				if err != nil {
					t.Fatalf("Execute() error = %v", err)
				}
				var envelope map[string]interface{}
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
					t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
				}
				if strings.Contains(stderr.String(), "unknown format") {
					t.Fatalf("stderr unexpectedly warned for uppercase JSON: %q", stderr.String())
				}
			})

			for _, format := range []string{"pretty", "table", "ndjson", "csv"} {
				t.Run(format+" stays on the standard renderer", func(t *testing.T) {
					stdout, stderr, err := runMountedMessageListFormat(t, shortcut, format)
					if err != nil {
						t.Fatalf("Execute() error = %v", err)
					}
					if strings.Contains(stdout.String(), "## Summary") || strings.Contains(stderr.String(), "unknown format") {
						t.Fatalf("format %q was routed through concise: stdout=%q stderr=%q", format, stdout.String(), stderr.String())
					}
					switch format {
					case "pretty":
						if !strings.Contains(stdout.String(), "No messages") {
							t.Fatalf("pretty stdout = %q, want legacy empty-state text", stdout.String())
						}
					case "table":
						if !strings.Contains(stdout.String(), "(empty)") || !strings.Contains(stdout.String(), "Pagination:") {
							t.Fatalf("table stdout = %q, want standard empty table with pagination", stdout.String())
						}
					case "ndjson":
						if stdout.Len() != 0 {
							t.Fatalf("ndjson stdout = %q, want no records for an empty list", stdout.String())
						}
					case "csv":
						if got := stdout.String(); got != "(empty)\n" {
							t.Fatalf("csv stdout = %q, want standard empty CSV output", got)
						}
					}
				})
			}

			t.Run("jq rejects concise", func(t *testing.T) {
				_, _, err := runMountedMessageListFormat(t, shortcut, "concise", "--jq", ".data")
				if err == nil || !strings.Contains(err.Error(), "--jq and --format concise are mutually exclusive") {
					t.Fatalf("Execute() error = %v, want jq/concise conflict", err)
				}
			})
		})
	}
}

func TestMessageListMountedConciseOutput(t *testing.T) {
	for _, shortcut := range []common.Shortcut{ImChatMessageList, ImThreadsMessagesList} {
		t.Run(shortcut.Command, func(t *testing.T) {
			stdout, stderr, err := runMountedMessageListFormat(t, shortcut, "concise")
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			for _, want := range []string{"## Messages", "## Summary", "- messages: 0", "- has_more: false"} {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
				}
			}
			if strings.Contains(stdout.String(), `"ok"`) {
				t.Fatalf("concise stdout unexpectedly used JSON envelope:\n%s", stdout.String())
			}
			if strings.Contains(stderr.String(), "unknown format") {
				t.Fatalf("stderr unexpectedly warned for concise: %q", stderr.String())
			}
		})
	}
}

func runMountedMessageListFormat(t *testing.T, shortcut common.Shortcut, format string, extraArgs ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()

	transport := shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != imMessagesListPath {
			return nil, fmt.Errorf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":      []interface{}{},
				"has_more":   false,
				"page_token": "",
			},
		}), nil
	})
	httpClient := &http.Client{Transport: transport}
	sdk := lark.NewClient(
		"test-app",
		"test-secret",
		lark.WithEnableTokenCache(false),
		lark.WithLogLevel(larkcore.LogLevelError),
		lark.WithHttpClient(httpClient),
	)
	config := &core.CliConfig{
		AppID:      "test-app",
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_test",
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	factory := &cmdutil.Factory{
		Config:         func() (*core.CliConfig, error) { return config, nil },
		HttpClient:     func() (*http.Client, error) { return httpClient, nil },
		LarkClient:     func() (*lark.Client, error) { return sdk, nil },
		Credential:     credential.NewCredentialProvider(nil, nil, &staticShortcutTokenResolver{}, nil),
		FileIOProvider: fileio.GetProvider(),
		IOStreams:      &cmdutil.IOStreams{Out: stdout, ErrOut: stderr},
	}

	parent := &cobra.Command{Use: "root"}
	shortcut.Mount(parent, factory)
	args := []string{shortcut.Command}
	if shortcut.Command == ImChatMessageList.Command {
		args = append(args, "--chat-id", "oc_test")
	} else {
		args = append(args, "--thread", "omt_test")
	}
	args = append(args, "--no-reactions", "--format", format)
	args = append(args, extraArgs...)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	err := parent.ExecuteContext(context.Background())
	return stdout, stderr, err
}
