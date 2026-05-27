// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// TestMailDraftSend_Metadata pins the public surface of the +draft-send
// shortcut: command name, risk level, scopes, auth type, and the three
// declared flags. Changing any of these is a public-contract change and must
// be intentional.
func TestMailDraftSend_Metadata(t *testing.T) {
	if MailDraftSend.Service != "mail" {
		t.Errorf("Service = %q, want %q", MailDraftSend.Service, "mail")
	}
	if MailDraftSend.Command != "+draft-send" {
		t.Errorf("Command = %q, want %q", MailDraftSend.Command, "+draft-send")
	}
	if MailDraftSend.Risk != "high-risk-write" {
		t.Errorf("Risk = %q, want %q", MailDraftSend.Risk, "high-risk-write")
	}
	if !MailDraftSend.HasFormat {
		t.Error("HasFormat must be true so --format is auto-injected")
	}
	if len(MailDraftSend.AuthTypes) != 1 || MailDraftSend.AuthTypes[0] != "user" {
		t.Errorf("AuthTypes = %v, want [user]", MailDraftSend.AuthTypes)
	}
	// Minimum-permission rule: only :send. Adding :modify or :readonly here is
	// an explicit scope-policy regression.
	if len(MailDraftSend.Scopes) != 1 || MailDraftSend.Scopes[0] != "mail:user_mailbox.message:send" {
		t.Errorf("Scopes = %v, want [mail:user_mailbox.message:send]", MailDraftSend.Scopes)
	}

	flagByName := map[string]common.Flag{}
	for _, fl := range MailDraftSend.Flags {
		flagByName[fl.Name] = fl
	}
	mailbox, ok := flagByName["mailbox"]
	if !ok {
		t.Fatal("missing --mailbox flag")
	}
	if mailbox.Required {
		t.Error("--mailbox must NOT be Required (defaults to me via resolveComposeMailboxID)")
	}
	if mailbox.Default != "" {
		t.Errorf("--mailbox Default should be empty (let resolveComposeMailboxID supply 'me'); got %q", mailbox.Default)
	}
	draftID, ok := flagByName["draft-id"]
	if !ok {
		t.Fatal("missing --draft-id flag")
	}
	if !draftID.Required {
		t.Error("--draft-id must be Required so cobra rejects missing-flag invocations")
	}
	if draftID.Type != "string_slice" {
		t.Errorf("--draft-id Type = %q, want %q", draftID.Type, "string_slice")
	}
	stopOnErr, ok := flagByName["stop-on-error"]
	if !ok {
		t.Fatal("missing --stop-on-error flag")
	}
	if stopOnErr.Required {
		t.Error("--stop-on-error must be optional")
	}
	if stopOnErr.Type != "bool" {
		t.Errorf("--stop-on-error Type = %q, want %q", stopOnErr.Type, "bool")
	}
}

// stubDraftSend registers a stub for POST .../drafts/<draftID>/send with the
// supplied response body. Used to assemble multi-draft test scenarios.
func stubDraftSend(reg *httpmock.Registry, draftID string, body map[string]interface{}) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts/" + draftID + "/send",
		Body:   body,
	}
	reg.Register(stub)
	return stub
}

// TestMailDraftSend_AllSuccess verifies the happy path: every draft sends
// successfully, sent[] is fully populated, failed[] is omitted from the JSON,
// and exit code = 0 (err == nil).
func TestMailDraftSend_AllSuccess(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubDraftSend(reg, "d1", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"message_id": "msg_1",
			"thread_id":  "thread_1",
		},
	})
	stubDraftSend(reg, "d2", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"message_id": "msg_2",
			"thread_id":  "thread_2",
		},
	})

	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", "d1,d2",
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected nil err on full success, got %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["total"].(float64) != 2 {
		t.Errorf("total = %v, want 2", data["total"])
	}
	if data["success_count"].(float64) != 2 {
		t.Errorf("success_count = %v, want 2", data["success_count"])
	}
	if data["failure_count"].(float64) != 0 {
		t.Errorf("failure_count = %v, want 0", data["failure_count"])
	}
	sent, ok := data["sent"].([]interface{})
	if !ok || len(sent) != 2 {
		t.Fatalf("sent[] missing or wrong size: %#v", data["sent"])
	}
	if _, exists := data["failed"]; exists {
		t.Errorf("failed[] should be omitted on full success; got %#v", data["failed"])
	}
	first := sent[0].(map[string]interface{})
	if first["draft_id"] != "d1" || first["message_id"] != "msg_1" || first["thread_id"] != "thread_1" {
		t.Errorf("first sent entry shape unexpected: %#v", first)
	}
}

