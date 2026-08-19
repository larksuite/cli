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
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

func threadManageID(suffix string) string {
	return "thread_abcdefghijklmnop_" + suffix
}

func threadManageIDs(count int) []string {
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		ids = append(ids, threadManageID(fmt.Sprintf("%02d", i+1)))
	}
	return ids
}

func stubThreadManagePost(reg *httpmock.Registry, endpoint string, body map[string]interface{}) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/threads/" + endpoint,
		Body:   body,
	}
	reg.Register(stub)
	return stub
}

func TestThreadModify_Metadata(t *testing.T) {
	if MailThreadModify.Command != "+thread-modify" {
		t.Fatalf("Command = %q", MailThreadModify.Command)
	}
	if MailThreadModify.Risk != "write" {
		t.Errorf("Risk = %q, want write", MailThreadModify.Risk)
	}
	if len(MailThreadModify.AuthTypes) != 1 || MailThreadModify.AuthTypes[0] != "user" {
		t.Errorf("AuthTypes = %v, want [user]", MailThreadModify.AuthTypes)
	}
	if len(MailThreadModify.Scopes) != 1 || MailThreadModify.Scopes[0] != "mail:user_mailbox.message:modify" {
		t.Errorf("Scopes = %v, want [mail:user_mailbox.message:modify]", MailThreadModify.Scopes)
	}
	flags := map[string]common.Flag{}
	for _, fl := range MailThreadModify.Flags {
		flags[fl.Name] = fl
	}
	for _, name := range []string{"mailbox", "thread-ids", "add-label-ids", "remove-label-ids", "add-folder"} {
		if _, ok := flags[name]; !ok {
			t.Fatalf("missing --%s flag", name)
		}
	}
	if flags["thread-ids"].Type != "string_array" || !flags["thread-ids"].Required {
		t.Errorf("--thread-ids = %#v, want required string_array", flags["thread-ids"])
	}
	if got := strings.Join(flags["add-folder"].Aliases, ","); got != "folder-id" {
		t.Errorf("--add-folder aliases = %q, want folder-id", got)
	}
}

func TestThreadTrash_Metadata(t *testing.T) {
	if MailThreadTrash.Command != "+thread-trash" {
		t.Fatalf("Command = %q", MailThreadTrash.Command)
	}
	if MailThreadTrash.Risk != "high-risk-write" {
		t.Errorf("Risk = %q, want high-risk-write", MailThreadTrash.Risk)
	}
	if len(MailThreadTrash.AuthTypes) != 1 || MailThreadTrash.AuthTypes[0] != "user" {
		t.Errorf("AuthTypes = %v, want [user]", MailThreadTrash.AuthTypes)
	}
}

func TestThreadManage_NormalizeThreadIDs(t *testing.T) {
	id1 := threadManageID("1")
	id2 := threadManageID("2")
	got, err := normalizeThreadManageIDs([]string{" " + id1 + ", , " + id2 + " ", id1})
	if err != nil {
		t.Fatalf("normalizeThreadManageIDs returned error: %v", err)
	}
	if len(got) != 2 || got[0] != id1 || got[1] != id2 {
		t.Fatalf("ids = %v, want [%s %s]", got, id1, id2)
	}
	for _, tc := range [][]string{{}, {""}, {" , "}} {
		_, err := normalizeThreadManageIDs(tc)
		requireMessageManageValidationParam(t, err, "--thread-ids")
	}
	tooMany := threadManageIDs(mailThreadManageMaxIDs + 1)
	_, err = normalizeThreadManageIDs(tooMany)
	validationErr := requireMessageManageValidationParam(t, err, "--thread-ids")
	if !strings.Contains(validationErr.Error(), "thread_ids") || !strings.Contains(validationErr.Error(), "20") {
		t.Fatalf("error = %v, want thread_ids max 20 validation", validationErr)
	}

	withDuplicates := append(threadManageIDs(mailThreadManageMaxIDs), threadManageID("01"))
	got, err = normalizeThreadManageIDs(withDuplicates)
	if err != nil {
		t.Fatalf("normalizeThreadManageIDs with duplicate over raw max returned error: %v", err)
	}
	if len(got) != mailThreadManageMaxIDs {
		t.Fatalf("ids len = %d, want %d after dedupe", len(got), mailThreadManageMaxIDs)
	}
}

