// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

func messageManageID(suffix string) string {
	return "msg_abcdefghijklmnop_" + suffix
}

func stubMessageManagePost(reg *httpmock.Registry, endpoint string, body map[string]interface{}) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/messages/" + endpoint,
		Body:   body,
	}
	reg.Register(stub)
	return stub
}

func decodeMessageManageSummary(t *testing.T, data map[string]interface{}) ([]interface{}, []interface{}) {
	t.Helper()
	success, ok := data["success_message_ids"].([]interface{})
	if !ok {
		t.Fatalf("success_message_ids = %#v, want array", data["success_message_ids"])
	}
	failed, ok := data["failed_message_ids"].([]interface{})
	if !ok {
		t.Fatalf("failed_message_ids = %#v, want array", data["failed_message_ids"])
	}
	return success, failed
}

func requireMessageManageValidationParam(t *testing.T, err error, param string) *errs.ValidationError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation error for %s, got nil", param)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError for %s, got %T", param, err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed Problem for %s, got %T", param, err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %s/%s, want validation/invalid_argument", problem.Category, problem.Subtype)
	}
	if validationErr.Param != param {
		t.Fatalf("param = %q, want %q", validationErr.Param, param)
	}
	return validationErr
}

func requireMessageManageFailedPrecondition(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected failed precondition error, got nil")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed Problem, got %T", err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("problem = %s/%s, want validation/failed_precondition", problem.Category, problem.Subtype)
	}
}

func TestMessageManage_NormalizeMessageIDs(t *testing.T) {
	id1 := messageManageID("1")
	id2 := messageManageID("2")
	got, err := normalizeMessageManageIDs([]string{id1, id2, id1})
	if err != nil {
		t.Fatalf("normalizeMessageManageIDs returned error: %v", err)
	}
	if len(got) != 2 || got[0] != id1 || got[1] != id2 {
		t.Fatalf("ids = %v, want [%s %s]", got, id1, id2)
	}
	got, err = normalizeMessageManageIDs([]string{id1 + "," + id2, id1})
	if err != nil {
		t.Fatalf("normalizeMessageManageIDs CSV/repeated returned error: %v", err)
	}
	if len(got) != 2 || got[0] != id1 || got[1] != id2 {
		t.Fatalf("CSV/repeated ids = %v, want [%s %s]", got, id1, id2)
	}

	cases := [][]string{
		{""},
		{" id_with_leading_space_12345"},
		{"msg_abcdefghijklmnop_1,msg_abcdefghijklmnop_2 "},
		{"1234567890123456"},
		{"short"},
		{"msg_abcdefghijklmnop!"},
		{"msg_abcdefghijklmnop\t"},
		{"msg_abcdefghijklmnop_1\nmsg_abcdefghijklmnop_2"},
		{"msg_abcdefghijklmnop_1", "msg_abcdefghijklmnop_2 "},
	}
	for _, tc := range cases {
		_, err := normalizeMessageManageIDs(tc)
		requireMessageManageValidationParam(t, err, "--message-ids")
	}
}

func TestMessageModify_Metadata(t *testing.T) {
	if MailMessageModify.Command != "+message-modify" {
		t.Fatalf("Command = %q", MailMessageModify.Command)
	}
	if MailMessageModify.Risk != "write" {
		t.Errorf("Risk = %q, want write", MailMessageModify.Risk)
	}
	if len(MailMessageModify.AuthTypes) != 1 || MailMessageModify.AuthTypes[0] != "user" {
		t.Errorf("AuthTypes = %v, want [user]", MailMessageModify.AuthTypes)
	}
	requiredScopes := map[string]bool{
		"mail:user_mailbox.message:modify": true,
	}
	for _, scope := range MailMessageModify.Scopes {
		delete(requiredScopes, scope)
	}
	if len(requiredScopes) != 0 {
		t.Errorf("Scopes missing %v", requiredScopes)
	}
	if len(MailMessageModify.ConditionalScopes) != 1 || MailMessageModify.ConditionalScopes[0] != "mail:user_mailbox.folder:read" {
		t.Errorf("ConditionalScopes = %v, want [mail:user_mailbox.folder:read]", MailMessageModify.ConditionalScopes)
	}
	flags := map[string]common.Flag{}
	for _, fl := range MailMessageModify.Flags {
		flags[fl.Name] = fl
	}
	for _, name := range []string{"mailbox", "message-ids", "add-label-ids", "remove-label-ids", "add-folder"} {
		if _, ok := flags[name]; !ok {
			t.Fatalf("missing --%s flag", name)
		}
	}
	if flags["message-ids"].Type != "string_array" || !flags["message-ids"].Required {
		t.Errorf("--message-ids = %#v, want required string_array", flags["message-ids"])
	}
}