// TestMailDraftSend_ProgressWritesToStderr verifies long sends do not look
// hung: per-draft progress is emitted on stderr while stdout remains the
// final machine-readable JSON ledger.
func TestMailDraftSend_ProgressWritesToStderr(t *testing.T) {
	f, stdout, stderr, reg := mailShortcutTestFactory(t)
	stubDraftSend(reg, "d1", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"message_id": "msg_1",
		},
	})
	stubDraftSend(reg, "d2", map[string]interface{}{
		"code": 230001,
		"msg":  "draft not found",
	})
	stubDraftSend(reg, "d3", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"message_id": "msg_3",
		},
	})

	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", "d1,d2,d3",
		"--yes",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected partial_failure error, got nil")
	}

	progress := stderr.String()
	for _, want := range []string{
		"mail +draft-send: [1/3] sending draft d1",
		"mail +draft-send: [1/3] sent draft d1 message_id=msg_1",
		"mail +draft-send: [2/3] sending draft d2",
		"mail +draft-send: [2/3] failed draft d2:",
		"mail +draft-send: [3/3] sending draft d3",
		"mail +draft-send: [3/3] sent draft d3 message_id=msg_3",
	} {
		if !strings.Contains(progress, want) {
			t.Errorf("stderr missing %q; got %s", want, progress)
		}
	}
	if strings.Contains(stdout.String(), "mail +draft-send:") {
		t.Errorf("stdout must not contain progress lines; got %s", stdout.String())
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["success_count"].(float64) != 2 || data["failure_count"].(float64) != 1 {
		t.Errorf("unexpected aggregate counts: %#v", data)
	}
}

// TestMailDraftSend_PartialFailure verifies that one recoverable per-draft
// failure does not abort the batch; the remaining drafts are attempted; both
// arrays are populated; and the call returns ExitAPI/"partial_failure".
func TestMailDraftSend_PartialFailure(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubDraftSend(reg, "d1", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"message_id": "msg_1"},
	})
	// Non-fatal code (not in the {auth, app_status, config, permission,
	// network, 1234013, 1236007, 1236008, 1236009, 1236010, 1236013}
	// set) → recoverable.
	stubDraftSend(reg, "d2", map[string]interface{}{
		"code": 230001,
		"msg":  "draft not found or already sent",
	})
	stubDraftSend(reg, "d3", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"message_id": "msg_3"},
	})

	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", "d1,d2,d3",
		"--yes",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected partial_failure error, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitAPI {
		t.Errorf("Code = %d, want ExitAPI=%d", exitErr.Code, output.ExitAPI)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "partial_failure" {
		t.Errorf("Detail.Type = %v, want partial_failure", exitErr.Detail)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["total"].(float64) != 3 {
		t.Errorf("total = %v, want 3", data["total"])
	}
	if data["success_count"].(float64) != 2 {
		t.Errorf("success_count = %v, want 2", data["success_count"])
	}
	if data["failure_count"].(float64) != 1 {
		t.Errorf("failure_count = %v, want 1", data["failure_count"])
	}
	failed, ok := data["failed"].([]interface{})
	if !ok || len(failed) != 1 {
		t.Fatalf("failed[] missing or wrong size: %#v", data["failed"])
	}
	failedEntry := failed[0].(map[string]interface{})
	if failedEntry["draft_id"] != "d2" {
		t.Errorf("failed entry draft_id = %v, want d2", failedEntry["draft_id"])
	}
	if !strings.Contains(strings.ToLower(failedEntry["error"].(string)), "draft not found") {
		t.Errorf("failed entry error should contain server msg, got %q", failedEntry["error"])
	}
}

// TestMailDraftSend_StopOnError verifies --stop-on-error short-circuits at the
// first recoverable failure. d3 is intentionally NOT stubbed: if the loop
// kept going, the httpmock RoundTripper would return "no stub for POST
// /user_mailboxes/me/drafts/d3/send" and Execute would surface it.
func TestMailDraftSend_StopOnError(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubDraftSend(reg, "d1", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"message_id": "msg_1"},
	})
	stubDraftSend(reg, "d2", map[string]interface{}{
		"code": 230001,
		"msg":  "draft not found",
	})

	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", "d1,d2,d3",
		"--yes",
		"--stop-on-error",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected partial_failure error, got nil")
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["success_count"].(float64) != 1 {
		t.Errorf("success_count = %v, want 1", data["success_count"])
	}
	if data["failure_count"].(float64) != 1 {
		t.Errorf("failure_count = %v, want 1", data["failure_count"])
	}
	if data["total"].(float64) != 3 {
		t.Errorf("total = %v, want 3", data["total"])
	}
}

