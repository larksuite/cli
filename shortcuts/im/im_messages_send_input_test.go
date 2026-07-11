// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/vfs"
	_ "github.com/larksuite/cli/internal/vfs/localfileio"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestImMessagesSendContentFlagsSupportFileAndStdin(t *testing.T) {
	want := map[string]bool{
		"content":  false,
		"text":     false,
		"markdown": false,
	}

	for _, fl := range ImMessagesSend.Flags {
		if _, ok := want[fl.Name]; !ok {
			continue
		}
		want[fl.Name] = true
		if !hasInputSource(fl, common.File) {
			t.Fatalf("--%s Input = %#v, want file support", fl.Name, fl.Input)
		}
		if !hasInputSource(fl, common.Stdin) {
			t.Fatalf("--%s Input = %#v, want stdin support", fl.Name, fl.Input)
		}
	}

	for name, seen := range want {
		if !seen {
			t.Fatalf("ImMessagesSend flag --%s not found", name)
		}
	}
}

func TestImMessagesSendDryRunResolvesContentFileInput(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	card := `{"type":"template","data":{"template_id":"ctp_test"}}`
	if err := vfs.WriteFile("card.json", []byte(card), 0644); err != nil {
		t.Fatal(err)
	}

	factory, stdout, _ := newMessagesSendInputFactory(t)
	err := runIMShortcutForInputTest(t, ImMessagesSend, []string{
		"+messages-send",
		"--chat-id", "oc_test",
		"--msg-type", "interactive",
		"--content", "@card.json",
		"--dry-run",
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("runIMShortcutForInputTest() error = %v", err)
	}

	body := firstDryRunBodyForInputTest(t, stdout.String())
	if got := body["msg_type"]; got != "interactive" {
		t.Fatalf("msg_type = %#v, want interactive", got)
	}
	if got := body["content"]; got != card {
		t.Fatalf("content = %#v, want file contents %q", got, card)
	}
}

func TestImMessagesSendDryRunResolvesMarkdownFileInput(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	markdown := "hello from file\n\nsecond paragraph"
	if err := vfs.WriteFile("message.md", []byte(markdown), 0644); err != nil {
		t.Fatal(err)
	}

	factory, stdout, _ := newMessagesSendInputFactory(t)
	err := runIMShortcutForInputTest(t, ImMessagesSend, []string{
		"+messages-send",
		"--chat-id", "oc_test",
		"--markdown", "@message.md",
		"--dry-run",
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("runIMShortcutForInputTest() error = %v", err)
	}

	body := firstDryRunBodyForInputTest(t, stdout.String())
	if got := body["msg_type"]; got != "post" {
		t.Fatalf("msg_type = %#v, want post", got)
	}
	content, ok := body["content"].(string)
	if !ok {
		t.Fatalf("content type = %T, want string", body["content"])
	}
	if !strings.Contains(content, "hello from file") {
		t.Fatalf("content = %q, want markdown file content", content)
	}
}

func hasInputSource(fl common.Flag, source string) bool {
	for _, got := range fl.Input {
		if got == source {
			return true
		}
	}
	return false
}

func newMessagesSendInputFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	config := &core.CliConfig{
		AppID:     "cli_test",
		AppSecret: "test_secret",
		Brand:     core.BrandFeishu,
	}
	factory, stdout, stderr, _ := cmdutil.TestFactory(t, config)
	return factory, stdout, stderr
}

func runIMShortcutForInputTest(t *testing.T, shortcut common.Shortcut, args []string, factory *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()

	parent := &cobra.Command{Use: "im"}
	shortcut.Mount(parent, factory)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	stdout.Reset()
	if stderr, ok := factory.IOStreams.ErrOut.(*bytes.Buffer); ok {
		stderr.Reset()
	}
	return parent.ExecuteContext(context.Background())
}

func firstDryRunBodyForInputTest(t *testing.T, raw string) map[string]interface{} {
	t.Helper()

	var payload struct {
		API []struct {
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, raw=%s", err, raw)
	}
	if len(payload.API) != 1 {
		t.Fatalf("api length = %d, want 1; raw=%s", len(payload.API), raw)
	}
	if payload.API[0].Body == nil {
		t.Fatalf("api[0].body missing; raw=%s", raw)
	}
	return payload.API[0].Body
}