func TestMessageTrash_Metadata(t *testing.T) {
	if MailMessageTrash.Command != "+message-trash" {
		t.Fatalf("Command = %q", MailMessageTrash.Command)
	}
	if MailMessageTrash.Risk != "high-risk-write" {
		t.Errorf("Risk = %q, want high-risk-write", MailMessageTrash.Risk)
	}
	if len(MailMessageTrash.AuthTypes) != 1 || MailMessageTrash.AuthTypes[0] != "user" {
		t.Errorf("AuthTypes = %v, want [user]", MailMessageTrash.AuthTypes)
	}
	if len(MailMessageTrash.Scopes) != 1 || MailMessageTrash.Scopes[0] != "mail:user_mailbox.message:modify" {
		t.Errorf("Scopes = %v, want [mail:user_mailbox.message:modify]", MailMessageTrash.Scopes)
	}
}

func TestMessageModify_LabelOnlyDoesNotRequireFolderReadScope(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	token := auth.GetStoredToken("test-app", "ou_testuser")
	if token == nil {
		t.Fatal("expected test token")
	}
	token.Scope = strings.ReplaceAll(token.Scope, " mail:user_mailbox.folder:read", "")
	if err := auth.SetStoredToken(token); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	id := messageManageID("1")
	post := stubMessageManagePost(reg, "batch_modify", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})

	err := runMountedMailShortcut(t, MailMessageModify, []string{
		"+message-modify",
		"--message-ids", id,
		"--remove-label-ids", "UNREAD",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(post.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	removeLabels := body["remove_label_ids"].([]interface{})
	if len(removeLabels) != 1 || removeLabels[0] != "UNREAD" {
		t.Fatalf("remove_label_ids = %#v, want [UNREAD]", removeLabels)
	}
}

func TestMessageModify_ReadReceiptRequestLabelIsSystemLabel(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id := messageManageID("1")
	post := stubMessageManagePost(reg, "batch_modify", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})

	err := runMountedMailShortcut(t, MailMessageModify, []string{
		"+message-modify",
		"--message-ids", id,
		"--remove-label-ids", "read_receipt_request",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(post.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	removeLabels := body["remove_label_ids"].([]interface{})
	if len(removeLabels) != 1 || removeLabels[0] != "READ_RECEIPT_REQUEST" {
		t.Fatalf("remove_label_ids = %#v, want [READ_RECEIPT_REQUEST]", removeLabels)
	}
}

func TestMessageModify_LabelFolderNormalizationAndValidationAPIs(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id := messageManageID("1")
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/user_mailboxes/me/labels/customA", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"label_id": "customA"}}})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/user_mailboxes/me/folders/folderA", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"folder_id": "folderA"}}})
	post := stubMessageManagePost(reg, "batch_modify", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})

	err := runMountedMailShortcut(t, MailMessageModify, []string{
		"+message-modify",
		"--message-ids", id,
		"--add-label-ids", "unread,customA",
		"--remove-label-ids", "FLAGGED",
		"--add-folder", "folderA",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(post.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	if got := body["add_folder"]; got != "folderA" {
		t.Errorf("add_folder = %v, want folderA", got)
	}
	addLabels := body["add_label_ids"].([]interface{})
	if addLabels[0] != "UNREAD" || addLabels[1] != "customA" {
		t.Errorf("add_label_ids = %#v, want [UNREAD customA]", addLabels)
	}
	removeLabels := body["remove_label_ids"].([]interface{})
	if removeLabels[0] != "FLAGGED" {
		t.Errorf("remove_label_ids = %#v, want [FLAGGED]", removeLabels)
	}
	success, failed := decodeMessageManageSummary(t, decodeShortcutEnvelopeData(t, stdout))
	if len(success) != 1 || success[0] != id || len(failed) != 0 {
		t.Errorf("summary success=%v failed=%v", success, failed)
	}
}

func TestMessageModify_RejectsLabelIntersectionAndTrashFolder(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	id := messageManageID("1")
	err := runMountedMailShortcut(t, MailMessageModify, []string{
		"+message-modify",
		"--message-ids", id,
		"--add-label-ids", "unread",
		"--remove-label-ids", "UNREAD",
	}, f, stdout)
	requireMessageManageValidationParam(t, err, "--add-label-ids")
	if !strings.Contains(err.Error(), "label cannot be both added and removed") {
		t.Fatalf("error = %v, want label intersection validation", err)
	}

	err = runMountedMailShortcut(t, MailMessageModify, []string{
		"+message-modify",
		"--message-ids", id,
		"--add-folder", "trash",
	}, f, stdout)
	requireMessageManageValidationParam(t, err, "--add-folder")
	if !strings.Contains(err.Error(), "use +message-trash") {
		t.Fatalf("error = %v, want TRASH validation", err)
	}
}

func TestMessageModify_EmptyOperationDoesNotCallPost(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	id1 := messageManageID("1")
	id2 := messageManageID("2")
	err := runMountedMailShortcut(t, MailMessageModify, []string{
		"+message-modify",
		"--message-ids", id1 + "," + id2 + "," + id1,
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	success, failed := decodeMessageManageSummary(t, decodeShortcutEnvelopeData(t, stdout))
	if len(success) != 2 || success[0] != id1 || success[1] != id2 || len(failed) != 0 {
		t.Fatalf("summary success=%v failed=%v", success, failed)
	}
}

func TestMessageModify_BatchesAndAggregatesPartialFailure(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	ids := make([]string, 41)
	for i := range ids {
		ids[i] = messageManageID(fmt.Sprintf("%02d", i))
	}
	first := stubMessageManagePost(reg, "batch_modify", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})
	second := stubMessageManagePost(reg, "batch_modify", map[string]interface{}{"code": 1230001, "msg": "bad request"})
	third := stubMessageManagePost(reg, "batch_modify", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})

	err := runMountedMailShortcut(t, MailMessageModify, []string{
		"+message-modify",
		"--message-ids", strings.Join(ids, ","),
		"--add-folder", "archive",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for idx, stub := range []*httpmock.Stub{first, second, third} {
		var body map[string]interface{}
		if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
			t.Fatalf("batch %d body unmarshal: %v", idx+1, err)
		}
		messageIDs := body["message_ids"].([]interface{})
		want := []int{20, 20, 1}[idx]
		if len(messageIDs) != want {
			t.Fatalf("batch %d size = %d, want %d", idx+1, len(messageIDs), want)
		}
		if body["add_folder"] != "ARCHIVED" {
			t.Fatalf("batch %d add_folder = %v, want ARCHIVED", idx+1, body["add_folder"])
		}
	}
	success, failed := decodeMessageManageSummary(t, decodeShortcutEnvelopeData(t, stdout))
	if len(success) != 21 || len(failed) != 20 {
		t.Fatalf("success=%d failed=%d, want 21/20", len(success), len(failed))
	}
}

func TestMessageModify_AllBatchesFailReturnsError(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id := messageManageID("1")
	stubMessageManagePost(reg, "batch_modify", map[string]interface{}{"code": 1230001, "msg": "bad request"})

	err := runMountedMailShortcut(t, MailMessageModify, []string{
		"+message-modify",
		"--message-ids", id,
		"--add-folder", "archive",
	}, f, stdout)
	requireMessageManageFailedPrecondition(t, err)
}

func TestMessageModify_DryRunShowsPlanWithoutValidationGET(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	id1 := messageManageID("1")
	id2 := messageManageID("2")
	err := runMountedMailShortcut(t, MailMessageModify, []string{
		"+message-modify",
		"--message-ids", id1 + "," + id2,
		"--add-label-ids", "customA",
		"--add-folder", "folderA",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		`/user_mailboxes/me/messages/batch_modify`,
		`validation_api_plan`,
		`/user_mailboxes/me/labels/customA`,
		`/user_mailboxes/me/folders/folderA`,
		`will_validate`,
		`batch_size`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q; got %s", want, out)
		}
	}
}

func TestMessageTrash_RequiresYesAndBatches(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id1 := messageManageID("1")
	id2 := messageManageID("2")
	err := runMountedMailShortcut(t, MailMessageTrash, []string{
		"+message-trash",
		"--message-ids", id1 + "," + id2,
	}, f, stdout)
	if err == nil {
		t.Fatal("expected confirmation error, got nil")
	}
	if code := output.ExitCodeOf(err); code != output.ExitConfirmationRequired {
		t.Fatalf("exit code = %d, want %d", code, output.ExitConfirmationRequired)
	}

	post := stubMessageManagePost(reg, "batch_trash", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})
	err = runMountedMailShortcut(t, MailMessageTrash, []string{
		"+message-trash",
		"--message-ids", id1 + "," + id2,
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err with --yes: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(post.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	if got := len(body["message_ids"].([]interface{})); got != 2 {
		t.Fatalf("message_ids len = %d, want 2", got)
	}
	success, failed := decodeMessageManageSummary(t, decodeShortcutEnvelopeData(t, stdout))
	if len(success) != 2 || len(failed) != 0 {
		t.Fatalf("summary success=%v failed=%v", success, failed)
	}
}

func TestMessageTrash_AllBatchesFailReturnsError(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id := messageManageID("1")
	stubMessageManagePost(reg, "batch_trash", map[string]interface{}{"code": 1230001, "msg": "bad request"})

	err := runMountedMailShortcut(t, MailMessageTrash, []string{
		"+message-trash",
		"--message-ids", id,
		"--yes",
	}, f, stdout)
	requireMessageManageFailedPrecondition(t, err)
}

func TestMessageManage_RejectsWhitespaceBeforeAPI(t *testing.T) {
	id1 := messageManageID("1")
	id2 := messageManageID("2")
	cases := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
	}{
		{
			name:     "trash newline in repeated flag",
			shortcut: MailMessageTrash,
			args:     []string{"+message-trash", "--message-ids", id1 + "\n" + id2, "--yes"},
		},
		{
			name:     "trash tab in csv flag",
			shortcut: MailMessageTrash,
			args:     []string{"+message-trash", "--message-ids", id1 + ",\t" + id2, "--yes"},
		},
		{
			name:     "modify space in repeated flag",
			shortcut: MailMessageModify,
			args:     []string{"+message-modify", "--message-ids", id1, "--message-ids", id2 + " ", "--add-folder", "archive"},
		},
		{
			name:     "modify space in csv flag",
			shortcut: MailMessageModify,
			args:     []string{"+message-modify", "--message-ids", id1 + ", " + id2, "--add-folder", "archive"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, tc.shortcut, tc.args, f, stdout)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if code := output.ExitCodeOf(err); code != output.ExitValidation {
				t.Fatalf("exit code = %d, want %d; err=%v", code, output.ExitValidation, err)
			}
			if !strings.Contains(err.Error(), "must not contain whitespace or control characters") {
				t.Fatalf("error = %v, want whitespace/control validation", err)
			}
		})
	}
}