// TestMailDraftSend_FatalAborts verifies that a fatal errno (mailbox not
// found) aborts the batch immediately and does NOT populate failed[]; the
// later drafts are not attempted (d2 is intentionally not stubbed — any
// attempt would be observable as a runner failure from the httpmock layer).
func TestMailDraftSend_FatalAborts(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubDraftSend(reg, "d1", map[string]interface{}{
		"code": output.LarkErrMailboxNotFound,
		"msg":  "mailbox not found",
	})

	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", "d1,d2",
		"--yes",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected fatal abort error, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if exitErr.Detail == nil || exitErr.Detail.Code != output.LarkErrMailboxNotFound {
		t.Errorf("expected Detail.Code = %d, got %#v", output.LarkErrMailboxNotFound, exitErr.Detail)
	}
	// No JSON envelope on stdout because Execute returned early before rt.Out.
	if stdout.Len() != 0 {
		t.Errorf("expected no JSON output on fatal abort, got %s", stdout.String())
	}
}

// TestMailDraftSend_FatalAfterSuccessEmitsLedger verifies that a fatal error
// after earlier side effects still emits the aggregate stdout ledger before
// returning the fatal stderr error. This lets callers avoid blindly retrying a
// draft that was already sent.
func TestMailDraftSend_FatalAfterSuccessEmitsLedger(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubDraftSend(reg, "d1", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"message_id": "msg_1"},
	})
	stubDraftSend(reg, "d2", map[string]interface{}{
		"code": output.LarkErrMailSendQuotaUser,
		"msg":  "user daily send count exceeded",
	})

	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", "d1,d2,d3",
		"--yes",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected fatal abort error, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if exitErr.Detail == nil || exitErr.Detail.Code != output.LarkErrMailSendQuotaUser {
		t.Errorf("expected Detail.Code = %d, got %#v", output.LarkErrMailSendQuotaUser, exitErr.Detail)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["total"].(float64) != 3 {
		t.Errorf("total = %v, want 3", data["total"])
	}
	if data["success_count"].(float64) != 1 {
		t.Errorf("success_count = %v, want 1", data["success_count"])
	}
	if data["failure_count"].(float64) != 1 {
		t.Errorf("failure_count = %v, want 1", data["failure_count"])
	}
	if got := gjsonLikeString(t, data, "sent", 0, "draft_id"); got != "d1" {
		t.Errorf("sent[0].draft_id = %q, want d1", got)
	}
	if got := gjsonLikeString(t, data, "failed", 0, "draft_id"); got != "d2" {
		t.Errorf("failed[0].draft_id = %q, want d2", got)
	}
}

