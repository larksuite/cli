// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/shortcuts/common"
)

func TestMailMessageHelpMentionsSingleMessageOnly(t *testing.T) {
	help := strings.ToLower(mountedShortcutHelp(t, MailMessage))
	for _, want := range []string{
		"single email",
		"mail +messages",
		"do not loop mail +message",
		"single email message id",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("mail +message help should mention %q; got:\n%s", want, help)
		}
	}
}

func TestMailMessagesHelpMentionsBatchLimitAndAutoChunk(t *testing.T) {
	help := strings.ToLower(mountedShortcutHelp(t, MailMessages))
	for _, want := range []string{
		"multiple emails",
		"at most 20 ids per batch_get request",
		"merges output",
		"current backend raw request validation rejects more than 50 ids",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("mail +messages help should mention %q; got:\n%s", want, help)
		}
	}
}

func TestMailMessageDryRunMentionsSingleMessageOnly(t *testing.T) {
	runtime := runtimeForShortcutDryRun(t, MailMessage, map[string]string{
		"message-id": "msg_1",
	})
	got := strings.ToLower(marshalDryRun(t, MailMessage.DryRun(context.Background(), runtime)))
	for _, want := range []string{
		"one email only",
		"for multiple ids use mail +messages",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("mail +message dry-run should mention %q; got:\n%s", want, got)
		}
	}
}

func TestMailMessagesDryRunMentionsChunkAndBackendLimit(t *testing.T) {
	runtime := runtimeForShortcutDryRun(t, MailMessages, map[string]string{
		"message-ids": "msg_1,msg_2",
	})
	got := strings.ToLower(marshalDryRun(t, MailMessages.DryRun(context.Background(), runtime)))
	for _, want := range []string{
		"auto-chunks at most 20 ids per request",
		"merges output",
		"backend raw request validation rejects more than 50 ids",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("mail +messages dry-run should mention %q; got:\n%s", want, got)
		}
	}
}

func mountedShortcutHelp(t *testing.T, shortcut common.Shortcut) string {
	t.Helper()
	f, _, _, _ := mailShortcutTestFactory(t)
	parent := &cobra.Command{Use: "mail"}
	shortcut.Mount(parent, f)
	cmd, _, err := parent.Find([]string{shortcut.Command})
	if err != nil {
		t.Fatalf("find mounted shortcut %s: %v", shortcut.Command, err)
	}
	if cmd == parent {
		t.Fatalf("shortcut %s was not mounted", shortcut.Command)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Help(); err != nil {
		t.Fatalf("help for %s: %v", shortcut.Command, err)
	}
	return cmd.Short + "\n" + out.String()
}

func runtimeForShortcutDryRun(t *testing.T, shortcut common.Shortcut, values map[string]string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: shortcut.Command}
	for _, fl := range shortcut.Flags {
		switch fl.Type {
		case "bool":
			cmd.Flags().Bool(fl.Name, fl.Default == "true", "")
		default:
			cmd.Flags().String(fl.Name, fl.Default, "")
		}
	}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("parse flags failed: %v", err)
	}
	for k, v := range values {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set flag --%s failed: %v", k, err)
		}
	}
	return &common.RuntimeContext{Cmd: cmd}
}

func marshalDryRun(t *testing.T, dry *common.DryRunAPI) string {
	t.Helper()
	raw, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	return string(raw)
}