func TestThreadModify_LabelFolderBodyAndOutputContract(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id1 := threadManageID("1")
	id2 := threadManageID("2")
	post := stubThreadManagePost(reg, "batch_modify", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})

	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-ids", id1 + "," + id2 + "," + id1,
		"--add-label-ids", "unread,customA",
		"--remove-label-ids", "FLAGGED",
		"--add-folder", "archive",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(post.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	threadIDs := body["thread_ids"].([]interface{})
	if len(threadIDs) != 2 || threadIDs[0] != id1 || threadIDs[1] != id2 {
		t.Fatalf("thread_ids = %#v, want deduped [%s %s]", threadIDs, id1, id2)
	}
	if got := body["add_folder"]; got != "ARCHIVED" {
		t.Fatalf("add_folder = %v, want ARCHIVED", got)
	}
	addLabels := body["add_label_ids"].([]interface{})
	if addLabels[0] != "UNREAD" || addLabels[1] != "customA" {
		t.Fatalf("add_label_ids = %#v, want [UNREAD customA]", addLabels)
	}
	removeLabels := body["remove_label_ids"].([]interface{})
	if len(removeLabels) != 1 || removeLabels[0] != "FLAGGED" {
		t.Fatalf("remove_label_ids = %#v, want [FLAGGED]", removeLabels)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["operation"] != "thread_modify" || data["mailbox"] != "me" {
		t.Fatalf("operation/mailbox = %v/%v", data["operation"], data["mailbox"])
	}
	if data["submitted_count"].(float64) != 2 {
		t.Fatalf("submitted_count = %v, want 2", data["submitted_count"])
	}
	if _, ok := data["updated_count"]; ok {
		t.Fatalf("updated_count must not be present: %#v", data)
	}
	if _, ok := data["failed_ids"]; ok {
		t.Fatalf("failed_ids must not be present: %#v", data)
	}
	if data["add_folder"] != "ARCHIVED" {
		t.Fatalf("add_folder = %v, want ARCHIVED", data["add_folder"])
	}
	if _, ok := data["folder_id"]; ok {
		t.Fatalf("folder_id must not be present: %#v", data)
	}
}

func TestThreadModify_FolderIDAliasMapsToAddFolder(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id := threadManageID("alias")
	post := stubThreadManagePost(reg, "batch_modify", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})

	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-ids", id,
		"--folder-id", "archive",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(post.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	if got := body["add_folder"]; got != "ARCHIVED" {
		t.Fatalf("add_folder = %v, want ARCHIVED", got)
	}
}

func TestThreadModify_Validation(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	id := threadManageID("1")
	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-ids", id,
	}, f, stdout)
	requireMessageManageValidationParam(t, err, "--thread-modify")
	if !strings.Contains(err.Error(), "provide at least one") {
		t.Fatalf("error = %v, want missing action validation", err)
	}

	err = runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-ids", id,
		"--add-label-ids", "unread",
		"--remove-label-ids", "UNREAD",
	}, f, stdout)
	requireMessageManageValidationParam(t, err, "--add-label-ids")

	err = runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-ids", id,
		"--folder-id", "trash",
	}, f, stdout)
	requireMessageManageValidationParam(t, err, "--folder-id")
	if !strings.Contains(err.Error(), "use +thread-trash") {
		t.Fatalf("error = %v, want +thread-trash hint", err)
	}
}

func TestThreadManage_ThreadIDsMaxValidationRunsBeforeAPI(t *testing.T) {
	for _, tc := range []struct {
		name     string
		shortcut common.Shortcut
		args     []string
		endpoint string
	}{
		{
			name:     "modify",
			shortcut: MailThreadModify,
			args: []string{
				"+thread-modify",
				"--thread-ids", strings.Join(threadManageIDs(mailThreadManageMaxIDs+1), ","),
				"--add-label-ids", "FLAGGED",
			},
			endpoint: "batch_modify",
		},
		{
			name:     "trash",
			shortcut: MailThreadTrash,
			args: []string{
				"+thread-trash",
				"--thread-ids", strings.Join(threadManageIDs(mailThreadManageMaxIDs+1), ","),
				"--yes",
			},
			endpoint: "batch_trash",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, reg := mailShortcutTestFactory(t)
			stub := stubThreadManagePost(reg, tc.endpoint, map[string]interface{}{"code": 0, "data": map[string]interface{}{}})
			stub.Optional = true

			err := runMountedMailShortcut(t, tc.shortcut, tc.args, f, stdout)
			validationErr := requireMessageManageValidationParam(t, err, "--thread-ids")
			if !strings.Contains(validationErr.Error(), "thread_ids") || !strings.Contains(validationErr.Error(), "20") {
				t.Fatalf("error = %v, want thread_ids max 20 validation", validationErr)
			}
			if len(stub.CapturedBody) != 0 {
				t.Fatalf("API was called with body %s, want local validation before request", string(stub.CapturedBody))
			}
		})
	}
}