// TestMailDraftSend_AutomationDisabled verifies that an HTTP-success response
// carrying the automation_send_disable signal aborts the batch with
// ExitAPI/"automation_send_disabled" and does NOT continue to subsequent
// drafts (d2 intentionally has no stub — any attempt would surface as an
// httpmock "no stub" failure).
func TestMailDraftSend_AutomationDisabled(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubDraftSend(reg, "d1", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"message_id": "msg_1",
			"automation_send_disable": map[string]interface{}{
				"reason": "policy: outbound automation disabled",
			},
		},
	})

	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", "d1,d2",
		"--yes",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected automation_send_disabled error, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if exitErr.Code != output.ExitAPI {
		t.Errorf("Code = %d, want ExitAPI=%d", exitErr.Code, output.ExitAPI)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "automation_send_disabled" {
		t.Errorf("Detail.Type = %v, want automation_send_disabled", exitErr.Detail)
	}
	if !strings.Contains(exitErr.Error(), "outbound automation disabled") {
		t.Errorf("error message should propagate reason, got %q", exitErr.Error())
	}
}

// TestMailDraftSend_AutomationDisabledAfterSuccessEmitsLedger verifies that an
// automation-send policy stop after earlier successful sends still writes the
// batch ledger to stdout before returning the structured fatal error.
func TestMailDraftSend_AutomationDisabledAfterSuccessEmitsLedger(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubDraftSend(reg, "d1", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"message_id": "msg_1"},
	})
	stubDraftSend(reg, "d2", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"message_id": "msg_2",
			"automation_send_disable": map[string]interface{}{
				"reason": "policy: outbound automation disabled",
			},
		},
	})

	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", "d1,d2,d3",
		"--yes",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected automation_send_disabled error, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "automation_send_disabled" {
		t.Errorf("Detail.Type = %v, want automation_send_disabled", exitErr.Detail)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["total"].(float64) != 3 {
		t.Errorf("total = %v, want 3", data["total"])
	}
	if data["success_count"].(float64) != 1 {
		t.Errorf("success_count = %v, want 1", data["success_count"])
	}
	if data["failure_count"].(float64) != 1 {
		t.Errorf("failure_count = %v, want 1", data["failure_count"])
	}
	if got := gjsonLikeString(t, data, "sent", 0, "draft_id"); got != "d1" {
		t.Errorf("sent[0].draft_id = %q, want d1", got)
	}
	if got := gjsonLikeString(t, data, "failed", 0, "draft_id"); got != "d2" {
		t.Errorf("failed[0].draft_id = %q, want d2", got)
	}
	if got := gjsonLikeString(t, data, "failed", 0, "error"); !strings.Contains(got, "outbound automation disabled") {
		t.Errorf("failed[0].error should contain reason, got %q", got)
	}
}

// TestMailDraftSend_ValidateErrors verifies that input-shape problems are
// caught in the pre-call layers (cobra Required + Validate). No network call
// is registered; the test should fail loudly if any HTTP call is attempted
// (httpmock returns "no stub" in that case).
func TestMailDraftSend_ValidateErrors(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantSub   string
		wantCobra bool // true → cobra-level MarkFlagRequired error path
	}{
		{
			name:      "missing draft-id",
			args:      []string{"+draft-send", "--yes"},
			wantSub:   `required flag(s) "draft-id" not set`,
			wantCobra: true,
		},
		{
			// cobra's StringSlice treats a bare "" as an unset flag, so pass a
			// whitespace-only element instead to drive the Validate-callback
			// empty-element branch.
			name:    "whitespace-only value",
			args:    []string{"+draft-send", "--draft-id", "   ", "--yes"},
			wantSub: "--draft-id contains empty value",
		},
		{
			name:    "exceeds cap",
			args:    []string{"+draft-send", "--draft-id", manyDraftIDs(MaxBatchSendDrafts + 1), "--yes"},
			wantSub: "too many drafts",
		},
		{
			name:    "duplicate value",
			args:    []string{"+draft-send", "--draft-id", "d1,d2,d1", "--yes"},
			wantSub: "--draft-id contains duplicate value: d1",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, MailDraftSend, c.args, f, stdout)
			if err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("err = %v, want substring %q", err, c.wantSub)
			}
		})
	}
}

