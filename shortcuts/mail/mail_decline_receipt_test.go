// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/shortcuts/common"
)

// TestMailDeclineReceipt_ShortcutMetadata verifies the shortcut is registered
// with the expected command name, risk level, and scopes. These are public
// contracts (they show up in `lark-cli mail --help` and the auth prompt);
// changes should be intentional.
func TestMailDeclineReceipt_ShortcutMetadata(t *testing.T) {
	if MailDeclineReceipt.Service != "mail" {
		t.Errorf("Service = %q, want %q", MailDeclineReceipt.Service, "mail")
	}
	if MailDeclineReceipt.Command != "+decline-receipt" {
		t.Errorf("Command = %q, want %q", MailDeclineReceipt.Command, "+decline-receipt")
	}
	// +decline-receipt only removes a local label, no outgoing mail — Risk is
	// "write", not "high-risk-write" that +send-receipt uses. Writers should
	// not need --yes.
	if MailDeclineReceipt.Risk != "write" {
		t.Errorf("Risk = %q, want %q", MailDeclineReceipt.Risk, "write")
	}
	// modify scope is required to flip label_ids; readonly scopes are
	// required because fetchFullMessage(..., false) hits plain_text_full
	// which the backend scope-checks against body:read.
	required := map[string]bool{
		"mail:user_mailbox.message:modify":    true,
		"mail:user_mailbox.message:readonly":  true,
		"mail:user_mailbox:readonly":          true,
		"mail:user_mailbox.message.body:read": true,
	}
	for _, s := range MailDeclineReceipt.Scopes {
		delete(required, s)
	}
	if len(required) != 0 {
		t.Errorf("MailDeclineReceipt.Scopes missing %v", required)
	}
	if len(MailDeclineReceipt.AuthTypes) != 1 || MailDeclineReceipt.AuthTypes[0] != "user" {
		t.Errorf("AuthTypes = %v, want [user]", MailDeclineReceipt.AuthTypes)
	}
	// --message-id must be marked Required so the framework fails fast
	// before we enter Execute; otherwise the fetchFullMessage call would
	// hit the API with an empty id.
	var found bool
	for _, f := range MailDeclineReceipt.Flags {
		if f.Name == "message-id" && f.Required {
			found = true
			break
		}
	}
	if !found {
		t.Error("--message-id flag must be marked Required")
	}
}

// runtimeForMailDeclineReceiptDryRun builds a minimal runtime with the flags
// MailDeclineReceipt declares, mirroring the pattern used by
// runtimeForMailTriageTest in mail_triage_test.go.
func runtimeForMailDeclineReceiptDryRun(t *testing.T, values map[string]string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	for _, fl := range MailDeclineReceipt.Flags {
		cmd.Flags().String(fl.Name, "", "")
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

// TestMailDeclineReceipt_DryRun verifies the DryRun plan prints the two calls
// the Execute path performs: a GET to fetch the original message (so we can
// check the READ_RECEIPT_REQUEST label is present) and a PUT to
// user_mailbox.message.modify that removes the label by its symbolic name.
// Pinning both URLs, methods, and the body shape here means a regression
// that reverts to the batch endpoint or leaks the numeric "-607" id shows
// up immediately without requiring a full integration round trip.
func TestMailDeclineReceipt_DryRun(t *testing.T) {
	runtime := runtimeForMailDeclineReceiptDryRun(t, map[string]string{
		"message-id": "msg-1",
	})

	dry := MailDeclineReceipt.DryRun(context.Background(), runtime)
	raw, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry-run failed: %v", err)
	}
	s := string(raw)

	for _, want := range []string{
		`"method":"GET"`,
		`/user_mailboxes/me/messages/msg-1`,
		`"method":"PUT"`,
		`/user_mailboxes/me/messages/msg-1/modify`,
		`"remove_label_ids":["READ_RECEIPT_REQUEST"]`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("dry-run JSON missing %q; got:\n%s", want, s)
		}
	}
	// Regression guards: batch endpoint shape and internal numeric id must
	// not leak into the OpenAPI payload.
	for _, forbidden := range []string{
		"batch_modify_message",
		`"-607"`,
		`"message_ids"`,
	} {
		if strings.Contains(s, forbidden) {
			t.Errorf("dry-run JSON should not contain %q; got:\n%s", forbidden, s)
		}
	}
}

// TestMailDeclineReceipt_DescriptionCoversFeatureIntent makes the Shortcut
// Description a tested string — it is the human-readable explanation piped
// into SKILL.md's Shortcut index table by the generator, so changes there
// should be intentional.
func TestMailDeclineReceipt_DescriptionCoversFeatureIntent(t *testing.T) {
	desc := strings.ToLower(MailDeclineReceipt.Description)
	for _, want := range []string{
		"dismiss",
		"without sending",
		"read_receipt_request",
		"idempotent",
	} {
		if !strings.Contains(desc, want) {
			t.Errorf("Description should mention %q; got: %s", want, MailDeclineReceipt.Description)
		}
	}
}