func TestThreadModify_DryRunShowsPostURLAndBody(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	id1 := threadManageID("1")
	id2 := threadManageID("2")
	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-ids", id1 + "," + id2,
		"--add-label-ids", "customA",
		"--add-folder", "folderA",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		`/user_mailboxes/me/threads/batch_modify`,
		`thread_ids`,
		`add_label_ids`,
		`add_folder`,
		`submitted_count is request-side only`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q; got %s", want, out)
		}
	}
}

func TestThreadModify_APIFailurePreservesDiagnostic(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubThreadManagePost(reg, "batch_modify", map[string]interface{}{"code": 1230001, "msg": "label not found"})

	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-ids", threadManageID("1"),
		"--add-label-ids", "missing_label",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	if !strings.Contains(err.Error(), "label not found") {
		t.Fatalf("error = %v, want backend diagnostic", err)
	}
	requireThreadManageAPIError(t, err)
	requireThreadManageDecoratorPreservesAPICause(t, "failed to modify threads")
}

func TestThreadTrash_RequiresYesAndOutputsSubmittedContract(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id1 := threadManageID("1")
	id2 := threadManageID("2")
	err := runMountedMailShortcut(t, MailThreadTrash, []string{
		"+thread-trash",
		"--thread-ids", id1 + "," + id2,
	}, f, stdout)
	if err == nil {
		t.Fatal("expected confirmation error, got nil")
	}
	if code := output.ExitCodeOf(err); code != output.ExitConfirmationRequired {
		t.Fatalf("exit code = %d, want %d", code, output.ExitConfirmationRequired)
	}

	post := stubThreadManagePost(reg, "batch_trash", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})
	err = runMountedMailShortcut(t, MailThreadTrash, []string{
		"+thread-trash",
		"--thread-ids", id1 + "," + id2,
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err with --yes: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(post.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	if got := len(body["thread_ids"].([]interface{})); got != 2 {
		t.Fatalf("thread_ids len = %d, want 2", got)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["operation"] != "thread_trash" || data["submitted_count"].(float64) != 2 {
		t.Fatalf("data = %#v, want thread_trash submitted_count=2", data)
	}
	if _, ok := data["trashed_count"]; ok {
		t.Fatalf("trashed_count must not be present: %#v", data)
	}
}

func TestThreadTrash_DryRunAndAPIFailure(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id := threadManageID("1")
	err := runMountedMailShortcut(t, MailThreadTrash, []string{
		"+thread-trash",
		"--thread-ids", id,
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if out := stdout.String(); !strings.Contains(out, `/user_mailboxes/me/threads/batch_trash`) || !strings.Contains(out, `thread_ids`) {
		t.Fatalf("dry-run output missing route/body: %s", out)
	}

	stubThreadManagePost(reg, "batch_trash", map[string]interface{}{"code": 1230001, "msg": "conflict, retry later"})
	err = runMountedMailShortcut(t, MailThreadTrash, []string{
		"+thread-trash",
		"--thread-ids", id,
		"--yes",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	if !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("error = %v, want conflict diagnostic", err)
	}
	requireThreadManageAPIError(t, err)
	requireThreadManageDecoratorPreservesAPICause(t, "failed to trash threads")
}

func requireThreadManageAPIError(t *testing.T, err error) {
	t.Helper()
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed Problem, got %T", err)
	}
	if problem.Category != errs.CategoryAPI {
		t.Fatalf("problem category = %s, want api", problem.Category)
	}
	if problem.Subtype == "" {
		t.Fatalf("problem subtype is empty: %+v", problem)
	}
}

func requireThreadManageDecoratorPreservesAPICause(t *testing.T, prefix string) {
	t.Helper()
	cause := errors.New("upstream API cause")
	err := errs.NewAPIError(errs.SubtypeUnknown, "backend diagnostic").WithCause(cause)
	decorated := mailDecorateProblemMessage(err, "%s", prefix)
	if !errors.Is(decorated, cause) {
		t.Fatalf("decorated API error lost cause %v: %v", cause, decorated)
	}
	requireThreadManageAPIError(t, decorated)
}