func TestMailDraftSend_DryRunValidateErrors(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{
			name:    "whitespace-only value",
			args:    []string{"+draft-send", "--draft-id", "   ", "--dry-run"},
			wantSub: "--draft-id contains empty value",
		},
		{
			name:    "exceeds cap",
			args:    []string{"+draft-send", "--draft-id", manyDraftIDs(MaxBatchSendDrafts + 1), "--dry-run"},
			wantSub: "too many drafts",
		},
		{
			name:    "duplicate value",
			args:    []string{"+draft-send", "--draft-id", "d1,d2,d1", "--dry-run"},
			wantSub: "--draft-id contains duplicate value: d1",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, MailDraftSend, c.args, f, stdout)
			if err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("err = %v, want substring %q", err, c.wantSub)
			}
			if stdout.Len() != 0 {
				t.Errorf("expected no dry-run output on validation error, got %s", stdout.String())
			}
		})
	}
}

// manyDraftIDs returns a CSV string with n synthesised IDs. Used to drive the
// >MaxBatchSendDrafts validation branch without bloating the test file with a
// hand-written list.
func manyDraftIDs(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "d" + strings.Repeat("x", 1) + intToString(i)
	}
	return strings.Join(parts, ",")
}

// intToString avoids the strconv import noise for a tiny test helper.
func intToString(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// TestMailDraftSend_MissingYes verifies the framework's high-risk-write
// confirmation gate triggers ExitConfirmationRequired (10) when --yes is
// omitted, before Execute is called.
func TestMailDraftSend_MissingYes(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", "d1",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected ExitConfirmationRequired, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if exitErr.Code != output.ExitConfirmationRequired {
		t.Errorf("Code = %d, want ExitConfirmationRequired=%d", exitErr.Code, output.ExitConfirmationRequired)
	}
}

// TestMailDraftSend_DryRun verifies --dry-run prints N POST calls in input
// order and does NOT touch the network.
func TestMailDraftSend_DryRun(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", " d1 , d2 ",
		"--draft-id", " d3 ",
		"--yes",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	s := stdout.String()
	for _, want := range []string{
		`/user_mailboxes/me/drafts/d1/send`,
		`/user_mailboxes/me/drafts/d2/send`,
		`/user_mailboxes/me/drafts/d3/send`,
		`"method"`,
		`"POST"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("dry-run output missing %q; got %s", want, s)
		}
	}
}

// TestMailDraftSend_NormalizesDraftIDs verifies request paths and output use
// trimmed draft IDs rather than preserving CLI whitespace.
func TestMailDraftSend_NormalizesDraftIDs(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubDraftSend(reg, "d1", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"message_id": "msg_1"},
	})
	stubDraftSend(reg, "d2", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"message_id": "msg_2"},
	})

	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", " d1 , d2 ",
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if got := gjsonLikeString(t, data, "sent", 0, "draft_id"); got != "d1" {
		t.Errorf("sent[0].draft_id = %q, want d1", got)
	}
	if got := gjsonLikeString(t, data, "sent", 1, "draft_id"); got != "d2" {
		t.Errorf("sent[1].draft_id = %q, want d2", got)
	}
}

// TestMailDraftSend_DryRunDirectInvocation drives dryRunDraftSend through a
// hand-built RuntimeContext so the dry-run plan can be inspected without the
// full Mount pipeline. Useful for catching path-encoding regressions in
// mailboxPath().
func TestMailDraftSend_DryRunDirectInvocation(t *testing.T) {
	rt := runtimeForMailDraftSendTest(t, map[string]string{
		"mailbox": "alice@example.com",
	}, []string{"d1", "d2"})
	api := dryRunDraftSend(context.Background(), rt)
	raw, err := json.Marshal(api)
	if err != nil {
		t.Fatalf("marshal dry-run failed: %v", err)
	}
	s := string(raw)
	for _, want := range []string{
		`/user_mailboxes/alice@example.com/drafts/d1/send`,
		`/user_mailboxes/alice@example.com/drafts/d2/send`,
		`"method":"POST"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("dry-run JSON missing %q; got %s", want, s)
		}
	}
}

