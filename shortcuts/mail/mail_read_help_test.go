// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestMailMessageHelpClarifiesSingleMessageOnly(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)

	err := runMountedMailShortcutWithCobraOutput(t, MailMessage, []string{"+message", "-h"}, f, stdout)
	if err != nil {
		t.Fatalf("help returned error: %v", err)
	}

	help := stdout.String()
	for _, want := range []string{
		"Use only when reading full content for one email by one message ID",
		"For multiple message IDs, use mail +messages; do not loop mail +message",
		"Single email message ID only",
		"mail +messages --message-ids",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q\n%s", want, help)
		}
	}
}

func TestMailMessagesHelpClarifiesBatchGetChunkingAndLimits(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)

	err := runMountedMailShortcutWithCobraOutput(t, MailMessages, []string{"+messages", "-h"}, f, stdout)
	if err != nil {
		t.Fatalf("help returned error: %v", err)
	}

	help := stdout.String()
	for _, want := range []string{
		"multiple emails by message ID",
		"handles them in batches of 20 and merges output",
		"Comma-separated email message IDs",
		"You may pass more than 20 IDs",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q\n%s", want, help)
		}
	}
	for _, disallowed := range []string{"messages.batch_get", "OAPI Meta", "gateway config", "50 IDs", "50 个"} {
		if strings.Contains(help, disallowed) {
			t.Fatalf("help must not expose internal wording %q\n%s", disallowed, help)
		}
	}
}

func TestMailMessagesDryRunMentionsBatchGetChunkingAndMerge(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	messageIDs := []string{
		validMessageIDForTest("dry-run-1"),
		validMessageIDForTest("dry-run-2"),
	}

	err := runMountedMailShortcut(t, MailMessages, []string{
		"+messages", "--message-ids", strings.Join(messageIDs, ","), "--dry-run", "--format", "json",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"chunks every 20 IDs",
		"merges output",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run missing %q\n%s", want, out)
		}
	}
}

func TestMailTriageTableHintRoutesSingleAndMultipleReads(t *testing.T) {
	f, stdout, stderr, reg := mailShortcutTestFactory(t)
	registerTriageReadHintStubs(reg)

	err := runMountedMailShortcut(t, MailTriage, []string{
		"+triage", "--max", "1",
	}, f, stdout)
	if err != nil {
		t.Fatalf("triage returned error: %v", err)
	}
	reg.Verify(t)

	errOut := stderr.String()
	for _, want := range []string{
		"tip: read full content:",
		"single message use mail +message --message-id <id>",
		"multiple messages use mail +messages --message-ids <id1>,<id2>,<id3>",
	} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("stderr missing %q\n%s", want, errOut)
		}
	}
}

func TestMailTriageJSONDoesNotEmitReadHint(t *testing.T) {
	f, stdout, stderr, reg := mailShortcutTestFactory(t)
	registerTriageReadHintStubs(reg)

	err := runMountedMailShortcut(t, MailTriage, []string{
		"+triage", "--format", "json", "--max", "1",
	}, f, stdout)
	if err != nil {
		t.Fatalf("triage returned error: %v", err)
	}
	reg.Verify(t)

	if strings.Contains(stderr.String(), "tip: read full content:") {
		t.Fatalf("json output must not emit table read hint\nstderr=%s", stderr.String())
	}
}

func TestMailMessagesExecuteChunksTwentyOneIDsIntoTwoBatchGetCalls(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method:   "POST",
		URL:      "/user_mailboxes/me/messages/batch_get",
		Reusable: true,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"messages": []interface{}{}},
		},
	}
	reg.Register(stub)

	ids := make([]string, 21)
	for i := range ids {
		ids[i] = validMessageIDForTest(fmt.Sprintf("batch-%02d", i+1))
	}
	err := runMountedMailShortcut(t, MailMessages, []string{
		"+messages", "--message-ids", strings.Join(ids, ","),
	}, f, stdout)
	if err != nil {
		t.Fatalf("messages returned error: %v", err)
	}

	if got := len(stub.CapturedBodies); got != 2 {
		t.Fatalf("expected 2 batch_get calls, got %d", got)
	}
	assertBatchGetMessageIDCount(t, stub.CapturedBodies[0], 20)
	assertBatchGetMessageIDCount(t, stub.CapturedBodies[1], 1)
}

func registerTriageReadHintStubs(reg *httpmock.Registry) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/messages",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":      []interface{}{"msg_1"},
				"has_more":   false,
				"page_token": "",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/messages/batch_get",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"messages": []interface{}{
					map[string]interface{}{
						"message_id": "msg_1",
						"subject":    "Quarterly update",
						"date":       "Thu, 04 Jun 2026 10:00:00 +0800",
						"from":       map[string]interface{}{"name": "Alice", "mail_address": "alice@example.com"},
					},
				},
			},
		},
	})
}

func assertBatchGetMessageIDCount(t *testing.T, body []byte, want int) {
	t.Helper()
	var payload struct {
		MessageIDs []string `json:"message_ids"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal batch_get body: %v\n%s", err, string(body))
	}
	if got := len(payload.MessageIDs); got != want {
		t.Fatalf("message_ids count mismatch: got %d want %d body=%s", got, want, string(body))
	}
}

func runMountedMailShortcutWithCobraOutput(t *testing.T, shortcut common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "test"}
	parent.SetOut(stdout)
	parent.SetErr(stdout)
	shortcut.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	stdout.Reset()
	return parent.Execute()
}