// runtimeForMailDraftSendTest builds a minimal RuntimeContext with the +draft-
// send flag set so the DryRun callback can be exercised directly. Mirrors
// runtimeForMailDeclineReceiptDryRun.
func runtimeForMailDraftSendTest(t *testing.T, strFlags map[string]string, draftIDs []string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("mailbox", "", "")
	cmd.Flags().StringSlice("draft-id", nil, "")
	cmd.Flags().Bool("stop-on-error", false, "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("parse flags failed: %v", err)
	}
	for k, v := range strFlags {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set flag --%s failed: %v", k, err)
		}
	}
	for _, id := range draftIDs {
		if err := cmd.Flags().Set("draft-id", id); err != nil {
			t.Fatalf("set draft-id failed: %v", err)
		}
	}
	return &common.RuntimeContext{Cmd: cmd}
}

// TestMailDraftSend_MailboxFallback verifies that omitting --mailbox falls
// through to "me" via resolveComposeMailboxID, and the output reflects it.
func TestMailDraftSend_MailboxFallback(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubDraftSend(reg, "d1", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"message_id": "msg_1"},
	})

	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", "d1",
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["mailbox_id"] != "me" {
		t.Errorf("mailbox_id = %v, want me (default)", data["mailbox_id"])
	}
}

// TestMailDraftSend_RepeatedFlagAndCSV verifies that string_slice supports
// both the repeated-flag form (--draft-id d1 --draft-id d2) and the
// comma-separated form (--draft-id d1,d2) — and mixing both in one invocation.
func TestMailDraftSend_RepeatedFlagAndCSV(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubDraftSend(reg, "d1", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"message_id": "msg_1"},
	})
	stubDraftSend(reg, "d2", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"message_id": "msg_2"},
	})
	stubDraftSend(reg, "d3", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"message_id": "msg_3"},
	})

	err := runMountedMailShortcut(t, MailDraftSend, []string{
		"+draft-send",
		"--draft-id", "d1,d2",
		"--draft-id", "d3",
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["success_count"].(float64) != 3 {
		t.Errorf("success_count = %v, want 3", data["success_count"])
	}
}

// TestIsFatalSendErr is a focused unit test for the classifier. Covers every
// branch documented in the doc comment so future tweaks immediately surface
// mis-categorisation.
func TestIsFatalSendErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil-like / unknown shape → fatal",
			err:  errors.New("raw network panic surfaced unwrapped"),
			want: true,
		},
		{
			name: "ExitError without Detail → fatal",
			err:  &output.ExitError{Code: output.ExitInternal},
			want: true,
		},
		{
			name: "auth → fatal",
			err: &output.ExitError{
				Code:   output.ExitAuth,
				Detail: &output.ErrDetail{Type: "auth", Message: "token expired"},
			},
			want: true,
		},
		{
			name: "app_status → fatal",
			err: &output.ExitError{
				Code:   output.ExitAuth,
				Detail: &output.ErrDetail{Type: "app_status", Message: "app disabled"},
			},
			want: true,
		},
		{
			name: "config → fatal",
			err: &output.ExitError{
				Code:   output.ExitAuth,
				Detail: &output.ErrDetail{Type: "config", Message: "bad app_id"},
			},
			want: true,
		},
		{
			name: "permission → fatal",
			err: &output.ExitError{
				Code:   output.ExitAPI,
				Detail: &output.ErrDetail{Type: "permission", Message: "denied"},
			},
			want: true,
		},
		{
			name: "rate_limit → fatal",
			err: &output.ExitError{
				Code:   output.ExitAPI,
				Detail: &output.ErrDetail{Type: "rate_limit", Code: output.LarkErrRateLimit},
			},
			want: true,
		},
		{
			name: "ExitNetwork → fatal",
			err: &output.ExitError{
				Code:   output.ExitNetwork,
				Detail: &output.ErrDetail{Type: "network", Message: "DNS timeout"},
			},
			want: true,
		},
		{
			name: "wrapped ExitNetwork → fatal",
			err:  output.Errorf(output.ExitAPI, "api_error", "API call failed: %s", output.ErrNetwork("DNS timeout")),
			want: true,
		},
		{
			name: "LarkErrMailboxNotFound → fatal",
			err: &output.ExitError{
				Code:   output.ExitAPI,
				Detail: &output.ErrDetail{Type: "api_error", Code: output.LarkErrMailboxNotFound},
			},
			want: true,
		},
		{
			name: "LarkErrMailSendQuotaUser → fatal",
			err: &output.ExitError{
				Code:   output.ExitAPI,
				Detail: &output.ErrDetail{Type: "api_error", Code: output.LarkErrMailSendQuotaUser},
			},
			want: true,
		},
		{
			name: "LarkErrTenantStorageLimit → fatal",
			err: &output.ExitError{
				Code:   output.ExitAPI,
				Detail: &output.ErrDetail{Type: "api_error", Code: output.LarkErrTenantStorageLimit},
			},
			want: true,
		},
		{
			name: "generic api_error → recoverable",
			err: &output.ExitError{
				Code:   output.ExitAPI,
				Detail: &output.ErrDetail{Type: "api_error", Code: 230001},
			},
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := isFatalSendErr(c.err)
			if got != c.want {
				t.Errorf("isFatalSendErr(%s) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

// TestExtractAutomationDisabledReason verifies all branches of the helper:
// missing key → "", malformed map → generic message, empty/whitespace reason
// → generic message, non-empty reason → trimmed value.
func TestExtractAutomationDisabledReason(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]interface{}
		want string
	}{
		{"missing key", map[string]interface{}{"message_id": "x"}, ""},
		{"non-map value", map[string]interface{}{
			"automation_send_disable": "not a map",
		}, "automation send disabled (no reason provided)"},
		{"map but no reason", map[string]interface{}{
			"automation_send_disable": map[string]interface{}{},
		}, "automation send disabled (no reason provided)"},
		{"reason empty", map[string]interface{}{
			"automation_send_disable": map[string]interface{}{"reason": "   "},
		}, "automation send disabled (no reason provided)"},
		{"reason populated", map[string]interface{}{
			"automation_send_disable": map[string]interface{}{"reason": "  policy block  "},
		}, "policy block"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractAutomationDisabledReason(c.in)
			if got != c.want {
				t.Errorf("extractAutomationDisabledReason() = %q, want %q", got, c.want)
			}
		})
	}
}

func gjsonLikeString(t *testing.T, data map[string]interface{}, arrayKey string, index int, field string) string {
	t.Helper()
	items, ok := data[arrayKey].([]interface{})
	if !ok {
		t.Fatalf("%s missing or wrong type: %#v", arrayKey, data[arrayKey])
	}
	if index >= len(items) {
		t.Fatalf("%s[%d] missing; len=%d", arrayKey, index, len(items))
	}
	item, ok := items[index].(map[string]interface{})
	if !ok {
		t.Fatalf("%s[%d] wrong type: %#v", arrayKey, index, items[index])
	}
	value, ok := item[field].(string)
	if !ok {
		t.Fatalf("%s[%d].%s missing or wrong type: %#v", arrayKey, index, field, item[field])
	}
	return value
}
